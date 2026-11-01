// Package monitor watches this computer's connections: it reads the socket
// tables over and over (internal/netstat), notices what opened and closed,
// finds where every address is (internal/geo) and what it's called
// (internal/rdns), logs it (internal/logs) and hands the UI a snapshot after
// every scan.
package monitor

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/v0-0x/Network-monitor/internal/geo"
	"github.com/v0-0x/Network-monitor/internal/logs"
	"github.com/v0-0x/Network-monitor/internal/netstat"
	"github.com/v0-0x/Network-monitor/internal/ping"
	"github.com/v0-0x/Network-monitor/internal/publicip"
	"github.com/v0-0x/Network-monitor/internal/rdns"
	"github.com/v0-0x/Network-monitor/internal/risk"
	"github.com/v0-0x/Network-monitor/internal/settings"
)

// Conn is one open connection.
type Conn struct {
	ID         string `json:"id"`
	Proto      string `json:"proto"`
	LocalIP    string `json:"localIp"`
	LocalPort  uint16 `json:"localPort"`
	RemoteIP   string `json:"remoteIp"`
	RemotePort uint16 `json:"remotePort"`
	State      string `json:"state"`
	PID        int32  `json:"pid"`
	Process    string `json:"process"`
	Exe        string `json:"exe,omitempty"`
	Dir        string `json:"dir"`   // "out": this computer connected; "in": the other end did
	Scope      string `json:"scope"` // public, local, loopback
	Service    string `json:"service,omitempty"`
	Since      int64  `json:"since"` // Unix ms when first seen
	Existing   bool   `json:"existing,omitempty"`

	// Traffic, where the system counts it (Snapshot.PerConn): bytes so
	// far, and bytes per second over the last scan.
	HasBytes bool    `json:"hasBytes,omitempty"`
	Rx       uint64  `json:"rx"`
	Tx       uint64  `json:"tx"`
	RxRate   float64 `json:"rxRate"`
	TxRate   float64 `json:"txRate"`
	RTT      float64 `json:"rtt,omitempty"` // the system's smoothed round-trip time, ms
}

// Host is what is known about a remote address.
type Host struct {
	IP    string `json:"ip"`
	Name  string `json:"name,omitempty"` // reverse DNS
	Scope string `json:"scope"`
	geo.Location
	Ping   *ping.Result    `json:"ping,omitempty"` // the last ping check
	Risk   risk.Assessment `json:"risk"`
	Listed []string        `json:"listed,omitempty"` // blocklists it's on
}

// Listener is a port this computer offers.
type Listener struct {
	Proto   string `json:"proto"`
	IP      string `json:"ip"`
	Port    uint16 `json:"port"`
	PID     int32  `json:"pid"`
	Process string `json:"process"`
	Exe     string `json:"exe,omitempty"`
	Service string `json:"service,omitempty"`
	Scope   string `json:"scope"` // "public": every network; "local"/"loopback": only there
	Since   int64  `json:"since"`
}

// Me is where "you" are.
type Me struct {
	Known  bool   `json:"known"`
	Manual bool   `json:"manual"`
	IP     string `json:"ip,omitempty"`
	Label  string `json:"label"`
	geo.Location
	Error   string `json:"error,omitempty"`
	Waiting bool   `json:"waiting,omitempty"` // for the location database
}

// Totals count what was seen since the app started.
type Totals struct {
	Since     int64 `json:"since"`
	Opened    int   `json:"opened"`
	Hosts     int   `json:"hosts"`
	Countries int   `json:"countries"`
	Inbound   int   `json:"inbound"`
}

// Snapshot is the state after a scan, for the UI.
type Snapshot struct {
	Time      int64           `json:"time"`
	Conns     []Conn          `json:"conns"`
	Hosts     map[string]Host `json:"hosts"`
	Listeners []Listener      `json:"listeners"`
	Me        Me              `json:"me"`
	Rx        float64         `json:"rx"` // bytes per second, all interfaces
	Tx        float64         `json:"tx"`
	ScanMs    float64         `json:"scanMs"`
	Method    string          `json:"method"`
	Error     string          `json:"error,omitempty"`
	Demo      bool            `json:"demo"`
	Totals    Totals          `json:"totals"`

	PerConn     bool   `json:"perConn"`     // traffic per connection is counted
	PerConnNote string `json:"perConnNote"` // and what to know about it
	ICMP        bool   `json:"icmp"`        // ping checks can use ICMP
	ICMPNote    string `json:"icmpNote,omitempty"`
}

