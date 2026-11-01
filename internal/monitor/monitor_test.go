package monitor

import (
	"context"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"

	"github.com/v0-0x/Network-monitor/internal/geo"
	"github.com/v0-0x/Network-monitor/internal/logs"
	"github.com/v0-0x/Network-monitor/internal/netstat"
	"github.com/v0-0x/Network-monitor/internal/settings"
)

type fakeSource struct {
	mu    sync.Mutex
	socks []netstat.Socket
}

func (f *fakeSource) Sockets(ctx context.Context) ([]netstat.Socket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]netstat.Socket(nil), f.socks...), nil
}
func (f *fakeSource) Processes(pids []int32) map[int32]netstat.Process {
	return map[int32]netstat.Process{10: {Name: "firefox"}, 20: {Name: "sshd"}}
}
func (f *fakeSource) Traffic() (uint64, uint64, bool) { return 0, 0, false }
func (f *fakeSource) Method() string                  { return "fake" }

func sock(proto, local, remote, state string, pid int32) netstat.Socket {
	s := netstat.Socket{Proto: proto, Local: netip.MustParseAddrPort(local), State: state, PID: pid}
	if remote != "" {
		s.Remote = netip.MustParseAddrPort(remote)
	}
	return s
}

func TestScanEvents(t *testing.T) {
	dir := t.TempDir()
	st := settings.Load(filepath.Join(dir, "settings.json"))
	st.Update(func(s *settings.Settings) { s.ResolveNames = false })
	lg, _ := logs.Open(filepath.Join(dir, "logs"))
	defer lg.Close() // Windows can't delete the test folder with the log open
	g := geo.Open(filepath.Join(dir, "geo"))
	defer g.Close()
	var snaps []Snapshot
	m := New(Options{Settings: st, Geo: g, Logs: lg, OnSnapshot: func(s Snapshot) { snaps = append(snaps, s) }})
	src := &fakeSource{}
	m.src = src

	src.socks = []netstat.Socket{
		sock("tcp", "0.0.0.0:22", "", netstat.Listen, 20),
		sock("tcp", "192.168.1.5:50000", "142.250.74.46:443", netstat.Established, 10),
		sock("tcp", "127.0.0.1:40000", "127.0.0.1:631", netstat.Established, 10), // loopback: hidden
	}
	m.scan(context.Background())
	if n := len(snaps[0].Conns); n != 1 {
		t.Fatalf("first scan: %d connections, want 1 (loopback hidden)", n)
	}
	if c := snaps[0].Conns[0]; c.Dir != "out" || c.Process != "firefox" || c.Service != "HTTPS" || !c.Existing || c.Scope != ScopePublic {
		t.Errorf("first connection: %+v", c)
	}
	if len(snaps[0].Listeners) != 1 || snaps[0].Listeners[0].Service != "SSH" || snaps[0].Listeners[0].Process != "sshd" {
		t.Errorf("listeners: %+v", snaps[0].Listeners)
	}

	// Someone connects to SSH; firefox's connection closes; a UDP port opens.
	src.socks = []netstat.Socket{
		sock("tcp", "0.0.0.0:22", "", netstat.Listen, 20),
		sock("tcp", "192.168.1.5:22", "81.2.69.142:61000", netstat.Established, 20),
		sock("udp", "0.0.0.0:5353", "", netstat.Listen, 0),
		sock("tcp", "192.168.1.5:50001", "142.250.74.46:443", netstat.TimeWait, 0),
	}
	m.scan(context.Background())
	s := snaps[1]
	if len(s.Conns) != 1 || s.Conns[0].Dir != "in" || s.Conns[0].Service != "SSH" || s.Conns[0].Existing {
		t.Fatalf("second scan: %+v", s.Conns)
	}
	if s.Totals.Opened != 1 || s.Totals.Inbound != 1 {
		t.Errorf("totals: %+v", s.Totals)
	}
	page, err := lg.Search(logs.Query{})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range page.Events {
		kinds[e.Kind]++
	}
	// Opened: firefox (existing) and the SSH visitor. Closed: firefox.
	// Listen: mDNS. Note: started.
	if kinds[logs.Opened] != 2 || kinds[logs.Closed] != 1 || kinds[logs.Listen] != 1 || kinds[logs.Note] != 1 {
		t.Errorf("events: %v\n%+v", kinds, page.Events)
	}
	for _, e := range page.Events {
		if e.Kind == logs.Closed && (e.Process != "firefox" || e.Duration < 0 || e.Port != 443) {
			t.Errorf("close event: %+v", e)
		}
	}
	// Search and CSV.
	if p, _ := lg.Search(logs.Query{Text: "ssh incoming"}); len(p.Events) != 1 {
		t.Errorf("search ssh incoming: %+v", p.Events)
	}
}
