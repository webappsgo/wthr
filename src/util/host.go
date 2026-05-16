package utils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetHostInfo detects the host information from request headers
func GetHostInfo(c *gin.Context) *HostInfo {
	// Detect protocol from headers or TLS
	protocol := c.GetHeader("X-Forwarded-Proto")
	if protocol == "" {
		if c.Request.TLS != nil {
			protocol = "https"
		} else {
			protocol = "http"
		}
	}

	// Get hostname from request (reverse proxy headers first)
	hostname := GetHostFromRequest(c)

	// Build full host URL
	fullHost := fmt.Sprintf("%s://%s", protocol, hostname)

	return &HostInfo{
		Protocol:   protocol,
		Hostname:   hostname,
		Port:       os.Getenv("PORT"),
		FullHost:   fullHost,
		ExampleURL: fmt.Sprintf("%s/London", fullHost),
	}
}

// GetHostFromRequest resolves hostname from request (TEMPLATE.md compliant)
// Priority: X-Forwarded-Host > X-Real-Host > X-Original-Host > GetFQDN()
func GetHostFromRequest(c *gin.Context) string {
	// 1. Reverse proxy headers (highest priority)
	for _, header := range []string{"X-Forwarded-Host", "X-Real-Host", "X-Original-Host"} {
		if host := c.GetHeader(header); host != "" {
			// Strip port if present
			if h, _, err := net.SplitHostPort(host); err == nil {
				return h
			}
			return host
		}
	}

	// 2. Fall back to static FQDN resolution
	return GetFQDN()
}

// GetFQDN resolves the fully qualified domain name (TEMPLATE.md compliant)
// Priority: DOMAIN env > os.Hostname() > HOSTNAME env > IPv6 > IPv4
func GetFQDN() string {
	// 1. DOMAIN env var (explicit user override)
	if domain := os.Getenv("DOMAIN"); domain != "" {
		return domain
	}

	// 2. os.Hostname() - cross-platform
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		if !isLoopback(hostname) {
			return hostname
		}
	}

	// 3. HOSTNAME env var
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		if !isLoopback(hostname) {
			return hostname
		}
	}

	// 4. Global IPv6 (preferred for modern networks)
	if ipv6 := getGlobalIPv6(); ipv6 != "" {
		return ipv6
	}

	// 5. Global IPv4
	if ipv4 := getGlobalIPv4(); ipv4 != "" {
		return ipv4
	}

	// Last resort
	return "localhost"
}

// isLoopback checks if host is a loopback address
func isLoopback(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// getGlobalIPv6 returns the first global unicast IPv6 address
func getGlobalIPv6() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.To4() == nil && ipnet.IP.IsGlobalUnicast() {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// getGlobalIPv4 returns the first global unicast IPv4 address
func getGlobalIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ip4 := ipnet.IP.To4(); ip4 != nil && ipnet.IP.IsGlobalUnicast() {
				return ip4.String()
			}
		}
	}
	return ""
}

// GetClientIP extracts the real client IP from reverse proxy headers
func GetClientIP(c *gin.Context) string {
	// Check Cloudflare header first
	if ip := c.GetHeader("CF-Connecting-IP"); ip != "" {
		return ip
	}

	// Check X-Real-IP
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For (use first IP)
	if ip := c.GetHeader("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
		// We want the first one (the actual client)
		for i, char := range ip {
			if char == ',' {
				return ip[:i]
			}
		}
		return ip
	}

	// Check True-Client-IP
	if ip := c.GetHeader("True-Client-IP"); ip != "" {
		return ip
	}

	// Fallback to Gin's ClientIP
	return c.ClientIP()
}

// IsLocalhost checks if an IP is localhost or private
// AI.md: Host-specific values detected at runtime
func IsLocalhost(ip string) bool {
	// Standard localhost addresses
	if ip == "127.0.0.1" || ip == "::1" || ip == "localhost" {
		return true
	}

	// Check Docker bridge IPs (detected at runtime)
	for _, dockerIP := range GetDockerBridgeIPs() {
		if ip == dockerIP {
			return true
		}
	}

	// Check for private IP ranges (RFC 1918)
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	ip4 := parsedIP.To4()
	if ip4 == nil {
		return false
	}

	// 10.0.0.0/8
	if ip4[0] == 10 {
		return true
	}
	// 172.16.0.0/12
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}

	return false
}

