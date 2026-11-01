// Package geo finds where an IP address is: city, region, country,
// coordinates and the network (ASN and its organization) it belongs to.
//
// It works offline, from DB-IP's free "Lite" databases (City and ASN, CC BY
// 4.0, updated monthly), so the addresses this computer talks to are never
// sent to a lookup service. They're downloaded on the first start (about
// 70 MB) and refreshed once a month; see download.go.
package geo

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Location is what is known about an address.
type Location struct {
	City        string  `json:"city,omitempty"`
	Region      string  `json:"region,omitempty"`
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"countryCode,omitempty"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	HasCoords   bool    `json:"hasCoords"`
	TimeZone    string  `json:"timeZone,omitempty"`
	Postcode    string  `json:"postcode,omitempty"`
	ASN         uint32  `json:"asn,omitempty"`
	Org         string  `json:"org,omitempty"`
}

// Place is "Stockholm, Sweden" (or as much of it as is known).
func (l Location) Place() string {
	var parts []string
	if l.City != "" {
		parts = append(parts, l.City)
	}
	if l.Region != "" && l.Region != l.City && l.City == "" {
		parts = append(parts, l.Region)
	}
	if l.Country != "" {
		parts = append(parts, l.Country)
	}
	return strings.Join(parts, ", ")
}

// Source is where one database came from.
type Source struct {
	From       string    `json:"from"`    // "db-ip.com" or "npm"
	Version    string    `json:"version"` // "2026-11", or the npm package version
	Files      []string  `json:"files"`
	Downloaded time.Time `json:"downloaded"`
	Checked    time.Time `json:"checked"`
}

type meta struct {
	City Source `json:"city"`
	ASN  Source `json:"asn"`
}

// Status is shown in Settings.
type Status struct {
	Ready       bool      `json:"ready"`
	HasASN      bool      `json:"hasAsn"`
	Downloading bool      `json:"downloading"`
	Stage       string    `json:"stage,omitempty"`
	Done        int64     `json:"done"`
	Total       int64     `json:"total"`
	Error       string    `json:"error,omitempty"`
	CityVersion string    `json:"cityVersion,omitempty"`
	ASNVersion  string    `json:"asnVersion,omitempty"`
	From        string    `json:"from,omitempty"`
	Downloaded  time.Time `json:"downloaded"`
	SizeBytes   int64     `json:"sizeBytes"`
}

// DB looks addresses up. It's safe for concurrent use.
type DB struct {
	dir string

	mu    sync.RWMutex
	city  []*maxminddb.Reader
	asn   []*maxminddb.Reader
	meta  meta
	cache map[netip.Addr]Location

	smu    sync.Mutex
	status Status
}

const cacheMax = 50000

// Open opens the databases in dir, if they were downloaded. A DB without
// them answers every lookup with an empty Location until Update has run.
func Open(dir string) *DB {
	d := &DB{dir: dir, cache: map[netip.Addr]Location{}}
	_ = os.MkdirAll(dir, 0o755)
	if data, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
		_ = json.Unmarshal(data, &d.meta)
	}
	d.mu.Lock()
	d.city = openAll(dir, d.meta.City.Files)
	d.asn = openAll(dir, d.meta.ASN.Files)
	d.mu.Unlock()
	d.refreshStatus()
	return d
}

func openAll(dir string, files []string) []*maxminddb.Reader {
	var out []*maxminddb.Reader
	for _, f := range files {
		if f != filepath.Base(f) {
			continue
		}
		r, err := maxminddb.Open(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Close closes the databases.
func (d *DB) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	closeAll(d.city)
	closeAll(d.asn)
	d.city, d.asn = nil, nil
}

func closeAll(rs []*maxminddb.Reader) {
	for _, r := range rs {
		_ = r.Close()
	}
}

// Ready reports whether the city database is there.
func (d *DB) Ready() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.city) > 0
}

// Lookup finds where ip is. Private, loopback and other special addresses
// come back empty.
func (d *DB) Lookup(ip netip.Addr) Location {
	ip = ip.Unmap().WithZone("")
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return Location{}
	}
	d.mu.RLock()
	if loc, ok := d.cache[ip]; ok {
		d.mu.RUnlock()
		return loc
	}
	var loc Location
	for _, r := range d.city {
		if decodeCity(r, ip, &loc) {
			break
		}
	}
	for _, r := range d.asn {
		if decodeASN(r, ip, &loc) {
			break
		}
	}
	d.mu.RUnlock()
	if loc.Country == "" && loc.CountryCode != "" {
		loc.Country = countryNames[strings.ToUpper(loc.CountryCode)]
	}
	d.mu.Lock()
	if len(d.cache) >= cacheMax {
		d.cache = map[netip.Addr]Location{}
	}
	d.cache[ip] = loc
	d.mu.Unlock()
	return loc
}

