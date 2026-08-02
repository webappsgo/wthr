package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
)

// Setting represents a configuration setting
type Setting struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
	// string, number, boolean, json
	Type string `json:"type"`
	// Human-readable description
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SettingsModel handles settings operations
type SettingsModel struct {
	DB *sql.DB
}

// Get retrieves a setting by key
func (m *SettingsModel) Get(key string) (*Setting, error) {
	setting := &Setting{}
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect,
		"SELECT key, value, type FROM server_config WHERE key = ?",
		key,
	).Scan(&setting.Key, &setting.Value, &setting.Type)

	if err != nil {
		return nil, err
	}

	return setting, nil
}

// GetString retrieves a setting value as string
func (m *SettingsModel) GetString(key, defaultValue string) string {
	setting, err := m.Get(key)
	if err != nil {
		return defaultValue
	}
	return setting.Value
}

// GetInt retrieves a setting value as int
func (m *SettingsModel) GetInt(key string, defaultValue int) int {
	setting, err := m.Get(key)
	if err != nil {
		return defaultValue
	}

	var value int
	if _, err := fmt.Sscanf(setting.Value, "%d", &value); err != nil {
		return defaultValue
	}
	return value
}

// GetBool retrieves a setting value as bool
func (m *SettingsModel) GetBool(key string, defaultValue bool) bool {
	setting, err := m.Get(key)
	if err != nil {
		return defaultValue
	}

	return setting.Value == "true" || setting.Value == "1"
}

// GetJSON retrieves a setting value as JSON
func (m *SettingsModel) GetJSON(key string, dest interface{}) error {
	setting, err := m.Get(key)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(setting.Value), dest)
}

// Set creates or updates a setting
func (m *SettingsModel) Set(key, value, settingType string) error {
	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_config (key, value, type)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, type = ?
	`, key, value, settingType, value, settingType)

	return err
}

// SetWithDescription sets a setting with description
func (m *SettingsModel) SetWithDescription(key, value, settingType, description string) error {
	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_config (key, value, type, description)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, type = ?, description = ?
	`, key, value, settingType, description, value, settingType, description)

	return err
}

// SetString sets a string setting
func (m *SettingsModel) SetString(key, value string) error {
	return m.Set(key, value, "string")
}

// SetInt sets an integer setting
func (m *SettingsModel) SetInt(key string, value int) error {
	return m.Set(key, fmt.Sprintf("%d", value), "number")
}

// SetBool sets a boolean setting
func (m *SettingsModel) SetBool(key string, value bool) error {
	stringValue := "false"
	if value {
		stringValue = "true"
	}
	return m.Set(key, stringValue, "boolean")
}

// SetJSON sets a JSON setting
func (m *SettingsModel) SetJSON(key string, value interface{}) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.Set(key, string(jsonBytes), "json")
}

// Delete removes a setting
func (m *SettingsModel) Delete(key string) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "DELETE FROM settings WHERE key = ?", key)
	return err
}

