package logs

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var got []Event
	s.Subscribe(func(e Event) { got = append(got, e) })
	now := time.Now().UnixMilli()
	for i := 0; i < 30; i++ {
		s.Add(Event{Time: now + int64(i), Kind: Opened, Process: "firefox", IP: "1.2.3.4", Port: 443})
	}
	s.Add(Event{Time: now + 100, Kind: Closed, Process: "steam", IP: "5.6.7.8", Port: 27015, Place: "Stockholm, Sweden"})
	if len(got) != 31 {
		t.Fatalf("subscriber got %d", len(got))
	}
	p, err := s.Search(Query{Limit: 10})
	if err != nil || len(p.Events) != 10 || !p.More || p.Total != 31 || p.Events[0].Process != "steam" {
		t.Fatalf("page 1: %+v %v", p, err)
	}
	p2, _ := s.Search(Query{Limit: 50, Before: p.Events[9].ID})
	if len(p2.Events) != 21 || p2.More {
		t.Errorf("page 2: %d more=%v", len(p2.Events), p2.More)
	}
	if p, _ := s.Search(Query{Text: "stockholm steam"}); len(p.Events) != 1 {
		t.Errorf("text search: %d", len(p.Events))
	}
	if p, _ := s.Search(Query{Kinds: []string{Closed}}); len(p.Events) != 1 {
		t.Errorf("kind filter: %d", len(p.Events))
	}
	var buf bytes.Buffer
	n, err := s.WriteCSV(Query{Text: "steam"}, &buf)
	if err != nil || n != 1 || !strings.Contains(buf.String(), "Stockholm, Sweden") {
		t.Errorf("csv: %d %v %q", n, err, buf.String())
	}
	if len(s.Days()) != 1 || len(s.Recent(5)) != 5 {
		t.Errorf("days %v recent %d", s.Days(), len(s.Recent(5)))
	}
	if _, err := s.Search(Query{Day: "../x"}); err == nil {
		t.Error("bad day accepted")
	}
}
