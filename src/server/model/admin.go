// Package models provides data models per TEMPLATE.md PART 22
package model

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"golang.org/x/crypto/argon2"
)

// Argon2id parameters per TEMPLATE.md PART 0
// CRITICAL: NEVER use bcrypt - MUST use Argon2id with these exact parameters
const (
	argon2Time = 3
	// 64 MB
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	argon2KeyLen  = 32
	saltLength    = 16
)

// Admin represents a server administrator (stored in server.db)
// Per TEMPLATE.md PART 22: Admins MUST be in server.db, NOT users.db
// AI.md PART 11: API tokens stored as SHA-256 hash, never plaintext
type Admin struct {
	ID             int64      `json:"id"`
	Username       string     `json:"username"`
	Email          string     `json:"email"`
	PasswordHash   string     `json:"-"`
	APITokenPrefix string     `json:"api_token_prefix,omitempty"`
	IsSuperAdmin   bool       `json:"is_super_admin"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
}

// AdminSession represents an active admin session
// Per TEMPLATE.md PART 22: Secure session management required
type AdminSession struct {
	ID      int64 `json:"id"`
	AdminID int64 `json:"admin_id"`
	// Secure random token
	SessionID  string    `json:"session_id"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// AdminPreferences stores admin-specific settings
type AdminPreferences struct {
	ID      int64 `json:"id"`
	AdminID int64 `json:"admin_id"`
	// light, dark, auto
	Theme                string    `json:"theme"`
	Language             string    `json:"language"`
	Timezone             string    `json:"timezone"`
	NotificationsEnabled bool      `json:"notifications_enabled"`
	EmailNotifications   bool      `json:"email_notifications"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// AdminNotification represents a WebUI notification for an admin
type AdminNotification struct {
	ID      int64 `json:"id"`
	AdminID int64 `json:"admin_id"`
	// success, info, warning, error, security
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Link      string    `json:"link,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminInvite represents an admin invitation
// TEMPLATE.md PART 22: 15-minute invite tokens REQUIRED
type AdminInvite struct {
	ID           int64     `json:"id"`
	Token        string    `json:"token"`
	InvitedEmail string    `json:"invited_email"`
	InvitedBy    int64     `json:"invited_by"`
	CreatedAt    time.Time `json:"created_at"`
	// MUST be 15 minutes from creation
	ExpiresAt time.Time  `json:"expires_at"`
	UsedBy    *int64     `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// HashPassword hashes a password using Argon2id per TEMPLATE.md PART 0
// CRITICAL: NEVER use bcrypt - MUST use Argon2id with exact parameters
// Returns hash in PHC string format: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
func HashPassword(password string) (string, error) {
	// Generate cryptographically secure random salt
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Generate Argon2id hash with TEMPLATE.md PART 0 parameters
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Encode to PHC format for storage
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Time, argon2Threads, b64Salt, b64Hash), nil
}

// VerifyPassword verifies a password against an Argon2id hash
// Uses constant-time comparison to prevent timing attacks
func VerifyPassword(password, hash string) (bool, error) {
	// Parse PHC format: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format: expected 6 parts, got %d", len(parts))
	}

	if parts[1] != "argon2id" {
		return false, fmt.Errorf("unsupported algorithm: %s (TEMPLATE.md requires argon2id)", parts[1])
	}

	// Parse parameters: m=memory,t=time,p=threads
	var memory, time uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return false, fmt.Errorf("failed to parse parameters: %w", err)
	}

	// Decode base64 salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Decode base64 hash
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("failed to decode hash: %w", err)
	}

	// Generate hash from provided password with same parameters
	providedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(providedHash, expectedHash) == 1 {
		return true, nil
	}

	return false, nil
}

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateAPIToken generates a new admin API token with adm_ prefix
// AI.md PART 11: Format is adm_{32_alphanumeric}
func GenerateAPIToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "adm_" + hex.EncodeToString(bytes), nil
}

// HashAPIToken creates SHA-256 hash of API token per AI.md PART 11
func HashAPIToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GetAPITokenPrefix returns first 8 chars for display per AI.md PART 11
func GetAPITokenPrefix(token string) string {
	if len(token) < 8 {
		return token
	}
	return token[:8]
}

// GenerateInviteToken generates a new invite token
func GenerateInviteToken() (string, error) {
	return GenerateSecureToken(32)
}

// IsExpired checks if an invite has expired (15-minute window per TEMPLATE.md PART 22)
func (i *AdminInvite) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// IsUsed checks if an invite has been used
func (i *AdminInvite) IsUsed() bool {
	return i.UsedAt != nil
}

// SessionExpired checks if a session has expired
func (s *AdminSession) SessionExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// AdminModel handles admin database operations
type AdminModel struct {
	DB *sql.DB
}

// getDB returns the server.db handle this model was constructed with. Every
// table AdminModel touches (server_admin_credentials,
// server_admin_preferences) is declared in database.ServerSchema, so the
// injected handle is the correct database for every query below.
// Fallback: when the injected handle is nil (unit tests, or construction
// before the global dual DB is wired) the process-global server handle is used
// instead, so a nil handle degrades to the previous behavior rather than
// panicking.
func (m *AdminModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}

	return database.GetServerDB()
}

// GetAll returns all admins (with privacy: no passwords, minimal info)
// TEMPLATE.md PART 22: Admin privacy - can't see other admin details
func (m *AdminModel) GetAll() ([]Admin, error) {
	rows, err := database.QueryContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT id, username, email, is_super_admin, is_active, created_at, updated_at, last_login_at
		FROM server_admin_credentials
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query admins: %w", err)
	}
	defer rows.Close()

	var admins []Admin
	for rows.Next() {
		var admin Admin
		// The three timestamp columns are scanned as raw driver values and
		// resolved with parseStoredTimestamp instead of being scanned into
		// time.Time/sql.NullTime directly. A row written by an older build holds
		// the local-zone time.Time.String() text, which database/sql cannot
		// convert to a time.Time - the whole listing failed with a scan error.
		var storedCreatedAt interface{}
		var storedUpdatedAt interface{}
		var storedLastLoginAt interface{}

		err := rows.Scan(
			&admin.ID,
			&admin.Username,
			&admin.Email,
			&admin.IsSuperAdmin,
			&admin.IsActive,
			&storedCreatedAt,
			&storedUpdatedAt,
			&storedLastLoginAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admin: %w", err)
		}

		if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
			admin.CreatedAt = parsed
		}

		if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
			admin.UpdatedAt = parsed
		}

		if parsed, ok := parseStoredTimestamp(storedLastLoginAt); ok {
			admin.LastLoginAt = &parsed
		}

		admins = append(admins, admin)
	}

	return admins, nil
}

// GetByID returns a single admin by ID
func (m *AdminModel) GetByID(id int64) (*Admin, error) {
	var admin Admin
	var apiTokenPrefix sql.NullString
	// See AdminModel.GetAll for why the timestamps are scanned as raw driver
	// values and parsed in Go.
	var storedCreatedAt interface{}
	var storedUpdatedAt interface{}
	var storedLastLoginAt interface{}

	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT id, username, email, password_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at, last_login_at
		FROM server_admin_credentials
		WHERE id = ?
	`, id).Scan(
		&admin.ID,
		&admin.Username,
		&admin.Email,
		&admin.PasswordHash,
		&apiTokenPrefix,
		&admin.IsSuperAdmin,
		&admin.IsActive,
		&storedCreatedAt,
		&storedUpdatedAt,
		&storedLastLoginAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get admin: %w", err)
	}

	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		admin.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
		admin.UpdatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedLastLoginAt); ok {
		admin.LastLoginAt = &parsed
	}

	if apiTokenPrefix.Valid {
		admin.APITokenPrefix = apiTokenPrefix.String
	}

	return &admin, nil
}

