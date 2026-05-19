// Package models provides user models per TEMPLATE.md PART 10
package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/casapps/wthr/src/database"
)

// User represents a user account (stored in users.db)
// Per AI.md PART 9: Users MUST be in users.db, NOT server.db
// Per AI.md PART 34: Multi-user support with full profile fields
type User struct {
	ID              int64      `json:"id"`
	Username        string     `json:"username"`
	DisplayName     string     `json:"display_name,omitempty"`
	// Account email - used for security & account recovery
	// Per AI.md PART 34: Password reset, 2FA recovery, security alerts
	Email           string     `json:"email"`
	// Notification email - used for non-security communications
	// Per AI.md PART 34: Newsletters, updates, marketing, general notifications
	NotificationEmail string   `json:"notification_email,omitempty"`
	Phone           string     `json:"phone,omitempty"`
	// Never serialize password hash
	PasswordHash    string     `json:"-"`
	EmailVerified   bool       `json:"email_verified"`
	IsActive        bool       `json:"is_active"`
	IsBanned        bool       `json:"is_banned"`
	BanReason       string     `json:"ban_reason,omitempty"`
	// user or admin
	Role            string     `json:"role"`
	// Profile visibility: public or private
	// Per AI.md PART 34: Private profiles hidden from search/listings/public pages
	Visibility      string     `json:"visibility"`
	TwoFactorEnabled bool      `json:"two_factor_enabled"`
	// Never serialize 2FA secret
	TwoFactorSecret string     `json:"-"`
	// Avatar settings per AI.md PART 34
	AvatarType      string     `json:"avatar_type,omitempty"`  // gravatar, upload, url
	AvatarURL       string     `json:"avatar_url,omitempty"`   // URL or path
	// Profile fields per AI.md PART 34
	Bio             string     `json:"bio,omitempty"`          // Short biography (max 500 chars)
	Location        string     `json:"location,omitempty"`     // Location (free text)
	Website         string     `json:"website,omitempty"`      // Personal website URL
	Timezone        string     `json:"timezone,omitempty"`     // IANA timezone
	Language        string     `json:"language,omitempty"`     // Preferred language
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP     string     `json:"last_login_ip,omitempty"`
}

// UserSession represents an active user session
// Per TEMPLATE.md PART 10: Session-based authentication for users
type UserSession struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	// Secure random token
	SessionID  string    `json:"session_id"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// UserPreferences stores user-specific settings
type UserPreferences struct {
	ID                   int64     `json:"id"`
	UserID               int64     `json:"user_id"`
	// light, dark, auto
	Theme                string    `json:"theme"`
	Language             string    `json:"language"`
	Timezone             string    `json:"timezone"`
	// celsius, fahrenheit
	TemperatureUnit      string    `json:"temperature_unit"`
	// hPa, inHg, mmHg
	PressureUnit         string    `json:"pressure_unit"`
	// kmh, mph, ms, kts
	WindSpeedUnit        string    `json:"wind_speed_unit"`
	// mm, in
	PrecipitationUnit    string    `json:"precipitation_unit"`
	NotificationsEnabled bool      `json:"notifications_enabled"`
	EmailNotifications   bool      `json:"email_notifications"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// UserEmailVerification represents an email verification token
type UserEmailVerification struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Token     string     `json:"token"`
	Email     string     `json:"email"`
	CreatedAt time.Time  `json:"created_at"`
	// 24 hours from creation
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// UserPasswordReset represents a password reset token
type UserPasswordReset struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Token     string     `json:"token"`
	CreatedAt time.Time  `json:"created_at"`
	// 1 hour from creation
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// UserActivityLog represents user activity logging
type UserActivityLog struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	// login, logout, password_change, etc.
	ActivityType string    `json:"activity_type"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	Details      string    `json:"details,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SessionExpired checks if a user session has expired
func (s *UserSession) SessionExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsExpired checks if an email verification token has expired
func (v *UserEmailVerification) IsExpired() bool {
	return time.Now().After(v.ExpiresAt)
}

// IsUsed checks if an email verification token has been used
func (v *UserEmailVerification) IsUsed() bool {
	return v.UsedAt != nil
}

