//go:build linux

package netstat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Linux: the kernel's tables in /proc/net/{tcp,tcp6,udp,udp6} list every
// socket of this network namespace with its inode. Which process owns an
// inode is found by reading the links in /proc/<pid>/fd, which is only
// possible for this user's own processes (root's and other users' sockets
// stay without a process). That walk is the expensive part, so it only runs
// again when a socket shows up whose owner isn't known yet.

type linuxScanner struct {
	procRoot string

	mu      sync.Mutex
	owner   map[uint64]int32     // socket inode -> pid
	unowned map[uint64]time.Time // inodes no readable process has (other users')
	names   map[int32]cachedProc
}

func newSysScanner() sysScanner {
	return &linuxScanner{procRoot: "/proc", owner: map[uint64]int32{}, unowned: map[uint64]time.Time{}, names: map[int32]cachedProc{}}
}

func (l *linuxScanner) method() string { return "/proc/net" }

type linuxSock struct {
	Socket
	inode uint64
}

func (l *linuxScanner) sockets(ctx context.Context) ([]Socket, error) {
	var all []linuxSock
	var firstErr error
	read := 0
	for _, t := range []struct{ file, proto string }{{"tcp", "tcp"}, {"tcp6", "tcp"}, {"udp", "udp"}, {"udp6", "udp"}} {
		data, err := os.ReadFile(filepath.Join(l.procRoot, "net", t.file))
		if err != nil {
			// No IPv6 in this kernel: no tcp6/udp6. Not an error.
			if !errors.Is(err, os.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		read++
		all = append(all, parseProcNet(data, t.proto)...)
	}
	if read == 0 {
		if firstErr == nil {
			firstErr = errors.New("no socket tables in /proc/net")
		}
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	present := make(map[uint64]bool, len(all))
	unknown := false
	for _, s := range all {
		if s.inode == 0 {
			continue
		}
		present[s.inode] = true
		if _, ok := l.owner[s.inode]; ok {
			continue
		}
		if _, ok := l.unowned[s.inode]; ok {
			continue
		}
		unknown = true
	}
	if unknown {
		l.owner = l.walkFDs()
		now := time.Now()
		for ino := range present {
			if _, ok := l.owner[ino]; !ok {
				l.unowned[ino] = now
			}
		}
	}
	// Forget sockets that are gone.
	for ino := range l.unowned {
		if !present[ino] {
			delete(l.unowned, ino)
		}
	}
	for ino := range l.owner {
		if !present[ino] {
			delete(l.owner, ino)
		}
	}
	out := make([]Socket, 0, len(all))
	for _, s := range all {
		if s.inode != 0 {
			s.PID = l.owner[s.inode]
		}
		out = append(out, s.Socket)
	}
	return out, nil
}

// walkFDs maps socket inodes to the processes that have them open.
func (l *linuxScanner) walkFDs() map[uint64]int32 {
	owner := map[uint64]int32{}
	procs, err := os.ReadDir(l.procRoot)
	if err != nil {
		return owner
	}
	for _, p := range procs {
		pid, err := strconv.ParseInt(p.Name(), 10, 32)
		if err != nil {
			continue
		}
		fdDir := filepath.Join(l.procRoot, p.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // another user's process
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			ino, err := strconv.ParseUint(strings.TrimSuffix(link[len("socket:["):], "]"), 10, 64)
			if err != nil {
				continue
			}
			// A socket shared by a parent and its children (a server that
			// forked): the lowest pid, the parent, is the one to show.
			if old, ok := owner[ino]; !ok || int32(pid) < old {
				owner[ino] = int32(pid)
			}
		}
	}
	return owner
}

var linuxStates = map[string]string{
	"01": Established, "02": SynSent, "03": SynRecv, "04": FinWait1, "05": FinWait2,
	"06": TimeWait, "07": Close, "08": CloseWait, "09": LastAck, "0A": Listen, "0B": Closing,
}

// parseProcNet parses one /proc/net/{tcp,tcp6,udp,udp6} file.
//
//	sl  local_address rem_address   st tx_queue:rx_queue tr:tm->when retrnsmt   uid  timeout inode
//	0: 0100007F:0277 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 23471 ...
func parseProcNet(data []byte, proto string) []linuxSock {
	var out []linuxSock
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue // header
		}
		f := strings.Fields(sc.Text())
		if len(f) < 10 {
			continue
		}
		local, ok1 := parseHexAddr(f[1])
		remote, ok2 := parseHexAddr(f[2])
		if !ok1 || !ok2 {
			continue
		}
		ino, _ := strconv.ParseUint(f[9], 10, 64)
		s := linuxSock{Socket: Socket{Proto: proto, Local: local, Remote: remote}, inode: ino}
		if proto == "tcp" {
			s.State = linuxStates[strings.ToUpper(f[3])]
		} else if f[3] == "07" && !s.HasRemote() {
			// An unconnected UDP socket: bound, waiting for packets.
			s.State = Listen
		} else if s.HasRemote() {
			s.State = Established
		}
		out = append(out, s)
	}
	return out
}

// parseHexAddr parses "0100007F:0277" (IPv4) or the 32-digit IPv6 form. The
// address is in the kernel's byte order: each 32-bit word little-endian on
// the machines this runs on.
func parseHexAddr(s string) (netip.AddrPort, bool) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return netip.AddrPort{}, false
	}
	port, err := strconv.ParseUint(s[i+1:], 16, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	raw, err := hex.DecodeString(s[:i])
	if err != nil {
		return netip.AddrPort{}, false
	}
	switch len(raw) {
	case 4:
		v := binary.LittleEndian.Uint32(raw)
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		return netip.AddrPortFrom(netip.AddrFrom4(b), uint16(port)), true
	case 16:
		var b [16]byte
		for w := 0; w < 4; w++ {
			v := binary.LittleEndian.Uint32(raw[w*4:])
			binary.BigEndian.PutUint32(b[w*4:], v)
		}
		return netip.AddrPortFrom(netip.AddrFrom16(b), uint16(port)), true
	}
	return netip.AddrPort{}, false
}

