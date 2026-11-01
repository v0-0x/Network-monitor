package monitor

import (
	"context"
	"math/rand"
	"net/netip"
	"sync"

	"github.com/v0-0x/Network-monitor/internal/netstat"
)

// Demo data (NETMON_DEMO=1): made-up connections from a few programs to
// servers around the world, so the globe can be tried and shown on a
// computer with little traffic. The app says "Demo data" while it's on.

type demoProc struct {
	pid  int32
	name string
	exe  string
}

var demoProcs = []demoProc{
	{4101, "firefox", "/usr/lib/firefox/firefox"},
	{4230, "discord", "/opt/discord/Discord"},
	{4388, "steam", "/usr/lib/steam/steam"},
	{4472, "spotify", "/opt/spotify/spotify"},
	{4519, "code", "/usr/lib/code/code"},
	{4602, "syncthing", "/usr/bin/syncthing"},
	{4705, "Aniko", "/usr/lib/aniko/aniko"},
}

type demoEndpoint struct {
	ip   string
	port uint16
	proc int // index in demoProcs
}

var demoEndpoints = []demoEndpoint{
	{"200.160.2.3", 443, 0}, {"196.216.2.6", 443, 0}, {"91.198.174.192", 443, 0}, {"208.80.154.224", 443, 0},
	{"77.88.55.242", 443, 0}, {"212.58.244.20", 443, 0}, {"195.154.122.1", 443, 0}, {"151.101.1.140", 443, 0},
	{"162.159.135.232", 443, 1}, {"104.16.132.229", 443, 1}, {"190.93.244.1", 443, 1},
	{"177.71.128.21", 27015, 2}, {"13.35.0.1", 443, 2}, {"176.32.103.205", 443, 2},
	{"35.186.224.25", 443, 3}, {"34.117.59.81", 4070, 3},
	{"20.70.246.20", 443, 4}, {"40.90.4.2", 443, 4}, {"185.199.108.153", 443, 4},
	{"84.17.46.1", 22000, 5}, {"192.36.148.17", 22000, 5},
	{"210.140.92.183", 443, 6}, {"202.12.27.33", 443, 6}, {"168.126.63.1", 443, 6},
	{"139.130.4.5", 443, 0}, {"122.56.237.1", 443, 0}, {"49.44.95.1", 443, 0}, {"41.203.67.183", 443, 0},
	{"102.132.96.35", 443, 0}, {"223.5.5.5", 443, 0}, {"94.140.14.14", 853, 0}, {"9.9.9.9", 853, 0},
}

// demoInbound are clients connecting to a port this computer offers.
var demoInbound = []string{"81.2.69.142", "5.255.255.70", "119.18.0.1", "200.221.11.100"}

type demoSource struct {
	mu   sync.Mutex
	rng  *rand.Rand
	open map[int]uint16 // endpoint index -> local port
	in   map[int]uint16
	rx   uint64
	tx   uint64
}

func newDemoSource() *demoSource {
	d := &demoSource{rng: rand.New(rand.NewSource(7)), open: map[int]uint16{}, in: map[int]uint16{}}
	for i := range demoEndpoints {
		if d.rng.Intn(3) != 0 {
			d.open[i] = uint16(40000 + d.rng.Intn(20000))
		}
	}
	d.in[0] = 51234
	return d
}

func (d *demoSource) Method() string { return "demo" }

func (d *demoSource) Sockets(ctx context.Context) ([]netstat.Socket, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Now and then one closes and another opens.
	for i := range demoEndpoints {
		_, isOpen := d.open[i]
		switch {
		case isOpen && d.rng.Intn(40) == 0:
			delete(d.open, i)
		case !isOpen && d.rng.Intn(25) == 0:
			d.open[i] = uint16(40000 + d.rng.Intn(20000))
		}
	}
	for i := range demoInbound {
		_, isOpen := d.in[i]
		switch {
		case isOpen && d.rng.Intn(30) == 0:
			delete(d.in, i)
		case !isOpen && d.rng.Intn(60) == 0:
			d.in[i] = uint16(40000 + d.rng.Intn(20000))
		}
	}
	local := netip.MustParseAddr("192.168.1.20")
	var out []netstat.Socket
	for i, lport := range d.open {
		e := demoEndpoints[i]
		proto := "tcp"
		if e.port == 443 && i%5 == 0 {
			proto = "udp" // QUIC
		}
		out = append(out, netstat.Socket{
			Proto:  proto,
			Local:  netip.AddrPortFrom(local, lport),
			Remote: netip.AddrPortFrom(netip.MustParseAddr(e.ip), e.port),
			State:  netstat.Established,
			PID:    demoProcs[e.proc].pid,
		})
	}
	// A game server this computer runs, and its visitors.
	out = append(out, netstat.Socket{Proto: "tcp", Local: netip.MustParseAddrPort("0.0.0.0:25565"), State: netstat.Listen, PID: 4800})
	out = append(out, netstat.Socket{Proto: "tcp", Local: netip.MustParseAddrPort("127.0.0.1:631"), State: netstat.Listen, PID: 0})
	for i, rport := range d.in {
		out = append(out, netstat.Socket{
			Proto:  "tcp",
			Local:  netip.AddrPortFrom(local, 25565),
			Remote: netip.AddrPortFrom(netip.MustParseAddr(demoInbound[i]), rport),
			State:  netstat.Established,
			PID:    4800,
		})
	}
	d.rx += uint64(200000 + d.rng.Intn(3000000))
	d.tx += uint64(20000 + d.rng.Intn(400000))
	return out, nil
}

func (d *demoSource) Processes(pids []int32) map[int32]netstat.Process {
	out := map[int32]netstat.Process{4800: {Name: "java", Exe: "/usr/bin/java (Minecraft server)"}}
	for _, p := range demoProcs {
		out[p.pid] = netstat.Process{Name: p.name, Exe: p.exe}
	}
	return out
}

func (d *demoSource) Traffic() (rx, tx uint64, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rx, d.tx, true
}
