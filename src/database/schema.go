package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 2

// Schema contains all table creation SQL
var Schema = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Users table
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE NOT NULL,
	phone TEXT UNIQUE,
	display_name TEXT,
	password_hash TEXT NOT NULL,
	role TEXT DEFAULT 'user' CHECK(role IN ('admin', 'user')),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);

-- Tokens table (TEMPLATE.md PART 11: supports admin, user, org tokens)
CREATE TABLE IF NOT EXISTS tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_type TEXT NOT NULL,       -- 'admin', 'user', 'org'
	owner_id INTEGER NOT NULL,      -- admin.id, user.id, or org.id
	
	-- Token identification
	name TEXT NOT NULL,             -- User-provided label: "default", "ci-cd"
	token_hash TEXT NOT NULL,       -- SHA-256 hash of full token
	token_prefix TEXT NOT NULL,     -- First 8 chars: "adm_a1b2"
	
	-- Token properties
	scope TEXT NOT NULL DEFAULT 'global',  -- 'global', 'read-write', 'read'
	expires_at DATETIME,            -- NULL = never expires
	
	-- Tracking
	last_used_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	
	UNIQUE(owner_type, owner_id, name)  -- One token per name per owner
);

CREATE INDEX IF NOT EXISTS idx_tokens_hash ON tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_tokens_owner ON tokens(owner_type, owner_id);

-- Saved Locations table
CREATE TABLE IF NOT EXISTS saved_locations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	timezone TEXT,
	alerts_enabled BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_locations_user ON saved_locations(user_id);

-- Weather Alerts table
CREATE TABLE IF NOT EXISTS weather_alerts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	location_id INTEGER NOT NULL,
	alert_type TEXT NOT NULL,
	severity TEXT NOT NULL CHECK(severity IN ('info', 'warning', 'severe', 'critical')),
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	source TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	FOREIGN KEY (location_id) REFERENCES saved_locations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_alerts_location ON weather_alerts(location_id);
CREATE INDEX IF NOT EXISTS idx_alerts_expires ON weather_alerts(expires_at);

-- Server Settings table
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT,
	type TEXT DEFAULT 'string',
	description TEXT,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Scheduled Tasks table
CREATE TABLE IF NOT EXISTS scheduled_tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	schedule TEXT NOT NULL,
	task_type TEXT NOT NULL,
	enabled BOOLEAN DEFAULT 1,
	last_run DATETIME,
	next_run DATETIME,
	last_result TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_enabled ON scheduled_tasks(enabled);
CREATE INDEX IF NOT EXISTS idx_tasks_next_run ON scheduled_tasks(next_run);

-- Audit Log table (optional, disabled by default)
CREATE TABLE IF NOT EXISTS audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER,
	action TEXT NOT NULL,
	resource TEXT,
	details TEXT,
	ip_address TEXT,
	user_agent TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);

-- Notifications table
CREATE TABLE IF NOT EXISTS notifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	type TEXT NOT NULL,
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	link TEXT,
	read BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notif_user ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notif_read ON notifications(read);
CREATE INDEX IF NOT EXISTS idx_notif_created ON notifications(created_at);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	data TEXT,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- Rate Limiting table
CREATE TABLE IF NOT EXISTS rate_limits (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	identifier TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	count INTEGER DEFAULT 1,
	window_start DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(identifier, endpoint, window_start)
);

CREATE INDEX IF NOT EXISTS idx_ratelimit_identifier ON rate_limits(identifier, endpoint);
CREATE INDEX IF NOT EXISTS idx_ratelimit_window ON rate_limits(window_start);

-- Notification Channels table (30+ channel configurations)
CREATE TABLE IF NOT EXISTS notification_channels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_type TEXT UNIQUE NOT NULL,
	channel_name TEXT NOT NULL,
	enabled BOOLEAN DEFAULT 0,
	state TEXT DEFAULT 'disabled' CHECK(state IN ('disabled', 'enabled', 'failed', 'testing')),
	config TEXT,
	last_test_at DATETIME,
	last_test_result TEXT,
	last_error TEXT,
	last_success_at DATETIME,
	failure_count INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_channels_type ON notification_channels(channel_type);
CREATE INDEX IF NOT EXISTS idx_channels_enabled ON notification_channels(enabled);
CREATE INDEX IF NOT EXISTS idx_channels_state ON notification_channels(state);

-- Notification Templates table
CREATE TABLE IF NOT EXISTS notification_templates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_type TEXT NOT NULL,
	template_name TEXT NOT NULL,
	template_type TEXT NOT NULL,
	subject_template TEXT,
	body_template TEXT NOT NULL,
	variables TEXT,
	is_default BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_type) REFERENCES notification_channels(channel_type) ON DELETE CASCADE,
	UNIQUE(channel_type, template_name)
);

