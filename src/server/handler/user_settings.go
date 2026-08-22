// Package handler provides HTTP handlers
// User settings handler per AI.md PART 34
package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

// UserSettingsHandler handles user settings pages and API
type UserSettingsHandler struct {
	DB *sql.DB
}

// NewUserSettingsHandler creates a new UserSettingsHandler
func NewUserSettingsHandler(db *sql.DB) *UserSettingsHandler {
	return &UserSettingsHandler{DB: db}
}

// ShowAccountSettings renders the account settings page
// Route: GET /users/settings
func (h *UserSettingsHandler) ShowAccountSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	NegotiateResponse(c, "page/user/settings.tmpl", util.TemplateData(c, gin.H{
		"title":       "Account Settings",
		"page":        "settings",
		"settingsTab": "account",
		"user":        user,
	}))
}

// ShowPrivacySettings renders the privacy settings page
// Route: GET /users/settings/privacy
func (h *UserSettingsHandler) ShowPrivacySettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	// Get user preferences for privacy settings
	prefs, _ := h.getOrCreatePreferences(user.ID)

	NegotiateResponse(c, "page/user/settings_privacy.tmpl", util.TemplateData(c, gin.H{
		"title":       "Privacy Settings",
		"page":        "settings",
		"settingsTab": "privacy",
		"user":        user,
		"preferences": prefs,
	}))
}

// ShowNotificationSettings renders the notification settings page
// Route: GET /users/settings/notifications
func (h *UserSettingsHandler) ShowNotificationSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	// Get user preferences
	prefs, _ := h.getOrCreatePreferences(user.ID)

	NegotiateResponse(c, "page/user/settings_notifications.tmpl", util.TemplateData(c, gin.H{
		"title":       "Notification Settings",
		"page":        "settings",
		"settingsTab": "notifications",
		"user":        user,
		"preferences": prefs,
	}))
}

// ShowAppearanceSettings renders the appearance settings page
// Route: GET /users/settings/appearance
func (h *UserSettingsHandler) ShowAppearanceSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	// Get user preferences
	prefs, _ := h.getOrCreatePreferences(user.ID)

	NegotiateResponse(c, "page/user/settings_appearance.tmpl", util.TemplateData(c, gin.H{
		"title":       "Appearance Settings",
		"page":        "settings",
		"settingsTab": "appearance",
		"user":        user,
		"preferences": prefs,
	}))
}

// ShowTokensSettings renders the API tokens management page
// Route: GET /users/settings/tokens
func (h *UserSettingsHandler) ShowTokensSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	// Get user's API tokens
	tokens, _ := h.getUserTokens(user.ID)

	NegotiateResponse(c, "page/user/settings_tokens.tmpl", util.TemplateData(c, gin.H{
		"title":       "API Tokens",
		"page":        "settings",
		"settingsTab": "tokens",
		"user":        user,
		"tokens":      tokens,
	}))
}

// UserSettingsResponse represents the full user settings response
// Per AI.md PART 34: GET /api/v1/users/settings
type UserSettingsResponse struct {
	Account       AccountSettings      `json:"account"`
	Privacy       PrivacySettings      `json:"privacy"`
	Notifications NotificationSettings `json:"notifications"`
	Appearance    AppearanceSettings   `json:"appearance"`
}

// AccountSettings represents account-related settings
type AccountSettings struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	Location    string `json:"location"`
	Website     string `json:"website"`
	Timezone    string `json:"timezone"`
	Language    string `json:"language"`
	DateFormat  string `json:"date_format"`
	TimeFormat  string `json:"time_format"`
}

// PrivacySettings represents privacy-related settings
type PrivacySettings struct {
	Visibility    string `json:"visibility"`
	ShowEmail     bool   `json:"show_email"`
	ShowActivity  bool   `json:"show_activity"`
	ShowOrgs      bool   `json:"show_orgs"`
	Searchable    bool   `json:"searchable"`
	OrgVisibility bool   `json:"org_visibility"`
}

