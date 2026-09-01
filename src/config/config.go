package config

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
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

// RegistrationConfig represents user registration settings per AI.md PART 34.
type RegistrationConfig struct {
	// Mode controls how new regular-user accounts are created.
	// Per AI.md PART 34 canonical values:
	//   open       — anyone can self-register (public)
	//   invite     — only admin-issued invite links create accounts (default, per IDEA.md)
	//   admin_only — only direct admin creation (no self-service)
	//   disabled   — no new regular-user accounts allowed
	// Legacy aliases accepted for backward compat (normalised in GetRegistrationMode):
	//   "public"  → "open"
	//   "private" → "invite"
	Mode string `yaml:"mode"`
	// RequireEmailVerification requires users to verify email before login.
	RequireEmailVerification bool `yaml:"require_email_verification"`
	// InviteExpirationDays controls how many days invite links remain valid.
	InviteExpirationDays int `yaml:"invite_expiration_days"`
}

// AppConfig represents the application configuration from server.yml per AI.md PART 4
type AppConfig struct {
	Server ServerConfig `yaml:"server"`
	Web    WebConfig    `yaml:"web"`
	// User/Multi-user settings per AI.md PART 33
	Users UsersConfig `yaml:"users"`
	// Weather-specific settings per AI.md PART 37
	Weather WeatherConfig `yaml:"weather"`
}

// ServerConfig represents server-specific configuration per AI.md PART 4
type ServerConfig struct {
	// Port: random 64xxx on first run, then persisted
	// int or string (for dual port "8090,8443")
	Port interface{} `yaml:"port"`
	FQDN string      `yaml:"fqdn"`
	// Default: [::]
	Address string `yaml:"address"`
	// production or development
	Mode string `yaml:"mode"`
	// AI.md: Admin panel URL path (configurable, default: "admin")
	AdminPath string `yaml:"admin_path"`
	// AI.md: API version prefix (default: "v1")
	APIVersion string `yaml:"api_version"`
	// AI.md PART 13: optional root /healthz compatibility alias
	Healthz HealthzConfig `yaml:"healthz"`
	// AI.md PART 31: server output language ("auto" = detect from LANG/LC_ALL)
	Lang     string         `yaml:"lang"`
	Branding BrandingConfig `yaml:"branding"`
	SEO      SEOConfig      `yaml:"seo"`
	User     string         `yaml:"user"`
	Group    string         `yaml:"group"`
	// bool or string path
	PIDFile       interface{}        `yaml:"pidfile"`
	Daemonize     bool               `yaml:"daemonize"`
	Admin         AdminConfig        `yaml:"admin"`
	SSL           SSLConfig          `yaml:"ssl"`
	Scheduler     SchedulerConfig    `yaml:"scheduler"`
	RateLimit     RateLimitConfig    `yaml:"rate_limit"`
	Database      DatabaseConfig     `yaml:"database"`
	Maintenance   MaintenanceConfig  `yaml:"maintenance"`
	Notifications NotificationConfig `yaml:"notifications"`
	Tor           TorConfig          `yaml:"tor"`
	// I2P holds the opt-in I2P eepsite settings per AI.md PART 32.2
	I2P      I2PConfig     `yaml:"i2p"`
	Features FeatureConfig `yaml:"features"`
	// Security holds project-level at-rest encryption settings per AI.md PART 11
	Security SecurityConfig `yaml:"security"`
	// TrustedProxies gates which peers may set forwarded/real-IP headers per AI.md PART 5/12
	TrustedProxies TrustedProxiesConfig `yaml:"trusted_proxies"`
}

// TrustedProxiesConfig represents the trusted-proxy allow-list per AI.md PART 12.
// Private ranges, link-local addresses, and the same /24 as the listen address
// are always trusted with no config; Additional extends that set with public
// upstream proxies (IP, CIDR, or DNS name), refreshed every 5 minutes.
type TrustedProxiesConfig struct {
	Additional []string `yaml:"additional"`
}

// SecurityConfig represents the server-wide at-rest encryption settings per
// AI.md PART 11 ("Cryptographic Keys" -> "Server Encryption Key"). This key is
// distinct from the app_secrets table (installation_secret, cookie_signing_key,
// csrf_token_secret) and is used for AES-256-GCM encryption of 2FA/TOTP
// secrets, security report bodies, and any future at-rest encrypted data.
type SecurityConfig struct {
	// EncryptionKey is a base64-encoded 32-byte AES-256-GCM key, generated
	// once on first run and persisted; never regenerated once present.
	EncryptionKey string `yaml:"encryption_key"`
	// EncryptionKeyVersion increments on manual admin-initiated rotation.
	EncryptionKeyVersion int `yaml:"encryption_key_version"`
}

