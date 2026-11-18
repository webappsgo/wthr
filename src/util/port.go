package util

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

const (
	// Port range for random selection (64000-64999)
	MinPort = 64000
	MaxPort = 64999
)

// IsPortAvailable checks if a port is available for binding
func IsPortAvailable(port int) bool {
	// Try to bind to the port
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// GetRandomAvailablePort finds a random available port in the 64000-64999 range
func GetRandomAvailablePort() (int, error) {
	// Try up to 100 times to find an available port
	for i := 0; i < 100; i++ {
		var b [4]byte
		if _, err := cryptorand.Read(b[:]); err != nil {
			panic("crypto/rand unavailable: " + err.Error())
		}
		port := int(binary.BigEndian.Uint32(b[:]))%(MaxPort-MinPort+1) + MinPort
		if IsPortAvailable(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("could not find available port in range %d-%d after 100 attempts", MinPort, MaxPort)
}

// GetServerIP returns the server's IP address
// Never returns 0.0.0.0, 127.0.0.1, or localhost
func GetServerIP() string {
	// Try to get IP from hostname command
	cmd := exec.Command("hostname", "-I")
	output, err := cmd.Output()
	if err == nil {
		ips := strings.Fields(string(output))
		if len(ips) > 0 {
			return ips[0]
		}
	}

	// Fallback: try to get IP by connecting to external service
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	// Last resort: get hostname
	hostname, err := os.Hostname()
	if err == nil {
		return hostname
	}

	return "localhost"
}

// GetServerAddress returns the full server address
func GetServerAddress(port int, https bool) string {
	ip := GetServerIP()

	protocol := "http"
	if https {
		protocol = "https"
	}

	// Omit standard ports
	if (protocol == "http" && port == 80) || (protocol == "https" && port == 443) {
		return fmt.Sprintf("%s://%s", protocol, ip)
	}

	return fmt.Sprintf("%s://%s:%d", protocol, ip, port)
}