type source interface {
	Sockets(ctx context.Context) ([]netstat.Socket, error)
	Processes(pids []int32) map[int32]netstat.Process
	Traffic() (rx, tx uint64, ok bool)
	Method() string
	PerConnection() (bool, string)
}

type tracked struct {
	Conn
	logged   bool
	openedAt time.Time
	bytesAt  time.Time // when Rx and Tx were last read
}

// Options configure a Monitor.
type Options struct {
	Settings   *settings.Store
	Geo        *geo.DB
	Blocklists *risk.Blocklists
	Logs       *logs.Store
	OnSnapshot func(Snapshot)
	Demo       bool
}

// Monitor is the watcher. Run starts it.
type Monitor struct {
	opt  Options
	src  source
	rdns *rdns.Resolver

	mu        sync.Mutex
	conns     map[string]*tracked
	listeners map[string]Listener
	scans     int
	me        Me
	publicIP  netip.Addr
	lastRx    uint64
	lastTx    uint64
	lastAt    time.Time
	rx, tx    float64
	started   time.Time
	opened    int
	inbound   int
	hostsSeen map[string]bool
	countries map[string]bool
	last      Snapshot
	wake      chan struct{}
	relocate  chan struct{}
	ifaces    string

	pings    map[string]ping.Result // by IP
	pinging  map[string]bool
	icmpOK   bool
	icmpNote string
	alerted  map[string]bool // hosts already logged as high risk
}

// New returns a Monitor.
func New(opt Options) *Monitor {
	m := &Monitor{
		opt:       opt,
		rdns:      rdns.New(),
		conns:     map[string]*tracked{},
		listeners: map[string]Listener{},
		started:   time.Now(),
		hostsSeen: map[string]bool{},
		countries: map[string]bool{},
		wake:      make(chan struct{}, 1),
		relocate:  make(chan struct{}, 1),
		pings:     map[string]ping.Result{},
		pinging:   map[string]bool{},
		icmpOK:    true,
		alerted:   map[string]bool{},
	}
	if opt.Demo {
		m.src = newDemoSource()
		if opt.Blocklists != nil {
			opt.Blocklists.AddDemo(netip.MustParseAddr(demoListed), risk.Lists[0])
		}
	} else {
		m.src = netstat.NewScanner()
	}
	return m
}

// Run watches until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	go m.locateLoop(ctx)
	go m.geoLoop(ctx)
	go m.blocklistLoop(ctx)
	m.opt.Logs.Prune(m.opt.Settings.Get().LogDays)
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		start := time.Now()
		m.scan(ctx)
		took := time.Since(start)
		interval := time.Duration(m.opt.Settings.Get().IntervalMs) * time.Millisecond
		// A slow table (thousands of sockets, or lsof on a Mac) isn't read
		// more than a quarter of the time.
		if 4*took > interval {
			interval = 4 * took
		}
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-prune.C:
			m.opt.Logs.Prune(m.opt.Settings.Get().LogDays)
		case <-time.After(interval):
		}
	}
}

// Rescan scans again now (after a setting changed).
func (m *Monitor) Rescan() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Relocate looks for "you" again now (the location setting changed).
func (m *Monitor) Relocate() {
	select {
	case m.relocate <- struct{}{}:
	default:
	}
}

// Last returns the newest snapshot.
func (m *Monitor) Last() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

func connKey(s netstat.Socket) string {
	return s.Proto + "|" + s.Local.String() + "|" + s.Remote.String()
}

