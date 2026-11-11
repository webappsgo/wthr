package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AI.md PART 10 Connection Pooling: small-deployment defaults.
const (
	poolMaxOpen     = 25
	poolMaxIdle     = 5
	poolMaxLifetime = 5 * time.Minute
	poolMaxIdleTime = time.Minute
)

// configurePool applies the AI.md PART 10 pool settings to db. Every database
// connection must be pooled, including the idle-time bound.
func configurePool(db *sql.DB) {
	db.SetMaxOpenConns(poolMaxOpen)
	db.SetMaxIdleConns(poolMaxIdle)
	db.SetConnMaxLifetime(poolMaxLifetime)
	db.SetConnMaxIdleTime(poolMaxIdleTime)
}

// serverSchemaUpdates holds idempotent schema updates applied to server.db on
// every startup. AI.md PART 10 forbids migration files and version tracking:
// the base ServerSchema creates every table, and each statement here brings an
// older database forward without dropping or renaming anything.
var serverSchemaUpdates = []string{
	`ALTER TABLE server_admin_sessions ADD COLUMN token_hash TEXT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_sessions_token_hash ON server_admin_sessions(token_hash)`,
	`ALTER TABLE server_audit_log ADD COLUMN source TEXT DEFAULT 'api'`,
}

// usersSchemaUpdates holds idempotent schema updates applied to users.db on
// every startup. Columns added here also exist in the base UsersSchema, so on a
// fresh database every statement is a no-op that reports "duplicate column".
var usersSchemaUpdates = []string{
	// Session 2FA pending-state storage
	`ALTER TABLE user_sessions ADD COLUMN data TEXT`,

	// Hashed session tokens replaced the original plaintext session_id column.
	// The old column is left in place and unused per AI.md PART 10 (add new,
	// never drop), and rows written before the change are never readable
	// because lookups now match on the SHA-256 hash only.
	`ALTER TABLE user_sessions ADD COLUMN token_hash TEXT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_hash ON user_sessions(token_hash)`,

	// Invite metadata for PART 34 invite-only registration
	`ALTER TABLE user_invites ADD COLUMN username TEXT`,
	`ALTER TABLE user_invites ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`,
}

// applySchemaUpdates runs every statement in updates against db, ignoring
// errors that only mean the column or index is already present. AI.md PART 10:
// schema/DDL statements use the Migration timeout tier.
func applySchemaUpdates(db *sql.DB, updates []string) error {
	ctx := context.Background()

	for _, stmt := range updates {
		if _, err := ExecContext(ctx, db, TimeoutMigration, stmt); err != nil {
			if isColumnExistsError(err) {
				continue
			}
			return fmt.Errorf("schema update %q: %w", stmt, err)
		}
	}

	return nil
}

// isColumnExistsError reports whether err is a driver complaint that the object
// a schema update adds is already present. SQLite, PostgreSQL and MySQL each
// word this differently, so every known phrasing is matched.
func isColumnExistsError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate key name")
}

// migrateLegacyAdminSessions replaces plaintext session IDs with UUID row IDs
// and stores a SHA-256 digest of each legacy cookie token. The migration is
// restart-safe: each row is updated only while its token_hash is NULL.
func migrateLegacyAdminSessions(db *sql.DB) error {
	rows, err := QueryContext(context.Background(), db, TimeoutMigration, `
		SELECT id FROM server_admin_sessions WHERE token_hash IS NULL
	`)
	if err != nil {
		return fmt.Errorf("query legacy admin sessions: %w", err)
	}
	var legacyTokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy admin session: %w", err)
		}
		legacyTokens = append(legacyTokens, token)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy admin sessions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy admin session rows: %w", err)
	}

	for _, rawToken := range legacyTokens {
		digest := sha256.Sum256([]byte(rawToken))
		_, err := ExecContext(context.Background(), db, TimeoutMigration, `
			UPDATE server_admin_sessions
			SET id = ?, token_hash = ?
			WHERE id = ? AND token_hash IS NULL
		`, uuid.NewString(), hex.EncodeToString(digest[:]), rawToken)
		if err != nil {
			return fmt.Errorf("migrate legacy admin session: %w", err)
		}
	}

	return nil
}