// IsExpired checks if a password reset token has expired
func (r *UserPasswordReset) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// IsUsed checks if a password reset token has been used
func (r *UserPasswordReset) IsUsed() bool {
	return r.UsedAt != nil
}

// UserModel handles user database operations
// Per TEMPLATE.md PART 9: All operations MUST use users.db
type UserModel struct {
	DB *sql.DB
}

// Create creates a new user account
// Per TEMPLATE.md PART 0: Uses Argon2id for password hashing
func (m *UserModel) Create(username, email, password string, role ...string) (*User, error) {
	// Default role to "user" if not provided
	userRole := "user"
	if len(role) > 0 && role[0] != "" {
		userRole = role[0]
	}

	// Hash password using Argon2id (TEMPLATE.md PART 0 requirement)
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user into users.db
	result, err := database.GetUsersDB().Exec(`
		INSERT INTO user_accounts (username, email, password_hash, role, email_verified, is_active, is_banned, two_factor_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, 1, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, username, email, passwordHash, userRole)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	// Create default preferences
	_, err = database.GetUsersDB().Exec(`
		INSERT INTO user_preferences (user_id, theme, language, timezone, temperature_unit, pressure_unit, wind_speed_unit, precipitation_unit, notifications_enabled, email_notifications, created_at, updated_at)
		VALUES (?, 'auto', 'en', 'UTC', 'celsius', 'hPa', 'kmh', 'mm', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	return m.GetByID(userID)
}

// GetByID retrieves a user by ID
func (m *UserModel) GetByID(id int64) (*User, error) {
	var user User
	var lastLoginAt sql.NullTime
	var displayName sql.NullString
	var notificationEmail sql.NullString
	var phone sql.NullString
	var banReason sql.NullString
	var visibility sql.NullString
	var twoFactorSecret sql.NullString
	var avatarType sql.NullString
	var avatarURL sql.NullString
	var bio sql.NullString
	var location sql.NullString
	var website sql.NullString
	var timezone sql.NullString
	var language sql.NullString
	var lastLoginIP sql.NullString

	err := database.GetUsersDB().QueryRow(`
		SELECT id, username, display_name, notification_email, email, phone, password_hash,
		       email_verified, is_active, is_banned, ban_reason, role, visibility,
		       two_factor_enabled, two_factor_secret, avatar_type, avatar_url, bio,
		       location, website, timezone, language, created_at, updated_at,
		       last_login_at, last_login_ip
		FROM user_accounts
		WHERE id = ?
	`, id).Scan(
		&user.ID,
		&user.Username,
		&displayName,
		&notificationEmail,
		&user.Email,
		&phone,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.IsActive,
		&user.IsBanned,
		&banReason,
		&user.Role,
		&visibility,
		&user.TwoFactorEnabled,
		&twoFactorSecret,
		&avatarType,
		&avatarURL,
		&bio,
		&location,
		&website,
		&timezone,
		&language,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
		&lastLoginIP,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if lastLoginIP.Valid {
		user.LastLoginIP = lastLoginIP.String
	}
	if banReason.Valid {
		user.BanReason = banReason.String
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	if notificationEmail.Valid {
		user.NotificationEmail = notificationEmail.String
	}
	if phone.Valid {
		user.Phone = phone.String
	}
	if visibility.Valid {
		user.Visibility = visibility.String
	}
	if twoFactorSecret.Valid {
		user.TwoFactorSecret = twoFactorSecret.String
	}
	if avatarType.Valid {
		user.AvatarType = avatarType.String
	}
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	if bio.Valid {
		user.Bio = bio.String
	}
	if location.Valid {
		user.Location = location.String
	}
	if website.Valid {
		user.Website = website.String
	}
	if timezone.Valid {
		user.Timezone = timezone.String
	}
	if language.Valid {
		user.Language = language.String
	}

	return &user, nil
}

// GetByUsername retrieves a user by username
func (m *UserModel) GetByUsername(username string) (*User, error) {
	var user User
	var lastLoginAt sql.NullTime
	var lastLoginIP, banReason, twoFactorSecret sql.NullString

	err := database.GetUsersDB().QueryRow(`
		SELECT id, username, email, password_hash, email_verified, is_active, is_banned, ban_reason, two_factor_enabled, two_factor_secret, created_at, updated_at, last_login_at, last_login_ip
		FROM user_accounts
		WHERE username = ?
	`, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.IsActive,
		&user.IsBanned,
		&banReason,
		&user.TwoFactorEnabled,
		&twoFactorSecret,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
		&lastLoginIP,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if lastLoginIP.Valid {
		user.LastLoginIP = lastLoginIP.String
	}
	if banReason.Valid {
		user.BanReason = banReason.String
	}
	if twoFactorSecret.Valid {
		user.TwoFactorSecret = twoFactorSecret.String
	}

	return &user, nil
}

// GetByEmail retrieves a user by email
func (m *UserModel) GetByEmail(email string) (*User, error) {
	var user User
	var lastLoginAt sql.NullTime
	var lastLoginIP, banReason, twoFactorSecret sql.NullString

	err := database.GetUsersDB().QueryRow(`
		SELECT id, username, email, password_hash, email_verified, is_active, is_banned, ban_reason, two_factor_enabled, two_factor_secret, created_at, updated_at, last_login_at, last_login_ip
		FROM user_accounts
		WHERE email = ?
	`, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.IsActive,
		&user.IsBanned,
		&banReason,
		&user.TwoFactorEnabled,
		&twoFactorSecret,
		&user.CreatedAt,
		&user.UpdatedAt,
		&lastLoginAt,
		&lastLoginIP,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if lastLoginIP.Valid {
		user.LastLoginIP = lastLoginIP.String
	}
	if banReason.Valid {
		user.BanReason = banReason.String
	}
	if twoFactorSecret.Valid {
		user.TwoFactorSecret = twoFactorSecret.String
	}

	return &user, nil
}

// ListUsers retrieves all users with pagination
func (m *UserModel) ListUsers(offset, limit int) ([]*User, error) {
	rows, err := database.GetUsersDB().Query(`
		SELECT id, username, email, email_verified, is_active, is_banned, ban_reason, two_factor_enabled, created_at, updated_at, last_login_at, last_login_ip
		FROM user_accounts
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		var lastLoginAt sql.NullTime
		var lastLoginIP, banReason sql.NullString

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.EmailVerified,
			&user.IsActive,
			&user.IsBanned,
			&banReason,
			&user.TwoFactorEnabled,
			&user.CreatedAt,
			&user.UpdatedAt,
			&lastLoginAt,
			&lastLoginIP,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}
		if lastLoginIP.Valid {
			user.LastLoginIP = lastLoginIP.String
		}
		if banReason.Valid {
			user.BanReason = banReason.String
		}

		users = append(users, &user)
	}

	return users, nil
}

// CountUsers returns the total number of users
func (m *UserModel) CountUsers() (int, error) {
	var count int
	err := database.GetUsersDB().QueryRow(`SELECT COUNT(*) FROM user_accounts`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return count, nil
}

// Update updates a user's information
func (m *UserModel) Update(id int64, username, email string, role ...string) error {
	// If role is provided, update it too
	if len(role) > 0 && role[0] != "" {
		_, err := database.GetUsersDB().Exec(`
			UPDATE user_accounts
			SET username = ?, email = ?, role = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, email, role[0], id)

		if err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}
	} else {
		_, err := database.GetUsersDB().Exec(`
			UPDATE user_accounts
			SET username = ?, email = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, email, id)

		if err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}
	}

	return nil
}

// UpdatePassword updates a user's password using Argon2id
// Per TEMPLATE.md PART 0: MUST use Argon2id for password hashing
func (m *UserModel) UpdatePassword(id int64, newPassword string) error {
	// Hash with Argon2id (TEMPLATE.md PART 0 requirement)
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET password_hash = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, passwordHash, id)

	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp and IP
func (m *UserModel) UpdateLastLogin(id int64, ipAddress string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET last_login_at = CURRENT_TIMESTAMP, last_login_ip = ?
		WHERE id = ?
	`, ipAddress, id)

	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// VerifyEmail marks a user's email as verified
func (m *UserModel) VerifyEmail(id int64) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET email_verified = 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)

	if err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	return nil
}

// SetActive sets the user's active status
func (m *UserModel) SetActive(id int64, isActive bool) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, isActive, id)

	if err != nil {
		return fmt.Errorf("failed to set active status: %w", err)
	}

	return nil
}

// BanUser bans a user with a reason
func (m *UserModel) BanUser(id int64, reason string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET is_banned = 1, ban_reason = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, reason, id)

	if err != nil {
		return fmt.Errorf("failed to ban user: %w", err)
	}

	return nil
}

// UnbanUser unbans a user
func (m *UserModel) UnbanUser(id int64) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET is_banned = 0, ban_reason = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)

	if err != nil {
		return fmt.Errorf("failed to unban user: %w", err)
	}

	return nil
}

