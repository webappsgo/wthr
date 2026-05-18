package database

// UsersSchema contains all user-related tables per TEMPLATE.md PART 31
const UsersSchema = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- User Accounts table (regular users only, NO admins)
-- Per AI.md PART 34: Multi-user support with profile fields
CREATE TABLE IF NOT EXISTS user_accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE NOT NULL,
	notification_email TEXT,
	phone TEXT UNIQUE,
	display_name TEXT,
	password_hash TEXT NOT NULL,
	role TEXT DEFAULT 'user',
	-- Profile visibility per AI.md PART 34: public or private
	visibility TEXT DEFAULT 'public',
	-- Avatar settings per AI.md PART 34: gravatar, upload, or url
	avatar_type TEXT DEFAULT 'gravatar',
	avatar_url TEXT,
	-- Profile fields per AI.md PART 34
	bio TEXT,
	website TEXT,
	location TEXT,
	timezone TEXT,
	language TEXT DEFAULT 'en',
	email_verified BOOLEAN DEFAULT 0,
	phone_verified BOOLEAN DEFAULT 0,
	is_active BOOLEAN DEFAULT 1,
	is_banned BOOLEAN DEFAULT 0,
	ban_reason TEXT,
	two_factor_enabled BOOLEAN DEFAULT 0,
	two_factor_secret TEXT,
	recovery_keys_hash TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_login_at DATETIME,
	last_login_ip TEXT
);

CREATE INDEX IF NOT EXISTS idx_user_username ON user_accounts(username);
CREATE INDEX IF NOT EXISTS idx_user_email ON user_accounts(email);
CREATE INDEX IF NOT EXISTS idx_user_phone ON user_accounts(phone);
CREATE INDEX IF NOT EXISTS idx_user_active ON user_accounts(is_active);
CREATE INDEX IF NOT EXISTS idx_user_banned ON user_accounts(is_banned);

-- User API Tokens table (AI.md PART 11: usr_ prefix tokens, SHA-256 hashed)
CREATE TABLE IF NOT EXISTS user_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	token_hash TEXT UNIQUE NOT NULL,
	token_prefix TEXT NOT NULL,
	name TEXT,
	scopes TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	last_used_at DATETIME,
	last_used_ip TEXT,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tokens_hash ON user_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_tokens_user ON user_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_tokens_expires ON user_tokens(expires_at);

-- User Sessions table (web sessions for regular users)
CREATE TABLE IF NOT EXISTS user_sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	data TEXT,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	ip_address TEXT,
	user_agent TEXT,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON user_sessions(expires_at);

