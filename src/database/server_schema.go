package database

// ServerSchema contains all server infrastructure tables per TEMPLATE.md PART 31
const ServerSchema = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Admin Credentials table (admins are NOT in users table)
-- AI.md PART 11: API tokens stored as SHA-256 hash, never plaintext
CREATE TABLE IF NOT EXISTS server_admin_credentials (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	api_token_hash TEXT UNIQUE,
	api_token_prefix TEXT,
	is_super_admin BOOLEAN DEFAULT 0,
	is_active BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_login_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_admin_username ON server_admin_credentials(username);
CREATE INDEX IF NOT EXISTS idx_admin_email ON server_admin_credentials(email);
CREATE INDEX IF NOT EXISTS idx_admin_token_hash ON server_admin_credentials(api_token_hash);

-- Admin Sessions table (admin panel sessions only)
CREATE TABLE IF NOT EXISTS server_admin_sessions (
	id TEXT PRIMARY KEY,
	admin_id INTEGER NOT NULL,
	data TEXT,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	ip_address TEXT,
	user_agent TEXT,
	FOREIGN KEY (admin_id) REFERENCES server_admin_credentials(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_admin ON server_admin_sessions(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON server_admin_sessions(expires_at);

-- Server Configuration table (all settings as key-value pairs)
CREATE TABLE IF NOT EXISTS server_config (
	key TEXT PRIMARY KEY,
	value TEXT,
	type TEXT DEFAULT 'string',
	description TEXT,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_config_updated ON server_config(updated_at);

-- Cluster State table (for future cluster mode support)
CREATE TABLE IF NOT EXISTS server_cluster_state (
	key TEXT PRIMARY KEY,
	value TEXT,
	node_id TEXT,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cluster_node ON server_cluster_state(node_id);

-- Scheduler State table (scheduled tasks)
CREATE TABLE IF NOT EXISTS server_scheduler_state (
	task_id TEXT PRIMARY KEY,
	task_name TEXT NOT NULL,
	schedule TEXT NOT NULL,
	last_run DATETIME,
	last_status TEXT,
	last_error TEXT,
	next_run DATETIME,
	run_count INTEGER DEFAULT 0,
	fail_count INTEGER DEFAULT 0,
	enabled BOOLEAN DEFAULT 1,
	locked_by TEXT,
	locked_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_scheduler_enabled ON server_scheduler_state(enabled);
CREATE INDEX IF NOT EXISTS idx_scheduler_next_run ON server_scheduler_state(next_run);
CREATE INDEX IF NOT EXISTS idx_scheduler_locked ON server_scheduler_state(locked_by);

-- Cluster Nodes table (for future cluster mode)
CREATE TABLE IF NOT EXISTS server_nodes (
	node_id TEXT PRIMARY KEY,
	hostname TEXT NOT NULL,
	ip_address TEXT,
	port INTEGER,
	status TEXT DEFAULT 'active',
	last_heartbeat DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	metadata TEXT
);

CREATE INDEX IF NOT EXISTS idx_nodes_status ON server_nodes(status);
CREATE INDEX IF NOT EXISTS idx_nodes_heartbeat ON server_nodes(last_heartbeat);

-- Node Join Tokens table (for cluster expansion)
CREATE TABLE IF NOT EXISTS server_join_tokens (
	token TEXT PRIMARY KEY,
	created_by TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	used_by TEXT,
	used_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_join_tokens_expires ON server_join_tokens(expires_at);

-- Admin Invite Tokens table (TEMPLATE.md Part 31: 15-minute expiry)
CREATE TABLE IF NOT EXISTS server_admin_invites (
	token TEXT PRIMARY KEY,
	invited_email TEXT NOT NULL,
	invited_by INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	used_by INTEGER,
	used_at DATETIME,
	FOREIGN KEY (invited_by) REFERENCES server_admin_credentials(id) ON DELETE CASCADE,
	FOREIGN KEY (used_by) REFERENCES server_admin_credentials(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_admin_invites_email ON server_admin_invites(invited_email);
CREATE INDEX IF NOT EXISTS idx_admin_invites_expires ON server_admin_invites(expires_at);
CREATE INDEX IF NOT EXISTS idx_admin_invites_invited_by ON server_admin_invites(invited_by);

-- Audit Log table (system-wide audit trail)
CREATE TABLE IF NOT EXISTS server_audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ulid TEXT UNIQUE NOT NULL,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	actor_type TEXT,
	actor_id TEXT,
	action TEXT NOT NULL,
	resource_type TEXT,
	resource_id TEXT,
	details TEXT,
	ip_address TEXT,
	user_agent TEXT,
	status TEXT,
	error TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_ulid ON server_audit_log(ulid);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON server_audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON server_audit_log(actor_type, actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_resource ON server_audit_log(resource_type, resource_id);

-- Rate Limiting table (global rate limits)
CREATE TABLE IF NOT EXISTS server_rate_limits (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	identifier TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	count INTEGER DEFAULT 1,
	window_start DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(identifier, endpoint, window_start)
);

CREATE INDEX IF NOT EXISTS idx_ratelimit_identifier ON server_rate_limits(identifier, endpoint);
CREATE INDEX IF NOT EXISTS idx_ratelimit_window ON server_rate_limits(window_start);

-- Notification Channels table (30+ channel configurations)
CREATE TABLE IF NOT EXISTS server_notification_channels (
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

CREATE INDEX IF NOT EXISTS idx_channels_type ON server_notification_channels(channel_type);
CREATE INDEX IF NOT EXISTS idx_channels_enabled ON server_notification_channels(enabled);
CREATE INDEX IF NOT EXISTS idx_channels_state ON server_notification_channels(state);

-- Notification Templates table
CREATE TABLE IF NOT EXISTS server_notification_templates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_type TEXT NOT NULL,
	template_name TEXT NOT NULL,
	template_type TEXT NOT NULL,
	subject TEXT,
	body TEXT NOT NULL,
	variables TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(channel_type, template_name, template_type)
);

CREATE INDEX IF NOT EXISTS idx_templates_channel ON server_notification_templates(channel_type);
CREATE INDEX IF NOT EXISTS idx_templates_name ON server_notification_templates(template_name);

-- Admin Notifications table (TEMPLATE.md Part 25: WebUI notifications for admins)
CREATE TABLE IF NOT EXISTS server_admin_notifications (
	id TEXT PRIMARY KEY,
	admin_id INTEGER NOT NULL,
	type TEXT NOT NULL CHECK(type IN ('success', 'info', 'warning', 'error', 'security')),
	display TEXT NOT NULL CHECK(display IN ('toast', 'banner', 'center')) DEFAULT 'toast',
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	action_json TEXT,
	read BOOLEAN DEFAULT 0,
	dismissed BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	FOREIGN KEY (admin_id) REFERENCES server_admin_credentials(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_admin_notif_admin ON server_admin_notifications(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_notif_read ON server_admin_notifications(read);
CREATE INDEX IF NOT EXISTS idx_admin_notif_created ON server_admin_notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_notif_expires ON server_admin_notifications(expires_at);

-- Admin Notification Preferences table (TEMPLATE.md Part 25)
CREATE TABLE IF NOT EXISTS server_admin_notification_preferences (
	admin_id INTEGER PRIMARY KEY,
	enable_toast BOOLEAN DEFAULT 1,
	enable_banner BOOLEAN DEFAULT 1,
	enable_center BOOLEAN DEFAULT 1,
	enable_sound BOOLEAN DEFAULT 0,
	toast_duration_success INTEGER DEFAULT 5,
	toast_duration_info INTEGER DEFAULT 5,
	toast_duration_warning INTEGER DEFAULT 10,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (admin_id) REFERENCES server_admin_credentials(id) ON DELETE CASCADE
);

-- Admin Preferences table (settings per admin)
CREATE TABLE IF NOT EXISTS server_admin_preferences (
	admin_id INTEGER PRIMARY KEY,
	preferences TEXT NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (admin_id) REFERENCES server_admin_credentials(id) ON DELETE CASCADE
);

-- Admin Passkeys (WebAuthn/FIDO2 credentials) per AI.md PART 17 line 28674-28683.
-- Mirrors user_passkeys (in users.db) but lives in server.db because admins
-- are stored in server.db, never in users.db.
CREATE TABLE IF NOT EXISTS server_admin_passkeys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	admin_id INTEGER NOT NULL,
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
	FOREIGN KEY (admin_id) REFERENCES server_admin_credentials(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_admin_passkey_admin ON server_admin_passkeys(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_passkey_cred ON server_admin_passkeys(credential_id);

-- Backup History table (backup metadata)
CREATE TABLE IF NOT EXISTS server_backup_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	filename TEXT NOT NULL,
	path TEXT NOT NULL,
	size INTEGER NOT NULL,
	compressed BOOLEAN DEFAULT 0,
	checksum TEXT,
	created_by TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME,
	status TEXT DEFAULT 'completed' CHECK(status IN ('in_progress', 'completed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_backup_created ON server_backup_history(created_at);
CREATE INDEX IF NOT EXISTS idx_backup_status ON server_backup_history(status);

-- SSL Certificates table (certificate tracking and renewal)
CREATE TABLE IF NOT EXISTS server_ssl_certificates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	domain TEXT UNIQUE NOT NULL,
	cert_path TEXT NOT NULL,
	key_path TEXT NOT NULL,
	issuer TEXT,
	subject TEXT,
	serial_number TEXT,
	not_before DATETIME,
	not_after DATETIME,
	auto_renew BOOLEAN DEFAULT 1,
	last_check DATETIME,
	last_renewal DATETIME,
	renewal_status TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ssl_domain ON server_ssl_certificates(domain);
CREATE INDEX IF NOT EXISTS idx_ssl_expiry ON server_ssl_certificates(not_after);
CREATE INDEX IF NOT EXISTS idx_ssl_auto_renew ON server_ssl_certificates(auto_renew);

-- GeoIP Cache table (cache GeoIP lookups)
CREATE TABLE IF NOT EXISTS server_geoip_cache (
	ip_address TEXT PRIMARY KEY,
	country_code TEXT,
	country_name TEXT,
	city TEXT,
	region TEXT,
	asn INTEGER,
	asn_org TEXT,
	latitude REAL,
	longitude REAL,
	cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_geoip_expires ON server_geoip_cache(expires_at);

-- Blocklists table (IP and country blocklists)
CREATE TABLE IF NOT EXISTS server_blocklists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type TEXT NOT NULL CHECK(type IN ('ip', 'country', 'asn', 'cidr')),
	value TEXT NOT NULL,
	reason TEXT,
	source TEXT DEFAULT 'manual',
	added_by TEXT,
	added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	UNIQUE(type, value)
);

CREATE INDEX IF NOT EXISTS idx_blocklist_type ON server_blocklists(type);
CREATE INDEX IF NOT EXISTS idx_blocklist_value ON server_blocklists(value);
CREATE INDEX IF NOT EXISTS idx_blocklist_expires ON server_blocklists(expires_at);

-- Setup State table (first-run setup status)
CREATE TABLE IF NOT EXISTS server_setup_state (
	key TEXT PRIMARY KEY,
	value TEXT,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Metrics table (application metrics history)
CREATE TABLE IF NOT EXISTS server_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	metric_name TEXT NOT NULL,
	metric_value REAL NOT NULL,
	metric_type TEXT NOT NULL CHECK(metric_type IN ('counter', 'gauge', 'histogram')),
	labels TEXT,
	recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	node_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_metrics_name ON server_metrics(metric_name);
CREATE INDEX IF NOT EXISTS idx_metrics_recorded ON server_metrics(recorded_at);
CREATE INDEX IF NOT EXISTS idx_metrics_node ON server_metrics(node_id);

-- Reserved domain mapping table (currently unused; PART 36 is not implemented)
CREATE TABLE IF NOT EXISTS custom_domains (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	domain TEXT UNIQUE NOT NULL,
	user_id INTEGER,
	is_verified BOOLEAN DEFAULT 0,
	is_active BOOLEAN DEFAULT 0,
	ssl_enabled BOOLEAN DEFAULT 0,
	ssl_cert_path TEXT,
	ssl_key_path TEXT,
	redirect_www BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	verified_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_domains_domain ON custom_domains(domain);
CREATE INDEX IF NOT EXISTS idx_domains_user ON custom_domains(user_id);
CREATE INDEX IF NOT EXISTS idx_domains_verified ON custom_domains(is_verified);
CREATE INDEX IF NOT EXISTS idx_domains_active ON custom_domains(is_active);

-- Notification Queue table (server-level delivery queue)
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
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_nq_user ON notification_queue(user_id);
CREATE INDEX IF NOT EXISTS idx_nq_channel ON notification_queue(channel_type);
CREATE INDEX IF NOT EXISTS idx_nq_state ON notification_queue(state);
CREATE INDEX IF NOT EXISTS idx_nq_priority ON notification_queue(priority);
CREATE INDEX IF NOT EXISTS idx_nq_retry ON notification_queue(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_nq_created ON notification_queue(created_at);

-- Notification History table (audit trail)
CREATE TABLE IF NOT EXISTS notification_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	queue_id INTEGER,
	user_id INTEGER,
	channel_type TEXT NOT NULL,
	status TEXT NOT NULL,
	subject TEXT,
	body TEXT,
	delivered_at DATETIME,
	error_message TEXT,
	metadata TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_nh_queue ON notification_history(queue_id);
CREATE INDEX IF NOT EXISTS idx_nh_user ON notification_history(user_id);
CREATE INDEX IF NOT EXISTS idx_nh_status ON notification_history(status);
`

const ServerSchemaVersion = 6
