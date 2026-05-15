package config

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WeatherConfig represents weather-specific configuration per AI.md PART 37
type WeatherConfig struct {
	// Number of days for forecast
	ForecastDays int `yaml:"forecast_days"`
	// Weather data update interval (seconds)
	UpdateInterval int `yaml:"update_interval"`
	// Enable location-based weather queries
	LocationSearchEnabled bool `yaml:"location_search_enabled"`
}

// UsersConfig represents user/multi-user settings per AI.md PART 33
type UsersConfig struct {
	// Multi-user support enabled
	Enabled bool `yaml:"enabled"`
	// Registration settings
	Registration RegistrationConfig `yaml:"registration"`
}

// RegistrationConfig represents user registration settings per AI.md PART 33
type RegistrationConfig struct {
	// Mode: public, private, disabled
	// public = anyone can register
	// private = invite only (admin creates invite links)
	// disabled = invite only with registration hidden
	Mode string `yaml:"mode"`
	// Require users to verify email before login
	RequireEmailVerification bool `yaml:"require_email_verification"`
	// Invite links remain valid for this many days
	InviteExpirationDays int `yaml:"invite_expiration_days"`
}

// AppConfig represents the application configuration from server.yml per AI.md PART 4
type AppConfig struct {
	Server  ServerConfig  `yaml:"server"`
	Web     WebConfig     `yaml:"web"`
	// User/Multi-user settings per AI.md PART 33
	Users   UsersConfig   `yaml:"users"`
	// Weather-specific settings per AI.md PART 37
	Weather WeatherConfig `yaml:"weather"`
}

// ServerConfig represents server-specific configuration per AI.md PART 4
type ServerConfig struct {
	// Port: random 64xxx on first run, then persisted
	// int or string (for dual port "8090,8443")
	Port     interface{}        `yaml:"port"`
	FQDN     string             `yaml:"fqdn"`
	// Default: [::]
	Address  string             `yaml:"address"`
	// production or development
	Mode     string             `yaml:"mode"`
	// AI.md: Admin panel URL path (configurable, default: "admin")
	AdminPath string            `yaml:"admin_path"`
	// AI.md: API version prefix (default: "v1")
	APIVersion string           `yaml:"api_version"`
	Branding BrandingConfig     `yaml:"branding"`
	SEO      SEOConfig          `yaml:"seo"`
	User     string             `yaml:"user"`
	Group    string             `yaml:"group"`
	// bool or string path
	PIDFile  interface{}        `yaml:"pidfile"`
	Daemonize bool              `yaml:"daemonize"`
	Admin    AdminConfig        `yaml:"admin"`
	SSL      SSLConfig          `yaml:"ssl"`
	Scheduler SchedulerConfig   `yaml:"scheduler"`
	RateLimit RateLimitConfig   `yaml:"rate_limit"`
	Database DatabaseConfig     `yaml:"database"`
	Maintenance MaintenanceConfig `yaml:"maintenance"`
	Notifications NotificationConfig `yaml:"notifications"`
	Tor      TorConfig          `yaml:"tor"`
	Features FeatureConfig      `yaml:"features"`
}

// AdminConfig represents admin panel configuration per AI.md PART 4
type AdminConfig struct {
	Email string `yaml:"email"`
	// Note: username, password, and token stored in database, not config file
}

// SSLConfig represents SSL/TLS configuration per AI.md PART 4
type SSLConfig struct {
	Enabled    bool              `yaml:"enabled"`
	// Manual cert path (optional)
	Cert       string            `yaml:"cert"`
	// Manual key path (optional)
	Key        string            `yaml:"key"`
	// TLS1.2, TLS1.3
	MinVersion string            `yaml:"min_version"`
	LetsEncrypt LetsEncryptConfig `yaml:"letsencrypt"`
}

// LetsEncryptConfig represents Let's Encrypt configuration per AI.md PART 4
type LetsEncryptConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Email     string `yaml:"email"`
	// http-01, tls-alpn-01, dns-01
	Challenge string `yaml:"challenge"`
	// Use staging server for testing
	Staging   bool   `yaml:"staging"`
}

// SchedulerConfig represents scheduler configuration per AI.md PART 4
type SchedulerConfig struct {
	Enabled bool                   `yaml:"enabled"`
	Tasks   map[string]SchedulerTask `yaml:"tasks"`
}

