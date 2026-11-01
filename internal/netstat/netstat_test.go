package netstat

import (
	"context"
	"net"
	"net/netip"
	"os"
	"runtime"
	"testing"
	"time"
)

// TestSeesOwnConnection opens a listener and a connection to it, and checks
// that the scanner sees both with this process as the owner. It runs on each
// system's CI, so every reader is tried for real.
func TestSeesOwnConnection(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	srv := <-accepted
	defer srv.Close()

	lport := uint16(ln.Addr().(*net.TCPAddr).Port)
	cport := uint16(c.LocalAddr().(*net.TCPAddr).Port)
	me := int32(os.Getpid())
	s := NewScanner()
	var socks []Socket
	deadline := time.Now().Add(5 * time.Second)
	for {
		socks, err = s.Sockets(context.Background())
		if err != nil {
			t.Fatalf("Sockets (%s): %v", s.Method(), err)
		}
		var listen, conn bool
		for _, so := range socks {
			if so.Proto != "tcp" {
				continue
			}
			if so.State == Listen && so.Local.Port() == lport {
				listen = true
				if so.PID != me {
					t.Errorf("listener's pid = %d, want %d", so.PID, me)
				}
			}
			if so.State == Established && so.Local.Port() == cport && so.Remote.Port() == lport {
				conn = true
				if so.PID != me {
					t.Errorf("connection's pid = %d, want %d", so.PID, me)
				}
				if so.Remote.Addr() != netip.MustParseAddr("127.0.0.1") {
					t.Errorf("remote = %v", so.Remote)
				}
			}
		}
		if listen && conn {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("listener seen %v, connection seen %v, among %d sockets (%s)", listen, conn, len(socks), s.Method())
		}
		time.Sleep(100 * time.Millisecond)
	}
	procs := s.Processes([]int32{me})
	if procs[me].Name == "" {
		t.Errorf("no name for this process: %+v", procs)
	}
	t.Logf("%d sockets via %s; this process is %q (%s)", len(socks), s.Method(), procs[me].Name, procs[me].Exe)
	if rx, tx, ok := s.Traffic(); ok {
		t.Logf("traffic: %d received, %d sent", rx, tx)
	} else if runtime.GOOS != "windows" {
		t.Logf("no traffic counters on %s", runtime.GOOS)
	}
}

func TestUDPConnected(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	c, err := net.Dial("udp4", pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if runtime.GOOS == "windows" {
		t.Skip("Windows' UDP table has no remote addresses")
	}
	socks, err := NewScanner().Sockets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := uint16(pc.LocalAddr().(*net.UDPAddr).Port)
	for _, s := range socks {
		if s.Proto == "udp" && s.Remote.Port() == want && s.State == Established {
			return
		}
	}
	t.Errorf("connected UDP socket to port %d not found", want)
}

func TestAppBundleName(t *testing.T) {
	cases := map[string]string{
		"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/1/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper": "Google Chrome",
		"/System/Applications/Mail.app/Contents/MacOS/Mail": "Mail",
		"/usr/libexec/trustd":                               "",
	}
	for in, want := range cases {
		if got := appBundleName(in); got != want {
			t.Errorf("appBundleName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cleanName(`C:\Program Files\Mozilla Firefox\firefox.exe`); got != "firefox" {
		t.Errorf("cleanName = %q", got)
	}
}
