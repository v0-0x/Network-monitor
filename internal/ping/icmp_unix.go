//go:build !windows

package ping

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// echo sends one ICMP echo through an unprivileged ping socket and waits
// for its answer.
func echo(ip netip.Addr, seq int, timeout time.Duration) (time.Duration, error) {
	network, laddr, proto := "udp4", "0.0.0.0", 1
	var typ icmp.Type = ipv4.ICMPTypeEcho
	var reply icmp.Type = ipv4.ICMPTypeEchoReply
	if ip.Is6() && !ip.Is4In6() {
		network, laddr, proto = "udp6", "::", 58
		typ, reply = ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply
	}
	c, err := icmp.ListenPacket(network, laddr)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EPROTONOSUPPORT) {
			return 0, ErrNoICMP
		}
		return 0, err
	}
	defer c.Close()
	id := os.Getpid() & 0xffff
	msg := icmp.Message{Type: typ, Body: &icmp.Echo{ID: id, Seq: seq, Data: []byte("network-monitor")}}
	b, err := msg.Marshal(nil)
	if err != nil {
		return 0, err
	}
	dst := &net.UDPAddr{IP: ip.Unmap().AsSlice()}
	start := time.Now()
	if _, err := c.WriteTo(b, dst); err != nil {
		return 0, err
	}
	_ = c.SetReadDeadline(start.Add(timeout))
	buf := make([]byte, 1500)
	for {
		n, peer, err := c.ReadFrom(buf)
		if err != nil {
			return 0, err
		}
		m, err := icmp.ParseMessage(proto, buf[:n])
		if err != nil || m.Type != reply {
			continue
		}
		e, ok := m.Body.(*icmp.Echo)
		// The kernel picks the id of a ping socket itself: match the
		// sequence number and the sender.
		if !ok || e.Seq != seq {
			continue
		}
		if u, ok := peer.(*net.UDPAddr); ok && !u.IP.Equal(dst.IP) {
			continue
		}
		return time.Since(start), nil
	}
}