-- User Invites table (for invite-only registration)
CREATE TABLE IF NOT EXISTS user_invites (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	code TEXT UNIQUE NOT NULL,
	username TEXT,
	email TEXT,
	invited_by INTEGER,
	role TEXT NOT NULL DEFAULT 'user',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	used_by INTEGER,
	used_at DATETIME,
	max_uses INTEGER DEFAULT 1,
	use_count INTEGER DEFAULT 0,
	FOREIGN KEY (invited_by) REFERENCES user_accounts(id) ON DELETE SET NULL,
	FOREIGN KEY (used_by) REFERENCES user_accounts(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_invites_code ON user_invites(code);
CREATE INDEX IF NOT EXISTS idx_invites_email ON user_invites(email);
CREATE INDEX IF NOT EXISTS idx_invites_expires ON user_invites(expires_at);

-- Saved Locations table (user-specific weather locations)
CREATE TABLE IF NOT EXISTS user_saved_locations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	timezone TEXT,
	alerts_enabled BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_locations_user ON user_saved_locations(user_id);

-- Weather Alerts table (alerts for saved locations)
CREATE TABLE IF NOT EXISTS user_weather_alerts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	location_id INTEGER NOT NULL,
	alert_type TEXT NOT NULL,
	severity TEXT NOT NULL CHECK(severity IN ('info', 'warning', 'severe', 'critical')),
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	source TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	FOREIGN KEY (location_id) REFERENCES user_saved_locations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_alerts_location ON user_weather_alerts(location_id);
CREATE INDEX IF NOT EXISTS idx_alerts_expires ON user_weather_alerts(expires_at);

-- Weather alert deduplication history (tracks which alerts have been sent to avoid re-notifying within 6 hours)
CREATE TABLE IF NOT EXISTS user_weather_alert_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	location_id INTEGER NOT NULL,
	alert_type TEXT NOT NULL,
	sent_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (location_id) REFERENCES user_saved_locations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_alert_hist_user ON user_weather_alert_history(user_id);
CREATE INDEX IF NOT EXISTS idx_alert_hist_location ON user_weather_alert_history(location_id);
CREATE INDEX IF NOT EXISTS idx_alert_hist_sent ON user_weather_alert_history(sent_at);

-- User Notifications table (TEMPLATE.md Part 25: WebUI notifications)
CREATE TABLE IF NOT EXISTS user_notifications (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	type TEXT NOT NULL CHECK(type IN ('success', 'info', 'warning', 'error', 'security')),
	display TEXT NOT NULL CHECK(display IN ('toast', 'banner', 'center')) DEFAULT 'toast',
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	action_json TEXT,
	read BOOLEAN DEFAULT 0,
	dismissed BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notif_user ON user_notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notif_read ON user_notifications(read);
CREATE INDEX IF NOT EXISTS idx_notif_created ON user_notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notif_expires ON user_notifications(expires_at);

-- User Notification Preferences table (TEMPLATE.md Part 25)
CREATE TABLE IF NOT EXISTS user_notification_preferences (
	user_id INTEGER PRIMARY KEY,
	enable_toast BOOLEAN DEFAULT 1,
	enable_banner BOOLEAN DEFAULT 1,
	enable_center BOOLEAN DEFAULT 1,
	enable_sound BOOLEAN DEFAULT 0,
	toast_duration_success INTEGER DEFAULT 5,
	toast_duration_info INTEGER DEFAULT 5,
	toast_duration_warning INTEGER DEFAULT 10,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

-- 2FA Recovery Keys table (TEMPLATE.md Part 31: 10 one-time recovery keys)
CREATE TABLE IF NOT EXISTS recovery_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	key_hash TEXT NOT NULL,
	used_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_recovery_user ON recovery_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_recovery_used ON recovery_keys(used_at);

-- User Preferences table (settings per user)
CREATE TABLE IF NOT EXISTS user_preferences (
	user_id INTEGER PRIMARY KEY,
	theme TEXT DEFAULT 'auto',
	language TEXT DEFAULT 'en',
	timezone TEXT DEFAULT 'UTC',
	temperature_unit TEXT DEFAULT 'celsius',
	pressure_unit TEXT DEFAULT 'hPa',
	wind_speed_unit TEXT DEFAULT 'kmh',
	precipitation_unit TEXT DEFAULT 'mm',
	notifications_enabled BOOLEAN DEFAULT 1,
	email_notifications BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

-- Passkeys/WebAuthn Credentials table
CREATE TABLE IF NOT EXISTS user_passkeys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	credential_id TEXT UNIQUE NOT NULL,
	public_key TEXT NOT NULL,
	aaguid TEXT,
	sign_count INTEGER DEFAULT 0,
	name TEXT NOT NULL,
	transport TEXT NOT NULL DEFAULT '[]',
	attestation_type TEXT NOT NULL DEFAULT '',
	backup_eligible BOOLEAN NOT NULL DEFAULT 0,
	backup_state BOOLEAN NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_passkey_user ON user_passkeys(user_id);
CREATE INDEX IF NOT EXISTS idx_passkey_cred ON user_passkeys(credential_id);

-- OIDC Identity Mappings table (external identity providers)
CREATE TABLE IF NOT EXISTS user_oidc_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	provider_name TEXT NOT NULL,
	provider_user_id TEXT NOT NULL,
	issuer TEXT NOT NULL,
	email TEXT,
	name TEXT,
	claims TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_login_at DATETIME,
	UNIQUE(provider_name, provider_user_id),
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_oidc_user ON user_oidc_mappings(user_id);
CREATE INDEX IF NOT EXISTS idx_oidc_provider ON user_oidc_mappings(provider_name, provider_user_id);

-- LDAP Identity Mappings table
CREATE TABLE IF NOT EXISTS user_ldap_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	ldap_server TEXT NOT NULL,
	ldap_dn TEXT NOT NULL,
	ldap_uid TEXT NOT NULL,
	groups TEXT,
	attributes TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_sync_at DATETIME,
	UNIQUE(ldap_server, ldap_dn),
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ldap_user ON user_ldap_mappings(user_id);
CREATE INDEX IF NOT EXISTS idx_ldap_server ON user_ldap_mappings(ldap_server, ldap_dn);

-- Email Verification Tokens table
CREATE TABLE IF NOT EXISTS user_email_verifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	email TEXT NOT NULL,
	token TEXT UNIQUE NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	used_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_verify_token ON user_email_verifications(token);
CREATE INDEX IF NOT EXISTS idx_email_verify_user ON user_email_verifications(user_id);
CREATE INDEX IF NOT EXISTS idx_email_verify_expires ON user_email_verifications(expires_at);

-- Password Reset Tokens table
CREATE TABLE IF NOT EXISTS user_password_resets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	token TEXT UNIQUE NOT NULL,
	ip_address TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	used_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_password_reset_token ON user_password_resets(token);
CREATE INDEX IF NOT EXISTS idx_password_reset_user ON user_password_resets(user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_expires ON user_password_resets(expires_at);

-- User Activity Log table (login history, security events)
CREATE TABLE IF NOT EXISTS user_activity_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	activity_type TEXT NOT NULL,
	description TEXT,
	ip_address TEXT,
	user_agent TEXT,
	location TEXT,
	status TEXT DEFAULT 'success',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_activity_user ON user_activity_log(user_id);
CREATE INDEX IF NOT EXISTS idx_activity_type ON user_activity_log(activity_type);
CREATE INDEX IF NOT EXISTS idx_activity_created ON user_activity_log(created_at);
`

const UsersSchemaVersion = 5