// GetByEmail returns a single admin by email
func (m *AdminModel) GetByEmail(email string) (*Admin, error) {
	var admin Admin
	var apiTokenPrefix sql.NullString
	// See AdminModel.GetAll for why the timestamps are scanned as raw driver
	// values and parsed in Go.
	var storedCreatedAt interface{}
	var storedUpdatedAt interface{}
	var storedLastLoginAt interface{}

	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT id, username, email, password_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at, last_login_at
		FROM server_admin_credentials
		WHERE email = ?
	`, email).Scan(
		&admin.ID,
		&admin.Username,
		&admin.Email,
		&admin.PasswordHash,
		&apiTokenPrefix,
		&admin.IsSuperAdmin,
		&admin.IsActive,
		&storedCreatedAt,
		&storedUpdatedAt,
		&storedLastLoginAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get admin by email: %w", err)
	}

	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		admin.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
		admin.UpdatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedLastLoginAt); ok {
		admin.LastLoginAt = &parsed
	}

	if apiTokenPrefix.Valid {
		admin.APITokenPrefix = apiTokenPrefix.String
	}

	return &admin, nil
}

// GetByAPIToken retrieves an admin by API token using SHA-256 hash lookup
// AI.md PART 11: Query by hash, never store or query plaintext
func (m *AdminModel) GetByAPIToken(token string) (*Admin, error) {
	// Hash the provided token to look up
	tokenHash := HashAPIToken(token)

	var admin Admin
	var apiTokenPrefix sql.NullString
	// See AdminModel.GetAll for why the timestamps are scanned as raw driver
	// values and parsed in Go.
	var storedCreatedAt interface{}
	var storedUpdatedAt interface{}
	var storedLastLoginAt interface{}

	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT id, username, email, password_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at, last_login_at
		FROM server_admin_credentials
		WHERE api_token_hash = ? AND is_active = 1
	`, tokenHash).Scan(
		&admin.ID,
		&admin.Username,
		&admin.Email,
		&admin.PasswordHash,
		&apiTokenPrefix,
		&admin.IsSuperAdmin,
		&admin.IsActive,
		&storedCreatedAt,
		&storedUpdatedAt,
		&storedLastLoginAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("admin not found or inactive")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get admin by API token: %w", err)
	}

	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		admin.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
		admin.UpdatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedLastLoginAt); ok {
		admin.LastLoginAt = &parsed
	}

	if apiTokenPrefix.Valid {
		admin.APITokenPrefix = apiTokenPrefix.String
	}

	return &admin, nil
}

