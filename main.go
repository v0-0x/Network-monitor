// Network Monitor: every connection this computer makes, live, on a globe.
//
// One program: the Go side reads the system's connection tables, finds
// where each address is, keeps the log and updates the app; the page in
// frontend/ draws it, in the system's own web view (WebKitGTK on Linux,
// WebView2 on Windows, WKWebView on macOS) through Wails. Nothing listens
// on a port and nothing runs in the background after the window closes.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/v0-0x/Network-monitor/internal/netstat"
	"github.com/v0-0x/Network-monitor/internal/update"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// Set by the release builds (-ldflags "-X main.version=…").
var (
	version = "dev"
	channel = "dev" // "beta" or "stable"
	commit  = ""
)

func main() {
	demo := os.Getenv("NETMON_DEMO") == "1"
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version", "-v":
			fmt.Println("Network Monitor " + version)
			return
		case "--self-test":
			os.Exit(selfTest())
		case "--demo":
			demo = true
		}
	}
	update.CleanupOld()

	app, err := NewApp(demo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Network Monitor couldn't start:", err)
		os.Exit(1)
	}
	err = wails.Run(&options.App{
		Title:            "Network Monitor",
		Width:            1440,
		Height:           900,
		MinWidth:         960,
		MinHeight:        620,
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 11, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "app.networkmonitor.desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.onSecondInstance(nil) },
		},
		Bind: []any{app},
		Linux: &linux.Options{
			Icon:             icon,
			WebviewGpuPolicy: linux.WebviewGpuPolicyAlways, // the globe is WebGL
			ProgramName:      "network-monitor",
		},
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
			About:      &mac.AboutInfo{Title: "Network Monitor", Message: "Version " + version, Icon: icon},
		},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Network Monitor:", err)
		os.Exit(1)
	}
}

// selfTest reads the connection table once, with a connection of its own
// to look for, and prints what it found: the release workflows run it on
// every system, without a window.
func selfTest() int {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		fmt.Println("self-test: listen:", err)
		return 1
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		fmt.Println("self-test: dial:", err)
		return 1
	}
	defer c.Close()
	time.Sleep(200 * time.Millisecond)

	s := netstat.NewScanner()
	socks, err := s.Sockets(context.Background())
	if err != nil {
		fmt.Println("self-test: reading the table:", err)
		return 1
	}
	me := int32(os.Getpid())
	port := uint16(c.LocalAddr().(*net.TCPAddr).Port)
	foundOwn := false
	states := map[string]int{}
	for _, so := range socks {
		states[so.Proto+" "+so.State]++
		if so.Proto == "tcp" && so.Local.Port() == port && so.PID == me {
			foundOwn = true
		}
	}
	rx, tx, traffic := s.Traffic()
	procs := s.Processes([]int32{me})
	out, _ := json.MarshalIndent(map[string]any{
		"version":    version,
		"method":     s.Method(),
		"sockets":    len(socks),
		"states":     states,
		"ownFound":   foundOwn,
		"ownProcess": procs[me],
		"traffic":    traffic,
		"rx":         rx,
		"tx":         tx,
	}, "", "  ")
	fmt.Println(string(out))
	if !foundOwn {
		fmt.Println("self-test: this process's own connection wasn't found")
		return 1
	}
	return 0
}