// AdminConfig represents admin panel configuration per AI.md PART 4
type AdminConfig struct {
	Email string `yaml:"email"`
	// Note: username, password, and token stored in database, not config file
}

// SSLConfig represents SSL/TLS configuration per AI.md PART 4
type SSLConfig struct {
	Enabled bool `yaml:"enabled"`
	// Manual cert path (optional)
	Cert string `yaml:"cert"`
	// Manual key path (optional)
	Key string `yaml:"key"`
	// TLS1.2, TLS1.3
	MinVersion  string            `yaml:"min_version"`
	LetsEncrypt LetsEncryptConfig `yaml:"letsencrypt"`
}

// LetsEncryptConfig represents Let's Encrypt configuration per AI.md PART 4
type LetsEncryptConfig struct {
	Enabled bool   `yaml:"enabled"`
	Email   string `yaml:"email"`
	// http-01, tls-alpn-01, dns-01
	Challenge string `yaml:"challenge"`
	// Use staging server for testing
	Staging bool `yaml:"staging"`
}

// SchedulerConfig represents scheduler configuration per AI.md PART 4
type SchedulerConfig struct {
	Enabled bool                     `yaml:"enabled"`
	Tasks   map[string]SchedulerTask `yaml:"tasks"`
}

// SchedulerTask represents a scheduled task per AI.md PART 4
type SchedulerTask struct {
	Enabled bool `yaml:"enabled"`
	// Cron format or @hourly/@daily
	Schedule    string `yaml:"schedule"`
	RetryOnFail bool   `yaml:"retry_on_fail"`
	// e.g., "1h"
	RetryDelay string `yaml:"retry_delay"`
	// e.g., "30d" (for log_rotation)
	MaxAge string `yaml:"max_age"`
	// e.g., "100MB" (for log_rotation)
	MaxSize string `yaml:"max_size"`
	// e.g., 4 (for backup)
	Retention int `yaml:"retention"`
	// e.g., "7d" (for ssl_renewal)
	RenewBefore string `yaml:"renew_before"`
}

// DatabaseConfig represents database configuration per AI.md PART 4
type DatabaseConfig struct {
	// file, sqlite, postgres, mysql, mariadb, mssql, mongodb
	Driver string `yaml:"driver"`
	// For remote databases
	Host string `yaml:"host"`
	// For remote databases
	Port int `yaml:"port"`
	// Database name
	Name string `yaml:"name"`
	// For remote databases
	Username string `yaml:"username"`
	// For remote databases
	Password string `yaml:"password"`
	// For PostgreSQL
	SSLMode string `yaml:"sslmode"`
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
	Enabled bool `yaml:"enabled"`
	// seconds between retry attempts
	RetryInterval int `yaml:"retry_interval"`
	// 0 = unlimited
	MaxAttempts int `yaml:"max_attempts"`
}

// CleanupConfig represents auto-cleanup thresholds per AI.md PART 4
type CleanupConfig struct {
	// Start cleanup when disk > X% full
	DiskThreshold int `yaml:"disk_threshold"`
	// Delete logs older than X days
	LogRetentionDays int `yaml:"log_retention_days"`
	// Keep last X backups
	BackupKeepCount int `yaml:"backup_keep_count"`
}

// NotifyConfig represents maintenance notification settings per AI.md PART 4
type NotifyConfig struct {
	// Notify when entering maintenance mode
	OnEnter bool `yaml:"on_enter"`
	// Notify when exiting maintenance mode
	OnExit bool `yaml:"on_exit"`
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
	Enabled bool `yaml:"enabled"`
	// Optional password hint (e.g., "First pet's name + year")
	Hint string `yaml:"hint"`
	// Password is NEVER stored - derived on-demand
}

// RateLimitConfig represents rate limiting configuration per AI.md PART 4
type RateLimitConfig struct {
	Enabled bool `yaml:"enabled"`
	// Requests per window
	Requests int `yaml:"requests"`
	// Window in seconds
	Window int `yaml:"window"`
}

