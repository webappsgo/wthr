package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/apimgr/weather/src/database"
	"github.com/apimgr/weather/src/util"
)

// RecoveryKey represents a 2FA recovery key
// TEMPLATE.md Part 31: 10 one-time recovery keys when 2FA enabled
type RecoveryKey struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	// Never expose hash
	KeyHash   string    `json:"-"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RecoveryKeyModel handles recovery key database operations
type RecoveryKeyModel struct {
	DB *sql.DB
}

// GenerateRecoveryKeys generates 10 recovery keys for a user
// Format: XXXX-XXXX-XXXX-XXXX (16 alphanumeric characters)
// Returns the plain-text keys (shown only once) and error
func (m *RecoveryKeyModel) GenerateRecoveryKeys(userID int) ([]string, error) {
	// Delete any existing keys for this user
	if err := m.DeleteAllForUser(userID); err != nil {
		return nil, fmt.Errorf("failed to delete existing keys: %w", err)
	}

	keys := make([]string, 10)

	for i := 0; i < 10; i++ {
		// Generate random 12-byte key (16 chars base64)
		keyBytes := make([]byte, 12)
		if _, err := rand.Read(keyBytes); err != nil {
			return nil, fmt.Errorf("failed to generate random key: %w", err)
		}

		// Encode to base64 and format
		encoded := base64.RawStdEncoding.EncodeToString(keyBytes)
		// Format as XXXX-XXXX-XXXX-XXXX
		formatted := formatRecoveryKey(encoded[:16])
		keys[i] = formatted

		// Hash the key for storage (using Argon2id per TEMPLATE.md)
		keyHash, err := utils.HashPassword(formatted)
		if err != nil {
			return nil, fmt.Errorf("failed to hash recovery key: %w", err)
		}

		// Store hashed key in database
		_, err = database.GetUsersDB().Exec(`
			INSERT INTO recovery_keys (user_id, key_hash, created_at)
			VALUES (?, ?, ?)
		`, userID, keyHash, time.Now())

		if err != nil {
			return nil, fmt.Errorf("failed to store recovery key: %w", err)
		}
	}

	return keys, nil
}

// VerifyAndUseRecoveryKey verifies a recovery key and marks it as used
// Returns true if key is valid and was used successfully
func (m *RecoveryKeyModel) VerifyAndUseRecoveryKey(userID int, key string) (bool, error) {
	// Normalize key (remove spaces, convert to uppercase)
	key = strings.ToUpper(strings.ReplaceAll(key, " ", ""))
	key = strings.ReplaceAll(key, "-", "")

	// Re-add dashes in proper format
	if len(key) == 16 {
		key = fmt.Sprintf("%s-%s-%s-%s", key[0:4], key[4:8], key[8:12], key[12:16])
	}

	// Get all unused keys for this user
	rows, err := database.GetUsersDB().Query(`
		SELECT id, key_hash FROM recovery_keys
		WHERE user_id = ? AND used_at IS NULL
	`, userID)

	if err != nil {
		return false, fmt.Errorf("failed to query recovery keys: %w", err)
	}
	defer rows.Close()

	// Check each key
	for rows.Next() {
		var id int
		var keyHash string

		if err := rows.Scan(&id, &keyHash); err != nil {
			continue
		}

		// Verify key against hash
		valid, err := utils.VerifyPassword(key, keyHash)
		if err != nil {
			continue
		}

		if valid {
			// Mark key as used
			now := time.Now()
			_, err := database.GetUsersDB().Exec(`
				UPDATE recovery_keys SET used_at = ? WHERE id = ?
			`, now, id)

			if err != nil {
				return false, fmt.Errorf("failed to mark key as used: %w", err)
			}

			return true, nil
		}
	}

	return false, nil
}

// GetUnusedKeysCount returns the count of unused recovery keys for a user
func (m *RecoveryKeyModel) GetUnusedKeysCount(userID int) (int, error) {
	var count int
	err := database.GetUsersDB().QueryRow(`
		SELECT COUNT(*) FROM recovery_keys
		WHERE user_id = ? AND used_at IS NULL
	`, userID).Scan(&count)

	return count, err
}

// GetAllKeysForUser returns all recovery keys for a user (for admin view)
func (m *RecoveryKeyModel) GetAllKeysForUser(userID int) ([]RecoveryKey, error) {
	rows, err := database.GetUsersDB().Query(`
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

		err := rows.Scan(&key.ID, &key.UserID, &key.KeyHash, &usedAt, &key.CreatedAt)
		if err != nil {
			return nil, err
		}

		if usedAt.Valid {
			key.UsedAt = &usedAt.Time
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// DeleteAllForUser deletes all recovery keys for a user
func (m *RecoveryKeyModel) DeleteAllForUser(userID int) error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM recovery_keys WHERE user_id = ?
	`, userID)

	return err
}

// formatRecoveryKey formats a 16-character string as XXXX-XXXX-XXXX-XXXX
func formatRecoveryKey(key string) string {
	if len(key) < 16 {
		// Pad with zeros if needed
		key = key + strings.Repeat("0", 16-len(key))
	}

	return fmt.Sprintf("%s-%s-%s-%s",
		strings.ToUpper(key[0:4]),
		strings.ToUpper(key[4:8]),
		strings.ToUpper(key[8:12]),
		strings.ToUpper(key[12:16]))
}
