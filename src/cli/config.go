package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateServerYML generates server.yml from current configuration
// This is called at runtime to keep server.yml in sync with database settings
func GenerateServerYML(configDir string) error {
	configPath := filepath.Join(configDir, "server.yml")

	// Read current settings from database (placeholder - actual implementation
	// would query the settings table and format as YAML)

	serverYML := `# Weather Service Configuration
# This file is auto-generated from database settings
# Edit via admin panel at /admin/settings
# Last updated: AUTO_GENERATED

# Server Configuration
server:
  title: "Weather Service"
  tagline: "Production-grade weather API server"
  description: "Global weather forecasts, alerts, and tracking"
  port: 80
  listen: "::"
  domain: ""  # Set via DOMAIN env var or admin panel
  mode: production  # production or development

# Admin Configuration
admin:
  enabled: true
  session_timeout: 2592000  # 30 days in seconds

# Database Configuration
database:
  driver: sqlite  # sqlite, postgres, mariadb, mysql, mssql, mongodb
  path: "/var/lib/casapps/wthr/db/server.db"

# SSL/TLS Configuration
ssl:
  enabled: false
  cert_file: "/etc/casapps/wthr/ssl/certs/cert.pem"
  key_file: "/etc/casapps/wthr/ssl/certs/key.pem"
  acme:
    enabled: false
    email: ""
    provider: "letsencrypt"  # letsencrypt or zerossl

# Authentication Configuration
auth:
  session_timeout: 2592000  # 30 days
  require_email_verification: false
  allow_registration: true

# Rate Limiting
rate_limit:
  enabled: true
  global: 100  # requests per second
  api: 100     # requests per 15 minutes
  admin: 30    # requests per 15 minutes
  window: 900  # window in seconds (15 minutes)

# Logging Configuration
log:
  level: info  # debug, info, warn, error
  format: apache  # apache or json
  retention_days: 30
  rotate_daily: true

# Backup Configuration
backup:
  enabled: true
  interval: 21600  # 6 hours in seconds
  retention_count: 7
  path: "/mnt/Backups/casapps/wthr"

# SMTP Configuration (for email notifications)
smtp:
  enabled: false
  host: ""
  port: 587
  username: ""
  password: ""
  from_address: "noreply@example.com"
  from_name: "Weather Service"
  use_tls: true
  test_recipient: ""

# Notifications Configuration
notifications:
  enabled: true
  queue_workers: 4
  retry_max: 3
  retry_backoff: exponential  # linear or exponential

# Weather Configuration
weather:
  refresh_interval: 1800  # 30 minutes
  default_units: auto  # auto, metric, imperial
  cache_enabled: true

# Severe Weather Alerts
alerts:
  enabled: true
  check_interval: 900  # 15 minutes
  sources:
    - noaa_nws      # US National Weather Service
    - noaa_nhc      # National Hurricane Center
    - env_canada    # Environment Canada
    - uk_met        # UK Met Office
    - australia_bom # Australia Bureau of Meteorology
    - japan_jma     # Japan Meteorological Agency
    - mexico_conagua # Mexico CONAGUA

# GeoIP Configuration
geoip:
  enabled: true
  update_interval: 604800  # 7 days in seconds
  databases:
    - geolite2-city-ipv4
    - geolite2-city-ipv6
    - geo-whois-asn-country
    - asn

# Scheduler Configuration
schedule:
  enabled: true
  tasks:
    rotate_logs:
      interval: daily
      time: "00:00"
    cleanup_sessions:
      interval: 3600  # 1 hour
    cleanup_rate_limits:
      interval: 3600  # 1 hour
    cleanup_audit_logs:
      interval: daily
      retention_days: 90
    check_weather_alerts:
      interval: 900  # 15 minutes
    daily_forecast:
      interval: daily
      time: "07:00"
    process_notification_queue:
      interval: 120  # 2 minutes
    cleanup_notifications:
      interval: daily
      retention_days: 30
    system_backup:
      interval: 21600  # 6 hours
    refresh_weather_cache:
      interval: 1800  # 30 minutes
    update_geoip_database:
      interval: weekly
      day: sunday
      time: "03:00"

# CORS Configuration
cors:
  enabled: true
  allowed_origins:
    - "*"
  allowed_methods:
    - GET
    - POST
    - PUT
    - DELETE
    - OPTIONS
  allowed_headers:
    - Origin
    - Content-Type
    - Accept
    - Authorization
  expose_headers:
    - Content-Length
  allow_credentials: true
  max_age: 43200  # 12 hours

# Security Headers
security:
  headers:
    x_frame_options: DENY
    x_content_type_options: nosniff
    x_xss_protection: "1; mode=block"
    referrer_policy: strict-origin-when-cross-origin
    content_security_policy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"

# Feature Flags
features:
  graphql: true
  swagger: true
  metrics: true
  debug_endpoints: false  # NEVER enable in production!

# Metrics Configuration
metrics:
  enabled: true
  prometheus: true
  endpoint: /metrics
`

	// Write to file
	if err := os.WriteFile(configPath, []byte(serverYML), 0644); err != nil {
		return fmt.Errorf("failed to write server.yml: %w", err)
	}

	return nil
}

// SyncConfigFromDatabase reads settings from database and updates server.yml
// This is called periodically to keep server.yml in sync with database
func SyncConfigFromDatabase(configDir string, dbSettings map[string]string) error {
	// This function would:
	// 1. Read all settings from database
	// 2. Generate YAML structure
	// 3. Write to server.yml

	// For now, just regenerate with defaults
	return GenerateServerYML(configDir)
}