// HealthzConfig represents health endpoint configuration per AI.md PART 13
type HealthzConfig struct {
	// Optional root /healthz compatibility alias
	Root HealthzRootConfig `yaml:"root"`
}

// HealthzRootConfig controls the optional root /healthz alias per AI.md PART 13
type HealthzRootConfig struct {
	// When true the same handler as /server/healthz is also mounted at /healthz
	Enabled bool `yaml:"enabled"`
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
	UI UIConfig `yaml:"ui"`
	// CORS setting, e.g., "*"
	CORS string `yaml:"cors"`
	// Custom robots.txt content
	RobotsTxt string `yaml:"robots_txt"`
	// Custom security.txt content
	SecurityTxt string `yaml:"security_txt"`
	// Custom favicon URL (empty = use embedded default)
	FaviconURL string `yaml:"favicon_url"`
	// Generated robots.txt policy (AI crawler access control)
	Robots RobotsConfig `yaml:"robots"`
}

// RobotsConfig represents the generated robots.txt policy per AI.md PART 14
type RobotsConfig struct {
	// Per-AI-crawler access control
	AIBots AIBotsConfig `yaml:"ai_bots"`
}

// AIBotsConfig controls which AI crawlers may index the site per AI.md PART 14
type AIBotsConfig struct {
	// Applies to any recognized AI bot not listed individually in Bots: allow | deny
	Default string `yaml:"default"`
	// Per-bot overrides: allow | deny
	Bots map[string]string `yaml:"bots"`
}

// RecognizedAIBots is the canonical AI crawler list from AI.md PART 14, in spec order
var RecognizedAIBots = []string{
	"GPTBot",
	"ChatGPT-User",
	"ClaudeBot",
	"anthropic-ai",
	"Claude-Web",
	"CCBot",
	"Google-Extended",
	"Bytespider",
	"PerplexityBot",
	"Applebot-Extended",
	"Amazonbot",
	"Diffbot",
	"FacebookBot",
	"cohere-ai",
}

// DeniedAIBots returns the recognized AI crawlers that must render their own
// "Disallow: /" stanza in robots.txt, in the canonical spec order.
// AI.md PART 14: default posture is allow; an explicit per-bot value always
// wins over ai_bots.default, and unknown values fall back to the default.
func (c AIBotsConfig) DeniedAIBots() []string {
	denyByDefault := strings.EqualFold(strings.TrimSpace(c.Default), "deny")

	denied := make([]string, 0, len(RecognizedAIBots))
	for _, bot := range RecognizedAIBots {
		deny := denyByDefault
		for name, value := range c.Bots {
			if !strings.EqualFold(strings.TrimSpace(name), bot) {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "deny":
				deny = true
			case "allow":
				deny = false
			}
			break
		}
		if deny {
			denied = append(denied, bot)
		}
	}
	return denied
}

// AIBotStanzas renders the robots.txt stanzas for every denied AI crawler.
// Returns an empty string when no bot is denied, so an all-allow policy adds
// nothing to the generated file.
func (c AIBotsConfig) AIBotStanzas() string {
	denied := c.DeniedAIBots()
	if len(denied) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# AI crawlers denied by server policy\n")
	for i, bot := range denied {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("User-agent: " + bot + "\n")
		b.WriteString("Disallow: /\n")
	}
	return b.String()
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
	// I2P mirrors server.i2p.enabled so feature-list surfaces can gate on it
	// without reaching into the transport config per AI.md PART 32.2
	I2P bool `yaml:"i2p"`
}

