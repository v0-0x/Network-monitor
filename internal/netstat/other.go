//go:build !linux && !darwin && !windows

package netstat

import "context"

type otherScanner struct{}

func newSysScanner() sysScanner { return otherScanner{} }

func (otherScanner) method() string { return "gopsutil" }

func (otherScanner) perConnection() (bool, string) {
	return false, "This system doesn't count traffic per connection."
}

func (otherScanner) sockets(ctx context.Context) ([]Socket, error) { return gopsutilConnections(ctx) }

func (otherScanner) processes(pids []int32) map[int32]Process { return map[int32]Process{} }

func (otherScanner) traffic() (rx, tx uint64, ok bool) { return gopsutilTraffic() }