// NotificationSettings represents notification-related settings
type NotificationSettings struct {
	EmailSecurity bool   `json:"email_security"`
	EmailMentions bool   `json:"email_mentions"`
	EmailUpdates  bool   `json:"email_updates"`
	EmailDigest   string `json:"email_digest"`
	PushEnabled   bool   `json:"push_enabled"`
	PushMentions  bool   `json:"push_mentions"`
}

// AppearanceSettings represents appearance-related settings
type AppearanceSettings struct {
	Theme        string `json:"theme"`
	FontSize     string `json:"font_size"`
	ReduceMotion bool   `json:"reduce_motion"`
}

// GetSettings returns all user settings
// Route: GET /api/v1/users/settings
// @Summary Get user settings
// @Description Get all settings for the current user (account, privacy, notifications, appearance).
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "User settings"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/settings [get]
func (h *UserSettingsHandler) GetSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	response, err := h.loadSettings(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get preferences"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// UpdateSettingsRequest represents a partial settings update
type UpdateSettingsRequest struct {
	Account       *AccountSettings      `json:"account,omitempty"`
	Privacy       *PrivacySettings      `json:"privacy,omitempty"`
	Notifications *NotificationSettings `json:"notifications,omitempty"`
	Appearance    *AppearanceSettings   `json:"appearance,omitempty"`
}

// UpdateSettings updates user settings (partial update)
// Route: PATCH /api/v1/users/settings
// @Summary Update user settings
// @Description Update user settings (partial update — only include changed sections).
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body UpdateSettingsRequest true "Settings to update"
// @Success 200 {object} map[string]interface{} "Updated settings"
// @Failure 400 {object} map[string]interface{} "Validation error"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/settings [patch]
func (h *UserSettingsHandler) UpdateSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.applySettingsUpdate(user.ID, &req); err != nil {
		switch err.Error() {
		case "bio must be 500 characters or fewer", "theme must be one of: dark, light, auto":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		}
		return
	}

	// Mirror the saved DB theme preference into the theme cookie (AI.md
	// PART 16) so the global InjectServerContext middleware, which only
	// reads the cookie, renders the correct <html> class on the next
	// request without a per-request DB lookup.
	if req.Appearance != nil && req.Appearance.Theme != "" {
		server.SetThemeCookie(c, req.Appearance.Theme)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// LoadUserSettings loads the live user settings payload used by /api/v1/users/settings.
func LoadUserSettings(db *sql.DB, userID int64) (*UserSettingsResponse, error) {
	return NewUserSettingsHandler(db).loadSettings(userID)
}

// ApplyUserSettingsUpdate applies the same section-based settings update used by PATCH /api/v1/users/settings.
func ApplyUserSettingsUpdate(db *sql.DB, userID int64, req *UpdateSettingsRequest) error {
	return NewUserSettingsHandler(db).applySettingsUpdate(userID, req)
}

func (h *UserSettingsHandler) loadSettings(userID int64) (*UserSettingsResponse, error) {
	user := &models.User{}
	var displayName, bio, location, website, timezone, language sql.NullString
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT id, email, username, display_name, bio, location, website, timezone, language,
		       role, visibility, is_active, email_verified, created_at, updated_at
		FROM user_accounts
		WHERE id = ?
	`, userID).Scan(
		&user.ID, &user.Email, &user.Username, &displayName, &bio, &location,
		&website, &timezone, &language, &user.Role, &user.Visibility,
		&user.IsActive, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	user.DisplayName = displayName.String
	user.Bio = bio.String
	user.Location = location.String
	user.Website = website.String
	user.Timezone = timezone.String
	user.Language = language.String

	prefs, err := h.getOrCreatePreferences(userID)
	if err != nil {
		return nil, err
	}

	extPrefs, err := h.getExtendedPreferences(userID)
	if err != nil {
		return nil, err
	}

	return &UserSettingsResponse{
		Account: AccountSettings{
			DisplayName: user.DisplayName,
			Bio:         user.Bio,
			Location:    user.Location,
			Website:     user.Website,
			Timezone:    user.Timezone,
			Language:    user.Language,
			DateFormat:  extPrefs.DateFormat,
			TimeFormat:  extPrefs.TimeFormat,
		},
		Privacy: PrivacySettings{
			Visibility:    user.Visibility,
			ShowEmail:     extPrefs.ShowEmail,
			ShowActivity:  extPrefs.ShowActivity,
			ShowOrgs:      extPrefs.ShowOrgs,
			Searchable:    extPrefs.Searchable,
			OrgVisibility: extPrefs.OrgVisibility,
		},
		Notifications: NotificationSettings{
			EmailSecurity: true,
			EmailMentions: prefs.EmailNotifications,
			EmailUpdates:  extPrefs.EmailUpdates,
			EmailDigest:   extPrefs.EmailDigest,
			PushEnabled:   prefs.NotificationsEnabled,
			PushMentions:  extPrefs.PushMentions,
		},
		Appearance: AppearanceSettings{
			Theme:        prefs.Theme,
			FontSize:     extPrefs.FontSize,
			ReduceMotion: extPrefs.ReduceMotion,
		},
	}, nil
}

func (h *UserSettingsHandler) applySettingsUpdate(userID int64, req *UpdateSettingsRequest) error {
	if req.Account != nil {
		if err := h.updateAccountSettings(userID, req.Account); err != nil {
			return err
		}
	}
	if req.Privacy != nil {
		if err := h.updatePrivacySettings(userID, req.Privacy); err != nil {
			return err
		}
	}
	if req.Notifications != nil {
		if err := h.updateNotificationSettings(userID, req.Notifications); err != nil {
			return err
		}
	}
	if req.Appearance != nil {
		if err := h.updateAppearanceSettings(userID, req.Appearance); err != nil {
			return err
		}
	}
	return nil
}

// ExtendedPreferences stores additional preference fields not in base UserPreferences
type ExtendedPreferences struct {
	DateFormat    string
	TimeFormat    string
	ShowEmail     bool
	ShowActivity  bool
	ShowOrgs      bool
	Searchable    bool
	OrgVisibility bool
	EmailUpdates  bool
	EmailDigest   string
	PushMentions  bool
	FontSize      string
	ReduceMotion  bool
}

// getOrCreatePreferences gets or creates user preferences
func (h *UserSettingsHandler) getOrCreatePreferences(userID int64) (*models.UserPreferences, error) {
	prefs := &models.UserPreferences{}

	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT user_id, theme, language, timezone, temperature_unit, pressure_unit,
		       wind_speed_unit, precipitation_unit, notifications_enabled, email_notifications,
		       created_at, updated_at
		FROM user_preferences WHERE user_id = ?
	`, userID).Scan(
		&prefs.UserID, &prefs.Theme, &prefs.Language, &prefs.Timezone,
		&prefs.TemperatureUnit, &prefs.PressureUnit, &prefs.WindSpeedUnit, &prefs.PrecipitationUnit,
		&prefs.NotificationsEnabled, &prefs.EmailNotifications, &prefs.CreatedAt, &prefs.UpdatedAt,
	)
	prefs.ID = userID

	if err == sql.ErrNoRows {
		// Create default preferences
		prefs = &models.UserPreferences{
			UserID:               userID,
			Theme:                "dark",
			Language:             "en",
			Timezone:             "UTC",
			TemperatureUnit:      "celsius",
			PressureUnit:         "hPa",
			WindSpeedUnit:        "kmh",
			PrecipitationUnit:    "mm",
			NotificationsEnabled: true,
			EmailNotifications:   true,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}

		// created_at/updated_at are bound as canonical UTC text. Binding a raw
		// time.Time makes the SQLite driver serialize it as time.Time.String()
		// in the host's LOCAL zone, which no reader can compare against a UTC
		// instant without guessing the writer's offset.
		_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
			INSERT INTO user_preferences (user_id, theme, language, timezone, temperature_unit,
			                              pressure_unit, wind_speed_unit, precipitation_unit,
			                              notifications_enabled, email_notifications, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, prefs.UserID, prefs.Theme, prefs.Language, prefs.Timezone, prefs.TemperatureUnit,
			prefs.PressureUnit, prefs.WindSpeedUnit, prefs.PrecipitationUnit,
			prefs.NotificationsEnabled, prefs.EmailNotifications,
			dbtime.FormatSQLTimestamp(prefs.CreatedAt), dbtime.FormatSQLTimestamp(prefs.UpdatedAt))

		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	return prefs, nil
}

// getExtendedPreferences gets extended preferences with defaults
func (h *UserSettingsHandler) getExtendedPreferences(userID int64) (*ExtendedPreferences, error) {
	// For now return defaults - these would be stored in user_preferences table
	// with additional columns in a real implementation
	return &ExtendedPreferences{
		DateFormat:    "YYYY-MM-DD",
		TimeFormat:    "24h",
		ShowEmail:     false,
		ShowActivity:  true,
		ShowOrgs:      true,
		Searchable:    true,
		OrgVisibility: true,
		EmailUpdates:  false,
		EmailDigest:   "weekly",
		PushMentions:  true,
		FontSize:      "medium",
		ReduceMotion:  false,
	}, nil
}

// updateAccountSettings updates account settings in users table
func (h *UserSettingsHandler) updateAccountSettings(userID int64, settings *AccountSettings) error {
	// Validate bio length (max 500 chars per AI.md PART 34)
	if len(settings.Bio) > 500 {
		return fmt.Errorf("bio must be 500 characters or fewer")
	}

	// Validate website URL if provided
	if settings.Website != "" && !strings.HasPrefix(settings.Website, "http://") && !strings.HasPrefix(settings.Website, "https://") {
		settings.Website = "https://" + settings.Website
	}

	// updated_at is bound as canonical UTC text rather than as a raw time.Time,
	// which the SQLite driver would serialize in the host's LOCAL zone.
	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE user_accounts
		SET display_name = ?, bio = ?, location = ?, website = ?, timezone = ?, language = ?, updated_at = ?
		WHERE id = ?
	`, settings.DisplayName, settings.Bio, settings.Location, settings.Website,
		settings.Timezone, settings.Language, dbtime.FormatSQLTimestamp(time.Now()), userID)

	return err
}

// updatePrivacySettings updates privacy settings
func (h *UserSettingsHandler) updatePrivacySettings(userID int64, settings *PrivacySettings) error {
	// Update visibility in users table.
	// updated_at is bound as canonical UTC text rather than as a raw time.Time,
	// which the SQLite driver would serialize in the host's LOCAL zone.
	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE user_accounts
		SET visibility = ?, updated_at = ?
		WHERE id = ?
	`, settings.Visibility, dbtime.FormatSQLTimestamp(time.Now()), userID)

	// Note: Other privacy settings (show_email, show_activity, etc.) would be stored
	// in user_preferences table with extended columns
	return err
}

// updateNotificationSettings updates notification settings
func (h *UserSettingsHandler) updateNotificationSettings(userID int64, settings *NotificationSettings) error {
	// email_security is always true and cannot be changed per AI.md PART 34.
	// updated_at is bound as canonical UTC text rather than as a raw time.Time,
	// which the SQLite driver would serialize in the host's LOCAL zone.
	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE user_preferences
		SET notifications_enabled = ?, email_notifications = ?, updated_at = ?
		WHERE user_id = ?
	`, settings.PushEnabled, settings.EmailMentions, dbtime.FormatSQLTimestamp(time.Now()), userID)

	return err
}

// updateAppearanceSettings updates appearance settings
func (h *UserSettingsHandler) updateAppearanceSettings(userID int64, settings *AppearanceSettings) error {
	// Validate theme
	validThemes := map[string]bool{"dark": true, "light": true, "auto": true}
	if !validThemes[settings.Theme] {
		return fmt.Errorf("theme must be one of: dark, light, auto")
	}

	// updated_at is bound as canonical UTC text rather than as a raw time.Time,
	// which the SQLite driver would serialize in the host's LOCAL zone.
	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE user_preferences
		SET theme = ?, updated_at = ?
		WHERE user_id = ?
	`, settings.Theme, dbtime.FormatSQLTimestamp(time.Now()), userID)

	return err
}

