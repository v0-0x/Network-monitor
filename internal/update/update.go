// Package update keeps Network Monitor up to date from its GitHub releases.
//
// Every release has the app for each system and a SHA256SUMS.txt; a
// download is installed only when its checksum matches. How it's installed
// depends on the system (apply_*.go):
//
//	Windows  the new .exe takes the running one's place (renamed to .old), and
//	         the app starts again
//	macOS    the new app (from the zip) takes the old one's place, and opens
//	Arch     the new package is installed with pacman, through pkexec (the
//	         system asks for your password); without pkexec it says which
//	         command to run
//
// Releases are betas (GitHub pre-releases) unless promoted; the beta setting
// says whether those are offered.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repo is where the releases are.
const Repo = "v0-0x/Network-monitor"

var apiBase = envOr("NETMON_UPDATE_API", "https://api.github.com")

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return strings.TrimRight(v, "/")
	}
	return def
}

// Available describes a newer version.
type Available struct {
	Version string `json:"version"`
	Beta    bool   `json:"beta"`
	Notes   string `json:"notes"`
	URL     string `json:"url"` // the release page
	Size    int64  `json:"size"`
}

// State is shown in the app.
type State struct {
	Current     string     `json:"current"`
	Channel     string     `json:"channel"` // the build's: "beta", "stable" or "dev"
	Beta        bool       `json:"beta"`    // betas are offered
	Checking    bool       `json:"checking"`
	LastCheck   time.Time  `json:"lastCheck"`
	Error       string     `json:"error,omitempty"`
	Available   *Available `json:"available,omitempty"`
	Downloading bool       `json:"downloading"`
	Done        int64      `json:"done"`
	Total       int64      `json:"total"`
	Installing  bool       `json:"installing"`
	Ready       bool       `json:"ready"`            // installed: restart to use it
	Manual      string     `json:"manual,omitempty"` // how to update by hand, when the app can't
}

// Updater checks for and installs updates.
type Updater struct {
	current string
	channel string
	dataDir string
	beta    func() bool
	skip    func() string
	client  *http.Client

	mu      sync.Mutex
	state   State
	release *release
	apply   applied
}

// New returns an Updater for the running version. beta and skip read the
// settings.
func New(current, channel, dataDir string, beta func() bool, skip func() string) *Updater {
	u := &Updater{current: current, channel: channel, dataDir: dataDir, beta: beta, skip: skip, client: &http.Client{Timeout: 30 * time.Minute}}
	u.state = State{Current: current, Channel: channel, Beta: beta()}
	if !isRelease(current) {
		u.state.Manual = "This is a development build: update it with git pull and a new build."
	} else if why := cantInstall(); why != "" {
		u.state.Manual = why
	}
	return u
}

// State returns the updater's state.
func (u *Updater) State() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.state
	s.Beta = u.beta()
	return s
}

func (u *Updater) set(f func(s *State)) {
	u.mu.Lock()
	f(&u.state)
	u.mu.Unlock()
}

// isRelease: a version the workflows built ("0.1.42"), not "dev".
func isRelease(v string) bool {
	_, ok := parseVersion(v)
	return ok
}

func parseVersion(v string) ([]int, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return nil, false
	}
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