// I2PConfig holds I2P eepsite configuration per AI.md PART 32.2.
// Unlike Tor (auto-enabled when the binary is found), I2P is strictly opt-in:
// no provider is contacted and no port is allocated unless Enabled is true.
type I2PConfig struct {
	// OPT-IN: I2P eepsite is created only when Enabled is true
	Enabled bool `yaml:"enabled" json:"enabled"`
	// i2pd binary path (empty = auto-detect); a found binary selects Model A
	Binary string `yaml:"binary" json:"binary"`
	// SAM bridge address for Model B, used only when no i2pd binary is found
	SAMAddress string `yaml:"sam_address" json:"sam_address"`
	// Virtual port the eepsite listens on, 1-65535, default 80
	VirtualPort int `yaml:"virtual_port" json:"virtual_port"`
	// Inbound tunnel hop count, 0-7, default 3
	InboundLength int `yaml:"inbound_length" json:"inbound_length"`
	// Outbound tunnel hop count, 0-7, default 3
	OutboundLength int `yaml:"outbound_length" json:"outbound_length"`
	// Parallel inbound tunnels, 1-16, default 5
	InboundQuantity int `yaml:"inbound_quantity" json:"inbound_quantity"`
	// Parallel outbound tunnels, 1-16, default 5
	OutboundQuantity int `yaml:"outbound_quantity" json:"outbound_quantity"`
	// SAM/destination signature type, 7 = EdDSA-SHA512-Ed25519
	SignatureType int `yaml:"signature_type" json:"signature_type"`
	// Seconds to wait for the destination and tunnels to become ready, 30-600
	BootstrapTimeout int `yaml:"bootstrap_timeout" json:"bootstrap_timeout"`
}

// DefaultI2PConfig returns the default (disabled) I2P configuration per AI.md PART 32.2
func DefaultI2PConfig() I2PConfig {
	return I2PConfig{
		Enabled:          false,
		Binary:           "",
		SAMAddress:       "127.0.0.1:7656",
		VirtualPort:      80,
		InboundLength:    3,
		OutboundLength:   3,
		InboundQuantity:  5,
		OutboundQuantity: 5,
		SignatureType:    7,
		BootstrapTimeout: 300,
	}
}

// ValidationError describes a single rejected configuration field
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface so a single ValidationError can be
// returned directly from update handlers
func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// i2pSignatureTypes lists the destination signature types the eepsite accepts
// per AI.md PART 32.2 (0 = DSA-SHA1 legacy, 7 = EdDSA-SHA512-Ed25519)
var i2pSignatureTypes = []int{0, 7}

// ValidateI2PConfig validates all I2P settings before saving per AI.md PART 32.2
func ValidateI2PConfig(cfg *I2PConfig) []ValidationError {
	var errs []ValidationError
	if cfg == nil {
		return []ValidationError{{Field: "i2p", Message: "configuration is missing"}}
	}
	if cfg.VirtualPort < 1 || cfg.VirtualPort > 65535 {
		errs = append(errs, ValidationError{Field: "virtual_port", Message: "must be between 1 and 65535"})
	}
	if cfg.InboundLength < 0 || cfg.InboundLength > 7 {
		errs = append(errs, ValidationError{Field: "inbound_length", Message: "must be between 0 and 7"})
	}
	if cfg.OutboundLength < 0 || cfg.OutboundLength > 7 {
		errs = append(errs, ValidationError{Field: "outbound_length", Message: "must be between 0 and 7"})
	}
	if cfg.InboundQuantity < 1 || cfg.InboundQuantity > 16 {
		errs = append(errs, ValidationError{Field: "inbound_quantity", Message: "must be between 1 and 16"})
	}
	if cfg.OutboundQuantity < 1 || cfg.OutboundQuantity > 16 {
		errs = append(errs, ValidationError{Field: "outbound_quantity", Message: "must be between 1 and 16"})
	}
	validSignature := false
	for _, allowed := range i2pSignatureTypes {
		if cfg.SignatureType == allowed {
			validSignature = true
			break
		}
	}
	if !validSignature {
		errs = append(errs, ValidationError{Field: "signature_type", Message: "must be 0 or 7"})
	}
	if cfg.BootstrapTimeout < 30 || cfg.BootstrapTimeout > 600 {
		errs = append(errs, ValidationError{Field: "bootstrap_timeout", Message: "must be between 30 and 600 seconds"})
	}
	if strings.TrimSpace(cfg.SAMAddress) != "" {
		if _, _, err := net.SplitHostPort(strings.TrimSpace(cfg.SAMAddress)); err != nil {
			errs = append(errs, ValidationError{Field: "sam_address", Message: "must be a host:port address"})
		}
	}
	return errs
}

