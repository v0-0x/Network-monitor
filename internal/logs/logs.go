// Package logs keeps the event log: every connection opened and closed,
// ports opened and closed, and notes (your public address changed, the
// databases were updated). It's written as one JSON line per event to a
// file per day (2026-11-01.jsonl), kept for as many days as the settings
// say, and the newest events are also kept in memory for the live views.
package logs

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event kinds.
const (
	Opened   = "open"     // a connection opened
	Closed   = "close"    // a connection closed
	Listen   = "listen"   // a port opened for others to connect to
	Unlisten = "unlisten" // that port closed
	Note     = "note"     // something worth knowing
	Alert    = "alert"    // a connection with a high risk score
	Error    = "error"
)

// Event is one line of the log.
type Event struct {
	ID       int64  `json:"id"`
	Time     int64  `json:"t"` // Unix milliseconds
	Kind     string `json:"kind"`
	Proto    string `json:"proto,omitempty"`
	Process  string `json:"process,omitempty"`
	PID      int32  `json:"pid,omitempty"`
	Local    string `json:"local,omitempty"` // this computer's address and port
	IP       string `json:"ip,omitempty"`    // the other end
	Port     uint16 `json:"port,omitempty"`  // the other end's port (or the listening port)
	Service  string `json:"service,omitempty"`
	Host     string `json:"host,omitempty"` // reverse DNS name
	Place    string `json:"place,omitempty"`
	Country  string `json:"cc,omitempty"`
	Org      string `json:"org,omitempty"`
	Dir      string `json:"dir,omitempty"`   // "in" or "out"
	Scope    string `json:"scope,omitempty"` // "public", "local" or "loopback"
	Duration int64  `json:"dur,omitempty"`   // milliseconds the connection was open (Close)
	Existing bool   `json:"existing,omitempty"`
	Rx       uint64 `json:"rx,omitempty"`   // bytes received over the connection (Close)
	Tx       uint64 `json:"tx,omitempty"`   // and sent
	Risk     int    `json:"risk,omitempty"` // the host's risk score (Alert)
	Message  string `json:"msg,omitempty"`
}

// Text is everything searchable in the event, lower case.
func (e Event) Text() string {
	dir := ""
	switch e.Dir {
	case "in":
		dir = "incoming"
	case "out":
		dir = "outgoing"
	}
	return strings.ToLower(strings.Join([]string{e.Kind, dir, e.Proto, e.Process, e.Local, e.IP, strconv.Itoa(int(e.Port)), e.Service, e.Host, e.Place, e.Country, e.Org, e.Message}, " "))
}

const memMax = 3000

// Store writes and reads the log.
type Store struct {
	dir string

	mu     sync.Mutex
	nextID int64
	day    string
	f      *os.File
	w      *bufio.Writer
	recent []Event
	subs   []func(Event)
}

// Open opens the log in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, nextID: time.Now().UnixMilli() * 1000}
	return s, nil
}

// Subscribe calls f for every new event (from the goroutine that adds it).
func (s *Store) Subscribe(f func(Event)) {
	s.mu.Lock()
	s.subs = append(s.subs, f)
	s.mu.Unlock()
}

// Add logs events. Each gets its id, and its time when it has none.
func (s *Store) Add(events ...Event) {
	if len(events) == 0 {
		return
	}
	s.mu.Lock()
	for i := range events {
		e := &events[i]
		s.nextID++
		e.ID = s.nextID
		if e.Time == 0 {
			e.Time = time.Now().UnixMilli()
		}
		s.write(*e)
		s.recent = append(s.recent, *e)
	}
	if len(s.recent) > memMax {
		s.recent = append([]Event(nil), s.recent[len(s.recent)-memMax:]...)
	}
	if s.w != nil {
		_ = s.w.Flush()
	}
	subs := s.subs
	s.mu.Unlock()
	for _, e := range events {
		for _, f := range subs {
			f(e)
		}
	}
}

func dayOf(ms int64) string { return time.UnixMilli(ms).Format("2006-01-02") }

