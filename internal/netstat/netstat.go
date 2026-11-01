// Package netstat reads this computer's socket tables: every TCP and UDP
// socket with its addresses, state and owning process, the way netstat, ss
// and lsof show them. Each system has its own reader (linux.go, darwin.go,
// windows.go); none of them needs administrator rights, although without
// them a few sockets of other users' processes come without a process.
package netstat

import (
	"context"
	"net/netip"
	"strings"
	"time"
)

// Socket is one row of the socket table.
type Socket struct {
	Proto  string         // "tcp" or "udp"
	Local  netip.AddrPort // the address on this computer
	Remote netip.AddrPort // the other end; not valid for listening sockets
	State  string         // see the State constants; "" for UDP
	PID    int32          // 0 when unknown (another user's process, or no process)
}

// TCP states, the same on every system.
const (
	Established = "ESTABLISHED"
	SynSent     = "SYN_SENT"
	SynRecv     = "SYN_RECV"
	FinWait1    = "FIN_WAIT1"
	FinWait2    = "FIN_WAIT2"
	TimeWait    = "TIME_WAIT"
	Close       = "CLOSE"
	CloseWait   = "CLOSE_WAIT"
	LastAck     = "LAST_ACK"
	Listen      = "LISTEN"
	Closing     = "CLOSING"
)

// Process is what is known about a process that owns sockets.
type Process struct {
	Name string // a short name to show ("firefox", "Google Chrome")
	Exe  string // the program's full path, when it can be read
}

type cachedProc struct {
	proc Process
	at   time.Time
}

// sysScanner is each system's reader.
type sysScanner interface {
	sockets(ctx context.Context) ([]Socket, error)
	processes(pids []int32) map[int32]Process
	traffic() (rx, tx uint64, ok bool)
	method() string
}

// Scanner reads the tables. Keep one for the life of the program: each
// system's reader keeps caches between scans (which socket belongs to which
// process, process names).
type Scanner struct {
	sys sysScanner
}

// NewScanner returns a Scanner for this system.
func NewScanner() *Scanner { return &Scanner{sys: newSysScanner()} }

// Sockets returns every TCP and UDP socket (IPv4 and IPv6).
func (s *Scanner) Sockets(ctx context.Context) ([]Socket, error) {
	socks, err := s.sys.sockets(ctx)
	for i := range socks {
		socks[i].Local = unmap(socks[i].Local)
		socks[i].Remote = unmap(socks[i].Remote)
	}
	return socks, err
}

// Processes returns the processes with these ids, as far as they can be read.
func (s *Scanner) Processes(pids []int32) map[int32]Process {
	return s.sys.processes(pids)
}

// Traffic returns the bytes received and sent by all network interfaces
// except loopback since the computer started. ok is false when the system
// doesn't say.
func (s *Scanner) Traffic() (rx, tx uint64, ok bool) {
	return s.sys.traffic()
}

// Method says how this system's table is read, for the About page.
func (s *Scanner) Method() string { return s.sys.method() }

// unmap turns IPv4-mapped IPv6 addresses (::ffff:1.2.3.4, as dual-stack
// sockets report them) into plain IPv4.
func unmap(ap netip.AddrPort) netip.AddrPort {
	if !ap.IsValid() {
		return ap
	}
	a := ap.Addr()
	if a.Is4In6() {
		return netip.AddrPortFrom(a.Unmap(), ap.Port())
	}
	return ap
}

// HasRemote reports whether the socket is connected to another address.
func (s Socket) HasRemote() bool {
	return s.Remote.IsValid() && !s.Remote.Addr().IsUnspecified() && s.Remote.Port() != 0
}

// cleanName tidies a process name for display: no ".exe", no path.
func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if len(name) > 4 && strings.EqualFold(name[len(name)-4:], ".exe") {
		name = name[:len(name)-4]
	}
	return name
}

// appBundleName returns "Google Chrome" for a path inside
// "/Applications/Google Chrome.app/…": the outermost app bundle, so that an
// app's helper processes are shown as the app.
func appBundleName(path string) string {
	i := strings.Index(path, ".app/")
	if i < 0 {
		if strings.HasSuffix(path, ".app") {
			i = len(path) - 4
		} else {
			return ""
		}
	}
	start := strings.LastIndex(path[:i], "/") + 1
	return path[start:i]
}
