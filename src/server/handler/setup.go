package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

type SetupHandler struct {
	DB *sql.DB
}

// ShowSetupTokenEntry shows the setup token entry form
// AI.md: First step of setup - user must enter setup token displayed in console
func (h *SetupHandler) ShowSetupTokenEntry(c *gin.Context) {
	c.HTML(http.StatusOK, "page/setup_token.tmpl", util.TemplateData(c, gin.H{
		"Title": "Server Setup - Enter Setup Token",
	}))
}

// VerifySetupTokenAtAdmin handles setup token verification at /server/admin/verify-token
// AI.md: Step 2: User navigates to /server/admin → Step 3: User enters setup token → Step 4: Redirect to setup wizard
func (h *SetupHandler) VerifySetupTokenAtAdmin(c *gin.Context) {
	// Get admin path from config
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	setupToken := c.PostForm("setup_token")
	if setupToken == "" {
		c.HTML(http.StatusBadRequest, "admin/setup_token.tmpl", util.TemplateData(c, gin.H{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": gin.H{
				"Title": title,
			},
			"error": "Setup token is required",
		}))
		return
	}

	// Validate setup token against stored hash
	configDir := path.GetConfigDir()
	valid, err := util.ValidateSetupToken(configDir, setupToken)
	if err != nil {
		c.HTML(http.StatusBadRequest, "admin/setup_token.tmpl", util.TemplateData(c, gin.H{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": gin.H{
				"Title": title,
			},
			"error": "Setup token not found or already used",
		}))
		return
	}

	if !valid {
		c.HTML(http.StatusUnauthorized, "admin/setup_token.tmpl", util.TemplateData(c, gin.H{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": gin.H{
				"Title": title,
			},
			"error": "Invalid setup token",
		}))
		return
	}

	// Store validated token in session for the admin creation step
	isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetCookie("setup_token_verified", "true", 3600, "/", "", isHTTPS, true)

	// Redirect to setup wizard at /{admin_path}/config/setup
	// AI.md: Step 4: Redirect to /{admin_path}/config/setup (setup wizard)
	c.Redirect(http.StatusFound, adminPath+"/config/setup")
}

// VerifySetupToken validates the setup token (API endpoint)
// AI.md: Setup token stored as SHA-256 hash in {config_dir}/setup_token.txt
func (h *SetupHandler) VerifySetupToken(c *gin.Context) {
	var input struct {
		SetupToken string `json:"setup_token" form:"setup_token" binding:"required"`
	}

	if err := c.ShouldBind(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Setup token is required"})
		return
	}

	// Validate setup token against stored hash
	configDir := path.GetConfigDir()
	valid, err := util.ValidateSetupToken(configDir, input.SetupToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Setup token not found or already used"})
		return
	}

	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid setup token"})
		return
	}

	// Store validated token in session for the admin creation step
	// Use a secure session cookie to track that token was validated
	isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetCookie("setup_token_verified", "true", 3600, "/", "", isHTTPS, true)

	// Get admin path for redirect
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"redirect": adminPath + "/config/setup",
	})
}

// ShowAdminSetup shows the admin creation form
// AI.md: Setup wizard step - create Primary Admin
func (h *SetupHandler) ShowAdminSetup(c *gin.Context) {
	cfg, _ := config.LoadConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	c.HTML(http.StatusOK, "page/setup_admin.tmpl", util.TemplateData(c, gin.H{
		"Title": "Create Administrator - " + title,
	}))
}

// setupError renders error for form submissions or returns JSON for API
func (h *SetupHandler) setupError(c *gin.Context, status int, errorMsg string) {
	// Check Accept header to determine response type
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/json") {
		c.JSON(status, gin.H{"error": errorMsg})
		return
	}

	// HTML form submission - re-render form with error
	cfg, _ := config.LoadConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	c.HTML(status, "page/setup_admin.tmpl", util.TemplateData(c, gin.H{
		"Title": "Create Administrator - " + title,
		"error": errorMsg,
	}))
}

