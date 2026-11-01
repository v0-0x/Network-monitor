// Package publicip finds this computer's public IP address (the one the rest
// of the internet sees, behind the router or VPN), which places "you" on the
// globe.
package publicip

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Services that answer with the caller's address as plain text. They're
// asked one after the other until one answers.
var services = []string{
	"https://api.ipify.org",
	"https://checkip.amazonaws.com",
	"https://icanhazip.com",
	"https://ifconfig.me/ip",
}

var client = &http.Client{Timeout: 8 * time.Second}

// Get returns the public IP address.
func Get(ctx context.Context) (netip.Addr, error) {
	var lastErr error = errors.New("no service answered")
	for _, url := range services {
		ip, err := ask(ctx, url)
		if err == nil {
			return ip, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return netip.Addr{}, ctx.Err()
		}
	}
	return netip.Addr{}, lastErr
}

func ask(ctx context.Context, url string) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	req.Header.Set("User-Agent", "NetworkMonitor")
	resp, err := client.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, errors.New(resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return netip.Addr{}, err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return netip.Addr{}, errors.New("unexpected answer from " + url)
	}
	return ip.Unmap(), nil
}
