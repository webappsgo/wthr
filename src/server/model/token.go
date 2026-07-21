package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// APIToken represents an API token
// AI.md PART 11: Never store plaintext tokens
type APIToken struct {
	ID          int            `json:"id"`
	UserID      int            `json:"user_id"`
	TokenPrefix string         `json:"token_prefix"`
	Name        string         `json:"name"`
	CreatedAt   time.Time      `json:"created_at"`
	LastUsedAt  *time.Time     `json:"last_used_at,omitempty"`
	ExpiresAt   sql.NullTime   `json:"expires_at,omitempty"`
	LastUsedIP  sql.NullString `json:"last_used_ip,omitempty"`
	// Token only populated on creation, never stored
	Token       string         `json:"token,omitempty"`
}

// TokenModel handles API token database operations
// AI.md PART 11: Tokens stored as SHA-256 hash, never plaintext
type TokenModel struct {
	DB *sql.DB
}

// GenerateToken creates a cryptographically secure random token with usr_ prefix
// AI.md PART 11: Format is usr_{32_alphanumeric}
func GenerateToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "usr_" + hex.EncodeToString(bytes), nil
}

// HashUserToken creates SHA-256 hash per AI.md PART 11 (user tokens)
// Note: Uses HashToken from token_v2.go for consistency
func HashUserToken(token string) string {
	return HashToken(token)
}

// GetUserTokenPrefix returns first 8 chars for display per AI.md PART 11
// Note: Uses GetTokenPrefix from token_v2.go for consistency
func GetUserTokenPrefix(token string) string {
	return GetTokenPrefix(token)
}

// Create creates a new API token for a user
// AI.md PART 11: Store SHA-256 hash, return full token only once
func (m *TokenModel) Create(userID int, name string) (*APIToken, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash token for storage (NEVER store plaintext)
	tokenHash := HashUserToken(token)
	tokenPrefix := GetUserTokenPrefix(token)

	result, err := database.GetUsersDB().Exec(`
		INSERT INTO user_tokens (user_id, token_hash, token_prefix, name, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, userID, tokenHash, tokenPrefix, name, time.Now())

	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &APIToken{
		ID:          int(id),
		UserID:      userID,
		TokenPrefix: tokenPrefix,
		Name:        name,
		CreatedAt:   time.Now(),
		Token:   token,
	}, nil
}

// GetByToken retrieves a token by its value using SHA-256 hash lookup
// AI.md PART 11: Query by hash, never store or query plaintext
func (m *TokenModel) GetByToken(token string) (*APIToken, error) {
	// Hash the provided token to look up
	tokenHash := HashUserToken(token)

	apiToken := &APIToken{}
	var lastUsed sql.NullTime

	err := database.GetUsersDB().QueryRow(`
		SELECT id, user_id, token_prefix, name, created_at, last_used_at
		FROM user_tokens WHERE token_hash = ?
	`, tokenHash).Scan(&apiToken.ID, &apiToken.UserID, &apiToken.TokenPrefix, &apiToken.Name,
		&apiToken.CreatedAt, &lastUsed)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("token not found")
	}
	if err != nil {
		return nil, err
	}

	if lastUsed.Valid {
		apiToken.LastUsedAt = &lastUsed.Time
	}

	return apiToken, nil
}

// GetByUserID retrieves all tokens for a user
func (m *TokenModel) GetByUserID(userID int) ([]*APIToken, error) {
	rows, err := database.GetUsersDB().Query(`
		SELECT id, user_id, token_prefix, name, created_at, last_used_at
		FROM user_tokens WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		token := &APIToken{}
		var lastUsed sql.NullTime
		err := rows.Scan(&token.ID, &token.UserID, &token.TokenPrefix, &token.Name, &token.CreatedAt, &lastUsed)
		if err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			token.LastUsedAt = &lastUsed.Time
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// UpdateLastUsed updates the last_used_at timestamp
func (m *TokenModel) UpdateLastUsed(tokenID int) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_tokens SET last_used_at = ?
		WHERE id = ?
	`, time.Now(), tokenID)
	return err
}

// Delete deletes a token
func (m *TokenModel) Delete(id int) error {
	_, err := database.GetUsersDB().Exec("DELETE FROM user_tokens WHERE id = ?", id)
	return err
}

// DeleteByUserID deletes all tokens for a user
func (m *TokenModel) DeleteByUserID(userID int) error {
	_, err := database.GetUsersDB().Exec("DELETE FROM user_tokens WHERE user_id = ?", userID)
	return err
}
