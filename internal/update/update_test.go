package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.10", "0.1.9", true}, {"0.1.9", "0.1.10", false}, {"0.2.0", "0.1.99", true},
		{"0.1.5", "0.1.5", false}, {"dev", "0.1.1", false}, {"1.0", "0.9.9", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func assetName(v string) string {
	switch runtime.GOOS {
	case "windows":
		return "NetworkMonitor-" + v + "-windows-x64.exe"
	case "darwin":
		return "NetworkMonitor-" + v + "-macos-universal.zip"
	}
	return "network-monitor-" + v + "-1-x86_64.pkg.tar.zst"
}

type fakeApplied struct{}

func (fakeApplied) restart() error { return nil }

func TestCheckAndInstall(t *testing.T) {
	payload := []byte("new version")
	sum := sha256.Sum256(payload)
	var srv *httptest.Server
	rel := func(v string, pre bool, draft bool, good bool) map[string]any {
		s := hex.EncodeToString(sum[:])
		if !good {
			s = hex.EncodeToString(make([]byte, 32))
		}
		return map[string]any{
			"tag_name": "v" + v, "prerelease": pre, "draft": draft, "body": "notes " + v, "html_url": "https://example/" + v,
			"assets": []map[string]any{
				{"name": assetName(v), "browser_download_url": srv.URL + "/dl/" + v, "size": len(payload)},
				{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/sums/" + v + "/" + s, "size": 80},
			},
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+Repo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]any{rel("0.1.30", false, true, true), rel("0.1.20", true, false, true), rel("0.1.12", false, false, true)})
	})
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rel("0.1.12", false, false, true))
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
	mux.HandleFunc("/sums/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitLast(r.URL.Path)
		v, s := parts[0], parts[1]
		fmt.Fprintf(w, "%s  %s\n%s  other-file\n", s, assetName(v), s)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	apiBase = srv.URL
	applyFn = func(ctx context.Context, file, dir string) (applied, error) {
		data, err := os.ReadFile(file)
		if err != nil || string(data) != string(payload) {
			t.Errorf("installed %q, %v", data, err)
		}
		return fakeApplied{}, nil
	}
	beta := true
	u := New("0.1.10", "beta", t.TempDir(), func() bool { return beta }, func() string { return "" })
	u.state.Manual = "" // whatever this test machine is, install anyway
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a := u.State().Available; a == nil || a.Version != "0.1.20" || !a.Beta {
		t.Fatalf("beta channel found %+v, want the 0.1.20 pre-release (not the draft)", a)
	}
	beta = false
	u.Check(context.Background())
	if a := u.State().Available; a == nil || a.Version != "0.1.12" || a.Beta {
		t.Fatalf("stable channel found %+v", a)
	}
	if err := u.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !u.State().Ready {
		t.Error("not ready after install")
	}
	if err := u.Restart(); err != nil {
		t.Error(err)
	}

	// Already newest: nothing offered.
	u2 := New("0.1.20", "beta", t.TempDir(), func() bool { return true }, func() string { return "" })
	u2.Check(context.Background())
	if a := u2.State().Available; a != nil {
		t.Errorf("offered %+v to the newest version", a)
	}
}

func splitLast(p string) [2]string {
	// /sums/<version>/<sum>
	var out [2]string
	rest := p[len("/sums/"):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			out[0], out[1] = rest[:i], rest[i+1:]
		}
	}
	return out
}

func TestDamagedDownload(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0", "assets": []map[string]any{
			{"name": assetName("0.2.0"), "browser_download_url": srv.URL + "/dl"},
			{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/sums"},
		}})
	})
	mux.HandleFunc("/dl", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("tampered")) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%064x  %s\n", 1, assetName("0.2.0"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	apiBase = srv.URL
	applyFn = func(ctx context.Context, file, dir string) (applied, error) {
		t.Error("a damaged download was installed")
		return fakeApplied{}, nil
	}
	u := New("0.1.0", "stable", t.TempDir(), func() bool { return false }, func() string { return "" })
	u.state.Manual = ""
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Install(context.Background()); err == nil {
		t.Error("damaged download accepted")
	}
}