// SchedulerTask represents a scheduled task per AI.md PART 4
type SchedulerTask struct {
	Enabled      bool   `yaml:"enabled"`
	// Cron format or @hourly/@daily
	Schedule     string `yaml:"schedule"`
	RetryOnFail  bool   `yaml:"retry_on_fail"`
	// e.g., "1h"
	RetryDelay   string `yaml:"retry_delay"`
	// e.g., "30d" (for log_rotation)
	MaxAge       string `yaml:"max_age"`
	// e.g., "100MB" (for log_rotation)
	MaxSize      string `yaml:"max_size"`
	// e.g., 4 (for backup)
	Retention    int    `yaml:"retention"`
	// e.g., "7d" (for ssl_renewal)
	RenewBefore  string `yaml:"renew_before"`
}

// DatabaseConfig represents database configuration per AI.md PART 4
type DatabaseConfig struct {
	// file, sqlite, postgres, mysql, mariadb, mssql, mongodb
	Driver   string `yaml:"driver"`
	// For remote databases
	Host     string `yaml:"host"`
	// For remote databases
	Port     int    `yaml:"port"`
	// Database name
	Name     string `yaml:"name"`
	// For remote databases
	Username string `yaml:"username"`
	// For remote databases
	Password string `yaml:"password"`
	// For PostgreSQL
	SSLMode  string `yaml:"sslmode"`
}

// MaintenanceConfig represents maintenance mode configuration per AI.md PART 4
type MaintenanceConfig struct {
	SelfHealing SelfHealingConfig `yaml:"self_healing"`
	Cleanup     CleanupConfig     `yaml:"cleanup"`
	Notify      NotifyConfig      `yaml:"notify"`
	Backup      BackupConfig      `yaml:"backup"`
}

// SelfHealingConfig represents self-healing settings per AI.md PART 4
type SelfHealingConfig struct {
	Enabled       bool `yaml:"enabled"`
	// seconds between retry attempts
	RetryInterval int  `yaml:"retry_interval"`
	// 0 = unlimited
	MaxAttempts   int  `yaml:"max_attempts"`
}

// CleanupConfig represents auto-cleanup thresholds per AI.md PART 4
type CleanupConfig struct {
	// Start cleanup when disk > X% full
	DiskThreshold     int `yaml:"disk_threshold"`
	// Delete logs older than X days
	LogRetentionDays  int `yaml:"log_retention_days"`
	// Keep last X backups
	BackupKeepCount   int `yaml:"backup_keep_count"`
}

// NotifyConfig represents maintenance notification settings per AI.md PART 4
type NotifyConfig struct {
	// Notify when entering maintenance mode
	OnEnter bool `yaml:"on_enter"`
	// Notify when exiting maintenance mode
	OnExit  bool `yaml:"on_exit"`
}

// BackupConfig represents backup settings per AI.md PART 19, PART 24
type BackupConfig struct {
	Encryption BackupEncryptionConfig `yaml:"encryption"`
	// AI.md PART 19 line 24812: Enable hourly incremental backup (disabled by default)
	HourlyEnabled bool `yaml:"hourly_enabled"`
}

// BackupEncryptionConfig represents backup encryption settings per AI.md PART 24
type BackupEncryptionConfig struct {
	// true if password was set during setup
	Enabled bool   `yaml:"enabled"`
	// Optional password hint (e.g., "First pet's name + year")
	Hint    string `yaml:"hint"`
	// Password is NEVER stored - derived on-demand
}

// RateLimitConfig represents rate limiting configuration per AI.md PART 4
type RateLimitConfig struct {
	Enabled  bool `yaml:"enabled"`
	// Requests per window
	Requests int  `yaml:"requests"`
	// Window in seconds
	Window   int  `yaml:"window"`
}

// BrandingConfig represents branding configuration per AI.md PART 4
type BrandingConfig struct {
	Title       string `yaml:"title"`
	Tagline     string `yaml:"tagline"`
	Description string `yaml:"description"`
}

// SEOConfig represents SEO configuration per AI.md PART 16
type SEOConfig struct {
	// Array of keywords
	Keywords []string `yaml:"keywords"`
	// Author/organization name
	Author string `yaml:"author"`
	// OpenGraph image URL for social sharing
	OGImage string `yaml:"og_image"`
	// Twitter @handle for cards
	TwitterHandle string `yaml:"twitter_handle"`
	// Site verification codes per AI.md PART 16
	Verification VerificationConfig `yaml:"verification"`
}

// VerificationConfig holds site verification codes per AI.md PART 16
type VerificationConfig struct {
	Google    string `yaml:"google"`
	Bing      string `yaml:"bing"`
	Yandex    string `yaml:"yandex"`
	Baidu     string `yaml:"baidu"`
	Pinterest string `yaml:"pinterest"`
	Facebook  string `yaml:"facebook"`
}