func (s *Store) write(e Event) {
	day := dayOf(e.Time)
	if day != s.day || s.f == nil {
		s.closeFile()
		f, err := os.OpenFile(filepath.Join(s.dir, day+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		s.f, s.w, s.day = f, bufio.NewWriterSize(f, 64*1024), day
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = s.w.Write(append(data, '\n'))
}

func (s *Store) closeFile() {
	if s.w != nil {
		_ = s.w.Flush()
	}
	if s.f != nil {
		_ = s.f.Close()
	}
	s.f, s.w, s.day = nil, nil, ""
}

// Close closes the day's file.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeFile()
}

// Recent returns up to n of the newest events, oldest first.
func (s *Store) Recent(n int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.recent) {
		n = len(s.recent)
	}
	return append([]Event(nil), s.recent[len(s.recent)-n:]...)
}

// Day is one day of logs.
type Day struct {
	Day  string `json:"day"`
	Size int64  `json:"size"`
}

// Days lists the days there are logs for, newest first.
func (s *Store) Days() []Day {
	entries, _ := os.ReadDir(s.dir)
	var out []Day
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") || e.IsDir() {
			continue
		}
		day := strings.TrimSuffix(name, ".jsonl")
		if _, err := time.Parse("2006-01-02", day); err != nil {
			continue
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		out = append(out, Day{Day: day, Size: size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day > out[j].Day })
	return out
}

// Query is a search of one day's log.
type Query struct {
	Day    string   `json:"day"`    // "2026-11-01"; "" is today
	Text   string   `json:"text"`   // words that must all appear
	Kinds  []string `json:"kinds"`  // only these kinds (none: all)
	Before int64    `json:"before"` // only events older than this id (paging); 0: from the newest
	Limit  int      `json:"limit"`
}

// Page is the answer to a Query, newest first.
type Page struct {
	Events []Event `json:"events"`
	More   bool    `json:"more"`
	Total  int     `json:"total"` // events on that day matching the search
}

var errBadDay = errors.New("no such day")

// Search reads a day's log and returns the events that match, newest first.
func (s *Store) Search(q Query) (Page, error) {
	if q.Day == "" {
		q.Day = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", q.Day); err != nil {
		return Page{}, errBadDay
	}
	if q.Limit <= 0 || q.Limit > 2000 {
		q.Limit = 500
	}
	s.mu.Lock()
	if s.w != nil {
		_ = s.w.Flush()
	}
	s.mu.Unlock()
	var matched []Event
	err := s.each(q.Day, func(e Event) {
		if q.matches(e) {
			matched = append(matched, e)
		}
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Page{}, err
	}
	page := Page{Total: len(matched)}
	for i := len(matched) - 1; i >= 0; i-- {
		e := matched[i]
		if q.Before != 0 && e.ID >= q.Before {
			continue
		}
		if len(page.Events) == q.Limit {
			page.More = true
			break
		}
		page.Events = append(page.Events, e)
	}
	if page.Events == nil {
		page.Events = []Event{}
	}
	return page, nil
}

func (q Query) matches(e Event) bool {
	if len(q.Kinds) > 0 {
		ok := false
		for _, k := range q.Kinds {
			if k == e.Kind {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if t := strings.TrimSpace(strings.ToLower(q.Text)); t != "" {
		text := e.Text()
		for _, w := range strings.Fields(t) {
			if !strings.Contains(text, w) {
				return false
			}
		}
	}
	return true
}

func (s *Store) each(day string, f func(Event)) error {
	file, err := os.Open(filepath.Join(s.dir, day+".jsonl"))
	if err != nil {
		return err
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			f(e)
		}
	}
	return sc.Err()
}

// WriteCSV writes the events of a search (oldest first) as CSV.
func (s *Store) WriteCSV(q Query, w io.Writer) (int, error) {
	if q.Day == "" {
		q.Day = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", q.Day); err != nil {
		return 0, errBadDay
	}
	s.mu.Lock()
	if s.w != nil {
		_ = s.w.Flush()
	}
	s.mu.Unlock()
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"time", "event", "protocol", "process", "pid", "local", "remote ip", "remote port", "service", "host", "place", "country", "network", "direction", "duration (s)", "received (bytes)", "sent (bytes)", "risk", "message"})
	n := 0
	err := s.each(q.Day, func(e Event) {
		if !q.matches(e) {
			return
		}
		n++
		dur := ""
		if e.Duration > 0 {
			dur = strconv.FormatFloat(float64(e.Duration)/1000, 'f', 1, 64)
		}
		port, pid := "", ""
		if e.Port != 0 {
			port = strconv.Itoa(int(e.Port))
		}
		if e.PID != 0 {
			pid = strconv.Itoa(int(e.PID))
		}
		num := func(n uint64) string {
			if n == 0 {
				return ""
			}
			return strconv.FormatUint(n, 10)
		}
		risk := ""
		if e.Risk > 0 {
			risk = strconv.Itoa(e.Risk)
		}
		_ = cw.Write([]string{time.UnixMilli(e.Time).Format(time.RFC3339), e.Kind, e.Proto, e.Process, pid, e.Local, e.IP, port, e.Service, e.Host, e.Place, e.Country, e.Org, e.Dir, dur, num(e.Rx), num(e.Tx), risk, e.Message})
	})
	cw.Flush()
	if err == nil {
		err = cw.Error()
	}
	return n, err
}

// Prune deletes the logs of days older than keep days.
func (s *Store) Prune(keep int) {
	cutoff := time.Now().AddDate(0, 0, -keep).Format("2006-01-02")
	for _, d := range s.Days() {
		if d.Day < cutoff {
			_ = os.Remove(filepath.Join(s.dir, d.Day+".jsonl"))
		}
	}
}

// Dir is the folder the logs are in.
func (s *Store) Dir() string { return s.dir }
