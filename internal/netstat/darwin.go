//go:build darwin

package netstat

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

type darwinScanner struct {
	mu       sync.Mutex
	names    map[int32]cachedProc
	useLsof  bool // the sysctls didn't work: lsof (this user's sockets only)
	lastComm map[int32]string
	commAt   time.Time
}

func newSysScanner() sysScanner {
	return &darwinScanner{names: map[int32]cachedProc{}}
}

func (d *darwinScanner) method() string {
	if d.useLsof {
		return "lsof"
	}
	return "sysctl pcblist_n"
}

func (d *darwinScanner) sockets(ctx context.Context) ([]Socket, error) {
	if !d.useLsof {
		var out []Socket
		var err error
		for _, t := range []struct{ mib, proto string }{{"net.inet.tcp.pcblist_n", "tcp"}, {"net.inet.udp.pcblist_n", "udp"}} {
			var buf []byte
			buf, err = sysctlRetry(t.mib)
			if err != nil {
				break
			}
			var socks []Socket
			socks, err = parsePcblist(buf, t.proto)
			if err != nil {
				break
			}
			out = append(out, socks...)
		}
		if err == nil {
			return out, nil
		}
		d.useLsof = true
	}
	return gopsutilConnections(ctx)
}

// sysctlRetry reads a sysctl that can grow between asking for its size and
// reading it (ENOMEM).
func sysctlRetry(name string) ([]byte, error) {
	var err error
	for i := 0; i < 5; i++ {
		var buf []byte
		buf, err = unix.SysctlRaw(name)
		if err == nil {
			return buf, nil
		}
		if !errors.Is(err, syscall.ENOMEM) {
			return nil, err
		}
	}
	return nil, err
}

func (d *darwinScanner) processes(pids []int32) map[int32]Process {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	out := make(map[int32]Process, len(pids))
	var missing []int32
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if c, ok := d.names[pid]; ok && now.Sub(c.at) < 30*time.Second {
			out[pid] = c.proc
			continue
		}
		missing = append(missing, pid)
	}
	if len(missing) > 0 {
		// Every process's short name, in one call (also other users').
		if d.lastComm == nil || now.Sub(d.commAt) > time.Second {
			d.lastComm = map[int32]string{}
			if procs, err := unix.SysctlKinfoProcSlice("kern.proc.all"); err == nil {
				for _, p := range procs {
					d.lastComm[p.Proc.P_pid] = unix.ByteSliceToString(p.Proc.P_comm[:])
				}
			}
			d.commAt = now
		}
		for _, pid := range missing {
			p := Process{Name: d.lastComm[pid]}
			// The program's path: this user's processes only.
			if args, err := unix.SysctlRaw("kern.procargs2", int(pid)); err == nil && len(args) > 4 {
				exe := args[4:]
				if i := strings.IndexByte(string(exe), 0); i >= 0 {
					exe = exe[:i]
				}
				p.Exe = string(exe)
			}
			if app := appBundleName(p.Exe); app != "" {
				p.Name = app
			} else if p.Exe != "" {
				p.Name = filepath.Base(p.Exe)
			}
			if p.Name == "" {
				continue
			}
			d.names[pid] = cachedProc{proc: p, at: now}
			out[pid] = p
		}
	}
	for pid, c := range d.names {
		if now.Sub(c.at) > 5*time.Minute {
			delete(d.names, pid)
		}
	}
	return out
}

func (d *darwinScanner) traffic() (rx, tx uint64, ok bool) {
	buf, err := route.FetchRIB(syscall.AF_UNSPEC, route.RIBType(unix.NET_RT_IFLIST2), 0)
	if err == nil {
		if rx, tx, ok = parseIfList2(buf); ok {
			return rx, tx, ok
		}
	}
	return gopsutilTraffic()
}
