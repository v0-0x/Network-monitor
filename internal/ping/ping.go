// Package ping measures how long a host takes to answer: ICMP echo (what
// the ping command sends), or when that isn't allowed, how long a TCP
// connection to it takes to open.
//
// No administrator rights: Linux and macOS allow ICMP through "ping
// sockets" (Linux: when net.ipv4.ping_group_range includes your group, as
// systemd sets it on Arch), and Windows has IcmpSendEcho.
package ping

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"
)

// Result is one check of a host.
type Result struct {
	Sent     int     `json:"sent"`
	Received int     `json:"received"`
	Min      float64 `json:"min"` // milliseconds
	Avg      float64 `json:"avg"`
	Max      float64 `json:"max"`
	Method   string  `json:"method"` // "icmp" or "tcp"
	At       int64   `json:"at"`     // Unix ms
	Error    string  `json:"error,omitempty"`
}

// Loss is the share of probes that got no answer, 0 to 1.
func (r Result) Loss() float64 {
	if r.Sent == 0 {
		return 0
	}
	return float64(r.Sent-r.Received) / float64(r.Sent)
}

// ErrNoICMP is returned when this system doesn't let the app send ICMP.
var ErrNoICMP = errors.New("ICMP isn't allowed for this user")

// ICMP pings ip count times, one probe a second at most.
func ICMP(ctx context.Context, ip netip.Addr, count int, timeout time.Duration) (Result, error) {
	r := Result{Method: "icmp", At: time.Now().UnixMilli()}
	var rtts []time.Duration
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		rtt, err := echo(ip, i+1, timeout)
		r.Sent++
		if errors.Is(err, ErrNoICMP) {
			return r, err
		}
		if err != nil && i == 0 && !isTimeout(err) {
			// Not a lost packet but a probe that can't be sent at all.
			r.Error = err.Error()
			return r, err
		}
		if err == nil {
			r.Received++
			rtts = append(rtts, rtt)
		}
		if i < count-1 {
			wait := time.Second - time.Since(start)
			if wait > 0 {
				select {
				case <-ctx.Done():
				case <-time.After(wait):
				}
			}
		}
	}
	r.summarize(rtts)
	return r, nil
}

// TCP times opening a connection to ip:port (closed right away), count
// times.
func TCP(ctx context.Context, ip netip.Addr, port uint16, count int, timeout time.Duration) Result {
	r := Result{Method: "tcp", At: time.Now().UnixMilli()}
	var rtts []time.Duration
	d := net.Dialer{Timeout: timeout}
	for i := 0; i < count && ctx.Err() == nil; i++ {
		start := time.Now()
		c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(int(port))))
		r.Sent++
		if err == nil {
			rtts = append(rtts, time.Since(start))
			r.Received++
			c.Close()
		} else if r.Error == "" {
			r.Error = fmt.Sprintf("no answer on TCP port %d", port)
		}
		if i < count-1 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	r.summarize(rtts)
	return r
}

func (r *Result) summarize(rtts []time.Duration) {
	if len(rtts) == 0 {
		if r.Error == "" {
			r.Error = "no answer"
		}
		return
	}
	r.Error = ""
	var sum time.Duration
	minD, maxD := rtts[0], rtts[0]
	for _, d := range rtts {
		sum += d
		minD = min(minD, d)
		maxD = max(maxD, d)
	}
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	r.Min, r.Max, r.Avg = ms(minD), ms(maxD), ms(sum/time.Duration(len(rtts)))
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout() || err.Error() == "no answer"
}
