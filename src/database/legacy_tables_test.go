package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// legacyServerSchemaSQL is a trimmed copy of the pre-PART-12 server.db layout:
// admin accounts and everything owned by them lived here under server_ names.
const legacyServerSchemaSQL = `
CREATE TABLE server_admin_credentials (
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
CREATE TABLE server_admin_sessions (
	id TEXT PRIMARY KEY,
	token_hash TEXT NOT NULL,
	admin_id INTEGER NOT NULL,
	data TEXT,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	ip_address TEXT,
	user_agent TEXT,
	FOREIGN KEY (admin_id) REFERENCES server_admin_credentials(id) ON DELETE CASCADE
);
CREATE TABLE server_admin_passkeys (
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
CREATE TABLE server_audit_log (
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
	source TEXT DEFAULT 'api',
	status TEXT,
	error TEXT
);
INSERT INTO server_admin_credentials (id, username, email, password_hash, api_token_hash, api_token_prefix, is_super_admin)
	VALUES (7, 'legacyroot', 'root@example.com', 'argon-hash', 'token-hash', 'adm_', 1);
INSERT INTO server_admin_sessions (id, token_hash, admin_id, expires_at)
	VALUES ('sess-1', 'hash-1', 7, '2099-01-01 00:00:00');
INSERT INTO server_admin_passkeys (admin_id, credential_id, public_key, name)
	VALUES (7, 'cred-admin', 'pk', 'Admin Key');
INSERT INTO server_audit_log (ulid, action) VALUES ('01ULID', 'login');
`

// legacyUsersSchemaSQL is a trimmed copy of the pre-PART-12 users.db layout.
const legacyUsersSchemaSQL = `
CREATE TABLE user_accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE NOT NULL,
	notification_email TEXT,
	phone TEXT UNIQUE,
	display_name TEXT,
	password_hash TEXT NOT NULL,
	role TEXT DEFAULT 'user',
	visibility TEXT DEFAULT 'public',
	avatar_type TEXT DEFAULT 'gravatar',
	avatar_url TEXT,
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
CREATE TABLE user_tokens (
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
CREATE TABLE user_passkeys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	credential_id TEXT UNIQUE NOT NULL,
	public_key TEXT NOT NULL,
	name TEXT NOT NULL,
	FOREIGN KEY (user_id) REFERENCES user_accounts(id) ON DELETE CASCADE
);
INSERT INTO user_accounts (id, username, email, password_hash) VALUES (3, 'legacyuser', 'user@example.com', 'user-hash');
INSERT INTO user_tokens (user_id, token_hash, token_prefix) VALUES (3, 'usr-hash', 'usr_');
INSERT INTO user_passkeys (user_id, credential_id, public_key, name) VALUES (3, 'cred-user', 'pk', 'User Key');
`

func countRows(t *testing.T, ddb *DualDB, db string, query string, args ...interface{}) int {
	t.Helper()
	handle := ddb.Server
	if db == "users" {
		handle = ddb.Users
	}
	var count int
	if err := handle.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return count
}

