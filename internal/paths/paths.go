// Package paths says where Network Monitor keeps its files.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir is the folder for settings, logs and the location databases:
//
//	Linux    $XDG_DATA_HOME/network-monitor (~/.local/share/network-monitor)
//	Windows  %LOCALAPPDATA%\Network Monitor
//	macOS    ~/Library/Application Support/Network Monitor
//
// NETMON_DATA_DIR overrides it (the tests and the release checks use it).
func DataDir() string {
	if d := os.Getenv("NETMON_DATA_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Network Monitor")
		}
		return filepath.Join(home, "AppData", "Local", "Network Monitor")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Network Monitor")
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" && filepath.IsAbs(d) {
			return filepath.Join(d, "network-monitor")
		}
		return filepath.Join(home, ".local", "share", "network-monitor")
	}
}

// Sub returns a folder inside DataDir, created if needed.
func Sub(name string) string {
	d := filepath.Join(DataDir(), name)
	_ = os.MkdirAll(d, 0o755)
	return d
}
