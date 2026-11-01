//go:build linux

package netstat

import (
	"encoding/binary"
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux's sock_diag (netlink): the kernel's own dump of every TCP socket
// with its tcp_info, which `ss -ti` shows: bytes acknowledged by the other
// end (sent) and received, and the smoothed round-trip time. No privileges
// needed. UDP sockets have no byte counts.

type tcpStats struct {
	rx, tx uint64
	rtt    time.Duration
}

const (
	inetDiagInfo  = 2  // INET_DIAG_INFO
	sockDiagMsgSz = 72 // struct inet_diag_msg
	diagInodeAt   = 68
	nlmsgHdrSz    = 16
	nlmDone       = 3
	nlmError      = 2
)

// tcpDiag returns the stats of every TCP socket, by inode.
func tcpDiag() (map[uint64]tcpStats, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	tv := unix.NsecToTimeval(int64(2 * time.Second))
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
	out := map[uint64]tcpStats{}
	for i, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		if err := diagDump(fd, family, uint32(i+1), out); err != nil {
			if family == unix.AF_INET6 && len(out) > 0 {
				continue // no IPv6 here
			}
			return nil, err
		}
	}
	return out, nil
}

func diagDump(fd int, family uint8, seq uint32, out map[uint64]tcpStats) error {
	// struct nlmsghdr + struct inet_diag_req_v2
	req := make([]byte, nlmsgHdrSz+56)
	le := binary.LittleEndian
	le.PutUint32(req[0:], uint32(len(req)))
	le.PutUint16(req[4:], unix.SOCK_DIAG_BY_FAMILY)
	le.PutUint16(req[6:], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	le.PutUint32(req[8:], seq)
	r := req[nlmsgHdrSz:]
	r[0] = family
	r[1] = unix.IPPROTO_TCP
	r[2] = 1 << (inetDiagInfo - 1)
	// Every state but LISTEN (bit 10) and TIME_WAIT (bit 6).
	le.PutUint32(r[4:], 0xffffffff&^(1<<10|1<<6))
	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return err
	}
	buf := make([]byte, 64*1024)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return err
		}
		data := buf[:n]
		for len(data) >= nlmsgHdrSz {
			l := int(le.Uint32(data))
			typ := le.Uint16(data[4:])
			if l < nlmsgHdrSz || l > len(data) {
				return errors.New("sock_diag: bad message")
			}
			msg := data[nlmsgHdrSz:l]
			data = data[align4(l):]
			switch typ {
			case nlmDone:
				return nil
			case nlmError:
				if len(msg) >= 4 && le.Uint32(msg) == 0 {
					continue // an acknowledgement
				}
				return errors.New("sock_diag: the kernel refused the request")
			case unix.SOCK_DIAG_BY_FAMILY:
				parseDiagMsg(msg, out)
			}
		}
	}
}

func align4(n int) int { return (n + 3) &^ 3 }

func parseDiagMsg(msg []byte, out map[uint64]tcpStats) {
	le := binary.LittleEndian
	if len(msg) < sockDiagMsgSz {
		return
	}
	inode := uint64(le.Uint32(msg[diagInodeAt:]))
	attrs := msg[sockDiagMsgSz:]
	for len(attrs) >= 4 {
		l := int(le.Uint16(attrs))
		typ := le.Uint16(attrs[2:])
		if l < 4 || l > len(attrs) {
			return
		}
		if typ == inetDiagInfo && inode != 0 {
			var info unix.TCPInfo
			raw := (*[unix.SizeofTCPInfo]byte)(unsafe.Pointer(&info))
			copy(raw[:], attrs[4:l])
			out[inode] = tcpStats{rx: info.Bytes_received, tx: info.Bytes_acked, rtt: time.Duration(info.Rtt) * time.Microsecond}
		}
		attrs = attrs[align4(l):]
	}
}