// CreateAdmin creates the Primary Admin account
// AI.md: Setup creates Primary Admin, requires setup token verification
// AI.md PART 16: Works without JavaScript - form POST returns redirect
func (h *SetupHandler) CreateAdmin(c *gin.Context) {
	// Verify setup token was validated
	verified, err := c.Cookie("setup_token_verified")
	if err != nil || verified != "true" {
		h.setupError(c, http.StatusUnauthorized, "Setup token not verified")
		return
	}

	var input struct {
		Username        string `json:"username" form:"username"`
		Email           string `json:"email" form:"email"`
		UseRandom       bool   `json:"use_random" form:"use_random"`
		Password        string `json:"password" form:"password"`
		ConfirmPassword string `json:"confirm_password" form:"confirm_password"`
	}

	// Accept both JSON and form data
	if err := c.ShouldBind(&input); err != nil {
		h.setupError(c, http.StatusBadRequest, "Invalid form data")
		return
	}

	// Validate email
	email := strings.TrimSpace(input.Email)
	if email == "" {
		h.setupError(c, http.StatusBadRequest, "Email is required")
		return
	}
	// Basic email validation
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		h.setupError(c, http.StatusBadRequest, "Invalid email format")
		return
	}

	// Set default username if empty
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = "administrator"
	}

	// Normalize username
	username = strings.ToLower(username)

	var password string
	var generatedPassword string

	if input.UseRandom {
		// Generate random password (32 characters, alphanumeric + special)
		var err error
		generatedPassword, err = generateRandomPassword(32)
		if err != nil {
			h.setupError(c, http.StatusInternalServerError, "Failed to generate secure password")
			return
		}
		password = generatedPassword
	} else {
		// Use custom password - must be confirmed
		if input.Password == "" {
			h.setupError(c, http.StatusBadRequest, "Password is required")
			return
		}
		// Passwords cannot start or end with whitespace
		if input.Password != strings.TrimSpace(input.Password) {
			h.setupError(c, http.StatusBadRequest, "Password cannot start or end with whitespace")
			return
		}
		if len(input.Password) < 12 {
			h.setupError(c, http.StatusBadRequest, "Password must be at least 12 characters")
			return
		}
		if input.Password != input.ConfirmPassword {
			h.setupError(c, http.StatusBadRequest, "Passwords do not match")
			return
		}
		password = input.Password
	}

	// Check if admin username already exists in server_admin_credentials
	var count int
	err = database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_admin_credentials WHERE username = ?", username).Scan(&count)
	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Database error")
		return
	}

	if count > 0 {
		h.setupError(c, http.StatusConflict, "Username already exists")
		return
	}

	// Hash password using Argon2id (AI.md PART 3 requirement)
	hashedPassword, err := util.HashPassword(password)
	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Create administrator account in server_admin_credentials (NOT user_accounts).
	// created_at/updated_at are bound as canonical UTC text rather than produced
	// by SQL's CURRENT_TIMESTAMP, which yields a different type and zone on
	// PostgreSQL, MySQL and SQL Server than it does on SQLite.
	createdAt := dbtime.FormatSQLTimestamp(time.Now())
	result, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_admin_credentials (username, email, password_hash, is_super_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, 1, 1, ?, ?)
	`, username, email, hashedPassword, createdAt, createdAt)

	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Failed to create administrator")
		return
	}

	// Get the newly created admin ID
	adminID, err := result.LastInsertId()
	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Failed to retrieve admin ID")
		return
	}

	// Create admin session (auto-login) in server_admin_sessions
	sessionID, err := generateSessionID()
	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Failed to generate session ID")
		return
	}
	// 7 days
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// created_at/expires_at are bound as canonical UTC text. Binding a raw
	// time.Time makes the SQLite driver serialize it as time.Time.String() in
	// the host's LOCAL zone, which no reader can compare against a UTC instant
	// without guessing the writer's offset.
	_, err = database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, adminID, c.ClientIP(), c.Request.UserAgent(), dbtime.FormatSQLTimestamp(time.Now()), dbtime.FormatSQLTimestamp(expiresAt))

	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Failed to create session")
		return
	}

	isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

	// Set admin_session cookie (separate from weather_session)
	c.SetCookie(
		"admin_session",
		sessionID,
		int(7*24*time.Hour.Seconds()),
		"/",
		"",
		isHTTPS,
		true,
	)

	// Delete setup token file after successful admin creation
	// AI.md: File deleted after successful setup completion
	configDir := path.GetConfigDir()
	if err := util.DeleteSetupToken(configDir); err != nil {
		// Log but don't fail - admin was created successfully
		log.Printf("WARNING: CreateAdmin: failed to delete setup token file: %v", err)
	}

	// Clear the setup_token_verified cookie
	c.SetCookie("setup_token_verified", "", -1, "/", "", false, true)

	// Generate API token for the new admin
	// AI.md: Step 2 - API Token auto-generated
	apiToken, err := generateAPIToken()
	if err != nil {
		h.setupError(c, http.StatusInternalServerError, "Failed to generate API token")
		return
	}

	// Hash and store the API token
	tokenHash := util.HashAPIToken(apiToken)
	// updated_at is bound as canonical UTC text rather than produced by SQL's
	// CURRENT_TIMESTAMP, which yields a different type and zone on PostgreSQL,
	// MySQL and SQL Server than it does on SQLite.
	_, err = database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		UPDATE server_admin_credentials
		SET api_token_hash = ?, api_token_prefix = 'adm_', updated_at = ?
		WHERE id = ?
	`, tokenHash, dbtime.FormatSQLTimestamp(time.Now()), adminID)
	if err != nil {
		// Log but don't fail - admin was created successfully
		log.Printf("WARNING: CreateAdmin: failed to store admin API token: %v", err)
	}

	// Store the generated password and token in session for display on next step
	// These are shown once and must be copied by user
	if generatedPassword != "" {
		c.SetCookie("setup_generated_password", generatedPassword, 3600, "/", "", isHTTPS, true)
	}
	c.SetCookie("setup_api_token", apiToken, 3600, "/", "", isHTTPS, true)
	c.SetCookie("setup_username", username, 3600, "/", "", isHTTPS, true)

	// Redirect to API token step (Step 2)
	// AI.md: Setup wizard Step 2 - API Token
	redirectURL := c.Request.URL.Path + "/api-token"

	// Check Accept header to determine response type
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/json") {
		response := gin.H{"ok": true, "redirect": redirectURL}
		if generatedPassword != "" {
			response["generated_password"] = generatedPassword
			response["username"] = username
		}
		response["api_token"] = apiToken
		c.JSON(http.StatusOK, response)
		return
	}

	// Form submission - redirect (works without JavaScript)
	c.Redirect(http.StatusFound, redirectURL)
}