// Delete deletes a user account
func (m *UserModel) Delete(id int64) error {
	// Delete user (cascades to sessions, preferences, etc.)
	_, err := database.GetUsersDB().Exec(`DELETE FROM user_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// VerifyCredentials verifies username/password and returns user if valid
// Per TEMPLATE.md PART 0: Uses Argon2id for password verification
func (m *UserModel) VerifyCredentials(username, password string) (*User, error) {
	// Get user by username or email
	user, err := m.GetByUsername(username)
	if err != nil {
		// Try email
		user, err = m.GetByEmail(username)
		if err != nil {
			return nil, fmt.Errorf("invalid credentials")
		}
	}

	// Verify password with Argon2id
	valid, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to verify password: %w", err)
	}

	if !valid {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if account is active
	if !user.IsActive {
		return nil, fmt.Errorf("account is disabled")
	}

	// Check if account is banned
	if user.IsBanned {
		return nil, fmt.Errorf("account is banned: %s", user.BanReason)
	}

	return user, nil
}

// Enable2FA enables two-factor authentication for a user
func (m *UserModel) Enable2FA(id int64, secret string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET two_factor_enabled = 1, two_factor_secret = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, secret, id)

	if err != nil {
		return fmt.Errorf("failed to enable 2FA: %w", err)
	}

	return nil
}

// Disable2FA disables two-factor authentication for a user
func (m *UserModel) Disable2FA(id int64) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET two_factor_enabled = 0, two_factor_secret = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)

	if err != nil {
		return fmt.Errorf("failed to disable 2FA: %w", err)
	}

	return nil
}

// GetAll returns all users (alias for ListUsers with no pagination)
func (m *UserModel) GetAll() ([]*User, error) {
	return m.ListUsers(0, 10000)
}

// Count returns total number of users (alias for CountUsers)
func (m *UserModel) Count() (int, error) {
	return m.CountUsers()
}

// CountByRole returns count of users by role
func (m *UserModel) CountByRole(role string) (int, error) {
	query := `SELECT COUNT(*) FROM user_accounts WHERE role = ?`
	var count int
	err := m.DB.QueryRow(query, role).Scan(&count)
	return count, err
}

// GetByIdentifier finds user by username or email
func (m *UserModel) GetByIdentifier(identifier string) (*User, error) {
	// Try username first
	user, err := m.GetByUsername(identifier)
	if err == nil {
		return user, nil
	}

	// Try email
	return m.GetByEmail(identifier)
}

// CheckPassword verifies a user's password
func (m *UserModel) CheckPassword(user *User, password string) bool {
	valid, err := VerifyPassword(password, user.PasswordHash)
	return err == nil && valid
}

// UpdateProfile updates current-user profile information.
func (m *UserModel) UpdateProfile(id int64, displayName, phone string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_accounts
		SET display_name = ?, phone = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, displayName, phone, id)

	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	return nil
}

// EnableTwoFactor is an alias for Enable2FA
func (m *UserModel) EnableTwoFactor(id int64, secret string) error {
	return m.Enable2FA(id, secret)
}

// DisableTwoFactor is an alias for Disable2FA
func (m *UserModel) DisableTwoFactor(id int64) error {
	return m.Disable2FA(id)
}

// UserSessionModel handles user session operations
// Per TEMPLATE.md PART 10: Session-based authentication required
type UserSessionModel struct {
	DB *sql.DB
}

// CreateSession creates a new user session
func (m *UserSessionModel) CreateSession(userID int64, ipAddress, userAgent string, duration time.Duration) (*UserSession, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	expiresAt := time.Now().Add(duration)

	result, err := database.GetUsersDB().Exec(`
		INSERT INTO user_sessions (user_id, session_id, ip_address, user_agent, created_at, expires_at, last_used_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?, CURRENT_TIMESTAMP)
	`, userID, sessionID, ipAddress, userAgent, expiresAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get session ID: %w", err)
	}

	return &UserSession{
		ID:         id,
		UserID:     userID,
		SessionID:  sessionID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
		LastUsedAt: time.Now(),
	}, nil
}

// GetSession retrieves a session by session ID
func (m *UserSessionModel) GetSession(sessionID string) (*UserSession, error) {
	var session UserSession

	err := database.GetUsersDB().QueryRow(`
		SELECT id, user_id, session_id, ip_address, user_agent, created_at, expires_at, last_used_at
		FROM user_sessions
		WHERE session_id = ?
	`, sessionID).Scan(
		&session.ID,
		&session.UserID,
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
func (m *UserSessionModel) UpdateSessionLastUsed(sessionID string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_sessions
		SET last_used_at = CURRENT_TIMESTAMP
		WHERE session_id = ?
	`, sessionID)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// DeleteSession deletes a session (logout)
func (m *UserSessionModel) DeleteSession(sessionID string) error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM user_sessions WHERE session_id = ?
	`, sessionID)

	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllSessionsForUser deletes all sessions for a user (logout all)
func (m *UserSessionModel) DeleteAllSessionsForUser(userID int64) error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM user_sessions WHERE user_id = ?
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

// DeleteExpiredSessions deletes all expired sessions (cleanup)
func (m *UserSessionModel) DeleteExpiredSessions() error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM user_sessions WHERE expires_at < CURRENT_TIMESTAMP
	`)

	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	return nil
}

