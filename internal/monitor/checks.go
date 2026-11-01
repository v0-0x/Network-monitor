package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/v0-0x/Network-monitor/internal/logs"
	"github.com/v0-0x/Network-monitor/internal/ping"
	"github.com/v0-0x/Network-monitor/internal/risk"
)

// Ping checks: every host on the internet is pinged (3 ICMP echoes) when
// it's first seen and again every minute while connected, at most four at
// a time. Only ICMP is sent by itself; where ICMP isn't allowed, the Ping
// button in the app times a TCP connection instead.

const (
	pingEvery   = time.Minute
	pingCount   = 3
	pingTimeout = time.Second
	pingWorkers = 4
)

// queuePings starts the pings that are due. Called with m.mu held.
func (m *Monitor) queuePings(ctx context.Context, hosts map[string]Host, now time.Time) {
	if !m.icmpOK {
		return
	}
	for ip, h := range hosts {
		if h.Scope != ScopePublic || m.pinging[ip] || len(m.pinging) >= pingWorkers {
			continue
		}
		if r, ok := m.pings[ip]; ok && now.Sub(time.UnixMilli(r.At)) < pingEvery {
			continue
		}
		a, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		m.pinging[ip] = true
		go func() {
			r, err := ping.ICMP(ctx, a, pingCount, pingTimeout)
			m.mu.Lock()
			defer m.mu.Unlock()
			delete(m.pinging, ip)
			if errors.Is(err, ping.ErrNoICMP) {
				m.icmpOK = false
				m.icmpNote = icmpHelp()
				return
			}
			if err != nil {
				r.Error = err.Error()
			}
			m.pings[ip] = r
		}()
	}
	// Forget hosts gone for a while.
	for ip, r := range m.pings {
		if _, ok := hosts[ip]; !ok && now.Sub(time.UnixMilli(r.At)) > 10*time.Minute {
			delete(m.pings, ip)
		}
	}
}

func icmpHelp() string {
	return "This system doesn't let programs send ICMP (ping) without administrator rights. On Linux, allow it with: sudo sysctl net.ipv4.ping_group_range='0 2147483647'. The Ping button then times a TCP connection instead."
}

// Ping checks one host now: ICMP when allowed, otherwise the time to open
// a TCP connection to the port it's connected on (or 443).
func (m *Monitor) Ping(ctx context.Context, ipStr string) (ping.Result, error) {
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return ping.Result{}, err
	}
	m.mu.Lock()
	icmpOK := m.icmpOK
	port := uint16(443)
	for _, t := range m.conns {
		if t.RemoteIP == ipStr && t.Dir == "out" && t.Proto == "tcp" {
			port = t.RemotePort
			break
		}
	}
	m.mu.Unlock()
	var r ping.Result
	if icmpOK {
		r, err = ping.ICMP(ctx, ip, 4, pingTimeout)
		if errors.Is(err, ping.ErrNoICMP) {
			m.mu.Lock()
			m.icmpOK, m.icmpNote = false, icmpHelp()
			m.mu.Unlock()
			icmpOK = false
		}
	}
	if !icmpOK || r.Received == 0 {
		// No ICMP, or no answer to it (many servers drop pings): try TCP.
		tr := ping.TCP(ctx, ip, port, 3, 2*time.Second)
		if tr.Received > 0 || !icmpOK {
			if icmpOK && tr.Received > 0 {
				tr.Error = "no answer to ping (many servers ignore it); timed a TCP connection instead"
			}
			r = tr
		} else {
			r.Error = "no answer to ping or on TCP port " + fmt.Sprint(port)
		}
	}
	m.mu.Lock()
	m.pings[ipStr] = r
	m.mu.Unlock()
	m.Rescan()
	return r, nil
}

func alertEvent(ip string, h Host, conns []risk.Conn, at int64) logs.Event {
	seen := map[string]bool{}
	var names []string
	for _, c := range conns {
		if c.Process != "" && !seen[c.Process] {
			seen[c.Process] = true
			names = append(names, c.Process)
		}
	}
	return logs.Event{
		Time: at, Kind: logs.Alert, IP: ip, Host: h.Name, Place: h.Place(), Country: h.CountryCode, Org: h.Org,
		Process: strings.Join(names, ", "), Risk: h.Risk.Score,
		Message: strings.Join(h.Risk.Reasons, ". "),
	}
}

// blocklistLoop downloads the blocklists when they're due (twice a day).
func (m *Monitor) blocklistLoop(ctx context.Context) {
	if m.opt.Blocklists == nil {
		return
	}
	check := func() {
		if !m.opt.Settings.Get().Blocklists || !m.opt.Blocklists.Due() {
			return
		}
		first := m.opt.Blocklists.Status().Updated.IsZero()
		if err := m.opt.Blocklists.Update(ctx); err != nil {
			m.opt.Logs.Add(logs.Event{Kind: logs.Error, Message: "Blocklists: " + err.Error()})
			return
		}
		if first {
			n := 0
			for _, l := range m.opt.Blocklists.Status().Lists {
				n += l.Entries
			}
			m.opt.Logs.Add(logs.Event{Kind: logs.Note, Message: fmt.Sprintf("Blocklists downloaded (%d entries); addresses on them get a high risk score", n)})
		}
		m.Rescan()
	}
	check()
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

// UpdateBlocklists downloads the lists now.
func (m *Monitor) UpdateBlocklists(ctx context.Context) error {
	if m.opt.Blocklists == nil {
		return nil
	}
	err := m.opt.Blocklists.Update(ctx)
	m.Rescan()
	return err
}
