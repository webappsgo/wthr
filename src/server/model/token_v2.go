// Package models provides token management per TEMPLATE.md PART 11
package model

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// Token represents an API token per TEMPLATE.md PART 11
type Token struct {
	ID          int64      `json:"id"`
	OwnerType   string     `json:"owner_type"` // 'admin', 'user', 'org'
	OwnerID     int64      `json:"owner_id"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"-"`            // Never expose hash
	TokenPrefix string     `json:"token_prefix"` // First 8 chars for display
	Scope       string     `json:"scope"`        // 'global', 'read-write', 'read'
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	// Only populated on creation, never stored
	Token string `json:"token,omitempty"`
}

// TokenScope types per TEMPLATE.md PART 11
const (
	ScopeGlobal    = "global"     // All permissions owner has
	ScopeReadWrite = "read-write" // Read and write, no delete/admin
	ScopeRead      = "read"       // Read-only
)

// Owner types per TEMPLATE.md PART 11
const (
	OwnerTypeAdmin = "admin"
	OwnerTypeUser  = "user"
	OwnerTypeOrg   = "org"
)

// Token prefixes per TEMPLATE.md PART 11 (NON-NEGOTIABLE)
const (
	PrefixAdmin    = "adm_"
	PrefixUser     = "usr_"
	PrefixOrg      = "org_"
	PrefixAdminAgt = "adm_agt_"
	PrefixUserAgt  = "usr_agt_"
	PrefixOrgAgt   = "org_agt_"
)

// Expiration options per TEMPLATE.md PART 11
var ExpirationOptions = map[string]time.Duration{
	"never":   0, // NULL in database
	"7days":   7 * 24 * time.Hour,
	"1month":  30 * 24 * time.Hour,
	"6months": 180 * 24 * time.Hour,
	"1year":   365 * 24 * time.Hour,
	// "custom" - user picks date from calendar
}

// GenerateTokenWithPrefix generates a token with proper prefix per TEMPLATE.md PART 11
// Format: {prefix}_{random_32_alphanumeric}
func GenerateTokenWithPrefix(prefix string) (string, error) {
	// Generate 32 alphanumeric characters (using hex for simplicity)
	bytes := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	random := hex.EncodeToString(bytes)
	return prefix + random, nil
}

// HashToken creates SHA-256 hash per TEMPLATE.md PART 11
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GetTokenPrefix returns first 8 chars for display per TEMPLATE.md PART 11
// Example: "adm_a1b2..."
func GetTokenPrefix(token string) string {
	if len(token) < 8 {
		return token
	}
	return token[:8]
}

// ValidateTokenFormat checks if token matches spec format
// Format: {prefix}_{32_alphanumeric} or {prefix}_agt_{32_alphanumeric}
func ValidateTokenFormat(token string) error {
	// Check for agent tokens first (compound prefix)
	if strings.HasPrefix(token, PrefixAdminAgt) {
		if len(token) != len(PrefixAdminAgt)+32 {
			return fmt.Errorf("invalid admin agent token format")
		}
		return nil
	}
	if strings.HasPrefix(token, PrefixUserAgt) {
		if len(token) != len(PrefixUserAgt)+32 {
			return fmt.Errorf("invalid user agent token format")
		}
		return nil
	}
	if strings.HasPrefix(token, PrefixOrgAgt) {
		if len(token) != len(PrefixOrgAgt)+32 {
			return fmt.Errorf("invalid org agent token format")
		}
		return nil
	}

	// Standard single-prefix tokens
	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 {
		return fmt.Errorf("token must have format: {prefix}_{random}")
	}

	prefix := parts[0] + "_"
	random := parts[1]

	// Validate prefix
	switch prefix {
	case PrefixAdmin, PrefixUser, PrefixOrg:
		// Valid prefix
	default:
		return fmt.Errorf("unknown token prefix: %s", prefix)
	}

	// Validate random part is 32 chars
	if len(random) != 32 {
		return fmt.Errorf("token random part must be 32 characters")
	}

	return nil
}

// TokenModelV2 handles token database operations per TEMPLATE.md PART 11
type TokenModelV2 struct {
	DB *sql.DB
}

// CreateToken creates a new token per TEMPLATE.md PART 11
func (m *TokenModelV2) CreateToken(ownerType string, ownerID int64, name, scope string, expiration time.Duration) (*Token, error) {
	// Validate owner type
	if ownerType != OwnerTypeAdmin && ownerType != OwnerTypeUser && ownerType != OwnerTypeOrg {
		return nil, fmt.Errorf("invalid owner type: %s", ownerType)
	}

	// user_tokens rows are owned by a user_accounts row (enforced foreign key), so only
	// user-owned tokens live here; admin tokens are stored in server.db by AdminModel
	if ownerType != OwnerTypeUser {
		return nil, fmt.Errorf("owner type %s is not stored in user_tokens", ownerType)
	}

	// Validate scope
	if scope != ScopeGlobal && scope != ScopeReadWrite && scope != ScopeRead {
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}

	fullToken, err := GenerateTokenWithPrefix(PrefixUser)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash token for storage (NEVER store plaintext)
	tokenHash := HashToken(fullToken)
	tokenPrefix := GetTokenPrefix(fullToken)

	// Calculate expiration
	var expiresAt *time.Time
	if expiration > 0 {
		exp := time.Now().Add(expiration)
		expiresAt = &exp
	}

	// Insert into database
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		INSERT INTO user_tokens (user_id, name, token_hash, token_prefix, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, ownerID, name, tokenHash, tokenPrefix, scope, expiresAt, time.Now())

	if err != nil {
		return nil, fmt.Errorf("failed to insert token: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get token id: %w", err)
	}

	// Return token with full token value (only shown once)
	return &Token{
		ID:          id,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		Name:        name,
		TokenHash:   tokenHash,
		TokenPrefix: tokenPrefix,
		Scope:       scope,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now(),
		Token:       fullToken, // Only returned on creation
	}, nil
}