// NormalizeI2PConfig replaces out-of-range or empty I2P values with their
// defaults. AI.md PART 5 forbids failing startup on a bad config value, so
// every rejected field silently falls back instead of erroring.
func NormalizeI2PConfig(cfg *I2PConfig) {
	if cfg == nil {
		return
	}
	defaults := DefaultI2PConfig()
	if strings.TrimSpace(cfg.SAMAddress) == "" {
		cfg.SAMAddress = defaults.SAMAddress
	}
	if cfg.VirtualPort < 1 || cfg.VirtualPort > 65535 {
		cfg.VirtualPort = defaults.VirtualPort
	}
	if cfg.InboundLength < 0 || cfg.InboundLength > 7 {
		cfg.InboundLength = defaults.InboundLength
	}
	if cfg.OutboundLength < 0 || cfg.OutboundLength > 7 {
		cfg.OutboundLength = defaults.OutboundLength
	}
	if cfg.InboundQuantity < 1 || cfg.InboundQuantity > 16 {
		cfg.InboundQuantity = defaults.InboundQuantity
	}
	if cfg.OutboundQuantity < 1 || cfg.OutboundQuantity > 16 {
		cfg.OutboundQuantity = defaults.OutboundQuantity
	}
	validSignature := false
	for _, allowed := range i2pSignatureTypes {
		if cfg.SignatureType == allowed {
			validSignature = true
			break
		}
	}
	if !validSignature {
		cfg.SignatureType = defaults.SignatureType
	}
	if cfg.BootstrapTimeout < 30 || cfg.BootstrapTimeout > 600 {
		cfg.BootstrapTimeout = defaults.BootstrapTimeout
	}
}

// randomPort returns a random port in the 64000-64999 range per AI.md PART 4
func randomPort() int {
	var b [4]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return 64000 + int(binary.BigEndian.Uint32(b[:]))%1000
}

