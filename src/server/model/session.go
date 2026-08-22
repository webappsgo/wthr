package model

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
)

// The timestamp helpers below are thin aliases over src/common/dbtime, the
// project's single source of truth for SQL timestamp formatting, parsing and
// comparison. They stay package-local so the many existing callers in this
// package keep reading naturally, and so package model never has to import
// package scheduler (scheduler depends on server/service, which depends on this
// package). dbtime imports only the standard library, so no cycle is possible.

// sqlTimestampLayout is the canonical "YYYY-MM-DD HH:MM:SS" layout SQLite's
// CURRENT_TIMESTAMP emits and that PostgreSQL/MySQL accept as a literal.
const sqlTimestampLayout = dbtime.SQLTimestampLayout

// sqlTimestamp renders t as canonical UTC text for binding into a query.
func sqlTimestamp(t time.Time) string {
	return dbtime.FormatSQLTimestamp(t)
}

// parseStoredTimestamp converts a value scanned from a DATETIME column into a
// UTC time.Time, reporting false for NULL and for layouts the project never
// writes.
func parseStoredTimestamp(value interface{}) (time.Time, bool) {
	return dbtime.ParseStoredTimestamp(value)
}

// isTimestampAfter reports whether a value scanned from a DATETIME column is
// strictly later than threshold, failing closed (false) for NULL and for
// layouts the project never writes.
func isTimestampAfter(value interface{}, threshold time.Time) bool {
	return dbtime.IsAfter(value, threshold)
}

// deleteRowsWithTimestampBefore deletes every row of table whose
// timestampColumn holds an instant earlier than cutoff (or equal to it when
// includeEqual is true), comparing in Go so mixed on-disk layouts and mixed
// timezones all resolve to the same absolute instant.
func deleteRowsWithTimestampBefore(db *sql.DB, table, idColumn, timestampColumn string, cutoff time.Time, includeEqual bool) (int64, error) {
	return dbtime.DeleteRowsWithTimestampBefore(db, table, idColumn, timestampColumn, cutoff, includeEqual)
}

// Session represents a user session.
// ID holds the raw bearer token (what the browser cookie contains).
// The token is never stored in plaintext — only its SHA-256 hash is persisted.
type Session struct {
	ID        string                 `json:"id"`
	UserID    int                    `json:"user_id"`
	Data      map[string]interface{} `json:"data,omitempty"`
	ExpiresAt time.Time              `json:"expires_at"`
	CreatedAt time.Time              `json:"created_at"`
}

// SessionModel handles session database operations
type SessionModel struct {
	DB *sql.DB
}

// getDB returns the users.db handle this model was constructed with. The only
// table SessionModel touches (user_sessions) is declared in
// database.UsersSchema, so the injected handle is the correct database for
// every query below.
// Fallback: when the injected handle is nil (unit tests, or construction
// before the global dual DB is wired) the process-global users handle is used
// instead, so a nil handle degrades to the previous behavior rather than
// panicking.
func (m *SessionModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}

	return database.GetUsersDB()
}

