package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/weather/src/config"
	"gopkg.in/yaml.v3"
)

// FirstRunConfig represents first-run auto-detected configuration
type FirstRunConfig struct {
	SMTPHost      string
	SMTPPort      int
	HTTPPort      int
	SetupToken    string
	IsFirstRun    bool
	IsDockerized  bool
	TorEnabled    bool
	OnionAddress  string
}

// DetectFirstRun checks if this is the first time the server is running
// AI.md: Database path is {data_dir}/db/server.db
func DetectFirstRun(dataDir string) bool {
	serverDBPath := filepath.Join(dataDir, "db", "server.db")
	_, err := os.Stat(serverDBPath)
	return os.IsNotExist(err)
}

// GenerateSetupToken generates a cryptographically secure one-time setup token
// AI.md: 32 hex characters (128 bits)
func GenerateSetupToken() (string, error) {
	// 128 bits = 16 bytes = 32 hex chars
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// HashSetupToken returns SHA-256 hash of the setup token
// AI.md: Setup token stored as SHA-256 hash in {config_dir}/setup_token.txt
func HashSetupToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// SaveSetupToken saves the setup token hash to {config_dir}/setup_token.txt
// AI.md: File contains SHA-256 hash of token, not plaintext
func SaveSetupToken(configDir, token string) error {
	// Ensure config directory exists
	dirPerm := os.FileMode(0700)
	if os.Geteuid() == 0 {
		dirPerm = 0755
	}
	if err := os.MkdirAll(configDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Hash the token
	hashedToken := HashSetupToken(token)

	// Write hash to file
	tokenPath := filepath.Join(configDir, "setup_token.txt")
	if err := os.WriteFile(tokenPath, []byte(hashedToken), 0600); err != nil {
		return fmt.Errorf("failed to write setup token file: %w", err)
	}

	return nil
}

// ValidateSetupToken validates a setup token against the stored hash
// AI.md: Compare SHA-256 hash of provided token with stored hash
func ValidateSetupToken(configDir, token string) (bool, error) {
	tokenPath := filepath.Join(configDir, "setup_token.txt")

	// Read stored hash
	storedHash, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("setup token file not found")
		}
		return false, fmt.Errorf("failed to read setup token file: %w", err)
	}

	// Hash the provided token and compare
	providedHash := HashSetupToken(strings.TrimSpace(token))
	return providedHash == strings.TrimSpace(string(storedHash)), nil
}

// DeleteSetupToken removes the setup token file after successful setup
// AI.md: File deleted after successful setup completion
func DeleteSetupToken(configDir string) error {
	tokenPath := filepath.Join(configDir, "setup_token.txt")
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete setup token file: %w", err)
	}
	return nil
}

// SetupTokenExists checks if a setup token file exists
func SetupTokenExists(configDir string) bool {
	tokenPath := filepath.Join(configDir, "setup_token.txt")
	_, err := os.Stat(tokenPath)
	return err == nil
}

// AutoDetectSMTP attempts to auto-detect available SMTP servers
// AI.md: Host-specific values detected at runtime
func AutoDetectSMTP() (string, int) {
	// Build list of SMTP servers to try, in order of preference
	smtpServers := []struct {
		Host string
		Port int
	}{
		{"localhost", 25},
		{"localhost", 587},
		{"127.0.0.1", 25},
	}

	// AI.md: Detect Docker gateway at runtime, not hardcoded
	if gwIP := GetDockerGatewayIP(); gwIP != "" {
		smtpServers = append(smtpServers, struct {
			Host string
			Port int
		}{gwIP, 25})
	}

	// Docker Desktop special hostname
	smtpServers = append(smtpServers, struct {
		Host string
		Port int
	}{"host.docker.internal", 25})

	for _, server := range smtpServers {
		addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			return server.Host, server.Port
		}
	}

	// Default fallback
	return "localhost", 25
}

// SelectRandomPort selects a random port in the 64000-64999 range
func SelectRandomPort() int {
	// Try random ports until we find an available one
	for attempts := 0; attempts < 100; attempts++ {
		port := MinPort + (int(time.Now().UnixNano()) % (MaxPort - MinPort + 1))
		if IsPortAvailable(port) {
			return port
		}
	}
	// Fallback to 64948 if we can't find a random one
	return 64948
}

// NOTE: IsDockerized is defined in host.go

// CreateDefaultServerYML creates server.yml with auto-detected settings
func CreateDefaultServerYML(configPath string, smtpHost string, smtpPort int) error {
	// Ensure config directory exists
	// AI.md PART 7: Permissions - root: 0755, user: 0700
	dirPerm := os.FileMode(0700)
	if os.Geteuid() == 0 {
		dirPerm = 0755
	}
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Default configuration
	config := map[string]interface{}{
		"mode": "production",
		"server": map[string]interface{}{
			"port":    80,
			"address": "::",
			"branding": map[string]interface{}{
				"title":       "Weather Service",
				"description": "Professional weather tracking and forecasting service with real-time updates, severe weather alerts, earthquake monitoring, and moon phase tracking",
				"tagline":     "Your reliable weather companion",
				"logo_url":    "",
				"favicon_url": "",
			},
			"seo": map[string]interface{}{
				"keywords":      []string{"weather", "forecast", "alerts", "tracking", "severe weather", "earthquakes", "moon phases"},
				"author":        "Weather Service",
				"canonical_url": "",
				"og_image":      "",
			},
			"theme": map[string]interface{}{
				"primary_color":   "#3b82f6",
				"secondary_color": "#8b5cf6",
				"dark_mode":       false,
			},
			"email": map[string]interface{}{
				"host":      smtpHost,
				"port":      smtpPort,
				"username":  "",
				"password":  "",
				"from":      config.DefaultEmailAddress("noreply", nil),
				"from_name": "Weather Service",
				"use_tls":   false,
			},
			"notifications": map[string]interface{}{
				"enabled":         true,
				"email_enabled":   true,
				"webhook_enabled": false,
			},
			"rate_limit": map[string]interface{}{
				"enabled":          true,
				"requests_per_min": 60,
				"burst_size":       10,
			},
			"web": map[string]interface{}{
				"robots_txt":   "User-agent: *\nAllow: /\nDisallow: /admin\nDisallow: /api/v1/admin",
				"security_txt": fmt.Sprintf("Contact: mailto:%s\nExpires: %s\nPreferred-Languages: en", config.DefaultEmailAddress("security", nil), time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)),
			},
			"tor": map[string]interface{}{
				"enabled":    false,
				"onion_addr": "",
			},
			"features": map[string]interface{}{
				"earthquakes":    true,
				"hurricanes":     true,
				"moon_phases":    true,
				"severe_weather": true,
				"audit_log":      true,
			},
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	header := "# Weather Service Configuration\n"
	header += "# Auto-generated on first run: " + time.Now().Format(time.RFC3339) + "\n"
	header += "# SMTP auto-detected: " + smtpHost + ":" + fmt.Sprintf("%d", smtpPort) + "\n\n"

	fullContent := header + string(data)
	if err := os.WriteFile(configPath, []byte(fullContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		 findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
