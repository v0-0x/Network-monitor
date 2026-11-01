package netstat

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func TestParseProcNet(t *testing.T) {
	tcp := []byte(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0277 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 23471 1 0000000000000000 100 0 0 10 0
   1: 0F02000A:D2B4 2E4AFA8E:01BB 01 00000000:00000000 02:000005BA 00000000  1000        0 98765 2 0000000000000000 20 4 30 10 -1
`)
	socks := parseProcNet(tcp, "tcp")
	if len(socks) != 2 {
		t.Fatalf("got %d sockets", len(socks))
	}
	if socks[0].Local != netip.MustParseAddrPort("127.0.0.1:631") || socks[0].State != Listen || socks[0].inode != 23471 {
		t.Errorf("listener: %+v", socks[0])
	}
	if socks[1].Local != netip.MustParseAddrPort("10.0.2.15:53940") || socks[1].Remote != netip.MustParseAddrPort("142.250.74.46:443") || socks[1].State != Established {
		t.Errorf("connection: %+v", socks[1])
	}
	tcp6 := []byte(`  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1234 1
   1: 0000000000000000FFFF00000F02000A:C350 0000000000000000FFFF00002E4AFA8E:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 5678 1
`)
	socks = parseProcNet(tcp6, "tcp")
	if len(socks) != 2 {
		t.Fatalf("got %d v6 sockets", len(socks))
	}
	if socks[0].Local != netip.MustParseAddrPort("[::1]:22") {
		t.Errorf("v6 listener: %v", socks[0].Local)
	}
	if unmap(socks[1].Remote) != netip.MustParseAddrPort("142.250.74.46:443") {
		t.Errorf("v4-mapped remote: %v", socks[1].Remote)
	}
	udp := []byte(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  100: 00000000:14E9 00000000:0000 07 00000000:00000000 00:00000000 00000000   104        0 2222 2 0000000000000000 0
  101: 0F02000A:E1E6 08080808:0035 01 00000000:00000000 00:00000000 00000000  1000        0 3333 2 0000000000000000 0
`)
	socks = parseProcNet(udp, "udp")
	if len(socks) != 2 || socks[0].State != Listen || socks[1].State != Established || socks[1].Remote != netip.MustParseAddrPort("8.8.8.8:53") {
		t.Errorf("udp: %+v", socks)
	}
}

func TestParseProcNetDev(t *testing.T) {
	dev := []byte(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000      10    0    0    0     0          0         0     1000      10    0    0    0     0       0          0
  eth0: 5000      50    0    0    0     0          0         0     3000      30    0    0    0     0       0          0
 wlan0: 700       7    0    0    0     0          0         0      300       3    0    0    0     0       0          0
`)
	rx, tx, ok := parseProcNetDev(dev)
	if !ok || rx != 5700 || tx != 3300 {
		t.Errorf("got %d %d %v", rx, tx, ok)
	}
}

func TestRunTarget(t *testing.T) {
	cases := map[string][]string{
		"discord":   {"electron", "--enable-features=x", "/usr/lib/discord/app.asar"},
		"server.py": {"python3", "-u", "server.py"},
		"":          {"node"},
	}
	for want, args := range cases {
		if got := runTarget(args); got != want {
			t.Errorf("runTarget(%q) = %q, want %q", args, got, want)
		}
	}
}

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
		buf = append(buf, pcbRecord(xsoStats, 168, nil)...)
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
		buf = append(buf, pcbRecord(xsoTcpcb, 208, func(b []byte) { le.PutUint32(b[tpState:], state) })...)
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