// generateRandomPassword generates a cryptographically secure random password
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#-_$"
	password := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := range password {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate secure random password: %w", err)
		}
		password[i] = charset[n.Int64()]
	}
	return string(password), nil
}

// generateAPIToken generates a cryptographically secure API token with admin prefix
// AI.md: API tokens use prefix: adm_ for admin
func generateAPIToken() (string, error) {
	// Generate 32 random bytes (256 bits)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate API token: %w", err)
	}
	// Encode as base64url without padding, prefix with adm_
	return "adm_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

// generateSessionID generates a cryptographically secure random session ID
func generateSessionID() (string, error) {
	// Generate 48 random bytes and encode as base64 (results in 64 chars)
	bytes := make([]byte, 48)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure session ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ShowAPIToken shows the API token page (Step 2)
// AI.md: Setup wizard Step 2 - API Token auto-generated, user MUST copy
func (h *SetupHandler) ShowAPIToken(c *gin.Context) {
	cfg, _ := config.LoadConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	// Get tokens from cookies (set during admin creation)
	apiToken, _ := c.Cookie("setup_api_token")
	generatedPassword, _ := c.Cookie("setup_generated_password")
	username, _ := c.Cookie("setup_username")

	c.HTML(http.StatusOK, "page/setup_api_token.tmpl", util.TemplateData(c, gin.H{
		"Title":             "API Token - " + title,
		"APIToken":          apiToken,
		"GeneratedPassword": generatedPassword,
		"Username":          username,
	}))
}

// ProcessAPIToken handles acknowledgment of API token (Step 2 → Step 3)
func (h *SetupHandler) ProcessAPIToken(c *gin.Context) {
	// Clear the sensitive cookies
	isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetCookie("setup_api_token", "", -1, "/", "", isHTTPS, true)
	c.SetCookie("setup_generated_password", "", -1, "/", "", isHTTPS, true)

	// Redirect to server configuration (Step 3)
	basePath := strings.TrimSuffix(c.Request.URL.Path, "/api-token")
	c.Redirect(http.StatusFound, basePath+"/config")
}

// ShowServerConfig shows the server configuration page (Step 3)
// AI.md: Setup wizard Step 3 - Server Configuration
func (h *SetupHandler) ShowServerConfig(c *gin.Context) {
	cfg, _ := config.LoadConfig()
	title := "Weather"
	defaultDomain := ""
	defaultMode := "production"
	defaultTimezone := "UTC"

	if cfg != nil {
		if cfg.Server.Branding.Title != "" {
			title = cfg.Server.Branding.Title
		}
		defaultMode = cfg.Server.Mode
	}

	// Check for skip parameter
	if c.Query("skip") == "true" {
		basePath := strings.TrimSuffix(c.Request.URL.Path, "/config")
		c.Redirect(http.StatusFound, basePath+"/security")
		return
	}

	c.HTML(http.StatusOK, "page/setup_server_config.tmpl", util.TemplateData(c, gin.H{
		"Title":           "Server Configuration - " + title,
		"DefaultAppName":  title,
		"DefaultDomain":   defaultDomain,
		"DefaultMode":     defaultMode,
		"DefaultTimezone": defaultTimezone,
	}))
}

// ProcessServerConfig handles server configuration submission (Step 3 → Step 4)
func (h *SetupHandler) ProcessServerConfig(c *gin.Context) {
	var input struct {
		AppName  string `form:"app_name"`
		Domain   string `form:"domain"`
		Mode     string `form:"mode"`
		Timezone string `form:"timezone"`
	}

	if err := c.ShouldBind(&input); err != nil {
		c.HTML(http.StatusBadRequest, "page/setup_server_config.tmpl", util.TemplateData(c, gin.H{
			"Title": "Server Configuration",
			"error": "Invalid form data",
		}))
		return
	}

	// Save settings to database
	settings := map[string]string{
		"server.branding.title": input.AppName,
		"server.domain":         input.Domain,
		"mode":                  input.Mode,
		"server.timezone":       input.Timezone,
	}

	// updated_at is bound as canonical UTC text rather than produced by SQLite's
	// datetime('now'), which does not exist on PostgreSQL or MySQL.
	now := dbtime.FormatSQLTimestamp(time.Now())

	for key, value := range settings {
		if value != "" {
			_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
				INSERT INTO server_config (key, value, updated_at)
				VALUES (?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
			`, key, value, now, value, now)
			if err != nil {
				log.Printf("WARNING: ProcessServerConfig: failed to save setting %s: %v", key, err)
			}
		}
	}

	// Redirect to security settings (Step 4)
	basePath := strings.TrimSuffix(c.Request.URL.Path, "/config")
	c.Redirect(http.StatusFound, basePath+"/security")
}

// ShowSecurity shows the security settings page (Step 4)
// AI.md: Setup wizard Step 4 - Security Settings
func (h *SetupHandler) ShowSecurity(c *gin.Context) {
	cfg, _ := config.LoadConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	// Check for skip parameter
	if c.Query("skip") == "true" {
		basePath := strings.TrimSuffix(c.Request.URL.Path, "/security")
		c.Redirect(http.StatusFound, basePath+"/services")
		return
	}

	c.HTML(http.StatusOK, "page/setup_security.tmpl", util.TemplateData(c, gin.H{
		"Title": "Security Settings - " + title,
	}))
}

// ProcessSecurity handles security settings submission (Step 4 → Step 5)
func (h *SetupHandler) ProcessSecurity(c *gin.Context) {
	var input struct {
		BackupPassword        string `form:"backup_password"`
		BackupPasswordConfirm string `form:"backup_password_confirm"`
		Enable2FA             bool   `form:"enable_2fa"`
		TOTPCode              string `form:"totp_code"`
	}

	if err := c.ShouldBind(&input); err != nil {
		c.HTML(http.StatusBadRequest, "page/setup_security.tmpl", util.TemplateData(c, gin.H{
			"Title": "Security Settings",
			"error": "Invalid form data",
		}))
		return
	}

	// Validate backup password match
	if input.BackupPassword != "" && input.BackupPassword != input.BackupPasswordConfirm {
		c.HTML(http.StatusBadRequest, "page/setup_security.tmpl", util.TemplateData(c, gin.H{
			"Title": "Security Settings",
			"error": "Backup passwords do not match",
		}))
		return
	}

	// Save backup encryption password if provided
	if input.BackupPassword != "" {
		// Hash the backup password
		hashedPassword, err := util.HashPassword(input.BackupPassword)
		if err == nil {
			// updated_at is bound as canonical UTC text rather than produced by
			// SQLite's datetime('now'), which does not exist on PostgreSQL or MySQL.
			now := dbtime.FormatSQLTimestamp(time.Now())
			database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
				INSERT INTO server_config (key, value, updated_at)
				VALUES ('backup.encryption_hash', ?, ?)
				ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
			`, hashedPassword, now, hashedPassword, now)
		}
	}

	// Handle 2FA setup (if enabled, this would involve TOTP verification)
	// For now, we skip this as it requires more complex flow

	// Redirect to optional services (Step 5)
	basePath := strings.TrimSuffix(c.Request.URL.Path, "/security")
	c.Redirect(http.StatusFound, basePath+"/services")
}

// ShowServices shows the optional services page (Step 5)
// AI.md: Setup wizard Step 5 - Optional Services
func (h *SetupHandler) ShowServices(c *gin.Context) {
	cfg, _ := config.LoadConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	// Check for skip parameter
	if c.Query("skip") == "true" {
		basePath := strings.TrimSuffix(c.Request.URL.Path, "/services")
		c.Redirect(http.StatusFound, basePath+"/complete")
		return
	}

	// Check if Tor is available
	torAvailable := util.IsTorAvailable()

	// Get admin email for SSL contact
	var adminEmail string
	database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT email FROM server_admin_credentials LIMIT 1").Scan(&adminEmail)

	c.HTML(http.StatusOK, "page/setup_services.tmpl", util.TemplateData(c, gin.H{
		"Title":        "Optional Services - " + title,
		"TorAvailable": torAvailable,
		"AdminEmail":   adminEmail,
	}))
}

// ProcessServices handles optional services submission (Step 5 → Step 6)
func (h *SetupHandler) ProcessServices(c *gin.Context) {
	var input struct {
		EnableSSL        bool   `form:"enable_ssl"`
		SSLDomain        string `form:"ssl_domain"`
		SSLEmail         string `form:"ssl_email"`
		EnableMultiUser  bool   `form:"enable_multiuser"`
		RegistrationMode string `form:"registration_mode"`
	}

	if err := c.ShouldBind(&input); err != nil {
		c.HTML(http.StatusBadRequest, "page/setup_services.tmpl", util.TemplateData(c, gin.H{
			"Title": "Optional Services",
			"error": "Invalid form data",
		}))
		return
	}

	// updated_at is bound as canonical UTC text rather than produced by SQLite's
	// datetime('now'), which does not exist on PostgreSQL or MySQL.
	now := dbtime.FormatSQLTimestamp(time.Now())

	// Save SSL settings
	if input.EnableSSL {
		database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
			INSERT INTO server_config (key, value, updated_at)
			VALUES ('ssl.enabled', 'true', ?)
			ON CONFLICT(key) DO UPDATE SET value = 'true', updated_at = ?
		`, now, now)
		if input.SSLDomain != "" {
			database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
				INSERT INTO server_config (key, value, updated_at)
				VALUES ('ssl.domain', ?, ?)
				ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
			`, input.SSLDomain, now, input.SSLDomain, now)
		}
		if input.SSLEmail != "" {
			database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
				INSERT INTO server_config (key, value, updated_at)
				VALUES ('ssl.email', ?, ?)
				ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
			`, input.SSLEmail, now, input.SSLEmail, now)
		}
	}

	// Save multi-user settings
	if input.EnableMultiUser {
		database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
			INSERT INTO server_config (key, value, updated_at)
			VALUES ('features.multiuser', 'true', ?)
			ON CONFLICT(key) DO UPDATE SET value = 'true', updated_at = ?
		`, now, now)
		if input.RegistrationMode != "" {
			database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
				INSERT INTO server_config (key, value, updated_at)
				VALUES ('users.registration_mode', ?, ?)
				ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
			`, input.RegistrationMode, now, input.RegistrationMode, now)
		}
	}

	// Redirect to complete (Step 6)
	basePath := strings.TrimSuffix(c.Request.URL.Path, "/services")
	c.Redirect(http.StatusFound, basePath+"/complete")
}

// CompleteSetup shows the setup completion page
// AI.md: Setup is complete when Primary Admin is created
func (h *SetupHandler) CompleteSetup(c *gin.Context) {
	// Mark setup as complete in database. updated_at is bound as canonical UTC
	// text rather than produced by SQLite's datetime('now'), which does not
	// exist on PostgreSQL or MySQL.
	now := dbtime.FormatSQLTimestamp(time.Now())
	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_config (key, value, type, description, updated_at)
		VALUES ('setup.completed', 'true', 'bool', 'Server setup completed', ?)
		ON CONFLICT(key) DO UPDATE SET value = 'true', updated_at = ?
	`, now, now)
	if err != nil {
		log.Printf("WARNING: CompleteSetup: failed to mark setup complete: %v", err)
	}

	// Get the admin path from config (derive from current URL)
	// Current URL is /server/{admin_path}/config/setup/complete
	path := c.Request.URL.Path
	parts := strings.Split(path, "/")
	adminPath := "/server/admin" // default
	if len(parts) > 2 && parts[1] == "server" && parts[2] != "" {
		adminPath = "/server/" + parts[2]
	}

	c.HTML(http.StatusOK, "page/setup_complete.tmpl", util.TemplateData(c, gin.H{
		"Title":     "Setup Complete - Weather",
		"AdminPath": adminPath,
	}))
}