// Newer reports whether version a is newer than b (as numbers: 0.1.10 is
// newer than 0.1.9).
func Newer(a, b string) bool {
	va, oka := parseVersion(a)
	vb, okb := parseVersion(b)
	if !oka || !okb {
		return false
	}
	for i := 0; i < len(va) || i < len(vb); i++ {
		var x, y int
		if i < len(va) {
			x = va[i]
		}
		if i < len(vb) {
			y = vb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // "sha256:…", on newer GitHub
}

type release struct {
	Tag        string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	HTMLURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

func (r release) version() string {
	t := r.Tag
	if i := strings.LastIndex(t, "v"); i >= 0 {
		t = t[i+1:]
	}
	return t
}

// platformAsset is the file this system installs from.
func (r release) platformAsset() *asset {
	for i, a := range r.Assets {
		if isPlatformAsset(a.Name, r.version()) {
			return &r.Assets[i]
		}
	}
	return nil
}

func isPlatformAsset(name, version string) bool {
	switch runtime.GOOS {
	case "windows":
		return name == "NetworkMonitor-"+version+"-windows-x64.exe"
	case "darwin":
		return name == "NetworkMonitor-"+version+"-macos-universal.zip"
	case "linux":
		return strings.HasPrefix(name, "network-monitor-"+version+"-") && strings.HasSuffix(name, "-x86_64.pkg.tar.zst")
	}
	return false
}

// Check looks for a newer release.
func (u *Updater) Check(ctx context.Context) error {
	if !isRelease(u.current) {
		return nil
	}
	u.set(func(s *State) { s.Checking, s.Error = true, "" })
	rel, err := u.newest(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Checking = false
	u.state.LastCheck = time.Now()
	if err != nil {
		u.state.Error = err.Error()
		return err
	}
	if rel == nil || !Newer(rel.version(), u.current) || rel.version() == u.skip() {
		if !u.state.Ready {
			u.state.Available = nil
			u.release = nil
		}
		return nil
	}
	a := rel.platformAsset()
	u.release = rel
	u.state.Available = &Available{Version: rel.version(), Beta: rel.Prerelease, Notes: rel.Body, URL: rel.HTMLURL, Size: a.Size}
	return nil
}

// newest is the newest release with a file for this system: with betas,
// the newest of the last 30; otherwise GitHub's latest (never a beta).
func (u *Updater) newest(ctx context.Context) (*release, error) {
	if !u.beta() {
		var r release
		if err := u.getJSON(ctx, apiBase+"/repos/"+Repo+"/releases/latest", &r); err != nil {
			var he httpError
			if errors.As(err, &he) && he.code == http.StatusNotFound {
				return nil, nil // no stable release yet
			}
			return nil, err
		}
		if r.Draft || r.Prerelease || r.platformAsset() == nil {
			return nil, nil
		}
		return &r, nil
	}
	var list []release
	if err := u.getJSON(ctx, apiBase+"/repos/"+Repo+"/releases?per_page=30", &list); err != nil {
		return nil, err
	}
	var best *release
	for i := range list {
		r := &list[i]
		if r.Draft || r.platformAsset() == nil {
			continue
		}
		if best == nil || Newer(r.version(), best.version()) {
			best = r
		}
	}
	return best, nil
}

type httpError struct {
	code   int
	status string
}

func (e httpError) Error() string { return "GitHub: " + e.status }

func (u *Updater) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NetworkMonitor/"+u.current)
	if tok := os.Getenv("NETMON_GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := u.client.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("couldn't reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpError{resp.StatusCode, resp.Status}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v)
}

// Install downloads the available update, checks it and puts it in place.
// Restart then starts it.
func (u *Updater) Install(ctx context.Context) error {
	u.mu.Lock()
	rel := u.release
	st := u.state
	if rel == nil || st.Available == nil {
		u.mu.Unlock()
		return errors.New("there's no update to install")
	}
	if st.Downloading || st.Installing {
		u.mu.Unlock()
		return errors.New("the update is already being installed")
	}
	if st.Manual != "" {
		u.mu.Unlock()
		return errors.New(st.Manual)
	}
	u.state.Downloading, u.state.Error, u.state.Done, u.state.Total = true, "", 0, 0
	u.mu.Unlock()

	err := u.install(ctx, rel)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Downloading, u.state.Installing = false, false
	if err != nil {
		var m manualError
		if errors.As(err, &m) {
			u.state.Manual = m.how
		}
		u.state.Error = err.Error()
		return err
	}
	u.state.Ready = true
	return nil
}

func (u *Updater) install(ctx context.Context, rel *release) error {
	a := rel.platformAsset()
	if a == nil {
		return errors.New("this release has no file for this system")
	}
	if a.Name != filepath.Base(a.Name) || strings.ContainsAny(a.Name, `\/:`) {
		return errors.New("unexpected file name in the release")
	}
	want, err := u.checksum(ctx, rel, a)
	if err != nil {
		return err
	}
	dir, err := privateDir(filepath.Join(u.dataDir, "update"))
	if err != nil {
		return err
	}
	file := filepath.Join(dir, a.Name)
	if err := u.download(ctx, a, file, want); err != nil {
		return err
	}
	u.set(func(s *State) { s.Downloading, s.Installing = false, true })
	ap, err := applyFn(ctx, file, dir)
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.apply = ap
	u.mu.Unlock()
	return nil
}

// checksum finds the file's SHA-256 in SHA256SUMS.txt (and, when GitHub
// gives one, checks it agrees with the asset's own digest).
func (u *Updater) checksum(ctx context.Context, rel *release, a *asset) (string, error) {
	var sums *asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == "SHA256SUMS.txt" {
			sums = &rel.Assets[i]
		}
	}
	if sums == nil {
		return "", errors.New("the release has no checksums (SHA256SUMS.txt)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sums.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	want := ""
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == a.Name {
			want = strings.ToLower(f[0])
		}
	}
	if len(want) != 64 {
		return "", errors.New("the checksums don't list " + a.Name)
	}
	if d, ok := strings.CutPrefix(a.Digest, "sha256:"); ok && !strings.EqualFold(d, want) {
		return "", errors.New("the release's checksums don't agree")
	}
	return want, nil
}

func (u *Updater) download(ctx context.Context, a *asset, file, want string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "NetworkMonitor/"+u.current)
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	u.set(func(s *State) { s.Total = resp.ContentLength })
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var done int64
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				d := done
				u.set(func(s *State) { s.Done = d })
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fmt.Errorf("download: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	u.set(func(s *State) { s.Done = done })
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("the download is damaged (its checksum doesn't match); try again")
	}
	return nil
}

// privateDir makes a new, empty folder only this user can use: another
// account can't swap the checked file before it's installed.
func privateDir(dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Restart starts the installed update; quit must then end this process.
func (u *Updater) Restart() error {
	u.mu.Lock()
	ap := u.apply
	ready := u.state.Ready
	u.mu.Unlock()
	if !ready || ap == nil {
		return errors.New("no update is installed")
	}
	return ap.restart()
}

// applied is an installed update, ready to start.
type applied interface {
	restart() error
}

// manualError is an update the app can't install itself.
type manualError struct{ how string }

func (m manualError) Error() string { return m.how }

// Loop checks 15 s after start and then every 6 hours, while auto says so.
func (u *Updater) Loop(ctx context.Context, auto func() bool) {
	if !isRelease(u.current) {
		return
	}
	t := time.NewTimer(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if auto() {
				_ = u.Check(ctx)
			}
			t.Reset(6 * time.Hour)
		}
	}
}

// applyFn installs a checked download (a variable for the tests).
var applyFn = applyUpdate