// GetCount returns the total number of admins
// TEMPLATE.md Part 31: Admins can see count but not details of others
func (m *AdminModel) GetCount() (int, error) {
	var count int
	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM server_admin_credentials
	`).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count admins: %w", err)
	}

	return count, nil
}

// Create creates a new admin account with Argon2id password hashing
// Per TEMPLATE.md PART 22: Admin accounts stored in server.db
// AI.md PART 11: API tokens stored as SHA-256 hash, never plaintext
func (m *AdminModel) Create(username, email, password string, isSuperAdmin bool) (*Admin, error) {
	// Hash password using Argon2id (TEMPLATE.md PART 0 requirement)
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Generate API token
	apiToken, err := GenerateAPIToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API token: %w", err)
	}

	// Hash token for storage (NEVER store plaintext per AI.md PART 11)
	tokenHash := HashAPIToken(apiToken)
	tokenPrefix := GetAPITokenPrefix(apiToken)

	// Insert admin into server.db.
	//
	// created_at/updated_at are bound as canonical UTC text rather than left to
	// CURRENT_TIMESTAMP: that keyword yields SQLite's UTC text but a session
	// timestamp with the server's zone on PostgreSQL/MySQL, so the same column
	// would hold a different layout per backend. Binding through sqlTimestamp
	// gives every writer and reader the one layout parseStoredTimestamp expects.
	now := time.Now()
	result, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		INSERT INTO server_admin_credentials (username, email, password_hash, api_token_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
	`, username, email, passwordHash, tokenHash, tokenPrefix, isSuperAdmin, sqlTimestamp(now), sqlTimestamp(now))

	if err != nil {
		return nil, fmt.Errorf("failed to create admin: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get admin ID: %w", err)
	}

	// Create default preferences.
	// server_admin_preferences (src/database/server_schema.go) stores
	// preferences as a single JSON blob column, not individual columns.
	defaultPrefs, err := json.Marshal(AdminPreferences{
		AdminID:              id,
		Theme:                "auto",
		Language:             "en",
		Timezone:             "UTC",
		NotificationsEnabled: true,
		EmailNotifications:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode default admin preferences: %w", err)
	}

	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		INSERT INTO server_admin_preferences (admin_id, preferences, updated_at)
		VALUES (?, ?, ?)
	`, id, string(defaultPrefs), sqlTimestamp(now))
	if err != nil {
		return nil, fmt.Errorf("failed to create admin preferences: %w", err)
	}

	return m.GetByID(id)
}

// Update updates an admin's information
func (m *AdminModel) Update(id int64, username, email string, opts ...interface{}) error {
	// Support both Update(id, username, email) and Update(id, username, email, isSuperAdmin, isActive)
	if len(opts) >= 2 {
		// Full update with flags
		isSuperAdmin, ok1 := opts[0].(bool)
		isActive, ok2 := opts[1].(bool)
		if !ok1 || !ok2 {
			return fmt.Errorf("admin update flags must be booleans")
		}
		// updated_at is bound as canonical UTC text; see AdminModel.Create for why
		// CURRENT_TIMESTAMP is not portable across the supported drivers.
		_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
			UPDATE server_admin_credentials
			SET username = ?, email = ?, is_super_admin = ?, is_active = ?, updated_at = ?
			WHERE id = ?
		`, username, email, isSuperAdmin, isActive, sqlTimestamp(time.Now()), id)

		if err != nil {
			return fmt.Errorf("failed to update admin: %w", err)
		}
	} else {
		// Simple update (username and email only)
		_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
			UPDATE server_admin_credentials
			SET username = ?, email = ?, updated_at = ?
			WHERE id = ?
		`, username, email, sqlTimestamp(time.Now()), id)

		if err != nil {
			return fmt.Errorf("failed to update admin: %w", err)
		}
	}

	return nil
}

// UpdatePassword updates an admin's password using Argon2id
// Per TEMPLATE.md PART 0: MUST use Argon2id for password hashing
func (m *AdminModel) UpdatePassword(id int64, newPassword string) error {
	// Hash with Argon2id (TEMPLATE.md PART 0 requirement)
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE server_admin_credentials
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, passwordHash, sqlTimestamp(time.Now()), id)

	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// RegenerateAPIToken generates a new API token for an admin
// AI.md PART 11: Store SHA-256 hash, return full token only once
func (m *AdminModel) RegenerateAPIToken(id int64) (string, error) {
	apiToken, err := GenerateAPIToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate API token: %w", err)
	}

	// Hash token for storage (NEVER store plaintext per AI.md PART 11)
	tokenHash := HashAPIToken(apiToken)
	tokenPrefix := GetAPITokenPrefix(apiToken)

	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE server_admin_credentials
		SET api_token_hash = ?, api_token_prefix = ?, updated_at = ?
		WHERE id = ?
	`, tokenHash, tokenPrefix, sqlTimestamp(time.Now()), id)

	if err != nil {
		return "", fmt.Errorf("failed to update API token: %w", err)
	}

	return apiToken, nil
}

