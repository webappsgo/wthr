// Package models provides data models per TEMPLATE.md PART 22
package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"github.com/casapps/wthr/src/database"
)

// Argon2id parameters per TEMPLATE.md PART 0
// CRITICAL: NEVER use bcrypt - MUST use Argon2id with these exact parameters
const (
	argon2Time    = 3
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
	ID         int64     `json:"id"`
	AdminID    int64     `json:"admin_id"`
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
	ID                   int64     `json:"id"`
	AdminID              int64     `json:"admin_id"`
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
	ID        int64     `json:"id"`
	AdminID   int64     `json:"admin_id"`
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
	ID           int64      `json:"id"`
	Token        string     `json:"token"`
	InvitedEmail string     `json:"invited_email"`
	InvitedBy    int64      `json:"invited_by"`
	CreatedAt    time.Time  `json:"created_at"`
	// MUST be 15 minutes from creation
	ExpiresAt    time.Time  `json:"expires_at"`
	UsedBy       *int64     `json:"used_by,omitempty"`
	UsedAt       *time.Time `json:"used_at,omitempty"`
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

// GetAll returns all admins (with privacy: no passwords, minimal info)
// TEMPLATE.md PART 22: Admin privacy - can't see other admin details
func (m *AdminModel) GetAll() ([]Admin, error) {
	rows, err := database.GetServerDB().Query(`
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
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&admin.ID,
			&admin.Username,
			&admin.Email,
			&admin.IsSuperAdmin,
			&admin.IsActive,
			&admin.CreatedAt,
			&admin.UpdatedAt,
			&lastLoginAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admin: %w", err)
		}

		if lastLoginAt.Valid {
			admin.LastLoginAt = &lastLoginAt.Time
		}

		admins = append(admins, admin)
	}

	return admins, nil
}

// GetByID returns a single admin by ID
func (m *AdminModel) GetByID(id int64) (*Admin, error) {
	var admin Admin
	var lastLoginAt sql.NullTime
	var apiTokenPrefix sql.NullString

	err := database.GetServerDB().QueryRow(`
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
		&admin.CreatedAt,
		&admin.UpdatedAt,
		&lastLoginAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get admin: %w", err)
	}

	if lastLoginAt.Valid {
		admin.LastLoginAt = &lastLoginAt.Time
	}

	if apiTokenPrefix.Valid {
		admin.APITokenPrefix = apiTokenPrefix.String
	}

	return &admin, nil
}

// GetByEmail returns a single admin by email
func (m *AdminModel) GetByEmail(email string) (*Admin, error) {
	var admin Admin
	var lastLoginAt sql.NullTime
	var apiTokenPrefix sql.NullString

	err := database.GetServerDB().QueryRow(`
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
		&admin.CreatedAt,
		&admin.UpdatedAt,
		&lastLoginAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get admin by email: %w", err)
	}

	if lastLoginAt.Valid {
		admin.LastLoginAt = &lastLoginAt.Time
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
	var lastLoginAt sql.NullTime
	var apiTokenPrefix sql.NullString

	err := database.GetServerDB().QueryRow(`
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
		&admin.CreatedAt,
		&admin.UpdatedAt,
		&lastLoginAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("admin not found or inactive")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get admin by API token: %w", err)
	}

	if lastLoginAt.Valid {
		admin.LastLoginAt = &lastLoginAt.Time
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
	err := database.GetServerDB().QueryRow(`
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

	// Insert admin into server.db
	result, err := database.GetServerDB().Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, api_token_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, username, email, passwordHash, tokenHash, tokenPrefix, isSuperAdmin)

	if err != nil {
		return nil, fmt.Errorf("failed to create admin: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get admin ID: %w", err)
	}

	// Create default preferences
	_, err = database.GetServerDB().Exec(`
		INSERT INTO server_admin_preferences (admin_id, theme, language, timezone, notifications_enabled, email_notifications, created_at, updated_at)
		VALUES (?, 'auto', 'en', 'UTC', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, id)
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
		isSuperAdmin := opts[0].(bool)
		isActive := opts[1].(bool)
		_, err := database.GetServerDB().Exec(`
			UPDATE server_admin_credentials
			SET username = ?, email = ?, is_super_admin = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, email, isSuperAdmin, isActive, id)

		if err != nil {
			return fmt.Errorf("failed to update admin: %w", err)
		}
	} else {
		// Simple update (username and email only)
		_, err := database.GetServerDB().Exec(`
			UPDATE server_admin_credentials
			SET username = ?, email = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, email, id)

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

	_, err = database.GetServerDB().Exec(`
		UPDATE server_admin_credentials
		SET password_hash = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, passwordHash, id)

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

	_, err = database.GetServerDB().Exec(`
		UPDATE server_admin_credentials
		SET api_token_hash = ?, api_token_prefix = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, tokenHash, tokenPrefix, id)

	if err != nil {
		return "", fmt.Errorf("failed to update API token: %w", err)
	}

	return apiToken, nil
}

// RevokeAPIToken clears the stored API token for an admin.
func (m *AdminModel) RevokeAPIToken(id int64) error {
	_, err := database.GetServerDB().Exec(`
		UPDATE server_admin_credentials
		SET api_token_hash = NULL, api_token_prefix = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
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
	err := database.GetServerDB().QueryRow(`
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
	_, err = database.GetServerDB().Exec(`
		DELETE FROM server_admin_credentials WHERE id = ?
	`, id)

	if err != nil {
		return fmt.Errorf("failed to delete admin: %w", err)
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp
func (m *AdminModel) UpdateLastLogin(id int64) error {
	_, err := database.GetServerDB().Exec(`
		UPDATE server_admin_credentials
		SET last_login_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)

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
		admin = &Admin{}
		err = database.GetServerDB().QueryRow(`
			SELECT id, username, email, password_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at, last_login_at
			FROM server_admin_credentials
			WHERE username = ? AND is_active = 1
		`, username).Scan(&admin.ID, &admin.Username, &admin.Email, &admin.PasswordHash,
			&apiTokenPrefix, &admin.IsSuperAdmin, &admin.IsActive, &admin.CreatedAt,
			&admin.UpdatedAt, &admin.LastLoginAt)
		if apiTokenPrefix.Valid {
			admin.APITokenPrefix = apiTokenPrefix.String
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

	result, err := database.GetServerDB().Exec(`
		INSERT INTO server_admin_invites (token, invited_email, invited_by, created_at, expires_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
	`, token, email, invitedBy, expiresAt)

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
	var usedAt sql.NullTime

	err := database.GetServerDB().QueryRow(`
		SELECT id, token, invited_email, invited_by, created_at, expires_at, used_by, used_at
		FROM server_admin_invites
		WHERE token = ?
	`, token).Scan(
		&invite.ID,
		&invite.Token,
		&invite.InvitedEmail,
		&invite.InvitedBy,
		&invite.CreatedAt,
		&invite.ExpiresAt,
		&usedBy,
		&usedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invite not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}

	if usedBy.Valid {
		invite.UsedBy = &usedBy.Int64
	}

	if usedAt.Valid {
		invite.UsedAt = &usedAt.Time
	}

	return &invite, nil
}

// MarkInviteUsed marks an invite as used
func (m *AdminInviteModel) MarkInviteUsed(token string, usedBy int64) error {
	_, err := database.GetServerDB().Exec(`
		UPDATE server_admin_invites
		SET used_by = ?, used_at = CURRENT_TIMESTAMP
		WHERE token = ?
	`, usedBy, token)

	if err != nil {
		return fmt.Errorf("failed to mark invite as used: %w", err)
	}

	return nil
}

// DeleteExpiredInvites removes expired and used invites (cleanup)
// Per TEMPLATE.md PART 22: Clean up expired invites regularly
func (m *AdminInviteModel) DeleteExpiredInvites() error {
	_, err := database.GetServerDB().Exec(`
		DELETE FROM server_admin_invites
		WHERE expires_at < CURRENT_TIMESTAMP OR used_at IS NOT NULL
	`)

	if err != nil {
		return fmt.Errorf("failed to delete expired invites: %w", err)
	}

	return nil
}

// GetPendingInvites returns all pending (unused, not expired) invites
func (m *AdminInviteModel) GetPendingInvites() ([]AdminInvite, error) {
	rows, err := database.GetServerDB().Query(`
		SELECT id, token, invited_email, invited_by, created_at, expires_at
		FROM server_admin_invites
		WHERE used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to query pending invites: %w", err)
	}
	defer rows.Close()

	var invites []AdminInvite
	for rows.Next() {
		var invite AdminInvite
		err := rows.Scan(
			&invite.ID,
			&invite.Token,
			&invite.InvitedEmail,
			&invite.InvitedBy,
			&invite.CreatedAt,
			&invite.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invite: %w", err)
		}
		invites = append(invites, invite)
	}

	return invites, nil
}

// AdminSessionModel handles admin session operations
// Per TEMPLATE.md PART 22: Secure session management required
type AdminSessionModel struct {
	DB *sql.DB
}

// CreateSession creates a new admin session
func (m *AdminSessionModel) CreateSession(adminID int64, ipAddress, userAgent string, duration time.Duration) (*AdminSession, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	expiresAt := time.Now().Add(duration)

	_, err = database.GetServerDB().Exec(`
		INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, created_at, expires_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
	`, sessionID, adminID, ipAddress, userAgent, expiresAt)

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
func (m *AdminSessionModel) GetSession(sessionID string) (*AdminSession, error) {
	var session AdminSession

	err := database.GetServerDB().QueryRow(`
		SELECT id, admin_id, session_id, ip_address, user_agent, created_at, expires_at, last_used_at
		FROM server_admin_sessions
		WHERE session_id = ?
	`, sessionID).Scan(
		&session.ID,
		&session.AdminID,
		&session.SessionID,
		&session.IPAddress,
		&session.UserAgent,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.LastUsedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &session, nil
}

// UpdateSessionLastUsed updates the last used timestamp
func (m *AdminSessionModel) UpdateSessionLastUsed(sessionID string) error {
	_, err := database.GetServerDB().Exec(`
		UPDATE server_admin_sessions
		SET last_used_at = CURRENT_TIMESTAMP
		WHERE session_id = ?
	`, sessionID)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// DeleteSession deletes a session (logout)
func (m *AdminSessionModel) DeleteSession(sessionID string) error {
	_, err := database.GetServerDB().Exec(`
		DELETE FROM server_admin_sessions WHERE session_id = ?
	`, sessionID)

	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllSessionsForAdmin deletes all sessions for an admin (logout all)
func (m *AdminSessionModel) DeleteAllSessionsForAdmin(adminID int64) error {
	_, err := database.GetServerDB().Exec(`
		DELETE FROM server_admin_sessions WHERE admin_id = ?
	`, adminID)

	if err != nil {
		return fmt.Errorf("failed to delete admin sessions: %w", err)
	}

	return nil
}

// DeleteExpiredSessions deletes all expired sessions (cleanup)
func (m *AdminSessionModel) DeleteExpiredSessions() error {
	_, err := database.GetServerDB().Exec(`
		DELETE FROM server_admin_sessions WHERE expires_at < CURRENT_TIMESTAMP
	`)

	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	return nil
}

// GetActiveSessions returns all active sessions for an admin
func (m *AdminSessionModel) GetActiveSessions(adminID int64) ([]AdminSession, error) {
	rows, err := database.GetServerDB().Query(`
		SELECT id, admin_id, session_id, ip_address, user_agent, created_at, expires_at, last_used_at
		FROM server_admin_sessions
		WHERE admin_id = ? AND expires_at > CURRENT_TIMESTAMP
		ORDER BY last_used_at DESC
	`, adminID)

	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []AdminSession
	for rows.Next() {
		var session AdminSession
		err := rows.Scan(
			&session.ID,
			&session.AdminID,
			&session.SessionID,
			&session.IPAddress,
			&session.UserAgent,
			&session.CreatedAt,
			&session.ExpiresAt,
			&session.LastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}