CREATE INDEX IF NOT EXISTS idx_templates_channel ON notification_templates(channel_type);
CREATE INDEX IF NOT EXISTS idx_templates_name ON notification_templates(template_name);
CREATE INDEX IF NOT EXISTS idx_templates_default ON notification_templates(is_default);

-- Notification Queue table
CREATE TABLE IF NOT EXISTS notification_queue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER,
	channel_type TEXT NOT NULL,
	template_id INTEGER,
	priority INTEGER DEFAULT 5,
	state TEXT DEFAULT 'created' CHECK(state IN ('created', 'queued', 'sending', 'delivered', 'failed', 'dead_letter')),
	subject TEXT,
	body TEXT NOT NULL,
	variables TEXT,
	retry_count INTEGER DEFAULT 0,
	max_retries INTEGER DEFAULT 3,
	next_retry_at DATETIME,
	delivered_at DATETIME,
	failed_at DATETIME,
	error_message TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (channel_type) REFERENCES notification_channels(channel_type) ON DELETE CASCADE,
	FOREIGN KEY (template_id) REFERENCES notification_templates(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_queue_user ON notification_queue(user_id);
CREATE INDEX IF NOT EXISTS idx_queue_channel ON notification_queue(channel_type);
CREATE INDEX IF NOT EXISTS idx_queue_state ON notification_queue(state);
CREATE INDEX IF NOT EXISTS idx_queue_priority ON notification_queue(priority);
CREATE INDEX IF NOT EXISTS idx_queue_retry ON notification_queue(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_queue_created ON notification_queue(created_at);

-- Notification History table (audit trail)
CREATE TABLE IF NOT EXISTS notification_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	queue_id INTEGER,
	user_id INTEGER,
	channel_type TEXT NOT NULL,
	status TEXT NOT NULL,
	subject TEXT,
	body TEXT,
	sent_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	delivered_at DATETIME,
	error_message TEXT,
	metadata TEXT,
	FOREIGN KEY (queue_id) REFERENCES notification_queue(id) ON DELETE SET NULL,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_history_queue ON notification_history(queue_id);
CREATE INDEX IF NOT EXISTS idx_history_user ON notification_history(user_id);
CREATE INDEX IF NOT EXISTS idx_history_channel ON notification_history(channel_type);
CREATE INDEX IF NOT EXISTS idx_history_status ON notification_history(status);
CREATE INDEX IF NOT EXISTS idx_history_sent ON notification_history(sent_at);

-- User Notification Preferences table
CREATE TABLE IF NOT EXISTS user_notification_preferences (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	channel_type TEXT NOT NULL,
	enabled BOOLEAN DEFAULT 1,
	priority INTEGER DEFAULT 5,
	quiet_hours_start TIME,
	quiet_hours_end TIME,
	config TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_type) REFERENCES notification_channels(channel_type) ON DELETE CASCADE,
	UNIQUE(user_id, channel_type)
);

CREATE INDEX IF NOT EXISTS idx_user_prefs_user ON user_notification_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_user_prefs_channel ON user_notification_preferences(channel_type);
CREATE INDEX IF NOT EXISTS idx_user_prefs_enabled ON user_notification_preferences(enabled);

-- Notification Subscriptions table
CREATE TABLE IF NOT EXISTS notification_subscriptions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	subscription_type TEXT NOT NULL,
	subscription_category TEXT NOT NULL,
	enabled BOOLEAN DEFAULT 1,
	config TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(user_id, subscription_type, subscription_category)
);

CREATE INDEX IF NOT EXISTS idx_subs_user ON notification_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subs_type ON notification_subscriptions(subscription_type);
CREATE INDEX IF NOT EXISTS idx_subs_category ON notification_subscriptions(subscription_category);
CREATE INDEX IF NOT EXISTS idx_subs_enabled ON notification_subscriptions(enabled);

