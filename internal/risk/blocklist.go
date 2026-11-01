package risk

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Public lists of addresses known for attacks, downloaded to the data
// folder and refreshed twice a day. Downloading a list sends nothing about
// your connections.
//
//	FireHOL level 1: attackers, botnets, hijacked networks and unallocated
//	  space, put together from Spamhaus DROP, DShield, abuse.ch and others,
//	  aimed at having no false positives.
//	abuse.ch Feodo Tracker: active botnet command servers.

// List is one blocklist.
type List struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Says   string `json:"-"` // the reason shown when an address is on it
	Points int    `json:"-"`
}

var Lists = []List{
	{ID: "feodo", Name: "abuse.ch Feodo Tracker", URL: envOr("NETMON_FEODO_URL", "https://feodotracker.abuse.ch/downloads/ipblocklist.txt"),
		Says: "abuse.ch's Feodo Tracker lists this address as a botnet command server", Points: 80},
	{ID: "firehol1", Name: "FireHOL level 1", URL: envOr("NETMON_FIREHOL_URL", "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset"),
		Says: "On FireHOL's level 1 blocklist (attackers, botnets and hijacked networks)", Points: 60},
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Status is shown in Settings.
type Status struct {
	Lists   []ListStatus `json:"lists"`
	Updated time.Time    `json:"updated"`
	Error   string       `json:"error,omitempty"`
}

// ListStatus is one list's state.
type ListStatus struct {
	Name    string `json:"name"`
	Entries int    `json:"entries"`
}

// Blocklists holds the lists.
type Blocklists struct {
	dir    string
	client *http.Client

	mu      sync.RWMutex
	sets    map[string][]netip.Prefix // sorted by address
	updated time.Time
	err     string
	extra   map[netip.Addr]List // demo entries
}

// OpenBlocklists loads the lists downloaded before.
func OpenBlocklists(dir string) *Blocklists {
	_ = os.MkdirAll(dir, 0o755)
	b := &Blocklists{dir: dir, client: &http.Client{Timeout: 60 * time.Second}, sets: map[string][]netip.Prefix{}}
	for _, l := range Lists {
		f, err := os.Open(filepath.Join(dir, l.ID+".txt"))
		if err != nil {
			continue
		}
		b.sets[l.ID] = parse(f)
		f.Close()
		if st, err := os.Stat(filepath.Join(dir, l.ID+".txt")); err == nil && st.ModTime().After(b.updated) {
			b.updated = st.ModTime()
		}
	}
	return b
}

// parse reads addresses and networks, one per line; # starts a comment.
func parse(r io.Reader) []netip.Prefix {
	var out []netip.Prefix
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		line = strings.Fields(line)[0]
		if p, err := netip.ParsePrefix(line); err == nil {
			out = append(out, p.Masked())
		} else if a, err := netip.ParseAddr(line); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Addr().Less(out[j].Addr()) })
	return out
}

// Due: never downloaded, or more than 12 hours ago.
func (b *Blocklists) Due() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sets) == 0 || time.Since(b.updated) > 12*time.Hour
}

// Update downloads every list.
func (b *Blocklists) Update(ctx context.Context) error {
	var errs []string
	for _, l := range Lists {
		set, err := b.fetch(ctx, l)
		if err != nil {
			errs = append(errs, l.Name+": "+err.Error())
			continue
		}
		b.mu.Lock()
		b.sets[l.ID] = set
		b.updated = time.Now()
		b.mu.Unlock()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.err = strings.Join(errs, "; ")
	if len(errs) == len(Lists) {
		return errors.New(b.err)
	}
	return nil
}

func (b *Blocklists) fetch(ctx context.Context, l List) ([]netip.Prefix, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NetworkMonitor")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	set := parse(strings.NewReader(string(data)))
	if len(set) == 0 {
		return nil, errors.New("the list was empty")
	}
	tmp := filepath.Join(b.dir, l.ID+".txt.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, err
	}
	return set, os.Rename(tmp, filepath.Join(b.dir, l.ID+".txt"))
}

// Lookup returns the lists ip is on.
func (b *Blocklists) Lookup(ip netip.Addr) []List {
	ip = ip.Unmap()
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []List
	if l, ok := b.extra[ip]; ok {
		out = append(out, l)
	}
	for _, l := range Lists {
		if contains(b.sets[l.ID], ip) {
			out = append(out, l)
		}
	}
	return out
}

// contains: a binary search for the last network starting at or before ip,
// then a look back over the few that could still hold it.
func contains(set []netip.Prefix, ip netip.Addr) bool {
	i := sort.Search(len(set), func(i int) bool { return ip.Less(set[i].Addr()) })
	for j := i - 1; j >= 0 && j >= i-64; j-- {
		if set[j].Contains(ip) {
			return true
		}
	}
	return false
}

// Status returns the lists' state.
func (b *Blocklists) Status() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s := Status{Updated: b.updated, Error: b.err}
	for _, l := range Lists {
		s.Lists = append(s.Lists, ListStatus{Name: l.Name, Entries: len(b.sets[l.ID])})
	}
	return s
}

// AddDemo lists an address (the demo data's made-up bad host).
func (b *Blocklists) AddDemo(ip netip.Addr, l List) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.extra == nil {
		b.extra = map[netip.Addr]List{}
	}
	b.extra[ip] = l
}
