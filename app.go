package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/v0-0x/Network-monitor/internal/geo"
	"github.com/v0-0x/Network-monitor/internal/logs"
	"github.com/v0-0x/Network-monitor/internal/monitor"
	"github.com/v0-0x/Network-monitor/internal/paths"
	"github.com/v0-0x/Network-monitor/internal/settings"
	"github.com/v0-0x/Network-monitor/internal/update"
)

// App is what the window's page can call (window.go.main.App.*). It owns
// the monitor, the log, the location databases and the updater.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	demo   bool

	settings *settings.Store
	geo      *geo.DB
	logs     *logs.Store
	mon      *monitor.Monitor
	upd      *update.Updater

	mu      sync.Mutex
	pending []logs.Event
}

// Info is about this copy of the app.
type Info struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	Commit  string `json:"commit"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	DataDir string `json:"dataDir"`
	LogsDir string `json:"logsDir"`
	Demo    bool   `json:"demo"`
}

// Tick is sent to the page after every scan.
type Tick struct {
	Snapshot monitor.Snapshot `json:"snapshot"`
	Events   []logs.Event     `json:"events"`
	Geo      geo.Status       `json:"geo"`
	Update   update.State     `json:"update"`
}

// NewApp opens everything; startup starts it.
func NewApp(demo bool) (*App, error) {
	dir := paths.DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	a := &App{demo: demo}
	a.settings = settings.Load(filepath.Join(dir, "settings.json"))
	a.geo = geo.Open(paths.Sub("geo"))
	lg, err := logs.Open(paths.Sub("logs"))
	if err != nil {
		return nil, err
	}
	a.logs = lg
	a.upd = update.New(version, channel, dir, a.betaUpdates, func() string { return a.settings.Get().Updates.Skip })
	return a, nil
}

// betaUpdates: what the settings say, or else what this build is (a beta
// keeps getting betas).
func (a *App) betaUpdates() bool {
	u := a.settings.Get().Updates
	if u.BetaChosen {
		return u.Beta
	}
	return channel == "beta"
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.logs.Subscribe(func(e logs.Event) {
		a.mu.Lock()
		a.pending = append(a.pending, e)
		a.mu.Unlock()
	})
	a.mon = monitor.New(monitor.Options{
		Settings:   a.settings,
		Geo:        a.geo,
		Logs:       a.logs,
		Demo:       a.demo,
		OnSnapshot: a.emit,
	})
	go a.mon.Run(a.ctx)
	go a.upd.Loop(a.ctx, func() bool { return a.settings.Get().Updates.Auto })
	// While something downloads, the page hears about it more often than
	// once a scan.
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-t.C:
				g, u := a.geo.Status(), a.upd.State()
				if g.Downloading || u.Downloading || u.Installing || u.Checking {
					wruntime.EventsEmit(a.ctx, "status", map[string]any{"geo": g, "update": u})
				}
			}
		}
	}()
}

func (a *App) emit(s monitor.Snapshot) {
	if a.ctx == nil {
		return
	}
	a.mu.Lock()
	events := a.pending
	a.pending = nil
	a.mu.Unlock()
	if events == nil {
		events = []logs.Event{}
	}
	wruntime.EventsEmit(a.ctx, "tick", Tick{Snapshot: s, Events: events, Geo: a.geo.Status(), Update: a.upd.State()})
}

func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	a.logs.Add(logs.Event{Kind: logs.Note, Message: "Monitoring stopped"})
	a.logs.Close()
	a.geo.Close()
}

func (a *App) onSecondInstance(_ any) {
	if a.ctx == nil {
		return
	}
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
}

// --- Called by the page ---

// GetInfo returns the version and folders.
func (a *App) GetInfo() Info {
	return Info{Version: version, Channel: channel, Commit: commit, OS: runtime.GOOS, Arch: runtime.GOARCH, DataDir: paths.DataDir(), LogsDir: a.logs.Dir(), Demo: a.demo}
}

// GetTick returns the newest state at once (when the page loads).
func (a *App) GetTick() Tick {
	var s monitor.Snapshot
	if a.mon != nil {
		s = a.mon.Last()
	}
	return Tick{Snapshot: s, Events: a.logs.Recent(300), Geo: a.geo.Status(), Update: a.upd.State()}
}

// GetSettings returns the settings.
func (a *App) GetSettings() settings.Settings { return a.settings.Get() }

// SaveSettings saves the settings and applies them.
func (a *App) SaveSettings(s settings.Settings) (settings.Settings, error) {
	old := a.settings.Get()
	saved, err := a.settings.Set(s)
	if err != nil {
		return saved, err
	}
	if saved.Location != old.Location && a.mon != nil {
		a.mon.Relocate()
	}
	if saved.Updates.Beta != old.Updates.Beta || saved.Updates.BetaChosen != old.Updates.BetaChosen {
		go a.upd.Check(a.ctx)
	}
	if saved.LogDays != old.LogDays {
		a.logs.Prune(saved.LogDays)
	}
	if a.mon != nil {
		a.mon.Rescan()
	}
	return saved, nil
}

// SearchLogs searches a day's log.
func (a *App) SearchLogs(q logs.Query) (logs.Page, error) { return a.logs.Search(q) }

// LogDays lists the days with logs.
func (a *App) LogDays() []logs.Day { return a.logs.Days() }

// ExportLogs asks where to save a search as CSV and saves it there. It
// returns the file's path ("" when cancelled).
func (a *App) ExportLogs(q logs.Query) (string, error) {
	day := q.Day
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	name, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export the log",
		DefaultFilename: "network-log-" + day + ".csv",
		Filters:         []wruntime.FileFilter{{DisplayName: "CSV (*.csv)", Pattern: "*.csv"}},
	})
	if err != nil || name == "" {
		return "", err
	}
	if filepath.Ext(name) == "" {
		name += ".csv"
	}
	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	if _, err := a.logs.WriteCSV(q, f); err != nil {
		f.Close()
		return "", err
	}
	return name, f.Close()
}

// OpenLogsFolder shows the logs folder in the file manager.
func (a *App) OpenLogsFolder() error { return openPath(a.logs.Dir()) }

// OpenURL opens a web page in the browser (release notes, DB-IP).
func (a *App) OpenURL(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") {
		return errors.New("not a web address")
	}
	wruntime.BrowserOpenURL(a.ctx, u)
	return nil
}

// UpdateGeo downloads the newest location databases (in the background).
func (a *App) UpdateGeo() {
	go func() { _ = a.mon.UpdateGeo(a.ctx) }()
}

// CheckForUpdates looks for a new version now.
func (a *App) CheckForUpdates() update.State {
	_ = a.upd.Check(a.ctx)
	return a.upd.State()
}

// InstallUpdate downloads and installs the new version (in the background;
// "status" events tell how it goes).
func (a *App) InstallUpdate() {
	go func() {
		err := a.upd.Install(a.ctx)
		st := a.upd.State()
		if err == nil {
			a.logs.Add(logs.Event{Kind: logs.Note, Message: "Version " + st.Available.Version + " is installed; it starts when Network Monitor restarts"})
		}
		wruntime.EventsEmit(a.ctx, "status", map[string]any{"geo": a.geo.Status(), "update": st})
	}()
}

// RestartToUpdate starts the installed version and quits this one.
func (a *App) RestartToUpdate() error {
	if err := a.upd.Restart(); err != nil {
		return err
	}
	wruntime.Quit(a.ctx)
	return nil
}

// SkipVersion stops offering a version.
func (a *App) SkipVersion(v string) {
	_, _ = a.settings.Update(func(s *settings.Settings) { s.Updates.Skip = v })
	_ = a.upd.Check(a.ctx)
}

func openPath(p string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", p)
	case "darwin":
		cmd = exec.Command("open", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't open %s: %w", strings.TrimSpace(p), err)
	}
	go cmd.Wait()
	return nil
}