-- Weather alert history table
CREATE TABLE IF NOT EXISTS weather_alert_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	location_id INTEGER NOT NULL,
	alert_type TEXT NOT NULL,
	sent_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (location_id) REFERENCES saved_locations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_alert_history_user ON weather_alert_history(user_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_location ON weather_alert_history(location_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_sent ON weather_alert_history(sent_at);

-- Notification Metrics table (for custom metrics and analytics)
CREATE TABLE IF NOT EXISTS notification_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	metric_type TEXT NOT NULL,
	channel_type TEXT,
	value REAL NOT NULL,
	recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_metrics_type ON notification_metrics(metric_type);
CREATE INDEX IF NOT EXISTS idx_metrics_channel ON notification_metrics(channel_type);
CREATE INDEX IF NOT EXISTS idx_metrics_recorded ON notification_metrics(recorded_at);
`

// DefaultSettings are inserted on first setup
var DefaultSettings = map[string]string{
	// Server settings
	// Will be set to actual port on first run
	"server.port":    "random",
	"server.address": "0.0.0.0",
	"server.theme":   "dark",

	// Auth settings
	// 24 hours
	"auth.session_timeout":            "86400",
	"auth.require_email_verification": "false",

	// Rate limiting
	"rate_limit.anonymous": "120",
	// 1 hour
	"rate_limit.window":    "3600",

	// Audit
	"audit.enabled": "false",

	// Legacy notification settings (deprecated, use notification_channels table)
	"notifications.email":   "false",
	"notifications.webhook": "false",
	"notifications.push":    "false",

	// Notification system settings
	"notifications.enabled":            "true",
	"notifications.retry_max":          "3",
	// linear or exponential
	"notifications.retry_backoff":      "exponential",
	"notifications.queue_workers":      "5",
	"notifications.batch_size":         "100",
	"notifications.rate_limit_per_min": "60",

	// SMTP settings (environment variable hints, web UI takes precedence)
	// SMTP_HOST env var
	"smtp.host":           "",
	// SMTP_PORT env var
	"smtp.port":           "587",
	// SMTP_USERNAME env var
	"smtp.username":       "",
	// SMTP_PASSWORD env var (encrypted in DB)
	"smtp.password":       "",
	// SMTP_FROM_ADDRESS env var
	"smtp.from_address":   "",
	"smtp.from_name":      "Weather",
	"smtp.use_tls":        "true",
	// Auto-enable on successful test
	"smtp.auto_enable":    "true",
	// Test email address
	"smtp.test_recipient": "",

	// Weather settings
	// 10 minutes
	"weather.refresh_interval": "600",
	"alerts.enabled":           "true",
	// 5 minutes
	"alerts.check_interval":    "300",

	// Backup settings
	"backup.enabled":  "true",
	// Daily
	"backup.interval": "86400",

	// Logging
	"log.format": "apache",
	"log.level":  "info",
}

// DB represents the database connection
type DB struct {
	*sql.DB
}

// runMigrations applies database migrations from current to target version
func runMigrations(db *sql.DB, fromVersion, toVersion int) error {
	log.Printf("Running database migrations from version %d to %d", fromVersion, toVersion)

	for v := fromVersion + 1; v <= toVersion; v++ {
		log.Printf("Applying migration to version %d", v)

		switch v {
		case 2:
			// Migration from v1 to v2: Add username and phone fields
			if err := migrateToV2(db); err != nil {
				return fmt.Errorf("migration to v2 failed: %w", err)
			}

		default:
			return fmt.Errorf("unknown migration version: %d", v)
		}

		// Update schema version
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", v); err != nil {
			return fmt.Errorf("failed to update schema version to %d: %w", v, err)
		}

		log.Printf("Successfully migrated to version %d", v)
	}

	return nil
}

// migrateToV2 adds username and phone fields to users table
func migrateToV2(db *sql.DB) error {
	// Add username column (will be populated with email prefix initially)
	if _, err := db.Exec("ALTER TABLE users ADD COLUMN username TEXT"); err != nil {
		return fmt.Errorf("failed to add username column: %w", err)
	}

	// Add phone column (nullable)
	if _, err := db.Exec("ALTER TABLE users ADD COLUMN phone TEXT"); err != nil {
		return fmt.Errorf("failed to add phone column: %w", err)
	}

	// Populate username from email (take part before @, add random suffix if needed for uniqueness)
	rows, err := db.Query("SELECT id, email FROM users")
	if err != nil {
		return fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return fmt.Errorf("failed to scan user: %w", err)
		}

		// Extract username from email (part before @)
		username := email
		if atIndex := strings.Index(email, "@"); atIndex > 0 {
			username = email[:atIndex]
		}

		// Make username unique by appending id if necessary
		// Clean username: only alphanumeric and underscore
		username = strings.ToLower(username)
		username = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return '_'
		}, username)

		// Check if username exists
		var count int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
		if count > 0 {
			// Append user ID to make it unique
			username = fmt.Sprintf("%s_%d", username, id)
		}

		// Update username
		if _, err := db.Exec("UPDATE users SET username = ? WHERE id = ?", username, id); err != nil {
			return fmt.Errorf("failed to update username for user %d: %w", id, err)
		}
	}

	// Now make username column NOT NULL and UNIQUE
	// SQLite doesn't support ALTER COLUMN, so we need to recreate the table
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create new users table with constraints
	_, err = tx.Exec(`
		CREATE TABLE users_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE NOT NULL,
			phone TEXT UNIQUE,
			display_name TEXT,
			password_hash TEXT NOT NULL,
			role TEXT DEFAULT 'user' CHECK(role IN ('admin', 'user')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create new users table: %w", err)
	}

	// Copy data
	_, err = tx.Exec(`
		INSERT INTO users_new (id, username, email, phone, display_name, password_hash, role, created_at, updated_at)
		SELECT id, username, email, phone,
			CASE WHEN EXISTS(SELECT 1 FROM pragma_table_info('users') WHERE name='display_name')
				THEN display_name
				ELSE NULL
			END,
			password_hash, role, created_at, updated_at FROM users
	`)
	if err != nil {
		return fmt.Errorf("failed to copy user data: %w", err)
	}

	// Drop old table
	if _, err = tx.Exec("DROP TABLE users"); err != nil {
		return fmt.Errorf("failed to drop old users table: %w", err)
	}

	// Rename new table
	if _, err = tx.Exec("ALTER TABLE users_new RENAME TO users"); err != nil {
		return fmt.Errorf("failed to rename users table: %w", err)
	}

	// Recreate indexes
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)")
	if err != nil {
		return fmt.Errorf("failed to create username index: %w", err)
	}

	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)")
	if err != nil {
		return fmt.Errorf("failed to create email index: %w", err)
	}

	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone)")
	if err != nil {
		return fmt.Errorf("failed to create phone index: %w", err)
	}

	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_users_role ON users(role)")
	if err != nil {
		return fmt.Errorf("failed to create role index: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// InitDB initializes the database and creates tables
func InitDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create schema
	if _, err := db.Exec(Schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Check schema version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to check schema version: %w", err)
	}

	// Insert schema version if new database
	if currentVersion == 0 {
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", SchemaVersion); err != nil {
			return nil, fmt.Errorf("failed to insert schema version: %w", err)
		}

		// Insert default settings
		for key, value := range DefaultSettings {
			_, err := db.Exec(`
				INSERT INTO settings (key, value) VALUES (?, ?)
				ON CONFLICT(key) DO NOTHING
			`, key, value)
			if err != nil {
				return nil, fmt.Errorf("failed to insert default setting %s: %w", key, err)
			}
		}
	} else if currentVersion < SchemaVersion {
		// Run migrations
		if err := runMigrations(db, currentVersion, SchemaVersion); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	return &DB{db}, nil
}

// InitDBFromConnectionString initializes database from a connection string
// Supports: sqlite:///path/to/db, postgres://user:pass@host/db, mysql://user:pass@host/db
func InitDBFromConnectionString(connString string) (*DB, error) {
	config, err := ParseConnectionString(connString)
	if err != nil {
		return nil, err
	}
	return InitDBWithConfig(config)
}

// IsFirstRun checks if this is the first run (no server admins exist)
// Per AI.md: Server Admins are in server_admin_credentials table (server DB)
func (db *DB) IsFirstRun() (bool, error) {
	// Server admin credentials are in the server database, not users database
	serverDB := GetServerDB()
	if serverDB == nil {
		return false, fmt.Errorf("server database not initialized")
	}

	var count int
	err := serverDB.QueryRow("SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// GetSetting retrieves a setting value
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value, err
}

// SetSetting updates or inserts a setting
func (db *DB) SetSetting(key, value string) error {
	_, err := db.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, key, value, time.Now(), value, time.Now())
	return err
}

// CleanupExpiredSessions removes expired sessions
func (db *DB) CleanupExpiredSessions() error {
	_, err := db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

// CleanupExpiredAlerts removes expired alerts
func (db *DB) CleanupExpiredAlerts() error {
	_, err := db.Exec("DELETE FROM weather_alerts WHERE expires_at IS NOT NULL AND expires_at < ?", time.Now())
	return err
}

// CleanupOldAuditLogs removes audit logs older than retention period
func (db *DB) CleanupOldAuditLogs(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	_, err := db.Exec("DELETE FROM audit_log WHERE created_at < ?", cutoff)
	return err
}

// CleanupRateLimits removes old rate limit entries
func (db *DB) CleanupRateLimits() error {
	// Remove entries older than 2 hours
	cutoff := time.Now().Add(-2 * time.Hour)
	_, err := db.Exec("DELETE FROM rate_limits WHERE window_start < ?", cutoff)
	return err
}

// HealthCheck returns database health status with latency
func (db *DB) HealthCheck() (status string, latencyMs int64, err error) {
	start := time.Now()

	// Simple query to check connection
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)

	latencyMs = time.Since(start).Milliseconds()

	if err != nil {
		return "disconnected", latencyMs, err
	}

	return "connected", latencyMs, nil
}

// GetSessionCount returns active session count
func (db *DB) GetSessionCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE expires_at > ?", time.Now()).Scan(&count)
	return count, err
}
