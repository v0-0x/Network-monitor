// Package risk gives each remote host a rough risk score, 0 to 100, with
// the reasons for it. It's a guess from simple signs, not a verdict: a
// high score means "worth a look", and every point comes with its reason.
//
// What counts: being on a public blocklist (blocklist.go), someone on the
// internet connecting in (more so to remote-access and database ports),
// unencrypted protocols, ports that malware and remote-control tools use,
// a program running from a temporary or downloads folder, a program that
// can't be identified, and an unusual port on a rented server. Where an
// address is (its country) doesn't count: it says nothing about intent.
package risk

import (
	"fmt"
	"sort"
	"strings"
)

// Conn is one connection to the host.
type Conn struct {
	Proto      string
	Dir        string // "in" or "out"
	LocalPort  uint16
	RemotePort uint16
	Process    string
	Exe        string
	PID        int32
}

// Host is what is known about the host.
type Host struct {
	Public bool
	Name   string // reverse DNS
	Org    string // the network's owner
	Listed []List // blocklists it's on
	Conns  []Conn
}

// Assessment is the score and why.
type Assessment struct {
	Score   int      `json:"score"`
	Level   string   `json:"level"` // "low", "medium" or "high"
	Reasons []string `json:"reasons"`
}

// Levels.
const (
	Low    = "low"
	Medium = "medium"
	High   = "high"
)

// Remote-access, file-sharing and database services: open to the internet,
// they're what attackers scan for.
var sensitive = map[uint16]string{
	21: "FTP", 22: "SSH", 23: "Telnet", 135: "Windows RPC", 139: "SMB", 445: "SMB (Windows file sharing)",
	1433: "SQL Server", 2375: "Docker's API", 3306: "MySQL", 3389: "Remote Desktop", 5432: "PostgreSQL",
	5900: "VNC", 5985: "WinRM", 6379: "Redis", 9200: "Elasticsearch", 11211: "Memcached", 27017: "MongoDB",
}

// Protocols that send everything readable along the way.
var plaintext = map[uint16]string{
	21: "FTP", 23: "Telnet", 25: "SMTP", 80: "HTTP", 110: "POP3", 143: "IMAP", 8080: "HTTP", 8000: "HTTP",
}

// Ports some malware and remote-control tools use by default.
var suspicious = map[uint16]string{
	1337: "often used by hacking tools", 31337: "often used by backdoors", 4444: "Metasploit's default",
	5555: "Android debugging (ADB)", 6666: "IRC, used by some botnets", 6667: "IRC, used by some botnets",
	6668: "IRC, used by some botnets", 6669: "IRC, used by some botnets", 2323: "a second Telnet, used by IoT botnets",
	12345: "used by old remote-control trojans", 54321: "used by old remote-control trojans", 7547: "router management (TR-069)",
}

var tor = map[uint16]bool{9001: true, 9030: true, 9050: true, 9150: true}

// Words in a network owner's name that mean rented servers.
var hosting = []string{"hosting", "host ", "server", "vps", "datacenter", "data center", "digitalocean", "ovh", "hetzner", "linode", "vultr", "choopa", "m247", "contabo", "leaseweb", "colocrossing", "frantech", "buyvm", "scaleway", "upcloud", "kamatera", "hostinger"}

var tempDirs = []string{"/tmp/", "/var/tmp/", "/dev/shm/", `\appdata\local\temp\`, `\windows\temp\`, `\downloads\`, "/downloads/", "/private/var/folders/"}

// Assess scores a host.
func Assess(h Host) Assessment {
	type reason struct {
		points int
		text   string
	}
	var rs []reason
	add := func(p int, f string, args ...any) { rs = append(rs, reason{p, fmt.Sprintf(f, args...)}) }
	seen := map[string]bool{}
	once := func(key string, p int, f string, args ...any) {
		if !seen[key] {
			seen[key] = true
			add(p, f, args...)
		}
	}

	for _, l := range h.Listed {
		once("list:"+l.ID, l.Points, "%s", l.Says)
	}
	for _, c := range h.Conns {
		if c.Dir == "in" && h.Public {
			once("in", 15, "Someone on the internet opened this connection to your computer")
			if s, ok := sensitive[c.LocalPort]; ok {
				once("in:"+s, 25, "They connected to %s (port %d), which attackers look for", s, c.LocalPort)
			}
		}
		port := c.RemotePort
		if c.Dir == "in" {
			port = c.LocalPort
		}
		if s, ok := plaintext[port]; ok && c.Proto == "tcp" {
			p := 10
			if s == "Telnet" || s == "FTP" {
				p = 20
			}
			once("plain:"+s, p, "Not encrypted: %s can be read on the way", s)
		}
		if s, ok := suspicious[port]; ok {
			once("port:"+fmt.Sprint(port), 25, "Port %d is %s", port, s)
		}
		if tor[port] {
			once("tor", 10, "Port %d is used by the Tor network", port)
		}
		exe := strings.ToLower(c.Exe)
		for _, d := range tempDirs {
			if exe != "" && strings.Contains(exe, d) {
				once("temp:"+c.Exe, 25, "%s runs from a temporary or downloads folder (%s)", nameOr(c.Process), c.Exe)
				break
			}
		}
		if c.PID == 0 && c.Dir == "out" && h.Public && c.Proto == "tcp" {
			once("unknown", 5, "The program couldn't be identified (another user's, or the system's)")
		}
		if c.Dir == "out" && h.Public && c.RemotePort > 1024 && !known(c.Proto, c.RemotePort) && isHosting(h.Org) {
			once("rented", 10, "An unusual port (%d) on a rented server (%s)", c.RemotePort, h.Org)
		}
	}
	if h.Public && h.Name == "" && h.Org == "" {
		add(5, "No host name and no known network owner")
	}

	a := Assessment{Reasons: []string{}}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].points > rs[j].points })
	for _, r := range rs {
		a.Score += r.points
		a.Reasons = append(a.Reasons, r.text)
	}
	a.Score = min(a.Score, 100)
	switch {
	case a.Score >= 50:
		a.Level = High
	case a.Score >= 20:
		a.Level = Medium
	default:
		a.Level = Low
	}
	return a
}

func nameOr(s string) string {
	if s == "" {
		return "The program"
	}
	return s
}

func isHosting(org string) bool {
	o := strings.ToLower(org) + " "
	for _, w := range hosting {
		if strings.Contains(o, w) {
			return true
		}
	}
	return false
}

// known: a port with a well-known, harmless use.
var knownPorts = map[uint16]bool{443: true, 80: true, 853: true, 993: true, 995: true, 465: true, 587: true, 5223: true, 5228: true, 3478: true, 3479: true, 3480: true, 19302: true, 4070: true, 8443: true, 22000: true, 27015: true, 25565: true, 1935: true, 5222: true, 6697: true, 9418: true}

func known(proto string, port uint16) bool {
	if knownPorts[port] {
		return true
	}
	// Steam, game and voice servers use wide UDP ranges.
	return proto == "udp" && (port >= 27000 && port <= 27100 || port >= 50000 && port <= 65535)
}