// generateEncryptionKey returns a base64-encoded 32-byte AES-256-GCM key per
// AI.md PART 11 ("Cryptographic Keys" -> "Server Encryption Key"). Generated
// once on first run (or once on upgrade if missing from an existing
// server.yml) and persisted — never regenerated once present, since a new
// key would make previously-encrypted at-rest data undecryptable.
func generateEncryptionKey() string {
	key := make([]byte, 32)
	if _, err := cryptorand.Read(key); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(key)
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

// GetAdminAPIPath returns the full admin API path prefix (e.g., "/api/v1/server/admin")
// AI.md: Admin API routes use /api/{api_version}/server/{admin_path}/ format
func (c *AppConfig) GetAdminAPIPath() string {
	return c.GetAPIPath() + "/server/" + c.GetAdminPath()
}

// IsHealthzRootAliasEnabled reports whether the optional root /healthz alias is mounted
// AI.md PART 13: canonical route is /server/healthz, default for the alias is false
func (c *AppConfig) IsHealthzRootAliasEnabled() bool {
	if c == nil {
		return false
	}
	return c.Server.Healthz.Root.Enabled
}

// ResolveLanguage returns the requested output language code per AI.md PART 31
// Priority: --lang flag (CLI_LANG) > server.yml lang > LC_ALL > LANG > "en"
// The caller validates the result against the supported languages and falls back to English
func ResolveLanguage(cfg *AppConfig) string {
	if lang := normalizeLanguageCode(os.Getenv("CLI_LANG")); lang != "" {
		return lang
	}
	if cfg != nil {
		if lang := normalizeLanguageCode(cfg.Server.Lang); lang != "" {
			return lang
		}
	}
	if lang := normalizeLanguageCode(os.Getenv("LC_ALL")); lang != "" {
		return lang
	}
	if lang := normalizeLanguageCode(os.Getenv("LANG")); lang != "" {
		return lang
	}
	return "en"
}

// normalizeLanguageCode reduces a locale string ("es_ES.UTF-8", "en-US") to its base code
// Values that carry no language preference ("", "auto", "c", "posix") return an empty string
func normalizeLanguageCode(value string) string {
	lang := strings.ToLower(strings.TrimSpace(value))
	if idx := strings.IndexAny(lang, "._@"); idx > 0 {
		lang = lang[:idx]
	}
	if idx := strings.IndexAny(lang, "-_"); idx > 0 {
		lang = lang[:idx]
	}
	switch lang {
	case "", "auto", "c", "posix":
		return ""
	}
	return lang
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
				// invite = admin-issued invite links only (per IDEA.md project default)
				Mode:                     "invite",
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
			Port: randomPort(),
			FQDN: hostname,
			// All interfaces IPv4/IPv6
			Address: "[::]",
			Mode:    "production",
			// AI.md: Admin panel URL path (configurable, default: "admin")
			AdminPath: "admin",
			// AI.md: API version prefix (default: "v1")
			APIVersion: "v1",
			// AI.md PART 31: "auto" resolves from LC_ALL/LANG, falling back to English
			Lang:      "auto",
			User:      "{auto}",
			Group:     "{auto}",
			PIDFile:   true,
			Daemonize: false,
			Branding: BrandingConfig{
				Title:       "wthr",
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
						Enabled: true,
						// Weekly Sunday 3am
						Schedule:    "0 3 * * 0",
						RetryOnFail: true,
						RetryDelay:  "1h",
					},
					"blocklist_update": {
						Enabled: true,
						// Daily 4am
						Schedule:    "0 4 * * *",
						RetryOnFail: true,
						RetryDelay:  "1h",
					},
					"cve_update": {
						Enabled: true,
						// Daily 5am
						Schedule:    "0 5 * * *",
						RetryOnFail: true,
						RetryDelay:  "1h",
					},
					"log_rotation": {
						Enabled: true,
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
						Enabled: true,
						// Daily 2am
						Schedule:  "0 2 * * *",
						Retention: 4,
					},
					"ssl_renewal": {
						Enabled: true,
						// Daily 3am
						Schedule:    "0 3 * * *",
						RenewBefore: "7d",
					},
					"health_check": {
						Enabled: true,
						// Every 5 minutes
						Schedule: "*/5 * * * *",
					},
					"tor_health": {
						Enabled: true,
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
					MaxAttempts: 0,
				},
				Cleanup: CleanupConfig{
					DiskThreshold:    90,
					LogRetentionDays: 7,
					// AI.md PART 22: Keep max 4 backups (storage management)
					BackupKeepCount: 4,
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
						Hint: "",
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
			I2P: DefaultI2PConfig(),
			Features: FeatureConfig{
				Earthquakes:   true,
				Hurricanes:    true,
				MoonPhases:    true,
				SevereWeather: true,
				AuditLog:      true,
				I2P:           false,
			},
			// AI.md PART 11: left empty here so the load path below can tell
			// "missing from server.yml" apart from "explicitly present" and
			// generate-once-and-persist accordingly (see LoadConfig).
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

		// AI.md PART 11: first run, generate the at-rest encryption key
		// before persisting the default config to disk.
		cfg.Server.Security = SecurityConfig{
			EncryptionKey:        generateEncryptionKey(),
			EncryptionKeyVersion: 1,
		}

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

	// AI.md PART 5: an invalid config value warns and falls back to the
	// default rather than failing startup, so I2P settings are normalized
	// in place immediately after decoding.
	NormalizeI2PConfig(&cfg.Server.I2P)

	// AI.md PART 11: an existing server.yml from before this key existed
	// (or one with the field blanked) must get a key generated ONCE and
	// persisted immediately — unlike Port, silently regenerating this value
	// every restart would make previously-encrypted at-rest data (2FA
	// secrets, security reports) permanently undecryptable.
	if cfg.Server.Security.EncryptionKey == "" {
		cfg.Server.Security.EncryptionKey = generateEncryptionKey()
		if cfg.Server.Security.EncryptionKeyVersion == 0 {
			cfg.Server.Security.EncryptionKeyVersion = 1
		}
		if err := SaveConfig(cfg); err != nil {
			// Log error but continue with the in-memory key so the process still
			// starts; the key will simply be regenerated again next restart until
			// the underlying write failure (e.g. read-only filesystem) is fixed.

			fmt.Fprintf(os.Stderr, "Warning: Could not persist generated encryption key: %v\n", err)
		}
	}

	return cfg, nil
}

// getConfigPath returns the config file path based on user privileges per AI.md PART 4
func getConfigPath() string {
	// Check if running as root
	if os.Geteuid() == 0 {
		// Root user: /etc/webappsgo/wthr/server.yml

		return "/etc/webappsgo/wthr/server.yml"
	}

	// Regular user: ~/.config/webappsgo/wthr/server.yml
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home not found

		return "server.yml"
	}
	return filepath.Join(home, ".config", "webappsgo", "wthr", "server.yml")
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
# Weather Configuration (AI.md PART 4)
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

// UpdateWebRobotsAIBots updates the AI crawler policy in server.yml.
// Values other than "allow"/"deny" are rejected so an invalid admin submission
// can never silently flip a bot's posture.
func UpdateWebRobotsAIBots(defaultPolicy string, bots map[string]string) error {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config not initialized")
	}

	normalized, err := normalizeAIBotPolicy(defaultPolicy)
	if err != nil {
		return fmt.Errorf("invalid ai_bots default: %w", err)
	}

	normalizedBots := make(map[string]string, len(bots))
	for name, value := range bots {
		canonical := ""
		for _, bot := range RecognizedAIBots {
			if strings.EqualFold(strings.TrimSpace(name), bot) {
				canonical = bot
				break
			}
		}
		if canonical == "" {
			return fmt.Errorf("unrecognized ai bot: %q", name)
		}
		policy, err := normalizeAIBotPolicy(value)
		if err != nil {
			return fmt.Errorf("invalid policy for %s: %w", canonical, err)
		}
		normalizedBots[canonical] = policy
	}

	cfg.Web.Robots.AIBots.Default = normalized
	cfg.Web.Robots.AIBots.Bots = normalizedBots
	return SaveConfig(cfg)
}

// UpdateI2PConfig validates and persists the I2P eepsite settings per AI.md PART 32.2.
// It also mirrors the enabled flag into server.features.i2p so feature-list
// surfaces can gate on a single value.
func UpdateI2PConfig(next I2PConfig) []ValidationError {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return []ValidationError{{Field: "i2p", Message: "global config not initialized"}}
	}

	next.SAMAddress = strings.TrimSpace(next.SAMAddress)
	next.Binary = strings.TrimSpace(next.Binary)
	if errs := ValidateI2PConfig(&next); len(errs) > 0 {
		return errs
	}

	cfg.Server.I2P = next
	cfg.Server.Features.I2P = next.Enabled
	if err := SaveConfig(cfg); err != nil {
		return []ValidationError{{Field: "i2p", Message: err.Error()}}
	}
	return nil
}