// GetActiveSessions returns all active sessions for a user
func (m *UserSessionModel) GetActiveSessions(userID int64) ([]UserSession, error) {
	rows, err := database.GetUsersDB().Query(`
		SELECT id, user_id, session_id, ip_address, user_agent, created_at, expires_at, last_used_at
		FROM user_sessions
		WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP
		ORDER BY last_used_at DESC
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []UserSession
	for rows.Next() {
		var session UserSession
		err := rows.Scan(
			&session.ID,
			&session.UserID,
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

// UserEmailVerificationModel handles email verification operations
type UserEmailVerificationModel struct {
	DB *sql.DB
}

// CreateVerification creates a new email verification token (24-hour expiry)
func (m *UserEmailVerificationModel) CreateVerification(userID int64, email string) (*UserEmailVerification, error) {
	// Generate secure token
	token, err := GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// 24-hour expiry
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenHash := HashAPIToken(token)

	result, err := database.GetUsersDB().Exec(`
		INSERT INTO user_email_verifications (user_id, token, email, created_at, expires_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
	`, userID, tokenHash, email, expiresAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create verification: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get verification ID: %w", err)
	}

	return &UserEmailVerification{
		ID:        id,
		UserID:    userID,
		Token:     token,
		Email:     email,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}, nil
}

// GetVerification retrieves a verification by token
func (m *UserEmailVerificationModel) GetVerification(token string) (*UserEmailVerification, error) {
	var verification UserEmailVerification
	var usedAt sql.NullTime

	err := database.GetUsersDB().QueryRow(`
		SELECT id, user_id, token, email, created_at, expires_at, used_at
		FROM user_email_verifications
		WHERE token = ?
	`, HashAPIToken(token)).Scan(
		&verification.ID,
		&verification.UserID,
		&verification.Token,
		&verification.Email,
		&verification.CreatedAt,
		&verification.ExpiresAt,
		&usedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("verification not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get verification: %w", err)
	}

	if usedAt.Valid {
		verification.UsedAt = &usedAt.Time
	}

	return &verification, nil
}

// MarkVerificationUsed marks a verification as used
func (m *UserEmailVerificationModel) MarkVerificationUsed(token string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_email_verifications
		SET used_at = CURRENT_TIMESTAMP
		WHERE token = ?
	`, HashAPIToken(token))

	if err != nil {
		return fmt.Errorf("failed to mark verification as used: %w", err)
	}

	return nil
}

// DeleteExpiredVerifications deletes expired and used verifications (cleanup)
func (m *UserEmailVerificationModel) DeleteExpiredVerifications() error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM user_email_verifications
		WHERE expires_at < CURRENT_TIMESTAMP OR used_at IS NOT NULL
	`)

	if err != nil {
		return fmt.Errorf("failed to delete expired verifications: %w", err)
	}

	return nil
}

// UserPasswordResetModel handles password reset operations
type UserPasswordResetModel struct {
	DB *sql.DB
}

// CreateReset creates a new password reset token (1-hour expiry)
func (m *UserPasswordResetModel) CreateReset(userID int64) (*UserPasswordReset, error) {
	// Generate secure token
	token, err := GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// 1-hour expiry
	expiresAt := time.Now().Add(1 * time.Hour)

	tokenHash := HashAPIToken(token)

	result, err := database.GetUsersDB().Exec(`
		INSERT INTO user_password_resets (user_id, token, created_at, expires_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
	`, userID, tokenHash, expiresAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create reset: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get reset ID: %w", err)
	}

	return &UserPasswordReset{
		ID:        id,
		UserID:    userID,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}, nil
}

// GetReset retrieves a reset by token
func (m *UserPasswordResetModel) GetReset(token string) (*UserPasswordReset, error) {
	var reset UserPasswordReset
	var usedAt sql.NullTime

	err := database.GetUsersDB().QueryRow(`
		SELECT id, user_id, token, created_at, expires_at, used_at
		FROM user_password_resets
		WHERE token = ?
	`, HashAPIToken(token)).Scan(
		&reset.ID,
		&reset.UserID,
		&reset.Token,
		&reset.CreatedAt,
		&reset.ExpiresAt,
		&usedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("reset not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reset: %w", err)
	}

	if usedAt.Valid {
		reset.UsedAt = &usedAt.Time
	}

	return &reset, nil
}

// MarkResetUsed marks a reset as used
func (m *UserPasswordResetModel) MarkResetUsed(token string) error {
	_, err := database.GetUsersDB().Exec(`
		UPDATE user_password_resets
		SET used_at = CURRENT_TIMESTAMP
		WHERE token = ?
	`, HashAPIToken(token))

	if err != nil {
		return fmt.Errorf("failed to mark reset as used: %w", err)
	}

	return nil
}

// DeleteExpiredResets deletes expired and used resets (cleanup)
func (m *UserPasswordResetModel) DeleteExpiredResets() error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM user_password_resets
		WHERE expires_at < CURRENT_TIMESTAMP OR used_at IS NOT NULL
	`)

	if err != nil {
		return fmt.Errorf("failed to delete expired resets: %w", err)
	}

	return nil
}