// GetSetupStatus returns the current setup status as a healthz endpoint
func (h *SetupHandler) GetSetupStatus(c *gin.Context) {
	cfg, _ := config.LoadConfig()
	adminPath := "/server/admin"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	// Check if a primary admin exists
	var adminCount int
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_admin_credentials").Scan(&adminCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to check admin status",
		})
		return
	}

	// No admin = setup not started
	if adminCount == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":      "not_started",
			"step":        0,
			"total_steps": 2,
			"message":     "Setup not started",
			"next_action": "Create primary admin account",
			"next_route":  adminPath,
			"is_complete": false,
		})
		return
	}

	// Check if setup is completed
	var setupComplete string
	err = database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'setup.completed'").Scan(&setupComplete)

	// If setup.completed doesn't exist or is not "true", continue the setup wizard
	if err != nil || setupComplete != "true" {
		c.JSON(http.StatusOK, gin.H{
			"status":      "admin_created",
			"step":        1,
			"total_steps": 2,
			"message":     "Primary admin created, setup wizard still in progress",
			"next_action": "Continue server setup",
			"next_route":  adminPath + "/config/setup/api-token",
			"is_complete": false,
		})
		return
	}

	// Setup is complete
	c.JSON(http.StatusOK, gin.H{
		"status":      "completed",
		"step":        2,
		"total_steps": 2,
		"message":     "Setup completed successfully",
		"next_action": "Access admin dashboard",
		"next_route":  adminPath,
		"is_complete": true,
	})
}