// normalizeAIBotPolicy validates and lowercases an AI crawler policy value
func normalizeAIBotPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "allow":
		return "allow", nil
	case "deny":
		return "deny", nil
	default:
		return "", fmt.Errorf("must be allow or deny, got %q", value)
	}
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

// GetRegistrationMode returns the canonical registration mode per AI.md PART 34.
// Valid canonical values: "open", "invite", "admin_only", "disabled".
// Legacy values "public" and "private" are normalised to their canonical equivalents.
// Default when no config is set: "invite" (per IDEA.md project specification).
func GetRegistrationMode() string {
	cfg := GetGlobalConfig()
	var mode string
	if cfg != nil {
		mode = cfg.Users.Registration.Mode
	}
	switch mode {
	case "public":
		return "open"
	case "private":
		return "invite"
	case "open", "invite", "admin_only", "disabled":
		return mode
	default:
		// Not configured or unknown value: default to invite per IDEA.md.
		return "invite"
	}
}

// GetUserInviteExpirationDays returns the configured invite expiration in days.
func GetUserInviteExpirationDays() int {
	cfg := GetGlobalConfig()
	if cfg == nil || cfg.Users.Registration.InviteExpirationDays <= 0 {
		return 7
	}

	return cfg.Users.Registration.InviteExpirationDays
}

// IsRegistrationOpen returns true when anyone can self-register (mode "open").
// Per AI.md PART 34: open mode = anyone can register.
func IsRegistrationOpen() bool {
	return GetRegistrationMode() == "open"
}

// IsRegistrationPublic is a backward-compatible alias for IsRegistrationOpen.
func IsRegistrationPublic() bool {
	return IsRegistrationOpen()
}

// IsRegistrationPrivate returns true when only invite links create accounts.
// Kept for backward compat; prefer IsRegistrationInviteOnly.
func IsRegistrationPrivate() bool {
	return IsRegistrationInviteOnly()
}

// IsRegistrationInviteOnly returns true when registration mode is "invite".
// Per AI.md PART 34: invite mode = admin-issued invite links only.
func IsRegistrationInviteOnly() bool {
	return GetRegistrationMode() == "invite"
}

// IsRegistrationAdminOnly returns true when only admins can directly create accounts.
// Per AI.md PART 34: admin_only mode = no self-service or invites.
func IsRegistrationAdminOnly() bool {
	return GetRegistrationMode() == "admin_only"
}

// IsRegistrationDisabled returns true when no new regular-user accounts are allowed.
// Per AI.md PART 34: disabled mode = existing users only.
func IsRegistrationDisabled() bool {
	return GetRegistrationMode() == "disabled"
}