// CountryName returns the English name of an ISO country code.
func CountryName(code string) string { return countryNames[strings.ToUpper(code)] }

// decodeCity reads a record in either layout: DB-IP's own (GeoIP2-like:
// city.names.en, country.iso_code, location.latitude…) or ip-location-db's
// flat one (city, country_code, state1, latitude…).
func decodeCity(r *maxminddb.Reader, ip netip.Addr, loc *Location) bool {
	res := r.Lookup(ip)
	if !res.Found() {
		return false
	}
	var rec map[string]any
	if err := res.Decode(&rec); err != nil {
		return false
	}
	switch c := rec["city"].(type) {
	case map[string]any:
		loc.City = enName(c)
	case string:
		loc.City = c
	}
	if c, ok := rec["country"].(map[string]any); ok {
		loc.CountryCode = str(c["iso_code"])
		loc.Country = enName(c)
	}
	if s := str(rec["country_code"]); s != "" {
		loc.CountryCode = s
	}
	if subs, ok := rec["subdivisions"].([]any); ok && len(subs) > 0 {
		if m, ok := subs[0].(map[string]any); ok {
			loc.Region = enName(m)
		}
	}
	if s := str(rec["state1"]); s != "" {
		loc.Region = s
	}
	if l, ok := rec["location"].(map[string]any); ok {
		loc.Lat, loc.Lon = num(l["latitude"]), num(l["longitude"])
		loc.HasCoords = l["latitude"] != nil && l["longitude"] != nil
		loc.TimeZone = str(l["time_zone"])
	}
	if rec["latitude"] != nil && rec["longitude"] != nil {
		loc.Lat, loc.Lon = num(rec["latitude"]), num(rec["longitude"])
		loc.HasCoords = true
	}
	if s := str(rec["timezone"]); s != "" {
		loc.TimeZone = s
	}
	if p, ok := rec["postal"].(map[string]any); ok {
		loc.Postcode = str(p["code"])
	}
	if s := str(rec["postcode"]); s != "" {
		loc.Postcode = s
	}
	return true
}

func decodeASN(r *maxminddb.Reader, ip netip.Addr, loc *Location) bool {
	res := r.Lookup(ip)
	if !res.Found() {
		return false
	}
	var rec map[string]any
	if err := res.Decode(&rec); err != nil {
		return false
	}
	loc.ASN = uint32(num(rec["autonomous_system_number"]))
	loc.Org = str(rec["autonomous_system_organization"])
	return loc.ASN != 0 || loc.Org != ""
}

func enName(m map[string]any) string {
	if names, ok := m["names"].(map[string]any); ok {
		if s := str(names["en"]); s != "" {
			return s
		}
		for _, v := range names {
			if s := str(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case uint64:
		return float64(n)
	case uint32:
		return float64(n)
	case uint16:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	}
	return 0
}

// Status returns the databases' state.
func (d *DB) Status() Status {
	d.smu.Lock()
	defer d.smu.Unlock()
	return d.status
}

func (d *DB) setStatus(f func(s *Status)) {
	d.smu.Lock()
	f(&d.status)
	d.smu.Unlock()
}

func (d *DB) refreshStatus() {
	d.mu.RLock()
	m := d.meta
	ready, hasASN := len(d.city) > 0, len(d.asn) > 0
	d.mu.RUnlock()
	var size int64
	for _, f := range append(append([]string{}, m.City.Files...), m.ASN.Files...) {
		if st, err := os.Stat(filepath.Join(d.dir, f)); err == nil {
			size += st.Size()
		}
	}
	d.setStatus(func(s *Status) {
		s.Ready, s.HasASN = ready, hasASN
		s.CityVersion, s.ASNVersion = m.City.Version, m.ASN.Version
		s.From = m.City.From
		s.Downloaded = m.City.Downloaded
		s.SizeBytes = size
	})
}

// Due reports whether it's time to look for newer databases: none yet, or
// last checked more than a month ago.
func (d *DB) Due() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.city) == 0 {
		return true
	}
	last := d.meta.City.Checked
	if last.IsZero() {
		last = d.meta.City.Downloaded
	}
	return time.Since(last) > 30*24*time.Hour
}

var errBusy = errors.New("the database download is already running")
