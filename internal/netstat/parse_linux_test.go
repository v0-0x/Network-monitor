//go:build linux

package netstat

import (
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
