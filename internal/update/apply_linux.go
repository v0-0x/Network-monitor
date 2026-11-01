//go:build linux

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

// Arch Linux: the release's package is installed with pacman. The password
// is asked by the system (pkexec and the desktop's polkit agent), never by
// the app. Without pkexec, or when it can't ask, the app shows the command.

const (
	pacman = "/usr/bin/pacman"
	pkexec = "/usr/bin/pkexec"
)

// installedExe is where the package puts the program.
const installedExe = "/usr/lib/network-monitor/network-monitor"

func cantInstall() string {
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	if _, err := os.Stat(pacman); err != nil {
		return "Updates install themselves on Arch Linux. Here: cd ~/Network-monitor && git pull && make && sudo make install"
	}
	if exe != installedExe {
		return "This copy wasn't installed as a package. Update it with: cd ~/Network-monitor && git pull && cd packaging/arch && makepkg -sif"
	}
	return ""
}

type linuxApplied struct{}

func applyUpdate(ctx context.Context, file, dir string) (applied, error) {
	manual := fmt.Sprintf("Install it with: sudo pacman -U %q", file)
	if _, err := os.Stat(pkexec); err != nil {
		return nil, manualError{manual}
	}
	cmd := exec.CommandContext(ctx, pkexec, pacman, "-U", "--noconfirm", file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			switch ee.ExitCode() {
			case 126:
				return nil, errors.New("the password wasn't given; try again")
			case 127:
				return nil, manualError{"No password dialog is available (polkit agent). " + manual}
			}
		}
		return nil, fmt.Errorf("pacman: %v %s", err, lastLine(string(out)))
	}
	return linuxApplied{}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func (linuxApplied) restart() error {
	cmd := exec.Command("/usr/bin/network-monitor")
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// CleanupOld does nothing on Linux.
func CleanupOld() {}