// UserToken represents a user API token for display
type UserToken struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      string     `json:"scopes"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// getUserTokens gets all API tokens for a user
func (h *UserSettingsHandler) getUserTokens(userID int64) ([]UserToken, error) {
	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT id, name, token_prefix, scopes, created_at, expires_at, last_used_at
		FROM user_tokens WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []UserToken
	for rows.Next() {
		var t UserToken
		var name, scopes sql.NullString
		// The three timestamps are scanned untyped and parsed in Go. Legacy rows
		// written before timestamps were canonicalized hold a Go
		// time.Time.String() layout the driver cannot scan into time.Time /
		// sql.NullTime, and the scan error below skips the row entirely — which
		// silently hid those tokens from the user's own token list.
		var createdAtRaw, expiresAtRaw, lastUsedAtRaw interface{}

		err := rows.Scan(&t.ID, &name, &t.TokenPrefix, &scopes, &createdAtRaw, &expiresAtRaw, &lastUsedAtRaw)
		if err != nil {
			continue
		}

		t.Name = name.String
		t.Scopes = scopes.String
		if parsed, ok := dbtime.ParseStoredTimestamp(createdAtRaw); ok {
			t.CreatedAt = parsed
		}
		if parsed, ok := dbtime.ParseStoredTimestamp(expiresAtRaw); ok {
			expires := parsed
			t.ExpiresAt = &expires
		}
		if parsed, ok := dbtime.ParseStoredTimestamp(lastUsedAtRaw); ok {
			lastUsed := parsed
			t.LastUsedAt = &lastUsed
		}

		tokens = append(tokens, t)
	}

	return tokens, nil
}

// CreateTokenRequest represents a request to create a new API token
type CreateTokenRequest struct {
	Name      string `json:"name" binding:"required"`
	Scopes    string `json:"scopes"`
	ExpiresIn int    `json:"expires_in"` // days, 0 = never
}

// CreateToken creates a new API token for the user
// Route: POST /api/v1/users/tokens
// @Summary Create API token
// @Description Create a named API token for programmatic access (max 5 per user).
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateTokenRequest true "Token name"
// @Success 201 {object} map[string]interface{} "Token created — plaintext token returned once"
// @Failure 400 {object} map[string]interface{} "Validation error or token limit reached"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/tokens [post]
func (h *UserSettingsHandler) CreateToken(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check token limit (max 5 per user per AI.md)
	var count int
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_tokens WHERE user_id = ?", user.ID).Scan(&count)
	if count >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum 5 tokens per user"})
		return
	}

	// Generate token with usr_ prefix per AI.md PART 11
	token, err := models.GenerateTokenWithPrefix(models.PrefixUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}
	tokenHash := models.HashToken(token)
	tokenPrefix := models.GetTokenPrefix(token) + "..."

	// expires_at stays NULL for never-expiring tokens; otherwise it is bound as
	// canonical UTC text so every reader parses one layout.
	var expiresAt interface{}
	if req.ExpiresIn > 0 {
		expiresAt = dbtime.FormatSQLTimestamp(time.Now().AddDate(0, 0, req.ExpiresIn))
	}

	// created_at is bound as canonical UTC text. Binding a raw time.Time makes
	// the SQLite driver serialize it as time.Time.String() in the host's LOCAL
	// zone, which no reader can compare against a UTC instant without guessing
	// the writer's offset.
	_, dbErr := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO user_tokens (user_id, token_hash, token_prefix, name, scopes, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, user.ID, tokenHash, tokenPrefix, req.Name, req.Scopes, dbtime.FormatSQLTimestamp(time.Now()), expiresAt)

	if dbErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	// Return the full token (only shown once)
	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"message": "Token created. This token will only be shown once.",
	})
}

// RevokeToken revokes an API token
// Route: DELETE /api/v1/users/tokens/:id
// @Summary Revoke API token
// @Description Permanently revoke a user API token.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Param id path string true "Token ID"
// @Success 200 {object} map[string]interface{} "Token revoked"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 404 {object} map[string]interface{} "Token not found"
// @Router /api/v1/users/tokens/{id} [delete]
func (h *UserSettingsHandler) RevokeToken(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	tokenID := c.Param("id")

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		DELETE FROM user_tokens WHERE id = ? AND user_id = ?
	`, tokenID, user.ID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token revoked"})
}

