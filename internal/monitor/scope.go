package monitor

import "net/netip"

// Scopes of an address: on the internet, on the home network (or a VPN's
// private network), or this computer itself.
const (
	ScopePublic   = "public"
	ScopeLocal    = "local"
	ScopeLoopback = "loopback"
)

var cgnat = netip.MustParsePrefix("100.64.0.0/10") // carrier NAT, Tailscale

func scopeOf(a netip.Addr) string {
	a = a.Unmap()
	switch {
	case a.IsLoopback():
		return ScopeLoopback
	case a.IsPrivate(), a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast(), a.IsMulticast(),
		a.IsUnspecified(), a.IsInterfaceLocalMulticast(), cgnat.Contains(a):
		return ScopeLocal
	case !a.IsGlobalUnicast():
		return ScopeLocal
	}
	return ScopePublic
}