// NotificationConfig represents notification settings per AI.md PART 4
type NotificationConfig struct {
	Enabled        bool `yaml:"enabled"`
	EmailEnabled   bool `yaml:"email_enabled"`
	WebhookEnabled bool `yaml:"webhook_enabled"`
}

// WebConfig represents web-specific configuration per AI.md PART 4
// WebConfig represents web interface configuration
type WebConfig struct {
	UI          UIConfig `yaml:"ui"`
	// CORS setting, e.g., "*"
	CORS        string   `yaml:"cors"`
	// Custom robots.txt content
	RobotsTxt   string   `yaml:"robots_txt"`
	// Custom security.txt content
	SecurityTxt string   `yaml:"security_txt"`
	// Custom favicon URL (empty = use embedded default)
	FaviconURL  string   `yaml:"favicon_url"`
}

// UIConfig represents UI configuration per AI.md PART 4
type UIConfig struct {
	// dark, light
	Theme string `yaml:"theme"`
}

// TorConfig represents Tor hidden service configuration per AI.md PART 4
type TorConfig struct {
	Enabled   bool   `yaml:"enabled"`
	OnionAddr string `yaml:"onion_addr"`
}

// FeatureConfig represents feature toggles per AI.md PART 4
type FeatureConfig struct {
	Earthquakes   bool `yaml:"earthquakes"`
	Hurricanes    bool `yaml:"hurricanes"`
	MoonPhases    bool `yaml:"moon_phases"`
	SevereWeather bool `yaml:"severe_weather"`
	AuditLog      bool `yaml:"audit_log"`
}

// randomPort returns a random port in the 64000-64999 range per AI.md PART 4
func randomPort() int {
	return 64000 + rand.Intn(1000)
}

// getDefaultFQDN returns the default FQDN (hostname) per AI.md PART 4
func getDefaultFQDN() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	return hostname
}

func defaultEmailAddressForHost(localPart, host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if host == "" || host == "localhost" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		host = "wthr.top"
	}
	if localPart == "" {
		localPart = "admin"
	}
	return fmt.Sprintf("%s@%s", localPart, host)
}

// DefaultEmailAddress returns a valid default email address for the current config.
func DefaultEmailAddress(localPart string, cfg *AppConfig) string {
	if cfg == nil {
		return defaultEmailAddressForHost(localPart, "")
	}
	return defaultEmailAddressForHost(localPart, cfg.Server.FQDN)
}

// GetAdminPath returns the admin panel URL path with default fallback
// AI.md: {admin_path} is configurable, default: "admin"
func (c *AppConfig) GetAdminPath() string {
	if c == nil || c.Server.AdminPath == "" {
		return "admin"
	}
	return c.Server.AdminPath
}

// GetAPIVersion returns the API version prefix with default fallback
// AI.md: {api_version} is configurable, default: "v1"
func (c *AppConfig) GetAPIVersion() string {
	if c == nil || c.Server.APIVersion == "" {
		return "v1"
	}
	return c.Server.APIVersion
}

// GetAPIPath returns the full API path prefix (e.g., "/api/v1")
// AI.md: Routes use /api/{api_version}/ format
func (c *AppConfig) GetAPIPath() string {
	return "/api/" + c.GetAPIVersion()
}

// GetAdminAPIPath returns the full admin API path prefix (e.g., "/api/v1/admin")
// AI.md: Admin API routes use /api/{api_version}/{admin_path}/ format
func (c *AppConfig) GetAdminAPIPath() string {
	return c.GetAPIPath() + "/" + c.GetAdminPath()
}