// ListTokens returns all tokens for the current user
// Route: GET /api/v1/users/tokens
// @Summary List API tokens
// @Description List all API tokens for the current user.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "Token list"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/tokens [get]
func (h *UserSettingsHandler) ListTokens(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	tokens, err := h.getUserTokens(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get tokens"})
		return
	}

	c.JSON(http.StatusOK, tokens)
}

// ListSessions returns the authenticated user's active sessions.
// Route: GET /api/v1/users/sessions
func (h *UserSettingsHandler) ListSessions(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	sessionModel := &models.UserSessionModel{DB: h.DB}
	sessions, err := sessionModel.GetActiveSessions(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list sessions"})
		return
	}

	// Determine the hash of the current request's session so we can mark it.
	// The middleware stores the Session struct under SessionContextKey ("session").
	// Session.ID is the raw bearer token; we hash it to compare against stored hashes.
	var currentTokenHash string
	if sessionVal, ok := c.Get(middleware.SessionContextKey); ok {
		if sess, ok := sessionVal.(*models.Session); ok && sess != nil {
			h := sha256.Sum256([]byte(sess.ID))
			currentTokenHash = hex.EncodeToString(h[:])
		}
	}

	type SessionItem struct {
		ID         int64     `json:"id"`
		IPAddress  string    `json:"ip_address"`
		UserAgent  string    `json:"user_agent"`
		CreatedAt  time.Time `json:"created_at"`
		ExpiresAt  time.Time `json:"expires_at"`
		LastUsedAt time.Time `json:"last_used_at"`
		IsCurrent  bool      `json:"is_current"`
	}

	items := make([]SessionItem, 0, len(sessions))
	for _, s := range sessions {
		item := SessionItem{
			ID:         s.ID,
			IPAddress:  s.IPAddress,
			UserAgent:  s.UserAgent,
			CreatedAt:  s.CreatedAt,
			ExpiresAt:  s.ExpiresAt,
			LastUsedAt: s.LastUsedAt,
		}
		// s.SessionID holds the stored token_hash; compare with the hash of the current token.
		if currentTokenHash != "" {
			item.IsCurrent = s.SessionID == currentTokenHash
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"sessions": items})
}