// UserActivityLogModel handles activity logging
type UserActivityLogModel struct {
	DB *sql.DB
}

// LogActivity logs a user activity
func (m *UserActivityLogModel) LogActivity(userID int64, activityType, ipAddress, userAgent, details string) error {
	_, err := database.GetUsersDB().Exec(`
		INSERT INTO user_activity_log (user_id, activity_type, ip_address, user_agent, details, created_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, userID, activityType, ipAddress, userAgent, details)

	if err != nil {
		return fmt.Errorf("failed to log activity: %w", err)
	}

	return nil
}

// GetActivities retrieves activities for a user with pagination
func (m *UserActivityLogModel) GetActivities(userID int64, offset, limit int) ([]UserActivityLog, error) {
	rows, err := database.GetUsersDB().Query(`
		SELECT id, user_id, activity_type, ip_address, user_agent, details, created_at
		FROM user_activity_log
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to query activities: %w", err)
	}
	defer rows.Close()

	var activities []UserActivityLog
	for rows.Next() {
		var activity UserActivityLog
		var details sql.NullString

		err := rows.Scan(
			&activity.ID,
			&activity.UserID,
			&activity.ActivityType,
			&activity.IPAddress,
			&activity.UserAgent,
			&details,
			&activity.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}

		if details.Valid {
			activity.Details = details.String
		}

		activities = append(activities, activity)
	}

	return activities, nil
}