// LoadConfig loads configuration from server.yml per AI.md PART 4
func LoadConfig() (*AppConfig, error) {
	// Get hostname for defaults
	hostname := getDefaultFQDN()
	adminEmail := defaultEmailAddressForHost("admin", hostname)

	// Default config with sane defaults per AI.md PART 4
	cfg := &AppConfig{
		// User/Multi-user defaults per AI.md PART 33
		Users: UsersConfig{
			Enabled: true,
			Registration: RegistrationConfig{
				// public = anyone can register (default)
				Mode:                     "public",
				RequireEmailVerification: true,
				InviteExpirationDays:     7,
			},
		},
		// Weather-specific defaults per AI.md PART 37
		Weather: WeatherConfig{
			// 7-day forecast by default
			ForecastDays: 7,
			// 3600 seconds (1 hour) update interval
			UpdateInterval: 3600,
			// Location search enabled by default
			LocationSearchEnabled: true,
		},
		Server: ServerConfig{
			// Random 64xxx on first run
			Port:      randomPort(),
			FQDN:      hostname,
			// All interfaces IPv4/IPv6
			Address:   "[::]",
			Mode:      "production",
			// AI.md: Admin panel URL path (configurable, default: "admin")
			AdminPath: "admin",
			// AI.md: API version prefix (default: "v1")
			APIVersion: "v1",
			User:      "{auto}",
			Group:     "{auto}",
			PIDFile:   true,
			Daemonize: false,
			Branding: BrandingConfig{
				Title:       "weather",
				Tagline:     "",
				Description: "",
			},
			SEO: SEOConfig{
				Keywords: []string{},
			},
			Admin: AdminConfig{
				Email: adminEmail,
			},
			SSL: SSLConfig{
				Enabled:    false,
				Cert:       "",
				Key:        "",
				MinVersion: "TLS1.2",
				LetsEncrypt: LetsEncryptConfig{
					Enabled:   false,
					Email:     adminEmail,
					Challenge: "http-01",
					Staging:   false,
				},
			},
			Scheduler: SchedulerConfig{
				Enabled: true,
				Tasks: map[string]SchedulerTask{
					"geoip_update": {
						Enabled:     true,
						// Weekly Sunday 3am
						Schedule:    "0 3 * * 0",
						RetryOnFail: true,
						RetryDelay:  "1h",
					},
					"blocklist_update": {
						Enabled:     true,
						// Daily 4am
						Schedule:    "0 4 * * *",
						RetryOnFail: true,
						RetryDelay:  "1h",
					},
					"cve_update": {
						Enabled:     true,
						// Daily 5am
						Schedule:    "0 5 * * *",
						RetryOnFail: true,
						RetryDelay:  "1h",
					},
					"log_rotation": {
						Enabled:  true,
						// Daily midnight
						Schedule: "0 0 * * *",
						MaxAge:   "30d",
						MaxSize:  "100MB",
					},
					"session_cleanup": {
						Enabled:  true,
						Schedule: "@hourly",
					},
					"backup": {
						Enabled:   true,
						// Daily 2am
						Schedule:  "0 2 * * *",
						Retention: 4,
					},
					"ssl_renewal": {
						Enabled:     true,
						// Daily 3am
						Schedule:    "0 3 * * *",
						RenewBefore: "7d",
					},
					"health_check": {
						Enabled:  true,
						// Every 5 minutes
						Schedule: "*/5 * * * *",
					},
					"tor_health": {
						Enabled:  true,
						// Every 10 minutes
						Schedule: "*/10 * * * *",
					},
				},
			},
			RateLimit: RateLimitConfig{
				Enabled:  true,
				Requests: 120,
				Window:   60,
			},
			Database: DatabaseConfig{
				Driver: "file",
			},
			Maintenance: MaintenanceConfig{
				SelfHealing: SelfHealingConfig{
					Enabled:       true,
					RetryInterval: 30,
					// Unlimited
					MaxAttempts:   0,
				},
				Cleanup: CleanupConfig{
					DiskThreshold:    90,
					LogRetentionDays: 7,
					// AI.md PART 22: Keep max 4 backups (storage management)
					BackupKeepCount:  4,
				},
				Notify: NotifyConfig{
					OnEnter: true,
					OnExit:  true,
				},
				Backup: BackupConfig{
					Encryption: BackupEncryptionConfig{
						// Set to true during setup wizard if password provided
						Enabled: false,
						// Optional password hint
						Hint:    "",
					},
				},
			},
			Notifications: NotificationConfig{
				Enabled:      true,
				EmailEnabled: true,
			},
			Tor: TorConfig{
				Enabled: false,
			},
			Features: FeatureConfig{
				Earthquakes:   true,
				Hurricanes:    true,
				MoonPhases:    true,
				SevereWeather: true,
				AuditLog:      true,
			},
		},
		Web: WebConfig{
			UI: UIConfig{
				Theme: "dark",
			},
			CORS: "*",
		},
	}

	// Try to load from server.yml
	configPath := findConfigFile()
	if configPath == "" {
// No config file found - create it on first run per AI.md PART 4

		configPath = getConfigPath()
		if err := createDefaultConfig(cfg, configPath); err != nil {
	// Log error but continue with defaults

			fmt.Fprintf(os.Stderr, "Warning: Could not create config file: %v\n", err)
		}
		return cfg, nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, err
	}

	// Parse YAML with strict mode per AI.md PART 5
	// Unknown keys are ERRORS, not silently ignored
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return cfg, fmt.Errorf("config error: %w (unknown fields are not allowed)", err)
	}

	return cfg, nil
}