// GenerateSessionID creates a cryptographically secure random session token.
// Returns the raw 32-byte value encoded as base64url; caller stores the hash.
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// hashToken returns the lower-hex SHA-256 digest of rawToken.
// This is the value stored in user_sessions.token_hash per IDEA.md security spec.
func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// Create creates a new session for a user and returns a Session whose ID is
// the raw bearer token (for placing in the HttpOnly cookie). Only the SHA-256
// hash of the token is written to the database.
func (m *SessionModel) Create(userID interface{}, sessionTimeout int) (*Session, error) {
	var uid int64
	switch v := userID.(type) {
	case int:
		uid = int64(v)
	case int64:
		uid = v
	default:
		return nil, fmt.Errorf("invalid userID type")
	}

	rawToken, err := GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(sessionTimeout) * time.Second)
	now := time.Now()

	// expires_at and created_at are bound as canonical UTC text, the same layout
	// UserSessionModel.CreateSession and CURRENT_TIMESTAMP write into this table.
	// Binding a raw time.Time would store the local-zone time.Time.String() form,
	// which compares wrongly against every other writer's rows.
	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		INSERT INTO user_sessions (token_hash, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, hashToken(rawToken), uid, sqlTimestamp(expiresAt), sqlTimestamp(now))

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Session{
		ID:        rawToken,
		UserID:    int(uid),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

// GetByID retrieves a session by the raw bearer token from the cookie.
// The token is hashed before the DB lookup; ID in the returned Session holds
// the raw token so callers can use it for subsequent Delete/UpdateData calls.
func (m *SessionModel) GetByID(rawToken string) (*Session, error) {
	session := &Session{}
	var dataJSON sql.NullString

	// expires_at and created_at are scanned as raw driver values and resolved
	// with parseStoredTimestamp rather than scanned straight into time.Time.
	// user_sessions has several writers, and a row an older build wrote holds the
	// local-zone time.Time.String() text; database/sql cannot convert that to a
	// time.Time, so the lookup failed with a scan error instead of returning the
	// session.
	var storedExpiresAt interface{}
	var storedCreatedAt interface{}

	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT token_hash, user_id, data, expires_at, created_at
		FROM user_sessions WHERE token_hash = ?
	`, hashToken(rawToken)).Scan(&session.ID, &session.UserID, &dataJSON, &storedExpiresAt, &storedCreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, err
	}

	// Return the raw token as the session ID so callers can use it in cookies
	// and follow-up model calls without re-hashing.
	session.ID = rawToken

	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		session.CreatedAt = parsed
	}

	// Check if session is expired. An expires_at this project cannot parse fails
	// closed - the session is treated as expired and removed rather than granted
	// unlimited life.
	expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
	if !ok || !time.Now().UTC().Before(expiresAt) {
		m.Delete(rawToken)
		return nil, fmt.Errorf("session expired")
	}

	session.ExpiresAt = expiresAt

	// Parse optional session data.
	if dataJSON.Valid && dataJSON.String != "" {
		if err := json.Unmarshal([]byte(dataJSON.String), &session.Data); err != nil {
			return nil, fmt.Errorf("failed to parse session data: %w", err)
		}
	}

	return session, nil
}

// UpdateData stores arbitrary key-value data on the session (used for 2FA
// pending state). rawToken is the bearer token from the cookie.
func (m *SessionModel) UpdateData(rawToken string, data map[string]interface{}) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE user_sessions SET data = ?
		WHERE token_hash = ?
	`, string(dataJSON), hashToken(rawToken))
	return err
}

// Extend pushes the session expiry forward by sessionTimeout seconds.
func (m *SessionModel) Extend(rawToken string, sessionTimeout int) error {
	expiresAt := time.Now().Add(time.Duration(sessionTimeout) * time.Second)

	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE user_sessions SET expires_at = ?
		WHERE token_hash = ?
	`, sqlTimestamp(expiresAt), hashToken(rawToken))
	return err
}

// Delete removes a single session by its raw bearer token.
func (m *SessionModel) Delete(rawToken string) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, "DELETE FROM user_sessions WHERE token_hash = ?", hashToken(rawToken))
	return err
}

// DeleteByUserID removes all sessions belonging to a user.
func (m *SessionModel) DeleteByUserID(userID int) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, "DELETE FROM user_sessions WHERE user_id = ?", userID)
	return err
}

// CleanupExpired removes sessions that have passed their expiry time.
// user_sessions.expires_at has more than one producer, so the expiry test runs
// in Go against a UTC cutoff instead of as an SQL text comparison - comparing
// mixed layouts lexicographically deleted sessions that had not expired. Rows
// whose expires_at is NULL or unparseable are left alone.
func (m *SessionModel) CleanupExpired() error {
	_, err := deleteRowsWithTimestampBefore(m.getDB(), "user_sessions", "id", "expires_at", time.Now().UTC(), false)
	return err
}