// ValidateToken validates a token and returns token info per TEMPLATE.md PART 11
func (m *TokenModelV2) ValidateToken(token string) (*Token, error) {
	// Validate format
	if err := ValidateTokenFormat(token); err != nil {
		return nil, err
	}

	// Hash token to look up in database
	tokenHash := HashToken(token)

	// Look up token
	var t Token
	var expiresAt sql.NullTime
	var lastUsedAt sql.NullTime

	var name sql.NullString
	var scopes sql.NullString

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, name, token_hash, token_prefix, scopes, expires_at, last_used_at, created_at
		FROM user_tokens
		WHERE token_hash = ?
	`, tokenHash).Scan(
		&t.ID, &t.OwnerID, &name, &t.TokenHash, &t.TokenPrefix,
		&scopes, &expiresAt, &lastUsedAt, &t.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid token")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Ownership is implied by the user_accounts foreign key on user_tokens
	t.OwnerType = OwnerTypeUser
	t.Name = name.String
	t.Scope = scopes.String

	// Check expiration
	if expiresAt.Valid {
		t.ExpiresAt = &expiresAt.Time
		if time.Now().After(*t.ExpiresAt) {
			return nil, fmt.Errorf("token expired")
		}
	}

	if lastUsedAt.Valid {
		t.LastUsedAt = &lastUsedAt.Time
	}

	return &t, nil
}

// UpdateLastUsed updates the last_used_at timestamp
func (m *TokenModelV2) UpdateLastUsed(tokenID int64) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		UPDATE user_tokens SET last_used_at = ?
		WHERE id = ?
	`, time.Now(), tokenID)
	return err
}

// ListTokens lists all tokens for an owner per TEMPLATE.md PART 11
func (m *TokenModelV2) ListTokens(ownerType string, ownerID int64) ([]*Token, error) {
	if ownerType != OwnerTypeUser {
		return nil, fmt.Errorf("owner type %s is not stored in user_tokens", ownerType)
	}

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, name, token_prefix, scopes, expires_at, last_used_at, created_at
		FROM user_tokens
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, ownerID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		var t Token
		var expiresAt sql.NullTime
		var lastUsedAt sql.NullTime
		var name sql.NullString
		var scopes sql.NullString

		err := rows.Scan(
			&t.ID, &t.OwnerID, &name, &t.TokenPrefix,
			&scopes, &expiresAt, &lastUsedAt, &t.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		t.OwnerType = OwnerTypeUser
		t.Name = name.String
		t.Scope = scopes.String

		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			t.LastUsedAt = &lastUsedAt.Time
		}

		tokens = append(tokens, &t)
	}

	return tokens, nil
}

// DeleteToken deletes a token per TEMPLATE.md PART 11
func (m *TokenModelV2) DeleteToken(id int64, ownerType string, ownerID int64) error {
	if ownerType != OwnerTypeUser {
		return fmt.Errorf("owner type %s is not stored in user_tokens", ownerType)
	}

	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		DELETE FROM user_tokens
		WHERE id = ? AND user_id = ?
	`, id, ownerID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("token not found or access denied")
	}

	return nil
}

// RotateToken generates a new token value while keeping settings per TEMPLATE.md PART 11
func (m *TokenModelV2) RotateToken(id int64, ownerType string, ownerID int64) (*Token, error) {
	if ownerType != OwnerTypeUser {
		return nil, fmt.Errorf("owner type %s is not stored in user_tokens", ownerType)
	}

	// Get existing token
	var existing Token
	var expiresAt sql.NullTime
	var name sql.NullString
	var scopes sql.NullString

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, name, scopes, expires_at, created_at
		FROM user_tokens
		WHERE id = ? AND user_id = ?
	`, id, ownerID).Scan(
		&existing.ID, &existing.OwnerID,
		&name, &scopes, &expiresAt, &existing.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("token not found")
	}
	if err != nil {
		return nil, err
	}

	existing.OwnerType = OwnerTypeUser
	existing.Name = name.String
	existing.Scope = scopes.String

	fullToken, err := GenerateTokenWithPrefix(PrefixUser)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	tokenHash := HashToken(fullToken)
	tokenPrefix := GetTokenPrefix(fullToken)

	// Update token
	_, err = database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		UPDATE user_tokens
		SET token_hash = ?, token_prefix = ?, last_used_at = NULL
		WHERE id = ?
	`, tokenHash, tokenPrefix, id)

	if err != nil {
		return nil, fmt.Errorf("failed to update token: %w", err)
	}

	// Return updated token with full token value
	existing.TokenHash = tokenHash
	existing.TokenPrefix = tokenPrefix
	existing.Token = fullToken
	existing.LastUsedAt = nil

	if expiresAt.Valid {
		existing.ExpiresAt = &expiresAt.Time
	}

	return &existing, nil
}