// RevokeAPIToken clears the stored API token for an admin.
func (m *AdminModel) RevokeAPIToken(id int64) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE server_admin_credentials
		SET api_token_hash = NULL, api_token_prefix = NULL, updated_at = ?
		WHERE id = ?
	`, sqlTimestamp(time.Now()), id)
	if err != nil {
		return fmt.Errorf("failed to revoke API token: %w", err)
	}

	return nil
}

// Delete removes an admin account
// Per TEMPLATE.md PART 22: Cannot delete the last super admin
func (m *AdminModel) Delete(id int64) error {
	// Check if this is the last super admin
	var superAdminCount int
	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM server_admin_credentials
		WHERE is_super_admin = 1 AND is_active = 1 AND id != ?
	`, id).Scan(&superAdminCount)
	if err != nil {
		return fmt.Errorf("failed to check super admin count: %w", err)
	}

	// Get the admin to check if they're a super admin
	admin, err := m.GetByID(id)
	if err != nil {
		return err
	}

	if admin.IsSuperAdmin && superAdminCount == 0 {
		return fmt.Errorf("cannot delete the last super admin account")
	}

	// Delete admin (cascades to sessions, preferences, etc.)
	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		DELETE FROM server_admin_credentials WHERE id = ?
	`, id)

	if err != nil {
		return fmt.Errorf("failed to delete admin: %w", err)
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp
func (m *AdminModel) UpdateLastLogin(id int64) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE server_admin_credentials
		SET last_login_at = ?
		WHERE id = ?
	`, sqlTimestamp(time.Now()), id)

	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// VerifyCredentials verifies username/password and returns admin if valid
