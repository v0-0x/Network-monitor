// Package settings keeps Network Monitor's settings in settings.json.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Location says where "you" are on the globe.
type Location struct {
	Mode  string  `json:"mode"` // "auto": from the public IP address; "manual": Lat and Lon
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label"`
}

// Updates are the updater's settings.
type Updates struct {
	Auto       bool   `json:"auto"`       // look for updates by itself
	Beta       bool   `json:"beta"`       // also take pre-releases
	BetaChosen bool   `json:"betaChosen"` // Beta was set by hand (otherwise it follows the build)
	Skip       string `json:"skip"`       // a version not to offer again
}

// Settings are all of them.
type Settings struct {
	Location     Location `json:"location"`
	IntervalMs   int      `json:"intervalMs"`   // how often the tables are read
	ResolveNames bool     `json:"resolveNames"` // reverse DNS for host names
	ShowLocal    bool     `json:"showLocal"`    // list connections inside this computer and the home network too
	LogDays      int      `json:"logDays"`      // how many days of logs to keep
	GeoAuto      bool     `json:"geoAuto"`      // refresh the location databases monthly
	Updates      Updates  `json:"updates"`
}

// Defaults are the settings of a new install.
func Defaults() Settings {
	return Settings{
		Location:     Location{Mode: "auto"},
		IntervalMs:   1000,
		ResolveNames: true,
		LogDays:      14,
		GeoAuto:      true,
		Updates:      Updates{Auto: true},
	}
}

// Normalize keeps values in range.
func (s *Settings) Normalize() {
	if s.Location.Mode != "manual" {
		s.Location.Mode = "auto"
	}
	if s.Location.Lat < -90 || s.Location.Lat > 90 {
		s.Location.Lat = 0
	}
	if s.Location.Lon < -180 || s.Location.Lon > 180 {
		s.Location.Lon = 0
	}
	if len(s.Location.Label) > 80 {
		s.Location.Label = s.Location.Label[:80]
	}
	switch {
	case s.IntervalMs < 500:
		s.IntervalMs = 500
	case s.IntervalMs > 10000:
		s.IntervalMs = 10000
	}
	switch {
	case s.LogDays < 1:
		s.LogDays = 1
	case s.LogDays > 365:
		s.LogDays = 365
	}
}

// Store loads and saves the settings.
type Store struct {
	path string
	mu   sync.Mutex
	s    Settings
}

// Load reads path; a missing or broken file gives the defaults.
func Load(path string) *Store {
	st := &Store{path: path, s: Defaults()}
	if data, err := os.ReadFile(path); err == nil {
		s := Defaults()
		if json.Unmarshal(data, &s) == nil {
			st.s = s
		}
	}
	st.s.Normalize()
	return st
}

// Get returns a copy of the settings.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// Set replaces the settings and saves them.
func (st *Store) Set(s Settings) (Settings, error) {
	s.Normalize()
	st.mu.Lock()
	st.s = s
	st.mu.Unlock()
	return s, st.save(s)
}

// Update changes the settings with f and saves them.
func (st *Store) Update(f func(s *Settings)) (Settings, error) {
	st.mu.Lock()
	s := st.s
	f(&s)
	s.Normalize()
	st.s = s
	st.mu.Unlock()
	return s, st.save(s)
}

func (st *Store) save(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}