func (l *linuxScanner) processes(pids []int32) map[int32]Process {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	out := make(map[int32]Process, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if c, ok := l.names[pid]; ok && now.Sub(c.at) < 30*time.Second {
			out[pid] = c.proc
			continue
		}
		p, ok := l.readProc(pid)
		if !ok {
			continue
		}
		l.names[pid] = cachedProc{proc: p, at: now}
		out[pid] = p
	}
	for pid, c := range l.names {
		if now.Sub(c.at) > 5*time.Minute {
			delete(l.names, pid)
		}
	}
	return out
}

// Programs that run other programs: the name shown is the script or app
// they run ("electron" running Discord's app.asar is "discord").
var runners = map[string]bool{"electron": true, "node": true, "python": true, "python3": true, "java": true, "ruby": true, "perl": true, "bun": true, "deno": true}

func (l *linuxScanner) readProc(pid int32) (Process, bool) {
	dir := filepath.Join(l.procRoot, strconv.Itoa(int(pid)))
	comm, err := os.ReadFile(filepath.Join(dir, "comm"))
	if err != nil {
		return Process{}, false
	}
	p := Process{Name: strings.TrimSpace(string(comm))}
	if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
		p.Exe = strings.TrimSuffix(exe, " (deleted)")
		base := filepath.Base(p.Exe)
		// comm is cut at 15 characters: the program's file name is the full one.
		if strings.HasPrefix(base, p.Name) && len(base) > len(p.Name) {
			p.Name = base
		}
	}
	runner := strings.TrimRight(strings.ToLower(p.Name), "0123456789.")
	if runners[runner] {
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			if target := runTarget(strings.Split(strings.TrimRight(string(cmd), "\x00"), "\x00")); target != "" {
				p.Name = target
			}
		}
	}
	return p, true
}

// runTarget finds what a runner runs: its first argument that isn't an
// option ("/usr/lib/discord/app.asar" -> "discord", "server.py" -> "server.py").
func runTarget(args []string) string {
	for _, a := range args[min(1, len(args)):] {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		base := filepath.Base(a)
		if base == "app.asar" || base == "app" || base == "main.js" || base == "index.js" {
			if d := filepath.Base(filepath.Dir(a)); d != "." && d != "/" && d != "resources" && d != "lib" {
				return d
			}
			if d := filepath.Base(filepath.Dir(filepath.Dir(a))); d != "." && d != "/" {
				return d
			}
		}
		return base
	}
	return ""
}

func (l *linuxScanner) traffic() (rx, tx uint64, ok bool) {
	data, err := os.ReadFile(filepath.Join(l.procRoot, "net", "dev"))
	if err != nil {
		return 0, 0, false
	}
	return parseProcNetDev(data)
}

// parseProcNetDev sums /proc/net/dev over every interface except loopback.
func parseProcNetDev(data []byte) (rx, tx uint64, ok bool) {
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, err1 := strconv.ParseUint(f[0], 10, 64)
		t, err2 := strconv.ParseUint(f[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		rx += r
		tx += t
		ok = true
	}
	return rx, tx, ok
}
