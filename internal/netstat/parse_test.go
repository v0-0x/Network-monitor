package netstat

import (
	"encoding/binary"
	"net/netip"
	"testing"
	"time"
)

// pcbRecord builds one record of a pcblist_n buffer, padded to 8 bytes.
func pcbRecord(kind uint32, size int, fill func(b []byte)) []byte {
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b, uint32(size))
	binary.LittleEndian.PutUint32(b[4:], kind)
	if fill != nil {
		fill(b)
	}
	for len(b)%8 != 0 {
		b = append(b, 0)
	}
	return b
}

func TestParsePcblist(t *testing.T) {
	le := binary.LittleEndian
	head := make([]byte, 24)
	le.PutUint32(head, 24)
	le.PutUint32(head[4:], 2)
	le.PutUint64(head[8:], 100) // generation
	buf := append([]byte{}, head...)

	sock := func(v4 bool, laddr, faddr netip.Addr, lport, fport uint16, lastPID, ePID uint32, state uint32, gen uint64) {
		buf = append(buf, pcbRecord(xsoSocket, 96, func(b []byte) {
			le.PutUint32(b[soLastPID:], lastPID)
			le.PutUint32(b[soEPID:], ePID)
		})...)
		buf = append(buf, pcbRecord(xsoRcvbuf, 32, nil)...)
		buf = append(buf, pcbRecord(xsoSndbuf, 32, nil)...)
		buf = append(buf, pcbRecord(xsoStats, 136, func(b []byte) {
			for i := 0; i < 4; i++ {
				le.PutUint64(b[8+i*32+8:], 1000) // rxbytes
				le.PutUint64(b[8+i*32+24:], 10)  // txbytes
			}
		})...)
		buf = append(buf, pcbRecord(xsoInpcb, 104, func(b []byte) {
			binary.BigEndian.PutUint16(b[inpFport:], fport)
			binary.BigEndian.PutUint16(b[inpLport:], lport)
			le.PutUint64(b[inpGencnt:], gen)
			if v4 {
				b[inpVflag] = inpIPv4
				l4, f4 := laddr.As4(), faddr.As4()
				copy(b[inpLaddr+12:], l4[:])
				copy(b[inpFaddr+12:], f4[:])
			} else {
				b[inpVflag] = inpIPv6
				l6, f6 := laddr.As16(), faddr.As16()
				copy(b[inpLaddr:], l6[:])
				copy(b[inpFaddr:], f6[:])
			}
		})...)
		buf = append(buf, pcbRecord(xsoTcpcb, 208, func(b []byte) {
			le.PutUint32(b[tpState:], state)
			le.PutUint32(b[tpSrtt:], 25*32) // 25 ms
		})...)
	}
	sock(true, netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("17.253.144.10"), 50000, 443, 321, 0, 4, 90)
	sock(false, netip.MustParseAddr("::"), netip.IPv6Unspecified(), 7000, 0, 55, 66, 1, 91)
	sock(true, netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("1.1.1.1"), 1, 2, 1, 0, 4, 101) // freed during copyout
	tail := make([]byte, 24)
	le.PutUint32(tail, 24)
	buf = append(buf, tail...)

	socks, err := parsePcblist(buf, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if len(socks) != 2 {
		t.Fatalf("got %d sockets: %+v", len(socks), socks)
	}
	a := socks[0]
	if a.Local != netip.MustParseAddrPort("192.168.1.20:50000") || a.Remote != netip.MustParseAddrPort("17.253.144.10:443") || a.PID != 321 || a.State != Established {
		t.Errorf("v4 socket: %+v", a)
	}
	if !a.HasBytes || a.Rx != 4000 || a.Tx != 40 || a.RTT != 25*time.Millisecond {
		t.Errorf("v4 socket's stats: %+v", a)
	}
	b := socks[1]
	if b.Local.Port() != 7000 || b.State != Listen || b.PID != 66 || !b.Local.Addr().Is6() {
		t.Errorf("v6 listener: %+v", b)
	}
	if _, err := parsePcblist(buf[:10], "tcp"); err == nil {
		t.Error("short buffer accepted")
	}
	// A buffer cut in the middle of a record stops cleanly.
	if socks, err := parsePcblist(buf[:200], "tcp"); err != nil || len(socks) != 0 {
		t.Errorf("cut buffer: %v %v", socks, err)
	}
}

func TestParseIfList2(t *testing.T) {
	le := binary.LittleEndian
	msg := func(flags uint32, ib, ob uint64) []byte {
		b := make([]byte, 160)
		le.PutUint16(b, 160)
		b[2] = 5
		b[3] = 0x12
		le.PutUint32(b[8:], flags)
		le.PutUint64(b[96:], ib)
		le.PutUint64(b[104:], ob)
		return b
	}
	other := make([]byte, 20)
	le.PutUint16(other, 20)
	other[3] = 0x0c // RTM_NEWADDR
	var buf []byte
	buf = append(buf, msg(0x8|0x1, 999, 999)...) // lo0
	buf = append(buf, other...)
	buf = append(buf, msg(0x1, 100, 50)...)
	buf = append(buf, msg(0x1, 10, 5)...)
	rx, tx, ok := parseIfList2(buf)
	if !ok || rx != 110 || tx != 55 {
		t.Errorf("got %d %d %v", rx, tx, ok)
	}
}