func (m *Monitor) scan(ctx context.Context) {
	start := time.Now()
	set := m.opt.Settings.Get()
	socks, err := m.src.Sockets(ctx)
	now := time.Now()
	nowMs := now.UnixMilli()

	// Ports this computer offers.
	tcpListen := map[uint16][]netip.Addr{}
	curListen := map[string]Listener{}
	for _, s := range socks {
		if s.State != netstat.Listen || s.HasRemote() {
			continue
		}
		if s.Proto == "tcp" {
			tcpListen[s.Local.Port()] = append(tcpListen[s.Local.Port()], s.Local.Addr())
		} else if s.Local.Port() >= 32768 {
			continue // a program's own UDP socket waiting for answers, not a service
		}
		scope := ScopePublic
		if !s.Local.Addr().IsUnspecified() {
			scope = scopeOf(s.Local.Addr())
		}
		k := s.Proto + "|" + s.Local.String()
		curListen[k] = Listener{Proto: s.Proto, IP: s.Local.Addr().String(), Port: s.Local.Port(), PID: s.PID, Scope: scope, Service: serviceName(s.Proto, s.Local.Port())}
	}

	// Connections.
	type found struct {
		s     netstat.Socket
		scope string
	}
	var cur []found
	pidSet := map[int32]bool{}
	for _, s := range socks {
		if !s.HasRemote() {
			continue
		}
		switch s.State {
		case netstat.Listen, netstat.TimeWait, netstat.Close:
			continue
		}
		scope := scopeOf(s.Remote.Addr())
		if scope == ScopeLoopback && !set.ShowLocal {
			continue
		}
		cur = append(cur, found{s, scope})
		if s.PID > 0 {
			pidSet[s.PID] = true
		}
	}
	for _, l := range curListen {
		if l.PID > 0 {
			pidSet[l.PID] = true
		}
	}
	pids := make([]int32, 0, len(pidSet))
	for p := range pidSet {
		pids = append(pids, p)
	}
	procs := m.src.Processes(pids)

	events, snap := func() ([]logs.Event, Snapshot) {
		m.mu.Lock()
		defer m.mu.Unlock()
		first := m.scans == 0
		m.scans++
		var events []logs.Event

		seen := make(map[string]bool, len(cur))
		for _, f := range cur {
			s := f.s
			k := connKey(s)
			seen[k] = true
			p := procs[s.PID]
			t, ok := m.conns[k]
			if !ok {
				dir := "out"
				if isInbound(s, tcpListen) {
					dir = "in"
				}
				svcPort := s.Remote.Port()
				if dir == "in" {
					svcPort = s.Local.Port()
				}
				t = &tracked{Conn: Conn{
					ID:         k,
					Proto:      s.Proto,
					LocalIP:    s.Local.Addr().String(),
					LocalPort:  s.Local.Port(),
					RemoteIP:   s.Remote.Addr().String(),
					RemotePort: s.Remote.Port(),
					Dir:        dir,
					Scope:      f.scope,
					Service:    serviceName(s.Proto, svcPort),
					Since:      nowMs,
					Existing:   first,
				}, openedAt: now}
				m.conns[k] = t
				if !first {
					m.opened++
					if dir == "in" {
						m.inbound++
					}
				}
			}
			t.State = s.State
			if s.PID > 0 || t.PID == 0 {
				t.PID = s.PID
			}
			if p.Name != "" {
				t.Process, t.Exe = p.Name, p.Exe
			}
			if s.HasBytes {
				if t.HasBytes && !t.bytesAt.IsZero() && s.Rx >= t.Rx && s.Tx >= t.Tx {
					if dt := now.Sub(t.bytesAt).Seconds(); dt > 0 {
						t.RxRate = float64(s.Rx-t.Rx) / dt
						t.TxRate = float64(s.Tx-t.Tx) / dt
					}
				}
				t.Rx, t.Tx, t.HasBytes, t.bytesAt = s.Rx, s.Tx, true, now
			}
			if s.RTT > 0 {
				t.RTT = float64(s.RTT.Microseconds()) / 1000
			}
		}

		// Hosts, with their places and names.
		hosts := map[string]Host{}
		for _, t := range m.conns {
			if !seen[t.ID] {
				continue
			}
			if _, ok := hosts[t.RemoteIP]; ok {
				continue
			}
			hosts[t.RemoteIP] = m.host(t.RemoteIP, t.Scope, set.ResolveNames)
		}

		// Risk and ping results.
		byHost := map[string][]risk.Conn{}
		for _, t := range m.conns {
			if seen[t.ID] {
				byHost[t.RemoteIP] = append(byHost[t.RemoteIP], risk.Conn{Proto: t.Proto, Dir: t.Dir, LocalPort: t.LocalPort, RemotePort: t.RemotePort, Process: t.Process, Exe: t.Exe, PID: t.PID})
			}
		}
		for ip, h := range hosts {
			in := risk.Host{Public: h.Scope == ScopePublic, Name: h.Name, Org: h.Org, Conns: byHost[ip]}
			if in.Public && set.Blocklists && m.opt.Blocklists != nil {
				if a, err := netip.ParseAddr(ip); err == nil {
					in.Listed = m.opt.Blocklists.Lookup(a)
					for _, l := range in.Listed {
						h.Listed = append(h.Listed, l.Name)
					}
				}
			}
			h.Risk = risk.Assess(in)
			if r, ok := m.pings[ip]; ok {
				r := r
				h.Ping = &r
			}
			hosts[ip] = h
			if h.Risk.Level == risk.High && !m.alerted[ip] {
				m.alerted[ip] = true
				events = append(events, alertEvent(ip, h, byHost[ip], nowMs))
			}
		}
		if set.PingHosts {
			m.queuePings(ctx, hosts, now)
		}

		// Log what opened (once its name is known, or after 3 s) and closed.
		for k, t := range m.conns {
			gone := !seen[k]
			if !t.logged && (gone || m.readyToLog(t, set, now)) {
				if m.shouldLog(t.Conn, set) {
					h := hosts[t.RemoteIP]
					if gone {
						h = m.host(t.RemoteIP, t.Scope, false)
					}
					events = append(events, connEvent(logs.Opened, t, h, t.Since))
				}
				t.logged = true
			}
			if gone {
				if m.shouldLog(t.Conn, set) {
					h := m.host(t.RemoteIP, t.Scope, false)
					e := connEvent(logs.Closed, t, h, nowMs)
					e.Duration = nowMs - t.Since
					e.Rx, e.Tx = t.Rx, t.Tx
					events = append(events, e)
				}
				delete(m.conns, k)
			}
		}

		// Ports opened and closed.
		for k, l := range curListen {
			old, ok := m.listeners[k]
			if ok {
				l.Since = old.Since
			} else {
				l.Since = nowMs
			}
			if p := procs[l.PID]; p.Name != "" {
				l.Process, l.Exe = p.Name, p.Exe
			} else if ok {
				l.Process, l.Exe = old.Process, old.Exe
			}
			curListen[k] = l
			if !ok && !first {
				events = append(events, listenEvent(logs.Listen, l, nowMs))
			}
		}
		for k, l := range m.listeners {
			if _, ok := curListen[k]; !ok {
				events = append(events, listenEvent(logs.Unlisten, l, nowMs))
			}
		}
		m.listeners = curListen

		// New countries and hosts, for the totals.
		for ip, h := range hosts {
			if h.Scope != ScopePublic {
				continue
			}
			m.hostsSeen[ip] = true
			if h.CountryCode != "" && !m.countries[h.CountryCode] {
				m.countries[h.CountryCode] = true
			}
		}

		// Throughput.
		if rx, tx, ok := m.src.Traffic(); ok {
			if !m.lastAt.IsZero() && rx >= m.lastRx && tx >= m.lastTx {
				dt := now.Sub(m.lastAt).Seconds()
				if dt > 0 {
					m.rx = float64(rx-m.lastRx) / dt
					m.tx = float64(tx-m.lastTx) / dt
				}
			}
			m.lastRx, m.lastTx, m.lastAt = rx, tx, now
		}

		if first {
			n := 0
			for _, t := range m.conns {
				if t.Scope == ScopePublic {
					n++
				}
			}
			events = append(events, logs.Event{Time: nowMs, Kind: logs.Note, Message: fmt.Sprintf("Monitoring started: %d connections and %d open ports", n, len(curListen))})
		}
		if err != nil {
			if m.scans == 1 || m.last.Error == "" {
				events = append(events, logs.Event{Time: nowMs, Kind: logs.Error, Message: "Couldn't read the connection table: " + err.Error()})
			}
		}

		snap := Snapshot{
			Time:      nowMs,
			Hosts:     hosts,
			Me:        m.me,
			Rx:        m.rx,
			Tx:        m.tx,
			ScanMs:    float64(time.Since(start).Microseconds()) / 1000,
			Method:    m.src.Method(),
			Demo:      m.opt.Demo,
			Conns:     make([]Conn, 0, len(m.conns)),
			Listeners: make([]Listener, 0, len(curListen)),
			ICMP:      m.icmpOK,
			ICMPNote:  m.icmpNote,
			Totals: Totals{
				Since:     m.started.UnixMilli(),
				Opened:    m.opened,
				Hosts:     len(m.hostsSeen),
				Countries: len(m.countries),
				Inbound:   m.inbound,
			},
		}
		if err != nil {
			snap.Error = err.Error()
		}
		snap.PerConn, snap.PerConnNote = m.src.PerConnection()
		for _, t := range m.conns {
			snap.Conns = append(snap.Conns, t.Conn)
		}
		sort.Slice(snap.Conns, func(i, j int) bool {
			if snap.Conns[i].Since != snap.Conns[j].Since {
				return snap.Conns[i].Since > snap.Conns[j].Since
			}
			return snap.Conns[i].ID < snap.Conns[j].ID
		})
		for _, l := range curListen {
			snap.Listeners = append(snap.Listeners, l)
		}
		sort.Slice(snap.Listeners, func(i, j int) bool {
			a, b := snap.Listeners[i], snap.Listeners[j]
			if a.Port != b.Port {
				return a.Port < b.Port
			}
			if a.Proto != b.Proto {
				return a.Proto < b.Proto
			}
			return a.IP < b.IP
		})
		m.last = snap
		return events, snap
	}()

	// Outside the lock: the log and the UI call back.
	m.opt.Logs.Add(events...)
	if m.opt.OnSnapshot != nil {
		m.opt.OnSnapshot(snap)
	}
}

