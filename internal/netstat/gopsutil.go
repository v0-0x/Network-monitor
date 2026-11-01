//go:build !linux

package netstat

import (
	"context"
	"net/netip"
	"strings"
	"syscall"

	gonet "github.com/shirou/gopsutil/v4/net"
)

// gopsutilConnections reads the socket table with gopsutil: on Windows the
// system's own GetExtendedTcpTable/GetExtendedUdpTable, on macOS (only when
// its sysctls fail) lsof, and elsewhere what gopsutil has.
func gopsutilConnections(ctx context.Context) ([]Socket, error) {
	conns, err := gonet.ConnectionsWithContext(ctx, "inet")
	if err != nil && len(conns) == 0 {
		return nil, err
	}
	out := make([]Socket, 0, len(conns))
	for _, c := range conns {
		var s Socket
		switch c.Type {
		case syscall.SOCK_STREAM:
			s.Proto = "tcp"
		case syscall.SOCK_DGRAM:
			s.Proto = "udp"
		default:
			continue
		}
		la, err := netip.ParseAddr(strings.Trim(c.Laddr.IP, "[]"))
		if err != nil {
			if c.Family == syscall.AF_INET6 {
				la = netip.IPv6Unspecified()
			} else {
				la = netip.IPv4Unspecified()
			}
		}
		s.Local = netip.AddrPortFrom(la.WithZone(""), uint16(c.Laddr.Port))
		if ra, err := netip.ParseAddr(strings.Trim(c.Raddr.IP, "[]")); err == nil {
			s.Remote = netip.AddrPortFrom(ra.WithZone(""), uint16(c.Raddr.Port))
		}
		s.PID = c.Pid
		if s.Proto == "tcp" {
			s.State = normalState(c.Status)
		} else if s.HasRemote() {
			s.State = Established
		} else {
			s.State = Listen
		}
		out = append(out, s)
	}
	return out, nil
}

// normalState maps gopsutil's state names (Windows' and lsof's) to ours.
func normalState(st string) string {
	switch strings.ToUpper(strings.ReplaceAll(st, "_", "")) {
	case "ESTABLISHED":
		return Established
	case "SYNSENT":
		return SynSent
	case "SYNRECEIVED", "SYNRECV":
		return SynRecv
	case "FINWAIT1":
		return FinWait1
	case "FINWAIT2":
		return FinWait2
	case "TIMEWAIT":
		return TimeWait
	case "CLOSED", "CLOSE", "DELETETCB":
		return Close
	case "CLOSEWAIT":
		return CloseWait
	case "LASTACK":
		return LastAck
	case "LISTEN":
		return Listen
	case "CLOSING":
		return Closing
	}
	return strings.ToUpper(st)
}

func gopsutilTraffic() (rx, tx uint64, ok bool) {
	counters, err := gonet.IOCounters(true)
	if err != nil {
		return 0, 0, false
	}
	for _, c := range counters {
		n := strings.ToLower(c.Name)
		if n == "lo" || n == "lo0" || strings.Contains(n, "loopback") {
			continue
		}
		rx += c.BytesRecv
		tx += c.BytesSent
		ok = true
	}
	return rx, tx, ok
}
