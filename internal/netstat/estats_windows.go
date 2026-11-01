//go:build windows

package netstat

import (
	"encoding/binary"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows counts traffic per TCP connection only when asked to for that
// connection (SetPerTcpConnectionEStats), and only administrators may ask.
// When the app runs as administrator, collection is switched on for each
// connection when it's first seen, and its counters are read on every scan
// (GetPerTcpConnectionEStats): so the bytes are counted from then on.

var (
	iphlpapi                        = windows.NewLazySystemDLL("iphlpapi.dll")
	procSetPerTcpEStats             = iphlpapi.NewProc("SetPerTcpConnectionEStats")
	procGetPerTcpEStats             = iphlpapi.NewProc("GetPerTcpConnectionEStats")
	procSetPerTcp6EStats            = iphlpapi.NewProc("SetPerTcp6ConnectionEStats")
	procGetPerTcp6EStats            = iphlpapi.NewProc("GetPerTcp6ConnectionEStats")
	tcpConnectionEstatsData uintptr = 1
)

const mibTCPStateEstab = 5

type mibTCPRow struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
}

type mibTCP6Row struct {
	State         uint32
	LocalAddr     [16]byte
	LocalScopeID  uint32
	LocalPort     uint32
	RemoteAddr    [16]byte
	RemoteScopeID uint32
	RemotePort    uint32
}

// TCP_ESTATS_DATA_ROD_v0
type estatsDataROD struct {
	DataBytesOut      uint64
	DataSegsOut       uint64
	DataBytesIn       uint64
	DataSegsIn        uint64
	SegsOut           uint64
	SegsIn            uint64
	SoftErrors        uint32
	SoftErrorReason   uint32
	SndUna            uint32
	SndNxt            uint32
	SndMax            uint32
	_                 uint32
	ThruBytesAcked    uint64
	RcvNxt            uint32
	_                 uint32
	ThruBytesReceived uint64
}

// portDW is a port as the tables keep it: network byte order in the low
// 16 bits.
func portDW(p uint16) uint32 { return uint32(p>>8 | p<<8) }

func addr4DW(a netip.Addr) uint32 {
	b := a.As4()
	return binary.LittleEndian.Uint32(b[:])
}

// estats reads (and when needed switches on) one connection's counters.
// It returns the system's error code when it fails.
func estats(s Socket, enable bool) (rx, tx uint64, errno uintptr) {
	var rod estatsDataROD
	rw := byte(1)
	if s.Local.Addr().Is4() && s.Remote.Addr().Is4() {
		row := mibTCPRow{State: mibTCPStateEstab, LocalAddr: addr4DW(s.Local.Addr()), LocalPort: portDW(s.Local.Port()), RemoteAddr: addr4DW(s.Remote.Addr()), RemotePort: portDW(s.Remote.Port())}
		if enable {
			r, _, _ := procSetPerTcpEStats.Call(uintptr(unsafe.Pointer(&row)), tcpConnectionEstatsData, uintptr(unsafe.Pointer(&rw)), 0, 1, 0)
			if r != 0 {
				return 0, 0, r
			}
		}
		r, _, _ := procGetPerTcpEStats.Call(uintptr(unsafe.Pointer(&row)), tcpConnectionEstatsData, 0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&rod)), 0, unsafe.Sizeof(rod))
		if r != 0 {
			return 0, 0, r
		}
		return rod.DataBytesIn, rod.DataBytesOut, 0
	}
	row := mibTCP6Row{State: mibTCPStateEstab, LocalAddr: s.Local.Addr().As16(), LocalPort: portDW(s.Local.Port()), RemoteAddr: s.Remote.Addr().As16(), RemotePort: portDW(s.Remote.Port())}
	if enable {
		r, _, _ := procSetPerTcp6EStats.Call(uintptr(unsafe.Pointer(&row)), tcpConnectionEstatsData, uintptr(unsafe.Pointer(&rw)), 0, 1, 0)
		if r != 0 {
			return 0, 0, r
		}
	}
	r, _, _ := procGetPerTcp6EStats.Call(uintptr(unsafe.Pointer(&row)), tcpConnectionEstatsData, 0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&rod)), 0, unsafe.Sizeof(rod))
	if r != 0 {
		return 0, 0, r
	}
	return rod.DataBytesIn, rod.DataBytesOut, 0
}