// List returns all settings
func (m *SettingsModel) List() ([]*Setting, error) {
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, "SELECT key, value, type FROM settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		setting := &Setting{}
		if err := rows.Scan(&setting.Key, &setting.Value, &setting.Type); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

// ListByPrefix returns all settings with a specific prefix
func (m *SettingsModel) ListByPrefix(prefix string) ([]*Setting, error) {
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect,
		"SELECT key, value, type FROM settings WHERE key LIKE ? ORDER BY key",
		prefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		setting := &Setting{}
		if err := rows.Scan(&setting.Key, &setting.Value, &setting.Type); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

// InitializeDefaults sets default values for all settings
// backupPath is optional - if empty, uses "/data/backups" as default
func (m *SettingsModel) InitializeDefaults(backupPath ...string) error {
	defaultBackupPath := "/data/backups"
	if len(backupPath) > 0 && backupPath[0] != "" {
		defaultBackupPath = backupPath[0]
	}

	defaults := map[string]Setting{
		// Server settings
		"server.title":         {Value: "Weather", Type: "string", Description: "Application display name shown in the header and page titles"},
		"server.tagline":       {Value: "Your personal weather dashboard", Type: "string", Description: "Short subtitle or slogan displayed under the title"},
		"server.description":   {Value: "A comprehensive platform for weather forecasts, moon phases, earthquakes, and hurricane tracking.", Type: "string", Description: "Full description shown on about page and in meta tags"},
		"server.port":          {Value: "64580", Type: "number", Description: "Server listen port (requires restart to apply)"},
		"server.mode":          {Value: "production", Type: "string", Description: "Server mode: production or development (requires restart)"},
		"server.fqdn":          {Value: "", Type: "string", Description: "Fully qualified domain name (auto-detected if empty)"},
		"server.address":       {Value: "[::]", Type: "string", Description: "Server listen address ([::] for all IPv4/IPv6, requires restart)"},
		"server.daemonize":     {Value: "false", Type: "boolean", Description: "Detach from terminal on start (requires restart, not for systemd/docker)"},
		"server.pidfile":       {Value: "true", Type: "boolean", Description: "Create PID file for process management (requires restart)"},
		"server.http_port":     {Value: "0", Type: "number", Description: "HTTP port number (requires restart to apply)"},
		"server.https_port":    {Value: "0", Type: "number", Description: "HTTPS port number (0 to disable, requires restart)"},
		"server.https_enabled": {Value: "false", Type: "boolean", Description: "Enable HTTPS/TLS connections"},
		"server.timezone":      {Value: "America/New_York", Type: "string", Description: "Default server timezone for date/time display"},
		"server.date_format":   {Value: "US", Type: "string", Description: "Date format: US (Jan 1, 2024), EU (1 Jan 2024), or ISO (2024-01-01)"},
		"server.time_format":   {Value: "12-hour", Type: "string", Description: "Time format: 12-hour (3:30 PM) or 24-hour (15:30)"},

		// Security settings
		"security.session_timeout":     {Value: "2592000", Type: "number", Description: "Session timeout in seconds (default: 2592000 = 30 days)"},
		"security.max_login_attempts":  {Value: "5", Type: "number", Description: "Maximum failed login attempts before account lockout"},
		"security.lockout_duration":    {Value: "30", Type: "number", Description: "Account lockout duration in minutes after max login attempts"},
		"security.password_min_length": {Value: "8", Type: "number", Description: "Minimum required password length for user accounts"},

		// security.txt (RFC 9116) settings
		"security.contact":         {Value: "", Type: "string", Description: "Security contact (email, URL, or phone) - comma separated for multiple"},
		"security.expires":         {Value: "", Type: "string", Description: "security.txt expiration date (RFC3339 format) - auto-renewed yearly"},
		"security.languages":       {Value: "en", Type: "string", Description: "Preferred languages for security reports (comma separated)"},
		"security.canonical":       {Value: "", Type: "string", Description: "Canonical URL for security.txt - auto-generated if empty"},
		"security.encryption":      {Value: "", Type: "string", Description: "PGP key URL for encrypted communications (optional)"},
		"security.acknowledgments": {Value: "", Type: "string", Description: "URL to security acknowledgments/hall of fame (optional)"},
		"security.policy":          {Value: "", Type: "string", Description: "URL to security policy/disclosure policy (optional)"},
		"security.hiring":          {Value: "", Type: "string", Description: "URL to security job postings (optional)"},

		// Web frontend settings (app.* are aliases for server.* for consistency)
		"app.name":        {Value: "Weather", Type: "string", Description: "Alias for server.title - Application display name"},
		"app.tagline":     {Value: "Your personal weather dashboard", Type: "string", Description: "Alias for server.tagline - Short subtitle"},
		"app.description": {Value: "A comprehensive platform for weather forecasts, moon phases, earthquakes, and hurricane tracking.", Type: "string", Description: "Alias for server.description - Full description"},

		// SEO settings
		"seo.keywords": {Value: "weather, forecast, alerts, temperature, humidity, precipitation", Type: "string", Description: "Meta keywords for search engines (comma-separated)"},
		"seo.author":   {Value: "", Type: "string", Description: "Website author or organization name"},
		"seo.og_image": {Value: "", Type: "string", Description: "Social media preview image URL (1200x630px recommended)"},

		// robots.txt
		"web.robots_txt": {Value: `User-agent: *
Allow: /
Disallow: /admin/
Disallow: /api/
Sitemap: {app_url}/sitemap.xml`, Type: "text", Description: "robots.txt content - {app_url} will be replaced with actual URL"},

		// security.txt (RFC 9116)
		"web.security_txt": {Value: fmt.Sprintf("Contact: mailto:%s\nExpires: %s\nPreferred-Languages: en\nCanonical: {app_url}/.well-known/security.txt", config.DefaultEmailAddress("security", config.GetGlobalConfig()), time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)), Type: "text", Description: "security.txt content (RFC 9116) - {app_url} will be replaced with actual URL"},

		// Features
		"features.registration_enabled": {Value: "true", Type: "boolean", Description: "Allow new users to register accounts"},
		"features.api_enabled":          {Value: "true", Type: "boolean", Description: "Enable JSON API endpoints"},
		"features.weather_alerts":       {Value: "true", Type: "boolean", Description: "Enable weather alert notifications"},

		// Database settings
		"database.pool_size":     {Value: "25", Type: "number", Description: "Maximum database connection pool size"},
		"database.timeout":       {Value: "30", Type: "number", Description: "Database query timeout in seconds"},
		"database.auto_optimize": {Value: "false", Type: "boolean", Description: "Automatically optimize database weekly"},

		// Backup settings
		"backup.enabled":   {Value: "true", Type: "boolean", Description: "Enable automatic backup system"},
		"backup.interval":  {Value: "6", Type: "number", Description: "Backup interval in hours"},
		"backup.retention": {Value: "30", Type: "number", Description: "Number of days to retain backups"},
		"backup.location":  {Value: defaultBackupPath, Type: "string", Description: "Directory path for storing backups"},

		// Logging settings
		"logging.level":         {Value: "info", Type: "string", Description: "Log level: debug, info, warn, error"},
		"logging.format":        {Value: "apache", Type: "string", Description: "Log format: apache, json, or plain"},
		"logging.access_log":    {Value: "true", Type: "boolean", Description: "Enable HTTP access logging"},
		"logging.error_log":     {Value: "true", Type: "boolean", Description: "Enable error logging"},
		"logging.audit_log":     {Value: "true", Type: "boolean", Description: "Enable audit logging for security events"},
		"logging.rotation_days": {Value: "30", Type: "number", Description: "Number of days to retain log files before rotation"},

		// SMTP settings
		"smtp.enabled":        {Value: "false", Type: "boolean", Description: "Enable email notifications via SMTP"},
		"smtp.host":           {Value: "", Type: "string", Description: "SMTP server hostname or IP address"},
		"smtp.port":           {Value: "587", Type: "number", Description: "SMTP server port (usually 587 for TLS, 465 for SSL)"},
		"smtp.username":       {Value: "", Type: "string", Description: "SMTP authentication username"},
		"smtp.password":       {Value: "", Type: "string", Description: "SMTP authentication password"},
		"smtp.from_address":   {Value: "", Type: "string", Description: "Email address to send notifications from"},
		"smtp.from_name":      {Value: "", Type: "string", Description: "Display name for sent emails"},
		"smtp.encryption":     {Value: "tls", Type: "string", Description: "Encryption method: tls (STARTTLS), ssl, or none"},
		"smtp.use_tls":        {Value: "true", Type: "boolean", Description: "Legacy: Use TLS/STARTTLS for secure SMTP connections"},
		"smtp.test_recipient": {Value: "", Type: "string", Description: "Test email address for verifying SMTP configuration"},

		// Security headers
		"security.csp":                {Value: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:;", Type: "text", Description: "Content-Security-Policy header value"},
		"security.hsts":               {Value: "max-age=31536000; includeSubDomains", Type: "string", Description: "Strict-Transport-Security header value"},
		"security.x_frame_options":    {Value: "DENY", Type: "string", Description: "X-Frame-Options header (DENY, SAMEORIGIN, or ALLOW-FROM)"},
		"security.x_content_type":     {Value: "nosniff", Type: "string", Description: "X-Content-Type-Options header"},
		"security.referrer_policy":    {Value: "strict-origin-when-cross-origin", Type: "string", Description: "Referrer-Policy header"},
		"security.permissions_policy": {Value: "geolocation=(), microphone=(), camera=()", Type: "text", Description: "Permissions-Policy header"},

		// CORS settings
		"security.cors_enabled":     {Value: "false", Type: "boolean", Description: "Enable Cross-Origin Resource Sharing"},
		"security.cors_origins":     {Value: "*", Type: "text", Description: "Allowed CORS origins (one per line, or * for all)"},
		"security.cors_methods":     {Value: "GET, POST, PUT, DELETE, OPTIONS", Type: "string", Description: "Allowed HTTP methods for CORS"},
		"security.cors_headers":     {Value: "Content-Type, Authorization, X-Requested-With", Type: "string", Description: "Allowed request headers for CORS"},
		"security.cors_credentials": {Value: "false", Type: "boolean", Description: "Allow credentials in CORS requests"},
		"security.cors_max_age":     {Value: "3600", Type: "number", Description: "CORS preflight cache duration in seconds"},

		// Rate limiting settings
		"rate_limit.enabled": {Value: "true", Type: "boolean", Description: "Enable rate limiting to prevent abuse"},
		"rate_limit.global":  {Value: "100", Type: "number", Description: "Global rate limit (requests per second)"},
		"rate_limit.per_ip":  {Value: "60", Type: "number", Description: "Per-IP rate limit (requests per minute)"},
		"rate_limit.api":     {Value: "120", Type: "number", Description: "API rate limit (requests per minute)"},
		"rate_limit.admin":   {Value: "300", Type: "number", Description: "Admin panel rate limit (requests per minute)"},
		"rate_limit.window":  {Value: "900", Type: "number", Description: "Rate limit window in seconds (default: 900 = 15 minutes)"},

		// SSL/TLS settings
		"ssl.enabled":       {Value: "false", Type: "boolean", Description: "Enable SSL/TLS for HTTPS connections"},
		"ssl.cert_file":     {Value: "", Type: "string", Description: "Path to SSL certificate file"},
		"ssl.key_file":      {Value: "", Type: "string", Description: "Path to SSL private key file"},
		"ssl.acme_enabled":  {Value: "false", Type: "boolean", Description: "Enable automatic certificate management via ACME (Let's Encrypt)"},
		"ssl.acme_email":    {Value: "", Type: "string", Description: "Email address for ACME certificate registration"},
		"ssl.acme_provider": {Value: "letsencrypt", Type: "string", Description: "ACME provider: letsencrypt or zerossl"},

		// Weather settings
		"weather.refresh_interval": {Value: "1800", Type: "number", Description: "Weather data refresh interval in seconds (default: 1800 = 30 minutes)"},
		"weather.default_units":    {Value: "auto", Type: "string", Description: "Default units: auto, metric, or imperial"},
		"weather.cache_enabled":    {Value: "true", Type: "boolean", Description: "Enable weather data caching for faster responses"},

		// Severe weather alerts settings
		"alerts.enabled":        {Value: "true", Type: "boolean", Description: "Enable severe weather alert monitoring"},
		"alerts.check_interval": {Value: "900", Type: "number", Description: "Alert check interval in seconds (default: 900 = 15 minutes)"},

		// Notifications settings
		"notifications.enabled":       {Value: "true", Type: "boolean", Description: "Enable notification system"},
		"notifications.queue_workers": {Value: "4", Type: "number", Description: "Number of notification queue worker threads"},
		"notifications.retry_max":     {Value: "3", Type: "number", Description: "Maximum retry attempts for failed notifications"},
		"notifications.retry_backoff": {Value: "exponential", Type: "string", Description: "Retry backoff strategy: linear or exponential"},

		// GeoIP settings
		"geoip.enabled":         {Value: "true", Type: "boolean", Description: "Enable GeoIP location detection"},
		"geoip.update_interval": {Value: "604800", Type: "number", Description: "GeoIP database update interval in seconds (default: 604800 = 7 days)"},

		// CORS settings
		"cors.enabled":           {Value: "true", Type: "boolean", Description: "Enable Cross-Origin Resource Sharing (CORS)"},
		"cors.allowed_origins":   {Value: "*", Type: "string", Description: "Allowed CORS origins (comma-separated, * for all)"},
		"cors.allow_credentials": {Value: "true", Type: "boolean", Description: "Allow credentials in CORS requests"},
		"cors.max_age":           {Value: "43200", Type: "number", Description: "CORS preflight cache duration in seconds (default: 43200 = 12 hours)"},

		// Scheduler settings
		"scheduler.enabled":                     {Value: "true", Type: "boolean", Description: "Enable scheduled task system"},
		"scheduler.cleanup_sessions_interval":   {Value: "3600", Type: "number", Description: "Session cleanup interval in seconds (default: 3600 = 1 hour)"},
		"scheduler.cleanup_audit_logs_days":     {Value: "90", Type: "number", Description: "Audit log retention in days"},
		"scheduler.weather_alerts_interval":     {Value: "900", Type: "number", Description: "Weather alerts check interval in seconds (default: 900 = 15 minutes)"},
		"scheduler.notification_queue_interval": {Value: "120", Type: "number", Description: "Notification queue processing interval in seconds (default: 120 = 2 minutes)"},
		"scheduler.cleanup_notifications_days":  {Value: "30", Type: "number", Description: "Notification retention in days"},
		"scheduler.geoip_update_day":            {Value: "sunday", Type: "string", Description: "Day of week for GeoIP database updates"},
		"scheduler.geoip_update_time":           {Value: "03:00", Type: "string", Description: "Time for GeoIP database updates (HH:MM format)"},

		// Tor hidden service settings (TEMPLATE.md PART 32 - NON-NEGOTIABLE)
		"tor.enabled":       {Value: "true", Type: "boolean", Description: "Enable Tor hidden service (.onion address)"},
		"tor.onion_address": {Value: "", Type: "string", Description: "Tor hidden service .onion address (auto-generated)"},
		"tor.socks_port":    {Value: "9050", Type: "number", Description: "Tor SOCKS5 proxy port"},
		"tor.control_port":  {Value: "9051", Type: "number", Description: "Tor control port"},
		"tor.data_dir":      {Value: "", Type: "string", Description: "Tor data directory (auto-set to {data_dir}/tor)"},

		// Historical weather settings
		"history.enabled":       {Value: "true", Type: "boolean", Description: "Enable historical weather data feature"},
		"history.default_years": {Value: "10", Type: "number", Description: "Default number of years to display in historical view (5-50)"},
		"history.min_years":     {Value: "5", Type: "number", Description: "Minimum number of years allowed for historical queries"},
		"history.max_years":     {Value: "50", Type: "number", Description: "Maximum number of years allowed for historical queries"},
	}

	for key, setting := range defaults {
		// Only set if doesn't exist
		_, err := m.Get(key)
		if err == sql.ErrNoRows {
			if err := m.SetWithDescription(key, setting.Value, setting.Type, setting.Description); err != nil {
				return fmt.Errorf("failed to set default %s: %w", key, err)
			}
		}
	}

	return nil
}
