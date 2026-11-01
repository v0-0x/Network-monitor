//go:build windows

package netstat

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows: the socket tables come from the system (iphlpapi's
// GetExtendedTcpTable and GetExtendedUdpTable, through gopsutil), with the
// owning process of every socket, also without administrator rights.
// Windows' UDP table has no remote addresses, so UDP shows only as open
// ports. Process names come from one snapshot of all processes (Toolhelp),
// which also names the system's own.

type windowsScanner struct {
	mu     sync.Mutex
	names  map[int32]cachedProc
	snapAt time.Time
	snap   map[int32]string

	denied  bool            // not administrator: no traffic per connection
	enabled map[string]bool // connections whose counters are switched on
}

func newSysScanner() sysScanner {
	return &windowsScanner{names: map[int32]cachedProc{}, enabled: map[string]bool{}}
}

func (w *windowsScanner) perConnection() (bool, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.denied {
		return false, "Windows only counts traffic per connection for administrators: open Network Monitor with Run as administrator to see it."
	}
	return true, "Counted from when Network Monitor first saw each connection; Windows doesn't count UDP."
}

func (w *windowsScanner) method() string { return "GetExtendedTcpTable" }

func (w *windowsScanner) sockets(ctx context.Context) ([]Socket, error) {
	socks, err := gopsutilConnections(ctx)
	if err != nil {
		return socks, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.denied {
		return socks, nil
	}
	seen := map[string]bool{}
	for i := range socks {
		s := &socks[i]
		if s.Proto != "tcp" || s.State != Established || !s.HasRemote() {
			continue
		}
		k := s.Local.String() + "|" + s.Remote.String()
		seen[k] = true
		rx, tx, errno := estats(*s, !w.enabled[k])
		if errno == uintptr(windows.ERROR_ACCESS_DENIED) {
			w.denied = true
			return socks, nil
		}
		if errno != 0 {
			continue
		}
		w.enabled[k] = true
		s.Rx, s.Tx, s.HasBytes = rx, tx, true
	}
	for k := range w.enabled {
		if !seen[k] {
			delete(w.enabled, k)
		}
	}
	return socks, nil
}

func (w *windowsScanner) traffic() (rx, tx uint64, ok bool) { return gopsutilTraffic() }

func (w *windowsScanner) processes(pids []int32) map[int32]Process {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	out := make(map[int32]Process, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if c, ok := w.names[pid]; ok && now.Sub(c.at) < 30*time.Second {
			out[pid] = c.proc
			continue
		}
		if w.snap == nil || now.Sub(w.snapAt) > time.Second {
			w.snap = processSnapshot()
			w.snapAt = now
		}
		exe, ok := w.snap[pid]
		if !ok {
			continue
		}
		p := Process{Name: cleanName(exe)}
		if full := imagePath(uint32(pid)); full != "" {
			p.Exe = full
		}
		switch pid {
		case 4:
			p.Name = "System"
		}
		w.names[pid] = cachedProc{proc: p, at: now}
		out[pid] = p
	}
	for pid, c := range w.names {
		if now.Sub(c.at) > 5*time.Minute {
			delete(w.names, pid)
		}
	}
	return out
}

// processSnapshot returns every process's program name ("chrome.exe").
func processSnapshot() map[int32]string {
	out := map[int32]string{}
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(h)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(h, &e); err == nil; err = windows.Process32Next(h, &e) {
		out[int32(e.ProcessID)] = windows.UTF16ToString(e.ExeFile[:])
	}
	return out
}

// imagePath returns a process's full program path, when this user may ask.
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
