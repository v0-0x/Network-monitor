package netstat

import (
	"encoding/binary"
	"errors"
	"net/netip"
)

// macOS: the kernel hands out its socket tables through the sysctls
// net.inet.tcp.pcblist_n and net.inet.udp.pcblist_n, the same ones netstat
// reads. No administrator rights are needed, and every socket comes with the
// process that last used it. This file only parses that buffer (so the tests
// can run anywhere); darwin.go reads it.
//
// The buffer is a struct xinpgen, then for each socket a run of records,
// each starting with its length and kind (xnu bsd/netinet/in_pcb.h,
// bsd/sys/socketvar.h, bsd/netinet/tcp_var.h; all packed to 4 bytes), each
// rounded up to 8 bytes, and a closing xinpgen. A socket is complete when
// one record of every kind was seen; network_cmds' netstat/inet.c reads it
// the same way.

const (
	xsoSocket = 0x001
	xsoRcvbuf = 0x002
	xsoSndbuf = 0x004
	xsoStats  = 0x008
	xsoInpcb  = 0x010
	xsoTcpcb  = 0x020

	xinpgenSize = 24

	inpIPv4 = 0x1
	inpIPv6 = 0x2

	// Offsets in struct xinpcb_n.
	inpFport  = 16 // u_short, network byte order
	inpLport  = 18
	inpGencnt = 28 // u_int64_t
	inpVflag  = 44
	inpFaddr  = 48 // 16 bytes; IPv4 in the last 4
	inpLaddr  = 64
	inpMinLen = 80

	// Offsets in struct xsocket_n.
	soLastPID = 68
	soEPID    = 72
	soMinLen  = 76

	// Offset in struct xtcpcb_n.
	tpState  = 36
	tpMinLen = 40
)

var darwinStates = [...]string{Close, Listen, SynSent, SynRecv, Established, CloseWait, FinWait1, Closing, LastAck, FinWait2, TimeWait}

func roundup64(n uint32) int {
	if n == 0 {
		return 8
	}
	return int(1 + ((n - 1) | 7))
}

var errPcblist = errors.New("unexpected socket list from the kernel")

// parsePcblist parses a pcblist_n buffer.
func parsePcblist(buf []byte, proto string) ([]Socket, error) {
	le := binary.LittleEndian
	if len(buf) < xinpgenSize {
		return nil, errPcblist
	}
	xigLen := le.Uint32(buf)
	if xigLen < xinpgenSize || int(xigLen) > len(buf) {
		return nil, errPcblist
	}
	gen := le.Uint64(buf[8:])
	want := uint32(xsoSocket | xsoRcvbuf | xsoSndbuf | xsoStats | xsoInpcb)
	if proto == "tcp" {
		want |= xsoTcpcb
	}
	var out []Socket
	var which uint32
	var inp, so, tp []byte
	for off := roundup64(xigLen); off+8 <= len(buf); {
		l := le.Uint32(buf[off:])
		kind := le.Uint32(buf[off+4:])
		if l <= xinpgenSize || off+int(l) > len(buf) {
			break // the closing xinpgen, or a cut-off buffer
		}
		rec := buf[off : off+int(l)]
		off += roundup64(l)
		if which&kind == 0 {
			which |= kind
			switch kind {
			case xsoSocket:
				so = rec
			case xsoInpcb:
				inp = rec
			case xsoTcpcb:
				tp = rec
			}
		}
		if which != want {
			continue
		}
		which = 0
		s, ok := pcbSocket(inp, so, tp, proto, gen)
		inp, so, tp = nil, nil, nil
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func pcbSocket(inp, so, tp []byte, proto string, gen uint64) (Socket, bool) {
	le := binary.LittleEndian
	if len(inp) < inpMinLen || len(so) < soMinLen {
		return Socket{}, false
	}
	// Freed while the kernel was copying the list out.
	if le.Uint64(inp[inpGencnt:]) > gen {
		return Socket{}, false
	}
	fport := binary.BigEndian.Uint16(inp[inpFport:])
	lport := binary.BigEndian.Uint16(inp[inpLport:])
	vflag := inp[inpVflag]
	var laddr, faddr netip.Addr
	switch {
	case vflag&inpIPv4 != 0:
		laddr = netip.AddrFrom4([4]byte(inp[inpLaddr+12 : inpLaddr+16]))
		faddr = netip.AddrFrom4([4]byte(inp[inpFaddr+12 : inpFaddr+16]))
	case vflag&inpIPv6 != 0:
		laddr = netip.AddrFrom16([16]byte(inp[inpLaddr : inpLaddr+16]))
		faddr = netip.AddrFrom16([16]byte(inp[inpFaddr : inpFaddr+16]))
	default:
		return Socket{}, false
	}
	s := Socket{Proto: proto, Local: netip.AddrPortFrom(laddr, lport), Remote: netip.AddrPortFrom(faddr, fport)}
	// The effective pid is the app a daemon works for (a download
	// nsurlsessiond makes for Safari); otherwise the last process that used
	// the socket.
	pid := int32(le.Uint32(so[soEPID:]))
	if pid <= 0 {
		pid = int32(le.Uint32(so[soLastPID:]))
	}
	if pid > 0 {
		s.PID = pid
	}
	if proto == "tcp" {
		if len(tp) < tpMinLen {
			return Socket{}, false
		}
		st := int32(le.Uint32(tp[tpState:]))
		if st >= 0 && int(st) < len(darwinStates) {
			s.State = darwinStates[st]
		}
	} else if s.HasRemote() {
		s.State = Established
	} else {
		s.State = Listen
	}
	return s, true
}

// parseIfList2 sums the byte counters of a NET_RT_IFLIST2 routing dump
// (RTM_IFINFO2 messages, struct if_msghdr2 with its struct if_data64),
// leaving out loopback interfaces.
func parseIfList2(buf []byte) (rx, tx uint64, ok bool) {
	const (
		rtmIfinfo2  = 0x12
		iffLoopback = 0x8
		ibytesAt    = 32 + 64
		obytesAt    = 32 + 72
		minLen      = obytesAt + 8
	)
	le := binary.LittleEndian
	for off := 0; off+4 <= len(buf); {
		l := int(le.Uint16(buf[off:]))
		if l < 4 || off+l > len(buf) {
			break
		}
		msg := buf[off : off+l]
		off += l
		if msg[3] != rtmIfinfo2 || len(msg) < minLen {
			continue
		}
		if le.Uint32(msg[8:])&iffLoopback != 0 {
			continue
		}
		rx += le.Uint64(msg[ibytesAt:])
		tx += le.Uint64(msg[obytesAt:])
		ok = true
	}
	return rx, tx, ok
}
