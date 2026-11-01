package monitor

// Well-known ports, for "what is this connection": the remote port of a
// connection this computer opened, or the local port of a listening socket.
var tcpServices = map[uint16]string{
	20: "FTP data", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 53: "DNS", 80: "HTTP", 110: "POP3",
	111: "RPC", 123: "NTP", 135: "Windows RPC", 137: "NetBIOS", 139: "SMB", 143: "IMAP", 179: "BGP",
	389: "LDAP", 443: "HTTPS", 445: "SMB", 465: "SMTPS", 514: "Syslog", 515: "Printer", 548: "AFP",
	554: "RTSP", 587: "SMTP", 631: "Printing (IPP)", 636: "LDAPS", 853: "DNS over TLS", 873: "rsync",
	990: "FTPS", 993: "IMAPS", 995: "POP3S", 1080: "SOCKS", 1194: "OpenVPN", 1433: "SQL Server",
	1521: "Oracle", 1723: "PPTP", 1883: "MQTT", 2049: "NFS", 2375: "Docker", 2376: "Docker",
	3000: "Dev server", 3074: "Xbox Live", 3128: "Proxy", 3306: "MySQL", 3389: "Remote Desktop",
	3478: "STUN", 3479: "STUN", 3480: "STUN", 4070: "Spotify", 4244: "WhatsApp", 5000: "UPnP/Dev",
	5222: "XMPP", 5223: "Apple Push", 5228: "Google Play", 5353: "mDNS", 5432: "PostgreSQL",
	5900: "VNC", 5938: "TeamViewer", 6379: "Redis", 6443: "Kubernetes", 6463: "Discord RPC",
	6881: "BitTorrent", 7000: "AirPlay", 8000: "HTTP alt", 8008: "HTTP alt", 8009: "Chromecast",
	8080: "HTTP alt", 8443: "HTTPS alt", 8883: "MQTT TLS", 8888: "HTTP alt", 9000: "HTTP alt",
	9050: "Tor", 9100: "Printer", 9418: "Git", 9993: "ZeroTier", 11211: "Memcached",
	25565: "Minecraft", 27017: "MongoDB", 27015: "Steam", 27036: "Steam", 32400: "Plex",
	51820: "WireGuard", 22000: "Syncthing", 1935: "RTMP", 6667: "IRC", 6697: "IRC (TLS)",
}

var udpServices = map[uint16]string{
	53: "DNS", 67: "DHCP", 68: "DHCP", 69: "TFTP", 123: "NTP", 137: "NetBIOS", 138: "NetBIOS",
	161: "SNMP", 443: "QUIC (HTTP/3)", 500: "IPsec", 514: "Syslog", 853: "DNS over QUIC",
	1194: "OpenVPN", 1900: "SSDP", 3074: "Xbox Live", 3478: "STUN", 3479: "STUN", 3480: "STUN",
	3702: "WS-Discovery", 4500: "IPsec NAT", 5060: "SIP", 5353: "mDNS", 5355: "LLMNR",
	6881: "BitTorrent", 21027: "Syncthing", 22000: "Syncthing", 27015: "Steam", 41641: "Tailscale", 51820: "WireGuard",
}

func serviceName(proto string, port uint16) string {
	if proto == "udp" {
		if s, ok := udpServices[port]; ok {
			return s
		}
		// Discord voice, game servers: a large range of ports.
		if port >= 50000 && port <= 50100 {
			return "Discord voice"
		}
		return ""
	}
	return tcpServices[port]
}