// Per TEMPLATE.md PART 0: Uses Argon2id for password verification
func (m *AdminModel) VerifyCredentials(username, password string) (*Admin, error) {
	// Get admin by username (could also be email)
	admin, err := m.GetByEmail(username)
	if err != nil {
		// Try username field
		var apiTokenPrefix sql.NullString
		// See AdminModel.GetAll for why the timestamps are scanned as raw driver
		// values and parsed in Go. Here it is a login-path concern: a legacy
		// local-zone timestamp made the scan fail, and the admin could not log in
		// by username at all.
		var storedCreatedAt interface{}
		var storedUpdatedAt interface{}
		var storedLastLoginAt interface{}
		admin = &Admin{}
		err = database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
			SELECT id, username, email, password_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at, last_login_at
			FROM server_admin_credentials
			WHERE username = ? AND is_active = 1
		`, username).Scan(&admin.ID, &admin.Username, &admin.Email, &admin.PasswordHash,
			&apiTokenPrefix, &admin.IsSuperAdmin, &admin.IsActive, &storedCreatedAt,
			&storedUpdatedAt, &storedLastLoginAt)
		if apiTokenPrefix.Valid {
			admin.APITokenPrefix = apiTokenPrefix.String
		}
		if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
			admin.CreatedAt = parsed
		}
		if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
			admin.UpdatedAt = parsed
		}
		if parsed, ok := parseStoredTimestamp(storedLastLoginAt); ok {
			admin.LastLoginAt = &parsed
		}
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get admin: %w", err)
		}
	}

	// Verify password with Argon2id
	valid, err := VerifyPassword(password, admin.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to verify password: %w", err)
	}

	if !valid {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if account is active
	if !admin.IsActive {
		return nil, fmt.Errorf("account is disabled")
	}

	return admin, nil
}

// AdminInviteModel handles admin invite operations
type AdminInviteModel struct {
	DB *sql.DB
}

// getDB returns the server.db handle this model was constructed with. The only
// table AdminInviteModel touches (server_admin_invites) is declared in
// database.ServerSchema, so the injected handle is the correct database for
// every query below.
// Fallback: when the injected handle is nil (unit tests, or construction
// before the global dual DB is wired) the process-global server handle is used
// instead, so a nil handle degrades to the previous behavior rather than
// panicking.
func (m *AdminInviteModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}

	return database.GetServerDB()
}

// CreateInvite creates a new admin invite token.
func (m *AdminInviteModel) CreateInvite(email string, invitedBy int64, expiresIn time.Duration) (*AdminInvite, error) {
	// Generate secure invite token
	token, err := GenerateInviteToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate invite token: %w", err)
	}

	if expiresIn <= 0 {
		return nil, fmt.Errorf("invite expiration must be greater than zero")
	}

	expiresAt := time.Now().Add(expiresIn)

	tokenHash := HashAPIToken(token)

	// Bind expires_at as canonical UTC text (the same layout CURRENT_TIMESTAMP
	// emits). Binding a raw time.Time makes the SQLite driver serialize it as
	// time.Time.String() in the host's LOCAL zone, which no reader can compare
	// against a UTC instant without guessing the writer's offset.
	result, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		INSERT INTO server_admin_invites (token, invited_email, invited_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, tokenHash, email, invitedBy, sqlTimestamp(time.Now()), sqlTimestamp(expiresAt))

	if err != nil {
		return nil, fmt.Errorf("failed to create invite: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get invite ID: %w", err)
	}

	return &AdminInvite{
		ID:           id,
		Token:        token,
		InvitedEmail: email,
		InvitedBy:    invitedBy,
		CreatedAt:    time.Now(),
		ExpiresAt:    expiresAt,
	}, nil
}

// GetInvite retrieves an invite by token
func (m *AdminInviteModel) GetInvite(token string) (*AdminInvite, error) {
	var invite AdminInvite
	var usedBy sql.NullInt64

	// created_at, expires_at and used_at are scanned as raw driver values and
	// parsed with parseStoredTimestamp rather than scanned straight into a
	// time.Time: rows written before timestamps were normalized still hold the
	// driver's local-zone time.Time.String() layout, which a direct scan
	// rejects outright.
	var storedCreatedAt, storedExpiresAt, storedUsedAt interface{}

	// server_admin_invites has no "id" column (PK is token); use the
	// implicit SQLite rowid, matching the value CreateInvite already
	// returns via LastInsertId().
	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT rowid, token, invited_email, invited_by, created_at, expires_at, used_by, used_at
		FROM server_admin_invites
		WHERE token = ?
	`, HashAPIToken(token)).Scan(
		&invite.ID,
		&invite.Token,
		&invite.InvitedEmail,
		&invite.InvitedBy,
		&storedCreatedAt,
		&storedExpiresAt,
		&usedBy,
		&storedUsedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invite not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}

	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		invite.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedExpiresAt); ok {
		invite.ExpiresAt = parsed
	}

	if usedBy.Valid {
		invite.UsedBy = &usedBy.Int64
	}

	if parsed, ok := parseStoredTimestamp(storedUsedAt); ok {
		invite.UsedAt = &parsed
	}

	return &invite, nil
}

