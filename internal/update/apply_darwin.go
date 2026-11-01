//go:build darwin

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// macOS: the zip holds "Network Monitor.app". It's unpacked with ditto
// (which keeps the signature and the bundle's links), moved next to the
// running app, and swapped in with two renames; the old one is removed. The
// app then opens the new one and quits.

func bundlePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	// …/Network Monitor.app/Contents/MacOS/network-monitor
	b := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(b, ".app") {
		return "", errors.New("Network Monitor isn't running from its app")
	}
	return b, nil
}

func cantInstall() string {
	b, err := bundlePath()
	if err != nil {
		return "Network Monitor isn't running as an app, so it can't update itself."
	}
	if strings.Contains(b, "/AppTranslocation/") {
		return "Move Network Monitor to Applications (drag it there in Finder) and open it from there; then it can update itself."
	}
	f, err := os.CreateTemp(filepath.Dir(b), ".write-test-*")
	if err != nil {
		return "The folder Network Monitor is in (" + filepath.Dir(b) + ") can't be written to: download the new version from the release page."
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return ""
}

type macApplied struct{ bundle string }

func applyUpdate(ctx context.Context, file, dir string) (applied, error) {
	bundle, err := bundlePath()
	if err != nil {
		return nil, err
	}
	unpacked := filepath.Join(dir, "unpacked")
	if out, err := exec.CommandContext(ctx, "/usr/bin/ditto", "-x", "-k", file, unpacked).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("unzipping the update: %v %s", err, strings.TrimSpace(string(out)))
	}
	newApp := ""
	entries, _ := os.ReadDir(unpacked)
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			newApp = filepath.Join(unpacked, e.Name())
		}
	}
	if newApp == "" {
		return nil, errors.New("the update has no app in it")
	}
	if _, err := os.Stat(filepath.Join(newApp, "Contents", "MacOS", filepath.Base(mustExe()))); err != nil {
		return nil, errors.New("the update's app is incomplete")
	}
	parent := filepath.Dir(bundle)
	staged := filepath.Join(parent, fmt.Sprintf(".Network Monitor-new-%d.app", os.Getpid()))
	old := filepath.Join(parent, fmt.Sprintf(".Network Monitor-old-%d.app", os.Getpid()))
	_ = os.RemoveAll(staged)
	if err := os.Rename(newApp, staged); err != nil {
		// Another volume: copy it over.
		if out, err := exec.CommandContext(ctx, "/usr/bin/ditto", newApp, staged).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("copying the update: %v %s", err, strings.TrimSpace(string(out)))
		}
	}
	if err := os.Rename(bundle, old); err != nil {
		os.RemoveAll(staged)
		return nil, err
	}
	if err := os.Rename(staged, bundle); err != nil {
		_ = os.Rename(old, bundle)
		return nil, err
	}
	_ = os.RemoveAll(old)
	// Downloads made by the app itself aren't quarantined, but a copy that
	// was might keep the flag: clear it so the new version opens.
	_ = exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", bundle).Run()
	return macApplied{bundle: bundle}, nil
}

func mustExe() string {
	exe, _ := os.Executable()
	return exe
}

func (m macApplied) restart() error {
	// Opened a moment after this one has quit (open would bring the
	// running one forward instead).
	cmd := exec.Command("/bin/sh", "-c", `sleep 1; /usr/bin/open -n "$0"`, m.bundle)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// CleanupOld does nothing on macOS: the old app is removed right away.
func CleanupOld() {}