// IsBrowser detects if the request is from a web browser
// AI.md PART 13: Content negotiation based on Accept header first, then User-Agent
func IsBrowser(c *gin.Context) bool {
	// Priority 1: Check Accept header (SPEC requirement)
	accept := c.GetHeader("Accept")
	if accept != "" {
		// Explicit text/html request = browser behavior
		if contains(accept, "text/html") {
			return true
		}
		// Explicit text/plain or application/json = non-browser
		if contains(accept, "text/plain") || contains(accept, "application/json") {
			return false
		}
	}

	// Priority 2: Check User-Agent for browser detection
	userAgent := c.GetHeader("User-Agent")
	if userAgent == "" {
		return false
	}

	// Simple browser detection
	ua := userAgent
	return contains(ua, "Mozilla") &&
		(contains(ua, "Chrome") || contains(ua, "Firefox") || contains(ua, "Safari") || contains(ua, "Edge"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

// IsDockerized checks if running inside a Docker container
// AI.md: Detect container environment at runtime
func IsDockerized() bool {
	// Check for Docker-specific files
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Check cgroup for docker
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		return strings.Contains(string(data), "docker") ||
			strings.Contains(string(data), "containerd")
	}
	return false
}

// GetDockerGatewayIP detects the Docker gateway IP at runtime
// AI.md: Host-specific values MUST be detected at runtime, NEVER hardcoded
func GetDockerGatewayIP() string {
	// Try to get default gateway from routing table
	// This works in most Docker environments
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			// Default route has destination 00000000
			if fields[1] == "00000000" {
				// Gateway is in hex, need to parse it
				gateway := fields[2]
				if len(gateway) == 8 {
					// Parse little-endian hex IP
					ip := parseHexIP(gateway)
					if ip != "" {
						return ip
					}
				}
			}
		}
	}
	return ""
}

// parseHexIP converts a hex route IP to dotted decimal
func parseHexIP(hex string) string {
	if len(hex) != 8 {
		return ""
	}
	// Linux stores IPs in little-endian hex
	var bytes [4]byte
	for i := 0; i < 4; i++ {
		var b int
		fmt.Sscanf(hex[i*2:i*2+2], "%02X", &b)
		bytes[3-i] = byte(b)
	}
	return fmt.Sprintf("%d.%d.%d.%d", bytes[0], bytes[1], bytes[2], bytes[3])
}

// GetDockerBridgeIPs returns common Docker bridge IPs for localhost checking
// AI.md: These are detected ranges, not hardcoded specific IPs
func GetDockerBridgeIPs() []string {
	ips := []string{}

	// Try to detect actual gateway first
	if gw := GetDockerGatewayIP(); gw != "" {
		ips = append(ips, gw)
	}

	// Also check common Docker network ranges (172.16.0.0/12)
	// These are fallbacks for environments where route parsing fails
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ip := ipnet.IP.To4()
				if ip != nil && ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
					// This is a Docker bridge network range
					// The gateway is typically .1
					gwIP := fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
					found := false
					for _, existing := range ips {
						if existing == gwIP {
							found = true
							break
						}
					}
					if !found {
						ips = append(ips, gwIP)
					}
				}
			}
		}
	}

	return ips
}

// IsTorAvailable checks if Tor binary is available on the system
// AI.md PART 32: Tor hidden service auto-enabled when Tor binary found
func IsTorAvailable() bool {
	// Check common Tor binary locations
	torPaths := []string{"tor", "/usr/bin/tor", "/usr/local/bin/tor"}
	for _, path := range torPaths {
		if _, err := exec.LookPath(path); err == nil {
			return true
		}
	}
	return false
}
