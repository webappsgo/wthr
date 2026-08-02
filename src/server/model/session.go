package models

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

	"github.com/webappsgo/wthr/src/database"
)

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

	_, err = database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `
		INSERT INTO user_sessions (token_hash, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, hashToken(rawToken), uid, expiresAt, now)

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

	err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, `
		SELECT token_hash, user_id, data, expires_at, created_at
		FROM user_sessions WHERE token_hash = ?
	`, hashToken(rawToken)).Scan(&session.ID, &session.UserID, &dataJSON, &session.ExpiresAt, &session.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, err
	}

	// Return the raw token as the session ID so callers can use it in cookies
	// and follow-up model calls without re-hashing.
	session.ID = rawToken

	// Check if session is expired.
	if time.Now().After(session.ExpiresAt) {
		m.Delete(rawToken)
		return nil, fmt.Errorf("session expired")
	}

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

	_, err = database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `
		UPDATE user_sessions SET data = ?
		WHERE token_hash = ?
	`, string(dataJSON), hashToken(rawToken))
	return err
}

// Extend pushes the session expiry forward by sessionTimeout seconds.
func (m *SessionModel) Extend(rawToken string, sessionTimeout int) error {
	expiresAt := time.Now().Add(time.Duration(sessionTimeout) * time.Second)

	_, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `
		UPDATE user_sessions SET expires_at = ?
		WHERE token_hash = ?
	`, expiresAt, hashToken(rawToken))
	return err
}

// Delete removes a single session by its raw bearer token.
func (m *SessionModel) Delete(rawToken string) error {
	_, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, "DELETE FROM user_sessions WHERE token_hash = ?", hashToken(rawToken))
	return err
}

// DeleteByUserID removes all sessions belonging to a user.
func (m *SessionModel) DeleteByUserID(userID int) error {
	_, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, "DELETE FROM user_sessions WHERE user_id = ?", userID)
	return err
}

// CleanupExpired removes sessions that have passed their expiry time.
func (m *SessionModel) CleanupExpired() error {
	_, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutBulk, "DELETE FROM user_sessions WHERE expires_at < ?", time.Now())
	return err
}