// RevokeSession deletes a specific session by its integer row ID.
// Route: DELETE /api/v1/users/sessions/:id
// The `:id` is the integer primary key from the session list, NOT the raw token.
func (h *UserSettingsHandler) RevokeSession(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	rowIDStr := c.Param("id")
	if rowIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required"})
		return
	}
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	// Verify the session belongs to the current user before deleting.
	// expires_at is compared in Go rather than in SQL: the sqlite driver
	// writes time.Time parameters using Go's default time.Time.String()
	// format (e.g. "2026-07-21 05:23:45.484833776 -0400 -0400"), which
	// SQLite's datetime()/julianday() functions cannot parse (they return
	// NULL), making any SQL-side "datetime(expires_at) > datetime('now')"
	// comparison silently match nothing. Parsing the stored text in Go
	// avoids depending on SQLite's date parser understanding that format.
	var ownerID int64
	var expiresAtRaw string
	err = database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect,
		`SELECT user_id, expires_at FROM user_sessions WHERE id = ?`,
		rowID,
	).Scan(&ownerID, &expiresAtRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}
	if !dbtime.IsAfter(expiresAtRaw, time.Now()) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}
	if ownerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Session does not belong to you"})
		return
	}

	sessionModel := &models.UserSessionModel{DB: h.DB}
	if err := sessionModel.DeleteSessionByRowID(rowID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Session revoked"})
}

// RevokeAllSessions deletes all sessions for the authenticated user (logout everywhere).
// Route: DELETE /api/v1/users/sessions
func (h *UserSettingsHandler) RevokeAllSessions(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	sessionModel := &models.UserSessionModel{DB: h.DB}
	if err := sessionModel.DeleteAllSessionsForUser(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All sessions revoked"})
}