// seedLegacyDatabases writes pre-PART-12 server.db and users.db files into a
// fresh data directory and returns the directory.
func seedLegacyDatabases(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	dbDir := filepath.Join(dataDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}

	for name, ddl := range map[string]string{"server.db": legacyServerSchemaSQL, "users.db": legacyUsersSchemaSQL} {
		raw, err := sql.Open("sqlite", filepath.Join(dbDir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		if _, err := raw.Exec(ddl); err != nil {
			_ = raw.Close()
			t.Fatalf("seed %s: %v", name, err)
		}
		_ = raw.Close()
	}
	return dataDir
}

// TestInitDualDB_UpgradesLegacyLayout proves an install created before AI.md
// PART 12 naming keeps every admin, session, passkey, audit row, user and token
// after startup, under the canonical table names and database placement.
func TestInitDualDB_UpgradesLegacyLayout(t *testing.T) {
	dataDir := seedLegacyDatabases(t)

	ddb, err := InitDualDB(dataDir)
	if err != nil {
		t.Fatalf("InitDualDB on a legacy layout: %v", err)
	}
	t.Cleanup(func() { _ = ddb.Close() })

	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM admins WHERE id = 7 AND username = 'legacyroot' AND password_hash = 'argon-hash'"); got != 1 {
		t.Errorf("admin legacyroot rows in users.db admins = %d, want 1 (same id, hashes intact)", got)
	}
	if got := countRows(t, ddb, "server", "SELECT COUNT(*) FROM admin_sessions WHERE admin_id = 7 AND token_hash = 'hash-1'"); got != 1 {
		t.Errorf("legacy admin session rows in server.db admin_sessions = %d, want 1", got)
	}
	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM passkeys WHERE user_type = 'admin' AND user_id = 7 AND credential_id = 'cred-admin'"); got != 1 {
		t.Errorf("admin passkey rows in passkeys = %d, want 1", got)
	}
	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM passkeys WHERE user_type = 'user' AND user_id = 3 AND credential_id = 'cred-user'"); got != 1 {
		t.Errorf("user passkey rows in passkeys = %d, want 1", got)
	}
	if got := countRows(t, ddb, "server", "SELECT COUNT(*) FROM audit_log WHERE ulid = '01ULID'"); got != 1 {
		t.Errorf("audit_log rows renamed from server_audit_log = %d, want 1", got)
	}
	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM users WHERE id = 3 AND username = 'legacyuser'"); got != 1 {
		t.Errorf("users rows renamed from user_accounts = %d, want 1", got)
	}
	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM tokens WHERE owner_type = 'user' AND owner_id = 3 AND token_hash = 'usr-hash'"); got != 1 {
		t.Errorf("user token rows copied from user_tokens = %d, want 1", got)
	}
	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM tokens WHERE owner_type = 'admin' AND owner_id = 7 AND name = 'default' AND token_hash = 'token-hash' AND token_prefix = 'adm_'"); got != 1 {
		t.Errorf("admin default token rows copied from the legacy credentials = %d, want 1", got)
	}
	if got := countRows(t, ddb, "users", "SELECT COUNT(*) FROM user_tokens WHERE token_hash = 'usr-hash'"); got != 0 {
		t.Errorf("legacy user_tokens still holds a usable hash, want it marked migrated")
	}

	// The legacy copy of the admin credentials must no longer hold a usable secret.
	if got := countRows(t, ddb, "server", "SELECT COUNT(*) FROM server_admin_credentials WHERE password_hash != '' OR api_token_hash IS NOT NULL"); got != 0 {
		t.Errorf("legacy server_admin_credentials still holds %d secret-bearing rows, want 0", got)
	}

	// A session of an admin created after the move must be insertable: the legacy
	// foreign key to the legacy credentials table must not have been carried over.
	if _, err := ddb.Server.Exec(`INSERT INTO admin_sessions (id, token_hash, admin_id, expires_at) VALUES ('sess-new', 'hash-new', 99, '2099-01-01 00:00:00')`); err != nil {
		t.Errorf("insert admin session for a post-upgrade admin: %v", err)
	}
}

// TestInitDualDB_LegacyUpgradeIsIdempotent proves a second start neither
// duplicates rows nor resurrects the blanked legacy credentials.
func TestInitDualDB_LegacyUpgradeIsIdempotent(t *testing.T) {
	dataDir := seedLegacyDatabases(t)

	first, err := InitDualDB(dataDir)
	if err != nil {
		t.Fatalf("first InitDualDB: %v", err)
	}
	if _, err := first.Users.Exec("DELETE FROM admins"); err != nil {
		t.Fatalf("delete admins: %v", err)
	}
	_ = first.Close()

	second, err := InitDualDB(dataDir)
	if err != nil {
		t.Fatalf("second InitDualDB: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if got := countRows(t, second, "users", "SELECT COUNT(*) FROM admins"); got != 0 {
		t.Errorf("admins after a restart with the legacy credentials blanked = %d, want 0 (no resurrection)", got)
	}
	if got := countRows(t, second, "users", "SELECT COUNT(*) FROM passkeys"); got != 2 {
		t.Errorf("passkeys after a restart = %d, want 2 (no duplication)", got)
	}
	if got := countRows(t, second, "server", "SELECT COUNT(*) FROM admin_sessions"); got != 1 {
		t.Errorf("admin_sessions after a restart = %d, want 1 (no duplication)", got)
	}
}

// TestInitDualDB_FreshInstallHasCanonicalLayout proves a new install has the
// PART 12 tables in their canonical database and none of the legacy ones.
func TestInitDualDB_FreshInstallHasCanonicalLayout(t *testing.T) {
	ddb, err := InitDualDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	t.Cleanup(func() { _ = ddb.Close() })

	serverTables := []string{"admin_sessions", "rate_limits", "audit_log", "scheduler_tasks", "scheduler_history", "backups"}
	for _, table := range serverTables {
		if exists, err := sqliteTableExists(ddb.Server, table); err != nil || !exists {
			t.Errorf("server.db is missing table %s (err=%v)", table, err)
		}
	}
	usersTables := []string{"admins", "admin_preferences", "users", "tokens", "passkeys", "password_resets", "email_verifications", "user_sessions"}
	for _, table := range usersTables {
		if exists, err := sqliteTableExists(ddb.Users, table); err != nil || !exists {
			t.Errorf("users.db is missing table %s (err=%v)", table, err)
		}
	}
	for _, table := range []string{"server_admin_credentials", "server_audit_log", "server_admin_sessions"} {
		if exists, _ := sqliteTableExists(ddb.Server, table); exists {
			t.Errorf("fresh server.db contains legacy table %s", table)
		}
	}
	for _, table := range []string{"user_accounts", "user_tokens", "user_passkeys"} {
		if exists, _ := sqliteTableExists(ddb.Users, table); exists {
			t.Errorf("fresh users.db contains legacy table %s", table)
		}
	}
}