func (m *Monitor) readyToLog(t *tracked, set settings.Settings, now time.Time) bool {
	if now.Sub(t.openedAt) >= 3*time.Second || !set.ResolveNames || t.Scope == ScopeLoopback {
		return true
	}
	ip, err := netip.ParseAddr(t.RemoteIP)
	if err != nil {
		return true
	}
	_, done := m.rdns.Cached(ip)
	return done
}

// shouldLog: connections inside this computer only when asked for.
func (m *Monitor) shouldLog(c Conn, set settings.Settings) bool {
	return c.Scope != ScopeLoopback || set.ShowLocal
}

func (m *Monitor) host(ipStr, scope string, resolve bool) Host {
	h := Host{IP: ipStr, Scope: scope}
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return h
	}
	if scope == ScopePublic {
		h.Location = m.opt.Geo.Lookup(ip)
	}
	if resolve && scope != ScopeLoopback {
		h.Name = m.rdns.Name(ip)
	} else {
		h.Name, _ = m.rdns.Cached(ip)
	}
	return h
}

func connEvent(kind string, t *tracked, h Host, at int64) logs.Event {
	return logs.Event{
		Time:     at,
		Kind:     kind,
		Proto:    t.Proto,
		Process:  t.Process,
		PID:      t.PID,
		Local:    net.JoinHostPort(t.LocalIP, strconv.Itoa(int(t.LocalPort))),
		IP:       t.RemoteIP,
		Port:     t.RemotePort,
		Service:  t.Service,
		Host:     h.Name,
		Place:    h.Place(),
		Country:  h.CountryCode,
		Org:      h.Org,
		Dir:      t.Dir,
		Scope:    t.Scope,
		Existing: t.Existing && kind == logs.Opened,
	}
}