// getConfigPath returns the config file path based on user privileges per AI.md PART 4
func getConfigPath() string {
	// Check if running as root
	if os.Geteuid() == 0 {
// Root user: /etc/apimgr/weather/server.yml

		return "/etc/apimgr/weather/server.yml"
	}

	// Regular user: ~/.config/apimgr/weather/server.yml
	home, err := os.UserHomeDir()
	if err != nil {
// Fallback to current directory if home not found

		return "server.yml"
	}
	return filepath.Join(home, ".config", "apimgr", "weather", "server.yml")
}

// findConfigFile searches for server.yml in common locations per AI.md PART 4
func findConfigFile() string {
	// Priority 1: Environment variable CONFIG_DIR
	if configDir := os.Getenv("CONFIG_DIR"); configDir != "" {
		path := filepath.Join(configDir, "server.yml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Priority 2: Standard location based on user
	standardPath := getConfigPath()
	if _, err := os.Stat(standardPath); err == nil {
		return standardPath
	}

	// Priority 3: Check for server.yaml (migrate to server.yml per AI.md PART 4)
	yamlPath := filepath.Join(filepath.Dir(standardPath), "server.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
// Auto-migrate from .yaml to .yml

		if err := os.Rename(yamlPath, standardPath); err == nil {
			return standardPath
		}
		return yamlPath
	}

	return ""
}

// createDefaultConfig creates a default server.yml file per AI.md PART 4
func createDefaultConfig(cfg *AppConfig, path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := `# =============================================================================
# Weather Service Configuration (AI.md PART 4)
# =============================================================================
# This file was auto-generated on first run with sane defaults.
# Edit as needed and restart the service to apply changes.
# =============================================================================

`
	fullData := append([]byte(header), data...)

	// Write to file
	if err := os.WriteFile(path, fullData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created default configuration: %s\n", path)
	return nil
}

// Global config instance for handler access
var globalConfig *AppConfig

// SetGlobalConfig sets the global config instance
func SetGlobalConfig(cfg *AppConfig) {
	globalConfig = cfg
}

// GetGlobalConfig returns the global config instance
func GetGlobalConfig() *AppConfig {
	return globalConfig
}

// SaveConfig saves the current configuration to server.yml per AI.md PART 4
func SaveConfig(cfg *AppConfig) error {
	configPath := findConfigFile()
	if configPath == "" {
		configPath = getConfigPath()
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// UpdateWebRobotsTxt updates the robots.txt content in server.yml
func UpdateWebRobotsTxt(content string) error {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config not initialized")
	}

	cfg.Web.RobotsTxt = content
	return SaveConfig(cfg)
}

// UpdateWebSecurityTxt updates the security.txt content in server.yml
func UpdateWebSecurityTxt(content string) error {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config not initialized")
	}

	cfg.Web.SecurityTxt = content
	return SaveConfig(cfg)
}

// IsMultiUserEnabled returns true if multi-user mode is enabled
// Per AI.md PART 33: Check users.enabled
func IsMultiUserEnabled() bool {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return true
	}
	return cfg.Users.Enabled
}

// GetRegistrationMode returns the current registration mode
// Per AI.md PART 33: public, private, or disabled
func GetRegistrationMode() string {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return "public"
	}
	mode := cfg.Users.Registration.Mode
	if mode == "" {
		return "public" // default
	}
	return mode
}

// GetUserInviteExpirationDays returns the configured invite expiration in days.
func GetUserInviteExpirationDays() int {
	cfg := GetGlobalConfig()
	if cfg == nil || cfg.Users.Registration.InviteExpirationDays <= 0 {
		return 7
	}

	return cfg.Users.Registration.InviteExpirationDays
}

// IsRegistrationPublic returns true if public registration is enabled
// Per AI.md PART 33: public mode = anyone can register
func IsRegistrationPublic() bool {
	return GetRegistrationMode() == "public"
}

// IsRegistrationPrivate returns true if invite-only registration is enabled
// Per AI.md PART 33: private mode = invite only
func IsRegistrationPrivate() bool {
	return GetRegistrationMode() == "private"
}

// IsRegistrationDisabled returns true if registration is disabled
// Per AI.md PART 33: disabled mode = admin creates accounts directly
func IsRegistrationDisabled() bool {
	return GetRegistrationMode() == "disabled"
}
