package risk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestAssess(t *testing.T) {
	// An ordinary HTTPS connection: low, no reasons.
	a := Assess(Host{Public: true, Name: "lhr25s34-in-f14.1e100.net", Org: "Google LLC", Conns: []Conn{{Proto: "tcp", Dir: "out", RemotePort: 443, Process: "firefox", PID: 10}}})
	if a.Level != Low || a.Score != 0 || len(a.Reasons) != 0 {
		t.Errorf("https: %+v", a)
	}
	// Someone connecting to SSH from the internet: medium at least.
	a = Assess(Host{Public: true, Org: "Some ISP", Conns: []Conn{{Proto: "tcp", Dir: "in", LocalPort: 22, Process: "sshd", PID: 5}}})
	if a.Score != 40 || a.Level != Medium || !strings.Contains(a.Reasons[0], "SSH") {
		t.Errorf("ssh in: %+v", a)
	}
	// A known botnet server on port 4444 from a program in Downloads: high.
	a = Assess(Host{Public: true, Listed: []List{Lists[0]}, Org: "Hosting Ltd",
		Conns: []Conn{{Proto: "tcp", Dir: "out", RemotePort: 4444, Process: "invoice", Exe: `C:\Users\me\Downloads\invoice.exe`, PID: 9}}})
	if a.Level != High || a.Score != 100 || !strings.Contains(a.Reasons[0], "botnet") {
		t.Errorf("bad: %+v", a)
	}
	// A visitor to a game server: low, but said.
	a = Assess(Host{Public: true, Org: "Some ISP", Conns: []Conn{{Proto: "tcp", Dir: "in", LocalPort: 25565, Process: "java", PID: 5}}})
	if a.Level != Low || len(a.Reasons) != 1 {
		t.Errorf("game visitor: %+v", a)
	}
	// Plain HTTP: a reason, low-ish.
	a = Assess(Host{Public: true, Org: "Example", Conns: []Conn{{Proto: "tcp", Dir: "out", RemotePort: 80, PID: 1}}})
	if a.Score != 10 || a.Level != Low {
		t.Errorf("http: %+v", a)
	}
}

func TestBlocklists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "feodo") {
			w.Write([]byte("# Feodo\n198.51.100.7\n"))
			return
		}
		w.Write([]byte("# level1\n203.0.113.0/24\n0.0.0.0/8 # bogons\n2001:db8::/32\n"))
	}))
	defer srv.Close()
	Lists[0].URL = srv.URL + "/feodo"
	Lists[1].URL = srv.URL + "/level1"
	dir := t.TempDir()
	b := OpenBlocklists(dir)
	if !b.Due() {
		t.Error("an empty set should be due")
	}
	if err := b.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if l := b.Lookup(netip.MustParseAddr("203.0.113.99")); len(l) != 1 || l[0].ID != "firehol1" {
		t.Errorf("203.0.113.99: %+v", l)
	}
	if l := b.Lookup(netip.MustParseAddr("198.51.100.7")); len(l) != 1 || l[0].ID != "feodo" {
		t.Errorf("198.51.100.7: %+v", l)
	}
	if l := b.Lookup(netip.MustParseAddr("2001:db8::1")); len(l) != 1 {
		t.Errorf("v6: %+v", l)
	}
	if l := b.Lookup(netip.MustParseAddr("8.8.8.8")); len(l) != 0 {
		t.Errorf("8.8.8.8 listed: %+v", l)
	}
	// Loaded again from disk.
	b2 := OpenBlocklists(dir)
	if l := b2.Lookup(netip.MustParseAddr("203.0.113.1")); len(l) != 1 {
		t.Errorf("from disk: %+v", l)
	}
	if st := b2.Status(); st.Lists[1].Entries != 3 {
		t.Errorf("status: %+v", st)
	}
}