func listenEvent(kind string, l Listener, at int64) logs.Event {
	return logs.Event{
		Time:    at,
		Kind:    kind,
		Proto:   l.Proto,
		Process: l.Process,
		PID:     l.PID,
		Local:   net.JoinHostPort(l.IP, strconv.Itoa(int(l.Port))),
		Port:    l.Port,
		Service: l.Service,
		Scope:   l.Scope,
	}
}

// isInbound: the local port is one this computer listens on.
func isInbound(s netstat.Socket, tcpListen map[uint16][]netip.Addr) bool {
	if s.Proto != "tcp" {
		return false
	}
	for _, a := range tcpListen[s.Local.Port()] {
		if a.IsUnspecified() || a == s.Local.Addr() {
			return true
		}
	}
	return false
}

// locateLoop finds "you": from the settings, or from the public IP address,
// again every 10 minutes and whenever this computer's addresses change
// (a VPN connecting, another network).
func (m *Monitor) locateLoop(ctx context.Context) {
	m.locate(ctx)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.relocate:
		case <-tick.C:
			changed := m.interfacesChanged()
			if !changed && time.Since(last) < 10*time.Minute && (m.Me().Known || time.Since(last) < time.Minute) {
				continue
			}
		}
		last = time.Now()
		m.locate(ctx)
		m.Rescan()
	}
}

// Me returns where "you" are.
func (m *Monitor) Me() Me {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.me
}

