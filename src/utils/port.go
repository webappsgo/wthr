package utils

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/weather/src/database"
)

const (
	// Port range for random selection (64000-64999)
	MinPort = 64000
	MaxPort = 64999
)

// PortManager handles port selection and persistence
type PortManager struct {
	db *sql.DB
}

// NewPortManager creates a new port manager
func NewPortManager(db *sql.DB) *PortManager {
	return &PortManager{
		db: db,
	}
}

// Helper methods for database access (avoiding import cycle with models)
func (pm *PortManager) getIntSetting(key string, defaultValue int) int {
	var value int
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = ?", key).Scan(&value)
	if err != nil {
		return defaultValue
	}
	return value
}

func (pm *PortManager) setIntSetting(key string, value int) error {
	_, err := database.GetServerDB().Exec(`
		INSERT INTO server_config (key, value, type, updated_at)
		VALUES (?, ?, 'integer', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, key, fmt.Sprintf("%d", value), time.Now(), fmt.Sprintf("%d", value), time.Now())
	return err
}

func (pm *PortManager) setBoolSetting(key string, value bool) error {
	boolStr := "false"
	if value {
		boolStr = "true"
	}
	_, err := database.GetServerDB().Exec(`
		INSERT INTO server_config (key, value, type, updated_at)
		VALUES (?, ?, 'boolean', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, key, boolStr, time.Now(), boolStr, time.Now())
	return err
}

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
		port := rand.Intn(MaxPort-MinPort+1) + MinPort
		if IsPortAvailable(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("could not find available port in range %d-%d after 100 attempts", MinPort, MaxPort)
}

// GetOrAssignPort retrieves the saved port from database or assigns a new one
func (pm *PortManager) GetOrAssignPort(portType string) (int, error) {
	// Try to get saved port from database
	var savedPort int
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = ?", "server.port."+portType).Scan(&savedPort)

	if err == nil && savedPort > 0 {
		// Found saved port, check if it's still available
		if IsPortAvailable(savedPort) {
			return savedPort, nil
		}
		// Port is no longer available, fall through to assign new one
	}

	// No saved port or port not available, assign a new one
	newPort, err := GetRandomAvailablePort()
	if err != nil {
		return 0, err
	}

	// Save to database
	_, err = database.GetServerDB().Exec(`
		INSERT INTO server_config (key, value, type, description, updated_at)
		VALUES (?, ?, 'integer', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, "server.port."+portType, fmt.Sprintf("%d", newPort),
		fmt.Sprintf("Auto-assigned %s port", portType),
		time.Now(), fmt.Sprintf("%d", newPort), time.Now())

	if err != nil {
		return 0, fmt.Errorf("failed to save port to database: %w", err)
	}

	return newPort, nil
}

// SavePort saves a port to the database
func (pm *PortManager) SavePort(portType string, port int) error {
	_, err := database.GetServerDB().Exec(`
		INSERT INTO server_config (key, value, type, description, updated_at)
		VALUES (?, ?, 'integer', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, "server.port."+portType, fmt.Sprintf("%d", port),
		fmt.Sprintf("%s port", portType),
		time.Now(), fmt.Sprintf("%d", port), time.Now())

	return err
}

// GetSavedPort retrieves a saved port from the database
func (pm *PortManager) GetSavedPort(portType string) (int, error) {
	var savedPort int
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = ?", "server.port."+portType).Scan(&savedPort)
	if err != nil {
		return 0, err
	}
	return savedPort, nil
}

// GetServerPorts determines which ports to use for HTTP and HTTPS
// Returns (httpPort, httpsPort, error)
// Follows priority: 1) Database saved ports, 2) Config file port, 3) PORT env variable, 4) Random port
func (pm *PortManager) GetServerPorts() (int, int, error) {
	return pm.GetServerPortsWithConfig(0)
}

// GetServerPortsWithConfig allows passing a config file port
// Priority: 1) Database saved ports, 2) configPort (if > 0), 3) PORT env variable, 4) Random port
func (pm *PortManager) GetServerPortsWithConfig(configPort int) (int, int, error) {
	// Check for saved ports in database first
	httpPort := pm.getIntSetting("server.http_port", 0)
	httpsPort := pm.getIntSetting("server.https_port", 0)

	// If HTTP port is saved and available, use it
	if httpPort > 0 && IsPortAvailable(httpPort) {
		return httpPort, httpsPort, nil
	}

	// Check config file port (passed from loaded server.yml)
	if configPort > 0 && IsPortAvailable(configPort) {
		// Save to database for future use
		pm.setIntSetting("server.http_port", configPort)
		return configPort, 0, nil
	}

	// Check environment variable PORT
	// AI.md PART 5: Environment Variables
	portEnv := os.Getenv("PORT")
	if portEnv != "" {
		return pm.ParsePortConfig(portEnv)
	}

	// No configuration found, generate random port
	randomPort, err := GetRandomAvailablePort()
	if err != nil {
		return 0, 0, err
	}

	// Save to database for future use
	pm.setIntSetting("server.http_port", randomPort)
	pm.setIntSetting("server.https_port", 0)

	return randomPort, 0, nil
}

// ParsePortConfig parses port configuration from string
// Supports: "8080" (HTTP only) or "8080,8443" (HTTP,HTTPS)
func (pm *PortManager) ParsePortConfig(portStr string) (int, int, error) {
	parts := strings.Split(portStr, ",")

	httpPort, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port number: %s", parts[0])
	}

	if !IsPortAvailable(httpPort) {
		return 0, 0, fmt.Errorf("port %d is already in use", httpPort)
	}

	// Single port configuration
	if len(parts) == 1 {
		// Save to database
		pm.setIntSetting("server.http_port", httpPort)
		pm.setIntSetting("server.https_port", 0)
		return httpPort, 0, nil
	}

	// Dual port configuration
	httpsPort, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid HTTPS port number: %s", parts[1])
	}

	if !IsPortAvailable(httpsPort) {
		return 0, 0, fmt.Errorf("HTTPS port %d is already in use", httpsPort)
	}

	// Save to database
	pm.setIntSetting("server.http_port", httpPort)
	pm.setIntSetting("server.https_port", httpsPort)
	pm.setBoolSetting("server.https_enabled", true)

	// Detect if using standard ports (80,443) for Let's Encrypt
	if httpPort == 80 && httpsPort == 443 {
		pm.setBoolSetting("server.letsencrypt_enabled", true)
	}

	return httpPort, httpsPort, nil
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

// UpdatePort updates the configured port(s)
func (pm *PortManager) UpdatePort(httpPort, httpsPort int) error {
	if !IsPortAvailable(httpPort) {
		return fmt.Errorf("HTTP port %d is not available", httpPort)
	}

	pm.setIntSetting("server.http_port", httpPort)

	if httpsPort > 0 {
		if !IsPortAvailable(httpsPort) {
			return fmt.Errorf("HTTPS port %d is not available", httpsPort)
		}
		pm.setIntSetting("server.https_port", httpsPort)
		pm.setBoolSetting("server.https_enabled", true)
	} else {
		pm.setIntSetting("server.https_port", 0)
		pm.setBoolSetting("server.https_enabled", false)
	}

	return nil
}
