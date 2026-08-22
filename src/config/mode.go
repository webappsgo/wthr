// Package config handles application configuration and mode detection per TEMPLATE.md PART 5
package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/webappsgo/wthr/src/common/display"
)

// ModeString represents the application execution mode as a string
type ModeString string

const (
	// ModeDevelopment is for local development (relaxed security, verbose logging)
	ModeDevelopment ModeString = "development"
	// ModeProduction is for production deployment (strict security, FQDN validation)
	ModeProduction ModeString = "production"
)

// ModeConfig holds mode-specific configuration and validation
type ModeConfig struct {
	Mode          ModeString
	IsProduction  bool
	IsDevelopment bool

	// Security settings per mode
	AllowInsecure        bool
	RequireFQDN          bool
	StrictHostValidation bool
	VerboseLogging       bool
	DebugEnabled         bool

	// Host configuration
	Host        string
	Port        int
	FQDN        string
	IsValidFQDN bool
}

// DetectMode determines the application mode from config and environment
// Priority per AI.md PART 5: 1. CLI/Env vars, 2. Config file, 3. Default (production)
func DetectMode(configMode string) ModeString {
	// Priority 1: Check environment variables FIRST (CLI flags set these)
	envMode := os.Getenv("MODE")
	if envMode == "" {
		envMode = os.Getenv("APP_MODE")
	}
	if envMode == "" {
		envMode = os.Getenv("ENVIRONMENT")
	}

	if envMode != "" {
		mode := strings.ToLower(strings.TrimSpace(envMode))
		if mode == "development" || mode == "dev" {
			return ModeDevelopment
		}
		if mode == "production" || mode == "prod" {
			return ModeProduction
		}
	}

	// Priority 2: Check config file value
	if configMode != "" {
		mode := strings.ToLower(strings.TrimSpace(configMode))
		if mode == "development" || mode == "dev" {
			return ModeDevelopment
		}
		if mode == "production" || mode == "prod" {
			return ModeProduction
		}
	}

	// Priority 3: Default to production for security (AI.md requirement)
	return ModeProduction
}

// NewModeConfig creates a new mode configuration with proper validation
func NewModeConfig(mode ModeString, host string, port int) (*ModeConfig, error) {
	mc := &ModeConfig{
		Mode:          mode,
		IsProduction:  mode == ModeProduction,
		IsDevelopment: mode == ModeDevelopment,
		Host:          host,
		Port:          port,
	}

	// Set mode-specific behaviors per TEMPLATE.md PART 5
	switch mode {
	case ModeDevelopment:
		mc.AllowInsecure = true
		mc.RequireFQDN = false
		mc.StrictHostValidation = false
		mc.VerboseLogging = true
		mc.DebugEnabled = true

	case ModeProduction:
		mc.AllowInsecure = false
		mc.RequireFQDN = true
		mc.StrictHostValidation = true
		mc.VerboseLogging = false
		mc.DebugEnabled = false
	}

	// Validate and set FQDN
	if err := mc.validateHost(); err != nil {
		if mc.IsProduction {
			return nil, fmt.Errorf("production mode validation failed: %w", err)
		}
		// In development, just log warning but continue
		fmt.Printf("Warning (development mode): %v\n", err)
	}

	return mc, nil
}

// validateHost validates the host configuration per mode requirements
func (mc *ModeConfig) validateHost() error {
	// Empty host means auto-detect
	if mc.Host == "" {
		fqdn, err := detectFQDN()
		if err != nil {
			if mc.IsProduction {
				return fmt.Errorf("failed to auto-detect FQDN in production mode: %w", err)
			}
			// Development: use localhost
			mc.FQDN = "localhost"
			mc.IsValidFQDN = false
			return nil
		}
		mc.Host = fqdn
		mc.FQDN = fqdn
		mc.IsValidFQDN = isValidFQDN(fqdn)
		return nil
	}

	// Host is explicitly set
	mc.FQDN = mc.Host
	mc.IsValidFQDN = isValidFQDN(mc.Host)

	// Production mode REQUIRES valid FQDN (TEMPLATE.md PART 5)
	if mc.IsProduction && !mc.IsValidFQDN {
		if isLocalhost(mc.Host) || isIPAddress(mc.Host) {
			return fmt.Errorf(
				"production mode requires a valid FQDN (not localhost or IP address): got '%s'. "+
					"Set server.host in server.yml to a proper domain name or use mode: development",
				mc.Host,
			)
		}
		return fmt.Errorf("invalid FQDN: %s", mc.Host)
	}

	return nil
}

// detectFQDN attempts to detect the system's fully qualified domain name
func detectFQDN() (string, error) {
	// Try hostname -f first
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}

	// Check if hostname is already FQDN
	if strings.Contains(hostname, ".") && isValidFQDN(hostname) {
		return hostname, nil
	}

	// Try to resolve hostname to get FQDN
	addrs, err := net.LookupHost(hostname)
	if err == nil && len(addrs) > 0 {
		// Try reverse lookup on first address
		names, err := net.LookupAddr(addrs[0])
		if err == nil && len(names) > 0 {
			// Remove trailing dot if present
			fqdn := strings.TrimSuffix(names[0], ".")
			if isValidFQDN(fqdn) {
				return fqdn, nil
			}
		}
	}

	// Couldn't detect FQDN
	return "", fmt.Errorf("could not auto-detect FQDN from hostname '%s'", hostname)
}

