package models

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// RecoveryKey represents a 2FA recovery key per AI.md PART 34.
// Format: {8-hex-chars}-{4-hex-chars} e.g. a1b2c3d4-e5f6
type RecoveryKey struct {
	ID        int        `json:"id"`
	UserID    int        `json:"user_id"`
	KeyHash   string     `json:"-"` // SHA-256 hash — never exposed
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// RecoveryKeyModel handles recovery key database operations.
type RecoveryKeyModel struct {
	DB *sql.DB
}

// GenerateRecoveryKeys generates 10 one-time recovery keys for a user per AI.md PART 34.
// Keys are formatted as {8-hex-chars}-{4-hex-chars} (e.g. a1b2c3d4-e5f6).
// Each key is SHA-256 hashed before storage; plain-text keys are returned once.
func (m *RecoveryKeyModel) GenerateRecoveryKeys(userID int) ([]string, error) {
	if err := m.DeleteAllForUser(userID); err != nil {
		return nil, fmt.Errorf("failed to delete existing keys: %w", err)
	}

	keys := make([]string, 10)

	for i := 0; i < 10; i++ {
		raw := make([]byte, 6) // 6 bytes = 12 hex chars
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("failed to generate random key: %w", err)
		}
		h := hex.EncodeToString(raw) // 12 lowercase hex chars
		formatted := h[0:8] + "-" + h[8:12]
		keys[i] = formatted

		keyHash := HashAPIToken(formatted) // SHA-256 per AI.md PART 34

		_, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `
			INSERT INTO recovery_keys (user_id, key_hash, created_at)
			VALUES (?, ?, ?)
		`, userID, keyHash, time.Now())
		if err != nil {
			return nil, fmt.Errorf("failed to store recovery key: %w", err)
		}
	}

	return keys, nil
}

// VerifyAndUseRecoveryKey verifies a recovery key (case-insensitive) and marks it as used.
func (m *RecoveryKeyModel) VerifyAndUseRecoveryKey(userID int, key string) (bool, error) {
	// Normalize: lowercase, strip spaces
	key = strings.ToLower(strings.ReplaceAll(key, " ", ""))

	// Accept with or without dash — rebuild canonical form
	stripped := strings.ReplaceAll(key, "-", "")
	if len(stripped) == 12 {
		key = stripped[0:8] + "-" + stripped[8:12]
	}

	keyHash := HashAPIToken(key)

	rows, err := database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, `
		SELECT id FROM recovery_keys
		WHERE user_id = ? AND key_hash = ? AND used_at IS NULL
	`, userID, keyHash)
	if err != nil {
		return false, fmt.Errorf("failed to query recovery keys: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return false, nil
	}

	var id int
	if err := rows.Scan(&id); err != nil {
		return false, fmt.Errorf("failed to scan recovery key: %w", err)
	}
	rows.Close()

	_, err = database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `
		UPDATE recovery_keys SET used_at = ? WHERE id = ?
	`, time.Now(), id)
	if err != nil {
		return false, fmt.Errorf("failed to mark key as used: %w", err)
	}

	return true, nil
}

// GetUnusedKeysCount returns the count of unused recovery keys for a user.
func (m *RecoveryKeyModel) GetUnusedKeysCount(userID int) (int, error) {
	var count int
	err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM recovery_keys
		WHERE user_id = ? AND used_at IS NULL
	`, userID).Scan(&count)
	return count, err
}

// GetAllKeysForUser returns all recovery keys for a user.
func (m *RecoveryKeyModel) GetAllKeysForUser(userID int) ([]RecoveryKey, error) {
	rows, err := database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, `
		SELECT id, user_id, key_hash, used_at, created_at
		FROM recovery_keys
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []RecoveryKey
	for rows.Next() {
		var key RecoveryKey
		var usedAt sql.NullTime
		if err := rows.Scan(&key.ID, &key.UserID, &key.KeyHash, &usedAt, &key.CreatedAt); err != nil {
			return nil, err
		}
		if usedAt.Valid {
			key.UsedAt = &usedAt.Time
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// DeleteAllForUser deletes all recovery keys for a user.
func (m *RecoveryKeyModel) DeleteAllForUser(userID int) error {
	_, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `DELETE FROM recovery_keys WHERE user_id = ?`, userID)
	return err
}