func (m *Monitor) interfacesChanged() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	var parts []string
	for _, a := range addrs {
		parts = append(parts, a.String())
	}
	sort.Strings(parts)
	s := strings.Join(parts, ",")
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := m.ifaces != "" && s != m.ifaces
	m.ifaces = s
	return changed
}

func (m *Monitor) locate(ctx context.Context) {
	set := m.opt.Settings.Get().Location
	var me Me
	switch {
	case m.opt.Demo:
		me = Me{Known: true, Label: "Amsterdam, Netherlands", IP: "203.0.113.7"}
		me.Lat, me.Lon, me.HasCoords = 52.3676, 4.9041, true
		me.City, me.Country, me.CountryCode = "Amsterdam", "Netherlands", "NL"
	case set.Mode == "manual":
		me = Me{Known: true, Manual: true, Label: set.Label}
		me.Lat, me.Lon, me.HasCoords = set.Lat, set.Lon, true
		if me.Label == "" {
			me.Label = fmt.Sprintf("%.2f, %.2f", set.Lat, set.Lon)
		}
	default:
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		ip, err := publicip.Get(cctx)
		cancel()
		if err != nil {
			old := m.Me()
			if old.Known && !old.Manual {
				return // keep the last place: probably offline for a moment
			}
			me = Me{Error: "Your public address couldn't be found (offline?). Set your location in Settings."}
			break
		}
		loc := m.opt.Geo.Lookup(ip)
		me = Me{IP: ip.String(), Location: loc, Known: loc.HasCoords, Label: loc.Place()}
		if !loc.HasCoords {
			if m.opt.Geo.Ready() {
				me.Error = "Your address " + ip.String() + " isn't in the location database. Set your location in Settings."
			} else {
				me.Error, me.Waiting = "Waiting for the location database.", true
			}
		}
		if m.publicIP.IsValid() && m.publicIP != ip {
			oldPlace := m.Me().Label
			m.opt.Logs.Add(logs.Event{Kind: logs.Note, IP: ip.String(), Place: loc.Place(), Country: loc.CountryCode, Org: loc.Org,
				Message: fmt.Sprintf("Your public address changed from %s (%s) to %s (%s)", m.publicIP, orUnknown(oldPlace), ip, orUnknown(loc.Place()))})
		}
		m.publicIP = ip
	}
	m.mu.Lock()
	m.me = me
	m.mu.Unlock()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown place"
	}
	return s
}

// geoLoop downloads the location databases when there are none, and
// refreshes them monthly when allowed.
func (m *Monitor) geoLoop(ctx context.Context) {
	check := func(force bool) {
		if !force && !m.opt.Geo.Due() {
			return
		}
		if m.opt.Geo.Ready() && !m.opt.Settings.Get().GeoAuto && !force {
			return
		}
		had := m.opt.Geo.Ready()
		before := m.opt.Geo.Status()
		err := m.opt.Geo.Update(ctx, false)
		after := m.opt.Geo.Status()
		switch {
		case err != nil:
			m.opt.Logs.Add(logs.Event{Kind: logs.Error, Message: "Location database: " + err.Error()})
		case !had && after.Ready:
			m.opt.Logs.Add(logs.Event{Kind: logs.Note, Message: "Location database downloaded (DB-IP Lite " + after.CityVersion + ")"})
		case after.CityVersion != before.CityVersion:
			m.opt.Logs.Add(logs.Event{Kind: logs.Note, Message: "Location database updated to " + after.CityVersion})
		}
		if !had && after.Ready {
			m.Relocate()
		}
	}
	check(false)
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	retry := time.NewTimer(10 * time.Minute)
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			check(false)
		case <-retry.C:
			if !m.opt.Geo.Ready() {
				check(false)
				retry.Reset(10 * time.Minute)
			}
		}
	}
}

// UpdateGeo downloads the newest location databases now.
func (m *Monitor) UpdateGeo(ctx context.Context) error {
	had := m.opt.Geo.Ready()
	before := m.opt.Geo.Status()
	err := m.opt.Geo.Update(ctx, false)
	after := m.opt.Geo.Status()
	if err == nil && after.CityVersion != before.CityVersion {
		m.opt.Logs.Add(logs.Event{Kind: logs.Note, Message: "Location database updated to " + after.CityVersion})
	}
	if !had && after.Ready {
		m.Relocate()
	}
	m.Rescan()
	return err
}
