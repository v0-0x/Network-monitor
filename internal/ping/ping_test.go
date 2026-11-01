package ping

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestICMPLoopback(t *testing.T) {
	r, err := ICMP(context.Background(), netip.MustParseAddr("127.0.0.1"), 2, time.Second)
	if errors.Is(err, ErrNoICMP) {
		t.Skip("ICMP isn't allowed here")
	}
	if err != nil {
		t.Fatal(err)
	}
	if r.Received != 2 || r.Avg <= 0 {
		t.Errorf("loopback ping: %+v", r)
	}
	t.Logf("%+v", r)
}

func TestTCP(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	r := TCP(context.Background(), netip.MustParseAddr("127.0.0.1"), port, 2, time.Second)
	if r.Received != 2 || r.Method != "tcp" {
		t.Errorf("tcp ping: %+v", r)
	}
	if r.Loss() != 0 {
		t.Errorf("loss %v", r.Loss())
	}
}
