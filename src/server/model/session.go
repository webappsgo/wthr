package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/casapps/wthr/src/database"
)

// Session represents a user session
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

// GenerateSessionID creates a cryptographically secure random session ID
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// Create creates a new session for a user
func (m *SessionModel) Create(userID interface{}, sessionTimeout int) (*Session, error) {
	// Convert userID to int64 if needed
	var uid int64
	switch v := userID.(type) {
	case int:
		uid = int64(v)
	case int64:
		uid = v
	default:
		return nil, fmt.Errorf("invalid userID type")
	}

	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(sessionTimeout) * time.Second)
	now := time.Now()

	_, err = database.GetUsersDB().Exec(`
		INSERT INTO user_sessions (id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, sessionID, uid, expiresAt, now)

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Session{
		ID:        sessionID,
		UserID:    int(uid),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

// GetByID retrieves a session by ID
func (m *SessionModel) GetByID(sessionID string) (*Session, error) {
	session := &Session{}
	var dataJSON sql.NullString

	err := database.GetUsersDB().QueryRow(`
		SELECT id, user_id, data, expires_at, created_at
		FROM user_sessions WHERE id = ?
	`, sessionID).Scan(&session.ID, &session.UserID, &dataJSON, &session.ExpiresAt, &session.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, err
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		m.Delete(sessionID)
		return nil, fmt.Errorf("session expired")
	}

	// Parse session data if present
	if dataJSON.Valid && dataJSON.String != "" {
		if err := json.Unmarshal([]byte(dataJSON.String), &session.Data); err != nil {
			return nil, fmt.Errorf("failed to parse session data: %w", err)
		}
	}

	return session, nil
}

// UpdateData updates session data
func (m *SessionModel) UpdateData(sessionID string, data map[string]interface{}) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	_, err = database.GetUsersDB().Exec(`
		UPDATE user_sessions SET data = ?
		WHERE id = ?
	`, string(dataJSON), sessionID)
	return err
}

// Extend extends session expiration time
func (m *SessionModel) Extend(sessionID string, sessionTimeout int) error {
	expiresAt := time.Now().Add(time.Duration(sessionTimeout) * time.Second)

	_, err := database.GetUsersDB().Exec(`
		UPDATE user_sessions SET expires_at = ?
		WHERE id = ?
	`, expiresAt, sessionID)
	return err
}

// Delete deletes a session
func (m *SessionModel) Delete(sessionID string) error {
	_, err := database.GetUsersDB().Exec("DELETE FROM user_sessions WHERE id = ?", sessionID)
	return err
}

// DeleteByUserID deletes all sessions for a user
func (m *SessionModel) DeleteByUserID(userID int) error {
	_, err := database.GetUsersDB().Exec("DELETE FROM user_sessions WHERE user_id = ?", userID)
	return err
}

// CleanupExpired removes expired sessions
func (m *SessionModel) CleanupExpired() error {
	_, err := database.GetUsersDB().Exec("DELETE FROM user_sessions WHERE expires_at < ?", time.Now())
	return err
}