// MarkInviteUsed marks an invite as used
func (m *AdminInviteModel) MarkInviteUsed(token string, usedBy int64) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		UPDATE server_admin_invites
		SET used_by = ?, used_at = ?
		WHERE token = ?
	`, usedBy, sqlTimestamp(time.Now()), HashAPIToken(token))

	if err != nil {
		return fmt.Errorf("failed to mark invite as used: %w", err)
	}

	return nil
}

// DeleteExpiredInvites removes expired and used invites (cleanup)
// Per TEMPLATE.md PART 22: Clean up expired invites regularly
func (m *AdminInviteModel) DeleteExpiredInvites() error {
	// Used invites carry no timestamp comparison, so they can go in one
	// statement.
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutBulk, `
		DELETE FROM server_admin_invites WHERE used_at IS NOT NULL
	`)

	if err != nil {
		return fmt.Errorf("failed to delete used invites: %w", err)
	}

	// Expiry is decided in Go against a UTC cutoff rather than by SQLite's
	// datetime('now'): expires_at may still hold a local-zone value written by
	// an older build, and datetime() returns NULL for that layout, so the SQL
	// comparison silently never matched those rows.
	if _, err := deleteRowsWithTimestampBefore(m.getDB(), "server_admin_invites", "rowid", "expires_at", time.Now().UTC(), false); err != nil {
		return fmt.Errorf("failed to delete expired invites: %w", err)
	}

	return nil
}

// GetPendingInvites returns all pending (unused, not expired) invites
func (m *AdminInviteModel) GetPendingInvites() ([]AdminInvite, error) {
	// server_admin_invites has no "id" column (PK is token); use the
	// implicit SQLite rowid, matching GetInvite.
	// The unexpired test is applied in Go, not as SQL, for the reason described
	// in DeleteExpiredInvites: datetime(expires_at) evaluates to NULL for a
	// local-zone value and compares in the wrong direction for any value whose
	// text ordering disagrees with its true instant.
	rows, err := database.QueryContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT rowid, token, invited_email, invited_by, created_at, expires_at
		FROM server_admin_invites
		WHERE used_at IS NULL AND expires_at IS NOT NULL
		ORDER BY created_at DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to query pending invites: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	var invites []AdminInvite
	for rows.Next() {
		var invite AdminInvite
		var storedCreatedAt, storedExpiresAt interface{}
		err := rows.Scan(
			&invite.ID,
			&invite.Token,
			&invite.InvitedEmail,
			&invite.InvitedBy,
			&storedCreatedAt,
			&storedExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invite: %w", err)
		}

		expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
		if !ok || !expiresAt.After(now) {
			continue
		}
		invite.ExpiresAt = expiresAt

		if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
			invite.CreatedAt = parsed
		}

		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate pending invites: %w", err)
	}

	return invites, nil
}

// AdminSessionModel handles admin session operations
// Per TEMPLATE.md PART 22: Secure session management required
type AdminSessionModel struct {
	DB *sql.DB
}

// getDB returns the server.db handle this model was constructed with. The only
// table AdminSessionModel touches (server_admin_sessions) is declared in
// database.ServerSchema, so the injected handle is the correct database for
// every query below.
// Fallback: when the injected handle is nil (unit tests, or construction
// before the global dual DB is wired) the process-global server handle is used
// instead, so a nil handle degrades to the previous behavior rather than
// panicking.
func (m *AdminSessionModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}

	return database.GetServerDB()
}

// CreateSession creates a new admin session
func (m *AdminSessionModel) CreateSession(adminID int64, ipAddress, userAgent string, duration time.Duration) (*AdminSession, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	expiresAt := time.Now().Add(duration)

	// See AdminInviteModel.CreateInvite for why expires_at must be bound
	// as SQLite's own canonical text format rather than a raw time.Time.
	_, err = database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, adminID, ipAddress, userAgent, sqlTimestamp(time.Now()), sqlTimestamp(expiresAt))

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &AdminSession{
		ID:         0,
		AdminID:    adminID,
		SessionID:  sessionID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
		LastUsedAt: time.Now(),
	}, nil
}

// GetSession retrieves a session by session ID
// Note: server_admin_sessions has no session_id or last_used_at columns
// (see src/database/server_schema.go) — the session token IS the "id"
// primary key, and there is no separate last-used tracking column, so
// LastUsedAt is approximated from CreatedAt.
func (m *AdminSessionModel) GetSession(sessionID string) (*AdminSession, error) {
	var session AdminSession

	// created_at and expires_at are parsed with parseStoredTimestamp instead of
	// scanned directly into a time.Time so that a row written by an older build
	// in the driver's local-zone layout still resolves to the correct absolute
	// instant rather than failing the scan.
	var storedCreatedAt, storedExpiresAt interface{}

	err := database.QueryRowContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT id, admin_id, ip_address, user_agent, created_at, expires_at
		FROM server_admin_sessions
		WHERE id = ?
	`, sessionID).Scan(
		&session.SessionID,
		&session.AdminID,
		&session.IPAddress,
		&session.UserAgent,
		&storedCreatedAt,
		&storedExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		session.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedExpiresAt); ok {
		session.ExpiresAt = parsed
	}
	session.LastUsedAt = session.CreatedAt

	return &session, nil
}

