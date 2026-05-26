package server

import (
	"net"
	"strings"
)

// PreferredOutboundIP returns the local interface IP used to reach the
// public internet (without actually sending traffic). Used to print a
// LAN-reachable URL on startup when the server is bound to 0.0.0.0.
//
// Returns "127.0.0.1" if no suitable interface is found.
func PreferredOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		addr := conn.LocalAddr().(*net.UDPAddr)
		return addr.IP.String()
	}
	// Fallback: scan interfaces for any private IPv4.
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, a := range addrs {
				ipNet, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				ip4 := ipNet.IP.To4()
				if ip4 == nil {
					continue
				}
				if ip4.IsPrivate() {
					return ip4.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// HostForURL picks the hostname to use in a printed access URL given a
// configured bind address.
//   - "0.0.0.0" or "::" → detect outbound interface IP
//   - "" → 127.0.0.1
//   - anything else → bind as-is (specific interface IP, or localhost)
func HostForURL(bind string) string {
	bind = strings.TrimSpace(bind)
	switch bind {
	case "", "127.0.0.1", "localhost", "::1":
		return "127.0.0.1"
	case "0.0.0.0", "::", "[::]":
		return PreferredOutboundIP()
	default:
		return bind
	}
}
