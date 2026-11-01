//go:build windows

package ping

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows' own ping function (iphlpapi IcmpSendEcho): no administrator
// rights needed. IPv6 isn't done this way; the caller falls back to TCP.

var (
	iphlpapi         = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreate   = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpClose    = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho = iphlpapi.NewProc("IcmpSendEcho")
	errNoAnswer      = errors.New("no answer")
	errNoV6          = errors.New("ICMP for IPv6 isn't done on Windows")
	invalidHandle    = ^uintptr(0)
)

func echo(ip netip.Addr, seq int, timeout time.Duration) (time.Duration, error) {
	ip = ip.Unmap()
	if !ip.Is4() {
		return 0, errNoV6
	}
	h, _, err := procIcmpCreate.Call()
	if h == invalidHandle || h == 0 {
		return 0, err
	}
	defer procIcmpClose.Call(h)
	a := ip.As4()
	dst := binary.LittleEndian.Uint32(a[:])
	data := []byte("network-monitor")
	reply := make([]byte, 256)
	start := time.Now()
	n, _, _ := procIcmpSendEcho.Call(h, uintptr(dst), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	elapsed := time.Since(start)
	if n == 0 {
		return 0, errNoAnswer
	}
	// ICMP_ECHO_REPLY: Address, Status, RoundTripTime (ms)…
	if status := binary.LittleEndian.Uint32(reply[4:]); status != 0 {
		return 0, errNoAnswer
	}
	rtt := time.Duration(binary.LittleEndian.Uint32(reply[8:])) * time.Millisecond
	if rtt == 0 {
		rtt = min(elapsed, time.Millisecond) // answered in under a millisecond
	}
	return rtt, nil
}
