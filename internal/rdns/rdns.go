// Package rdns finds the host names of addresses (reverse DNS, PTR
// records), in the background and cached, so a scan never waits for DNS.
package rdns

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type entry struct {
	name    string
	expires time.Time
}

// Resolver looks names up with at most a few lookups at once.
type Resolver struct {
	mu      sync.Mutex
	cache   map[netip.Addr]entry
	pending map[netip.Addr]bool
	sem     chan struct{}
	lookup  func(ctx context.Context, addr string) ([]string, error)
	// OnName is called (from another goroutine) when a lookup finishes with
	// a name.
	OnName func(ip netip.Addr, name string)
}

// New returns a Resolver.
func New() *Resolver {
	return &Resolver{
		cache:   map[netip.Addr]entry{},
		pending: map[netip.Addr]bool{},
		sem:     make(chan struct{}, 8),
		lookup:  net.DefaultResolver.LookupAddr,
	}
}

// Name returns the cached name of ip ("" when it has none or isn't known
// yet), and starts a lookup when there's nothing cached.
func (r *Resolver) Name(ip netip.Addr) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[ip]; ok && time.Now().Before(e.expires) {
		return e.name
	}
	if !r.pending[ip] && len(r.pending) < 512 {
		r.pending[ip] = true
		go r.resolve(ip)
	}
	if e, ok := r.cache[ip]; ok {
		return e.name // stale, until the new answer is in
	}
	return ""
}

// Cached returns the cached name of ip without starting a lookup; done is
// false while nothing is known yet.
func (r *Resolver) Cached(ip netip.Addr) (name string, done bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[ip]
	return e.name, ok
}

func (r *Resolver) resolve(ip netip.Addr) {
	r.sem <- struct{}{}
	defer func() { <-r.sem }()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	names, err := r.lookup(ctx, ip.String())
	cancel()
	name := ""
	if err == nil && len(names) > 0 {
		name = strings.TrimSuffix(names[0], ".")
	}
	ttl := time.Hour
	if name == "" {
		ttl = 15 * time.Minute
	}
	r.mu.Lock()
	delete(r.pending, ip)
	if len(r.cache) > 20000 {
		r.cache = map[netip.Addr]entry{}
	}
	r.cache[ip] = entry{name: name, expires: time.Now().Add(ttl)}
	cb := r.OnName
	r.mu.Unlock()
	if name != "" && cb != nil {
		cb(ip, name)
	}
}
