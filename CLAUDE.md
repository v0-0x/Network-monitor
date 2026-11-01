# Network Monitor

A standalone desktop app (Wails: Go + the system web view) that shows every connection this computer makes, live on a globe, with logs. Arch Linux, Windows and macOS. The owner's notes on it are in the claude-notes vault (`Apps/Network Monitor/`).

## Ground rules from the owner

- **Standalone app.** No server, no hub, no background service, nothing listening on a port ("for anything server related make it a standalone app for now"). If a server mode comes later, it's a separate ask.
- **Simple, clean UI. No themes**, no theme switcher, no accent picker. One dark look in the app family's style (Aniko, KumoBox, Sumo: dark greys, Inter, Lucide icons, a sidebar on the window colour and the content in a rounded inset panel).
- **No logo or app name inside the app.** They're on the app icon only.
- Commits are authored as `v0-0x <139093075+v0-0x@users.noreply.github.com>`, with no Co-Authored-By or other Claude lines.
- After every change, give the full copy-paste update command for Arch: `cd ~/Network-monitor && git pull && cd packaging/arch && makepkg -sif`.

## Layout

- `main.go` (flags: `--version`, `--self-test`, `--demo`; the Wails window), `app.go` (what the page can call; sends a `tick` event after every scan).
- `internal/netstat`: the connection tables. `linux.go` (/proc), `darwin.go` + `pcblist.go` (sysctl `net.inet.*.pcblist_n`, struct offsets from xnu, falls back to lsof), `windows.go` (+ `gopsutil.go`: GetExtendedTcpTable).
- `internal/monitor`: scans, diffs, direction (in/out), services, logs events, `demo.go` (made-up data, `NETMON_DEMO=1`).
- `internal/geo`: DB-IP Lite City + ASN (mmdb), downloaded to the data folder; DB-IP first, the `@ip-location-db` npm packages as fallback. Both record layouts are decoded.
- `internal/logs`: JSONL per day, search, CSV. `internal/update`: GitHub releases, SHA256SUMS, per-system install (`apply_*.go`). `internal/settings`, `internal/publicip`, `internal/rdns`, `internal/paths`.
- `frontend/`: React + TypeScript + globe.gl. `src/wailsjs` is generated (`wails generate module`); the page uses its own types in `src/lib/types.ts`. Host names come from DNS: escape everything put into HTML (`esc`).
- `packaging/arch/PKGBUILD` builds from the checkout; `build/` holds the Wails platform files (icon, NSIS script set to a per-user install, macOS Info.plist).

## Releases

`.github/workflows/release.yml` builds Arch (archlinux container, makepkg), Windows (exe + NSIS installer) and macOS (universal .app, zip + dmg) on every push and runs `--self-test` on each. On `main` every push is a beta pre-release `v0.1.<commit count>`; `[stable]` in the commit **title** or a manual run with channel `stable` promotes it (no rebuild); `demote` reverses. Release files: `network-monitor-<v>-1-x86_64.pkg.tar.zst`, `NetworkMonitor-<v>-windows-x64.exe`, `NetworkMonitor-Setup-<v>-windows-x64.exe`, `NetworkMonitor-<v>-macos-universal.zip` (the updater uses this one) and `.dmg`, `SHA256SUMS.txt`.

## Testing

- `go test ./internal/...` (the netstat test opens a real connection and finds it, on every system's CI).
- `make demo`, or `NETMON_DEMO=1 wails dev -tags webkit2_41` and open http://localhost:34115 in a browser to drive the page with Playwright. `NETMON_DATA_DIR` points it at a throwaway folder.
- Check new layouts at 1366×768 too.