// isValidFQDN checks if a string is a valid fully qualified domain name
func isValidFQDN(host string) bool {
	// Must not be empty
	if host == "" {
		return false
	}

	// Must not be localhost or IP
	if isLocalhost(host) || isIPAddress(host) {
		return false
	}

	// Must contain at least one dot
	if !strings.Contains(host, ".") {
		return false
	}

	// Must not start or end with dot
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}

	// Must not have consecutive dots
	if strings.Contains(host, "..") {
		return false
	}

	// Split into labels
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}

	// Validate each label
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}

		// Label must start with alphanumeric
		if !isAlphanumeric(rune(label[0])) {
			return false
		}

		// Label can contain alphanumeric and hyphens
		for _, ch := range label {
			if !isAlphanumeric(ch) && ch != '-' {
				return false
			}
		}

		// Label must not end with hyphen
		if strings.HasSuffix(label, "-") {
			return false
		}
	}

	// TLD must be alphabetic only (no numbers)
	tld := labels[len(labels)-1]
	for _, ch := range tld {
		if !isAlpha(ch) {
			return false
		}
	}

	return true
}

// isLocalhost checks if host is localhost or loopback
func isLocalhost(host string) bool {
	lower := strings.ToLower(host)
	return lower == "localhost" ||
		lower == "localhost.localdomain" ||
		strings.HasPrefix(lower, "127.") ||
		lower == "::1"
}

// isIPAddress checks if host is an IP address (IPv4 or IPv6)
func isIPAddress(host string) bool {
	return net.ParseIP(host) != nil
}

// isAlphanumeric checks if a rune is alphanumeric
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// isAlpha checks if a rune is alphabetic
func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// String returns a human-readable representation of the mode
func (m ModeString) String() string {
	return string(m)
}

// Validate checks if the mode is valid
func (m ModeString) Validate() error {
	if m != ModeDevelopment && m != ModeProduction {
		return fmt.Errorf("invalid mode: %s (must be 'development' or 'production')", m)
	}
	return nil
}

// GetLogLevel returns the appropriate log level for the mode
func (mc *ModeConfig) GetLogLevel() string {
	if mc.IsDevelopment {
		return "debug"
	}
	return "warn"
}

// GetSecurityHeaders returns whether to enforce strict security headers
func (mc *ModeConfig) GetSecurityHeaders() bool {
	return mc.IsProduction
}

// GetCORSPolicy returns the CORS policy based on mode
func (mc *ModeConfig) GetCORSPolicy() string {
	if mc.IsDevelopment {
		// Allow all origins in development
		return "*"
	}
	// Strict origin checking in production
	return "strict"
}

// GetRateLimitEnabled returns whether rate limiting should be enforced
func (mc *ModeConfig) GetRateLimitEnabled() bool {
	return mc.IsProduction
}

// ShouldValidateCSRF returns whether CSRF validation should be enforced
func (mc *ModeConfig) ShouldValidateCSRF() bool {
	return mc.IsProduction
}

// ShouldRequireHTTPS returns whether HTTPS should be required
func (mc *ModeConfig) ShouldRequireHTTPS() bool {
	return mc.IsProduction
}

// PrintConfig prints the mode configuration (useful for startup logs and --status)
func (mc *ModeConfig) PrintConfig() {
	fmt.Printf("Application Mode Configuration:\n")
	fmt.Printf("  Mode:                    %s\n", mc.Mode)
	fmt.Printf("  Host:                    %s\n", mc.Host)
	fmt.Printf("  Port:                    %d\n", mc.Port)
	fmt.Printf("  FQDN:                    %s\n", mc.FQDN)
	fmt.Printf("  Valid FQDN:              %t\n", mc.IsValidFQDN)
	fmt.Printf("  Strict Host Validation:  %t\n", mc.StrictHostValidation)
	fmt.Printf("  Require FQDN:            %t\n", mc.RequireFQDN)
	fmt.Printf("  Allow Insecure:          %t\n", mc.AllowInsecure)
	fmt.Printf("  Verbose Logging:         %t\n", mc.VerboseLogging)
	fmt.Printf("  Debug Enabled:           %t\n", mc.DebugEnabled)
}

// WarningMessage returns a warning message if in development mode
func (mc *ModeConfig) WarningMessage() string {
	if mc.IsDevelopment {
		return fmt.Sprintf(`
%s WARNING: Running in DEVELOPMENT mode
   - Relaxed security policies
   - Verbose logging enabled
   - FQDN validation disabled
   - NEVER use development mode in production!
`, display.Emoji("⚠️", "[!]"))
	}
	return ""
}