// UpdateSessionLastUsed is a no-op: server_admin_sessions has no
// last_used_at column (see src/database/server_schema.go), so there is
// nothing to persist. Kept for API symmetry with UserSessionModel.
func (m *AdminSessionModel) UpdateSessionLastUsed(sessionID string) error {
	return nil
}

// DeleteSession deletes a session (logout)
func (m *AdminSessionModel) DeleteSession(sessionID string) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		DELETE FROM server_admin_sessions WHERE id = ?
	`, sessionID)

	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllSessionsForAdmin deletes all sessions for an admin (logout all)
func (m *AdminSessionModel) DeleteAllSessionsForAdmin(adminID int64) error {
	_, err := database.ExecContext(context.Background(), m.getDB(), database.TimeoutWrite, `
		DELETE FROM server_admin_sessions WHERE admin_id = ?
	`, adminID)

	if err != nil {
		return fmt.Errorf("failed to delete admin sessions: %w", err)
	}

	return nil
}

// DeleteExpiredSessions deletes all expired sessions (cleanup)
func (m *AdminSessionModel) DeleteExpiredSessions() error {
	// Expiry is decided in Go against a UTC cutoff. The SQL comparison this
	// replaces logged admins out early or late by the host's UTC offset
	// whenever expires_at held a local-zone value, and matched nothing at all
	// for layouts SQLite's datetime() cannot parse.
	if _, err := deleteRowsWithTimestampBefore(m.getDB(), "server_admin_sessions", "id", "expires_at", time.Now().UTC(), false); err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	return nil
}

// GetActiveSessions returns all active sessions for an admin
func (m *AdminSessionModel) GetActiveSessions(adminID int64) ([]AdminSession, error) {
	// "Still active" is decided in Go for the reason described in
	// DeleteExpiredSessions - the SQL comparison could not be trusted across
	// timezones or across the two on-disk layouts this column can hold.
	rows, err := database.QueryContext(context.Background(), m.getDB(), database.TimeoutSimpleSelect, `
		SELECT id, admin_id, ip_address, user_agent, created_at, expires_at
		FROM server_admin_sessions
		WHERE admin_id = ? AND expires_at IS NOT NULL
		ORDER BY created_at DESC
	`, adminID)

	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()

	var sessions []AdminSession
	for rows.Next() {
		var session AdminSession
		var storedCreatedAt, storedExpiresAt interface{}
		err := rows.Scan(
			&session.SessionID,
			&session.AdminID,
			&session.IPAddress,
			&session.UserAgent,
			&storedCreatedAt,
			&storedExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
		if !ok || !expiresAt.After(now) {
			continue
		}
		session.ExpiresAt = expiresAt

		if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
			session.CreatedAt = parsed
		}

		session.LastUsedAt = session.CreatedAt
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate sessions: %w", err)
	}

	return sessions, nil
}
