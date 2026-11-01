//go:build windows

package update

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Windows: a running .exe can be renamed but not overwritten. The running
// one becomes NetworkMonitor.exe.old, the new one takes its name, and the
// next start removes the .old one.

func exePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func cantInstall() string {
	exe, err := exePath()
	if err != nil {
		return "Network Monitor can't find its own program file."
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".write-test-*")
	if err != nil {
		return "Network Monitor's folder (" + filepath.Dir(exe) + ") can't be written to: download the new version from the release page."
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return ""
}

type windowsApplied struct{ exe string }

func applyUpdate(ctx context.Context, file, dir string) (applied, error) {
	exe, err := exePath()
	if err != nil {
		return nil, err
	}
	old := exe + ".old"
	_ = os.Remove(old)
	// Into the program's folder first, under a temporary name: a rename
	// within one folder can't fail halfway.
	staged := exe + ".new"
	_ = os.Remove(staged)
	if err := copyFile(file, staged); err != nil {
		return nil, err
	}
	if err := os.Rename(exe, old); err != nil {
		os.Remove(staged)
		return nil, err
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(old, exe)
		return nil, err
	}
	return windowsApplied{exe: exe}, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (w windowsApplied) restart() error {
	cmd := exec.Command(w.exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	if err := cmd.Start(); err != nil {
		return errors.New("the new version didn't start: " + err.Error())
	}
	return cmd.Process.Release()
}

// CleanupOld removes the program file an update replaced.
func CleanupOld() {
	if exe, err := exePath(); err == nil {
		_ = os.Remove(exe + ".old")
		_ = os.Remove(exe + ".new")
	}
}