// DeleteOldActivities deletes activities older than specified duration
func (m *UserActivityLogModel) DeleteOldActivities(days int) error {
	_, err := database.GetUsersDB().Exec(`
		DELETE FROM user_activity_log
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`, days)

	if err != nil {
		return fmt.Errorf("failed to delete old activities: %w", err)
	}

	return nil
}

// UserInvite represents a user registration invite per AI.md PART 33
type UserInvite struct {
	ID        int64      `json:"id"`
	Token     string     `json:"token"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	CreatedBy int64      `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	MaxUses   int        `json:"max_uses"`
	UseCount  int        `json:"use_count"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// UserInviteModel handles user invite operations per AI.md PART 33
type UserInviteModel struct {
	DB *sql.DB
}

func (m *UserInviteModel) getDB() *sql.DB {
	if m.DB != nil {
		return m.DB
	}

	return database.GetUsersDB()
}

func (m *UserInviteModel) ensureInviteSchema() error {
	rows, err := m.getDB().Query(`PRAGMA table_info(user_invites)`)
	if err != nil {
		return fmt.Errorf("failed to inspect user_invites schema: %w", err)
	}
	defer rows.Close()

	hasUsername := false
	hasRole := false

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("failed to scan user_invites schema: %w", err)
		}

		switch name {
		case "username":
			hasUsername = true
		case "role":
			hasRole = true
		}
	}

	if !hasUsername {
		if _, err := m.getDB().Exec(`ALTER TABLE user_invites ADD COLUMN username TEXT`); err != nil {
			return fmt.Errorf("failed to add user_invites.username: %w", err)
		}
	}

	if !hasRole {
		if _, err := m.getDB().Exec(`ALTER TABLE user_invites ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`); err != nil {
			return fmt.Errorf("failed to add user_invites.role: %w", err)
		}
	}

	return nil
}

// CreateInvite creates a new user invite
func (m *UserInviteModel) CreateInvite(username, email, role string, expiresInDays int) (*UserInvite, error) {
	if err := m.ensureInviteSchema(); err != nil {
		return nil, err
	}

	// Generate random token
	token, err := GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	if expiresInDays <= 0 {
		return nil, fmt.Errorf("invite expiration must be greater than zero")
	}

	if role == "" {
		role = "user"
	}

	expiresAt := time.Now().Add(time.Duration(expiresInDays) * 24 * time.Hour)

	tokenHash := HashAPIToken(token)

	result, err := m.getDB().Exec(`
		INSERT INTO user_invites (code, email, invited_by, created_at, expires_at, used_by, used_at, max_uses, use_count, username, role)
		VALUES (?, ?, NULL, CURRENT_TIMESTAMP, ?, NULL, NULL, 1, 0, ?, ?)
	`, tokenHash, email, expiresAt, username, role)

	if err != nil {
		return nil, fmt.Errorf("failed to create invite: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get invite id: %w", err)
	}

	return &UserInvite{
		ID:        id,
		Token:     token,
		Username:  username,
		Email:     email,
		Role:      role,
		CreatedBy: 0,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		MaxUses:   1,
		UseCount:  0,
	}, nil
}

// GetByToken retrieves invite by token
func (m *UserInviteModel) GetByToken(token string) (*UserInvite, error) {
	if err := m.ensureInviteSchema(); err != nil {
		return nil, err
	}

	var invite UserInvite
	var createdBy sql.NullInt64
	var usedAt sql.NullTime

	err := m.getDB().QueryRow(`
		SELECT id, code, COALESCE(username, ''), COALESCE(email, ''), COALESCE(role, 'user'), invited_by, created_at, expires_at, max_uses, use_count, used_at
		FROM user_invites
		WHERE code = ?
	`, HashAPIToken(token)).Scan(
		&invite.ID,
		&invite.Token,
		&invite.Username,
		&invite.Email,
		&invite.Role,
		&createdBy,
		&invite.CreatedAt,
		&invite.ExpiresAt,
		&invite.MaxUses,
		&invite.UseCount,
		&usedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}

	if usedAt.Valid {
		invite.UsedAt = &usedAt.Time
	}
	if createdBy.Valid {
		invite.CreatedBy = createdBy.Int64
	}

	return &invite, nil
}

// GetByID retrieves invite by numeric identifier.
func (m *UserInviteModel) GetByID(id int64) (*UserInvite, error) {
	if err := m.ensureInviteSchema(); err != nil {
		return nil, err
	}

	var invite UserInvite
	var createdBy sql.NullInt64
	var usedAt sql.NullTime

	err := m.getDB().QueryRow(`
		SELECT id, code, COALESCE(username, ''), COALESCE(email, ''), COALESCE(role, 'user'), invited_by, created_at, expires_at, max_uses, use_count, used_at
		FROM user_invites
		WHERE id = ?
	`, id).Scan(
		&invite.ID,
		&invite.Token,
		&invite.Username,
		&invite.Email,
		&invite.Role,
		&createdBy,
		&invite.CreatedAt,
		&invite.ExpiresAt,
		&invite.MaxUses,
		&invite.UseCount,
		&usedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}

	if usedAt.Valid {
		invite.UsedAt = &usedAt.Time
	}
	if createdBy.Valid {
		invite.CreatedBy = createdBy.Int64
	}

	return &invite, nil
}

// VerifyInvite checks whether an invite token is still usable.
func (m *UserInviteModel) VerifyInvite(token string) (*UserInvite, error) {
	invite, err := m.GetByToken(token)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, fmt.Errorf("invalid invite token")
	}
	if time.Now().After(invite.ExpiresAt) {
		return nil, fmt.Errorf("invite token has expired")
	}
	if invite.UsedAt != nil {
		return nil, fmt.Errorf("invite token has already been used")
	}
	if invite.MaxUses > 0 && invite.UseCount >= invite.MaxUses {
		return nil, fmt.Errorf("invite token has already been used")
	}

	return invite, nil
}

// MarkUsed marks invite as used.
func (m *UserInviteModel) MarkUsed(token string, usedBy int64) error {
	if err := m.ensureInviteSchema(); err != nil {
		return err
	}

	_, err := m.getDB().Exec(`
		UPDATE user_invites
		SET used_by = ?, used_at = datetime('now'), use_count = use_count + 1
		WHERE code = ?
	`, usedBy, HashAPIToken(token))

	if err != nil {
		return fmt.Errorf("failed to mark invite used: %w", err)
	}

	return nil
}

// DeleteExpiredInvites removes expired invites
func (m *UserInviteModel) DeleteExpiredInvites() error {
	if err := m.ensureInviteSchema(); err != nil {
		return err
	}

	_, err := m.getDB().Exec(`
		DELETE FROM user_invites
		WHERE expires_at < datetime('now') OR used_at IS NOT NULL
	`)

	if err != nil {
		return fmt.Errorf("failed to delete expired invites: %w", err)
	}

	return nil
}

// ListInvites returns all invites ordered by creation time.
func (m *UserInviteModel) ListInvites() ([]UserInvite, error) {
	if err := m.ensureInviteSchema(); err != nil {
		return nil, err
	}

	rows, err := m.getDB().Query(`
		SELECT id, code, COALESCE(username, ''), COALESCE(email, ''), COALESCE(role, 'user'), invited_by, created_at, expires_at, max_uses, use_count, used_at
		FROM user_invites
		ORDER BY created_at DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to query invites: %w", err)
	}
	defer rows.Close()

	var invites []UserInvite
	for rows.Next() {
		var invite UserInvite
		var createdBy sql.NullInt64
		var usedAt sql.NullTime

		err := rows.Scan(
			&invite.ID,
			&invite.Token,
			&invite.Username,
			&invite.Email,
			&invite.Role,
			&createdBy,
			&invite.CreatedAt,
			&invite.ExpiresAt,
			&invite.MaxUses,
			&invite.UseCount,
			&usedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invite: %w", err)
		}

		if createdBy.Valid {
			invite.CreatedBy = createdBy.Int64
		}
		if usedAt.Valid {
			invite.UsedAt = &usedAt.Time
		}

		invites = append(invites, invite)
	}

	return invites, nil
}

// GetPendingInvites returns all pending (unused, non-expired) invites.
func (m *UserInviteModel) GetPendingInvites() ([]UserInvite, error) {
	invites, err := m.ListInvites()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	pending := make([]UserInvite, 0, len(invites))
	for _, invite := range invites {
		if invite.UsedAt == nil && invite.ExpiresAt.After(now) && (invite.MaxUses == 0 || invite.UseCount < invite.MaxUses) {
			pending = append(pending, invite)
		}
	}

	return pending, nil
}

// DeleteInvite removes an invite by identifier.
func (m *UserInviteModel) DeleteInvite(id int64) error {
	if err := m.ensureInviteSchema(); err != nil {
		return err
	}

	_, err := m.getDB().Exec(`DELETE FROM user_invites WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete invite: %w", err)
	}

	return nil
}
