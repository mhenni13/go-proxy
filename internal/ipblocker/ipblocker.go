package ipblocker

import (
	"bufio"
	"net"
	"os"
	"strings"
)

// IPBlocker manages IP allowlist and blocklist
type IPBlocker struct {
	allowlist []*net.IPNet
	blocklist []*net.IPNet
	mode      string // "allowlist", "blocklist", or "off"
}

// NewIPBlocker creates a new IP blocker from configuration
func NewIPBlocker(allowlist, blocklist []string, mode string) (*IPBlocker, error) {
	blocker := &IPBlocker{
		mode: mode,
	}

	// Parse allowlist
	for _, cidr := range allowlist {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try as single IP
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, err
			}
			// Convert single IP to CIDR
			if ip.To4() != nil {
				cidr = cidr + "/32"
			} else {
				cidr = cidr + "/128"
			}
			_, ipnet, _ = net.ParseCIDR(cidr)
		}
		blocker.allowlist = append(blocker.allowlist, ipnet)
	}

	// Parse blocklist
	for _, cidr := range blocklist {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try as single IP
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, err
			}
			// Convert single IP to CIDR
			if ip.To4() != nil {
				cidr = cidr + "/32"
			} else {
				cidr = cidr + "/128"
			}
			_, ipnet, _ = net.ParseCIDR(cidr)
		}
		blocker.blocklist = append(blocker.blocklist, ipnet)
	}

	return blocker, nil
}

// IsAllowed checks if an IP address is allowed based on the configured mode
func (ib *IPBlocker) IsAllowed(ipStr string) bool {
	if ib.mode == "off" || ib.mode == "" {
		return true
	}

	// Extract IP from "IP:port" format if present
	host, _, err := net.SplitHostPort(ipStr)
	if err != nil {
		// Assume it's just an IP without port
		host = ipStr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// If we can't parse the IP, block it for safety
		return false
	}

	switch ib.mode {
	case "allowlist":
		// In allowlist mode, IP must be in allowlist
		for _, ipnet := range ib.allowlist {
			if ipnet.Contains(ip) {
				return true
			}
		}
		return false

	case "blocklist":
		// In blocklist mode, IP must NOT be in blocklist
		for _, ipnet := range ib.blocklist {
			if ipnet.Contains(ip) {
				return false
			}
		}
		return true

	default:
		return true
	}
}

// GetMode returns the current mode
func (ib *IPBlocker) GetMode() string {
	return ib.mode
}

// ContainsInAllowlist checks if IP is in allowlist
func (ib *IPBlocker) ContainsInAllowlist(ipStr string) bool {
	host, _, err := net.SplitHostPort(ipStr)
	if err != nil {
		host = ipStr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, ipnet := range ib.allowlist {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// ContainsInBlocklist checks if IP is in blocklist
func (ib *IPBlocker) ContainsInBlocklist(ipStr string) bool {
	host, _, err := net.SplitHostPort(ipStr)
	if err != nil {
		host = ipStr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, ipnet := range ib.blocklist {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// ExtractIP extracts the real client IP considering X-Forwarded-For and X-Real-IP headers
func ExtractIP(remoteAddr, xForwardedFor, xRealIP string) string {
	// Priority: X-Real-IP > first IP in X-Forwarded-For > RemoteAddr
	if xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}

	if xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	return remoteAddr
}

// LoadIPsFromFile loads IP addresses or CIDRs from a file (one per line)
func LoadIPsFromFile(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ips = append(ips, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ips, nil
}

// NewIPBlockerFromFiles creates an IP blocker loading lists from files
func NewIPBlockerFromFiles(allowlistFile, blocklistFile string, mode string) (*IPBlocker, error) {
	var allowlist, blocklist []string
	var err error

	if allowlistFile != "" {
		allowlist, err = LoadIPsFromFile(allowlistFile)
		if err != nil {
			return nil, err
		}
	}

	if blocklistFile != "" {
		blocklist, err = LoadIPsFromFile(blocklistFile)
		if err != nil {
			return nil, err
		}
	}

	return NewIPBlocker(allowlist, blocklist, mode)
}
