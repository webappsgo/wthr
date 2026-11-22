package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

type SetupHandler struct {
	DB *sql.DB
}

// ShowSetupTokenEntry shows the setup token entry form
// AI.md: First step of setup - user must enter setup token displayed in console
func (h *SetupHandler) ShowSetupTokenEntry(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_token.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title": Translate(r, "setup.server_setup_enter_setup_token")}))
}

// VerifySetupTokenAtAdmin handles setup token verification at /server/administration/verify-token
// AI.md: Step 2: User navigates to /server/administration → Step 3: User enters setup token → Step 4: Redirect to setup wizard
func (h *SetupHandler) VerifySetupTokenAtAdmin(w http.ResponseWriter, r *http.Request) {
	// Get admin path from config
	cfg, _ := config.CurrentConfig()
	adminPath := "/server/administration"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	setupToken := r.PostFormValue("setup_token")
	if setupToken == "" {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "admin/setup_token.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": map[string]interface{}{
				"Title": title,
			},
			"error": Translate(r, "errors.setup.setup_token_is_required")}))
		return
	}

	// Validate setup token against stored hash
	configDir := path.GetConfigDir()
	valid, err := util.ValidateSetupToken(configDir, setupToken)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "admin/setup_token.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": map[string]interface{}{
				"Title": title,
			},
			"error": Translate(r, "errors.setup.setup_token_not_found_or_already_used")}))
		return
	}

	if !valid {
		middleware.RenderHTML(w, r, http.StatusUnauthorized, "admin/setup_token.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":      title + " - Setup",
			"admin_path": adminPath,
			"branding": map[string]interface{}{
				"Title": title,
			},
			"error": Translate(r, "errors.setup.invalid_setup_token")}))
		return
	}

	// Store validated token in session for the admin creation step
	setSetupVerifiedCookie(w, r, cfg)

	// Redirect to setup wizard at /{admin_path}/setup
	// AI.md: Step 4: Redirect to /{admin_path}/setup (setup wizard)
	http.Redirect(w, r, adminPath+"/setup", http.StatusFound)
}

// VerifySetupToken validates the setup token (API endpoint)
// AI.md: Setup token stored as SHA-256 hash in {config_dir}/setup_token.txt
func (h *SetupHandler) VerifySetupToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SetupToken string `json:"setup_token"`
	}

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			BadRequest(w, r, Translate(r, "errors.setup.setup_token_is_required"))
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			BadRequest(w, r, Translate(r, "errors.setup.setup_token_is_required"))
			return
		}
		input.SetupToken = r.FormValue("setup_token")
	}

	if input.SetupToken == "" {
		BadRequest(w, r, Translate(r, "errors.setup.setup_token_is_required"))
		return
	}

	// Validate setup token against stored hash
	configDir := path.GetConfigDir()
	valid, err := util.ValidateSetupToken(configDir, input.SetupToken)
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.setup.setup_token_not_found_or_already_used"))
		return
	}

	if !valid {
		Unauthorized(w, r, Translate(r, "errors.setup.invalid_setup_token"))
		return
	}

	// Store validated token in session for the admin creation step
	// Use a secure session cookie to track that token was validated
	// Get admin path for redirect
	cfg, _ := config.CurrentConfig()
	setSetupVerifiedCookie(w, r, cfg)

	adminPath := "/server/administration"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"redirect": adminPath + "/setup",
	})
}

// ShowAdminSetup shows the admin creation form
// AI.md: Setup wizard step - create Primary Admin
func (h *SetupHandler) ShowAdminSetup(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_admin.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title": Translate(r, "setup.create_administrator") + " - " + title,
	}))
}

// setSetupVerifiedCookie issues the setup_token_verified cookie after a
// successful setup-token verification. The value is an HMAC-signed proof
// (util.SignSetupProof) rather than a literal "true", so the wizard cannot be
// reached by a client that simply sets the cookie itself.
func setSetupVerifiedCookie(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig) {
	if cfg == nil {
		return
	}

	isHTTPS := util.TrustedIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "setup_token_verified",
		Value:    util.SignSetupProof(cfg.Server.Security.EncryptionKey, time.Now()),
		MaxAge:   3600,
		Path:     "/",
		Secure:   isHTTPS,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// setupTokenVerified reports whether the request carries a setup_token_verified
// cookie this server minted and has not yet expired.
func setupTokenVerified(r *http.Request) bool {
	cookie, err := r.Cookie("setup_token_verified")
	if err != nil {
		return false
	}

	cfg, _ := config.CurrentConfig()
	if cfg == nil {
		return false
	}

	return util.VerifySetupProof(cfg.Server.Security.EncryptionKey, cookie.Value, time.Now())
}

// setupError renders error for form submissions or returns JSON for API
func (h *SetupHandler) setupError(w http.ResponseWriter, r *http.Request, status int, errorMsg string) {
	// Check Accept header to determine response type
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		writeJSON(w, status, map[string]interface{}{"error": errorMsg})
		return
	}

	// HTML form submission - re-render form with error
	cfg, _ := config.CurrentConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	middleware.RenderHTML(w, r, status, "page/setup_admin.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title": Translate(r, "setup.create_administrator") + " - " + title,
		"error": errorMsg,
	}))
}

// CreateAdmin creates the Primary Admin account
// AI.md: Setup creates Primary Admin, requires setup token verification
// AI.md PART 16: Works without JavaScript - form POST returns redirect
func (h *SetupHandler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	// Verify setup token was validated
	if !setupTokenVerified(r) {
		h.setupError(w, r, http.StatusUnauthorized, Translate(r, "errors.setup.setup_token_not_verified"))
		return
	}

	var input struct {
		Username        string `json:"username"`
		Email           string `json:"email"`
		UseRandom       bool   `json:"use_random"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}

	// Accept both JSON and form data
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.invalid_form_data"))
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.invalid_form_data"))
			return
		}
		input.Username = r.FormValue("username")
		input.Email = r.FormValue("email")
		input.UseRandom = config.IsTruthy(r.FormValue("use_random"))
		input.Password = r.FormValue("password")
		input.ConfirmPassword = r.FormValue("confirm_password")
	}

	// Validate email
	email := strings.TrimSpace(input.Email)
	if email == "" {
		h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.email_is_required"))
		return
	}
	// Basic email validation
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.invalid_email_format"))
		return
	}

	// Set default username if empty
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = "administrator"
	}

	// Normalize username
	username = strings.ToLower(username)

	// AI.md PART 17: syntax and length rules, never an admin section name
	if err := util.ValidateAdminUsername(username); err != nil {
		h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.invalid_username"))
		return
	}

	var password string
	var generatedPassword string
	var err error

	if input.UseRandom {
		// Generate random password (32 characters, alphanumeric + special)
		generatedPassword, err = generateRandomPassword(32)
		if err != nil {
			h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.failed_to_generate_secure_password"))
			return
		}
		password = generatedPassword
	} else {
		// Use custom password - must be confirmed
		if input.Password == "" {
			h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.password_is_required"))
			return
		}
		// Passwords cannot start or end with whitespace
		if input.Password != strings.TrimSpace(input.Password) {
			h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.password_cannot_start_or_end_with_whitespace"))
			return
		}
		if len(input.Password) < 12 {
			h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.password_must_be_at_least_12_characters"))
			return
		}
		if input.Password != input.ConfirmPassword {
			h.setupError(w, r, http.StatusBadRequest, Translate(r, "errors.setup.passwords_do_not_match"))
			return
		}
		password = input.Password
	}

	// Check if admin username already exists in admins
	var count int
	err = database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM admins WHERE username = ?", username).Scan(&count)
	if err != nil {
		h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.database_error"))
		return
	}

	if count > 0 {
		h.setupError(w, r, http.StatusConflict, Translate(r, "errors.setup.username_already_exists"))
		return
	}

	// Hash password using Argon2id (AI.md PART 3 requirement)
	hashedPassword, err := util.HashPassword(password)
	if err != nil {
		h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.failed_to_hash_password"))
		return
	}

	// Create administrator account in admins (NOT users).
	// created_at/updated_at are bound as canonical UTC text rather than produced
	// by SQL's CURRENT_TIMESTAMP, which yields a different type and zone on
	// PostgreSQL, MySQL and SQL Server than it does on SQLite.
	createdAt := dbtime.FormatSQLTimestamp(time.Now())
	result, err := database.ExecContext(context.Background(), database.GetUsersDB(), database.TimeoutWrite, `
		INSERT INTO admins (username, email, password_hash, is_super_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, 1, 1, ?, ?)
	`, username, email, hashedPassword, createdAt, createdAt)

	if err != nil {
		h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.failed_to_create_administrator"))
		return
	}

	// Get the newly created admin ID
	adminID, err := result.LastInsertId()
	if err != nil {
		h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.failed_to_retrieve_admin_id"))
		return
	}

	// Create the initial administrator's session through the shared model so
	// the cookie token is hashed and the database row receives a UUID.
	sessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
	adminSessionLifetime := middleware.SessionSettingsFor(middleware.SessionAccountAdmin).MaxAge
	adminSession, err := sessionModel.CreateSession(adminID, util.TrustedGetClientIP(r), r.UserAgent(), adminSessionLifetime)
	if err != nil {
		h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.failed_to_create_session"))
		return
	}

	isHTTPS := util.TrustedIsHTTPS(r)

	// Set the admin session cookie (separate from the user session cookie)
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountAdmin, adminSession.SessionID, adminSessionLifetime))

	// Delete setup token file after successful admin creation
	// AI.md: File deleted after successful setup completion
	configDir := path.GetConfigDir()
	if err := util.DeleteSetupToken(configDir); err != nil {
		// Log but don't fail - admin was created successfully
		log.Printf("WARNING: CreateAdmin: failed to delete setup token file: %v", err)
	}

	// Clear the setup_token_verified cookie
	http.SetCookie(w, &http.Cookie{Name: "setup_token_verified", Value: "", MaxAge: -1, Path: "/", Secure: false, HttpOnly: true, SameSite: http.SameSiteStrictMode})

	// Generate API token for the new admin
	// AI.md: Step 2 - API Token auto-generated
	apiToken, err := generateAPIToken()
	if err != nil {
		h.setupError(w, r, http.StatusInternalServerError, Translate(r, "errors.setup.failed_to_generate_api_token"))
		return
	}

	// Store the API token's SHA-256 hash and display prefix in the unified tokens table
	err = (&model.AdminModel{DB: database.GetUsersDB()}).StoreDefaultAPIToken(adminID, apiToken)
	if err != nil {
		// Log but don't fail - admin was created successfully
		log.Printf("WARNING: CreateAdmin: failed to store admin API token: %v", err)
	}

	// Store the generated password and token in session for display on next step
	// These are shown once and must be copied by user
	if generatedPassword != "" {
		http.SetCookie(w, &http.Cookie{Name: "setup_generated_password", Value: generatedPassword, MaxAge: 3600, Path: "/", Secure: isHTTPS, HttpOnly: true})
	}
	http.SetCookie(w, &http.Cookie{Name: "setup_api_token", Value: apiToken, MaxAge: 3600, Path: "/", Secure: isHTTPS, HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "setup_username", Value: username, MaxAge: 3600, Path: "/", Secure: isHTTPS, HttpOnly: true})

	// Redirect to API token step (Step 2)
	// AI.md: Setup wizard Step 2 - API Token
	redirectURL := r.URL.Path + "/api-token"

	// Check Accept header to determine response type
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		response := map[string]interface{}{"ok": true, "redirect": redirectURL}
		if generatedPassword != "" {
			response["generated_password"] = generatedPassword
			response["username"] = username
		}
		response["api_token"] = apiToken
		writeJSON(w, http.StatusOK, response)
		return
	}

	// Form submission - redirect (works without JavaScript)
	http.Redirect(w, r, redirectURL, http.StatusFound)
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

// ShowAPIToken shows the API token page (Step 2)
// AI.md: Setup wizard Step 2 - API Token auto-generated, user MUST copy
func (h *SetupHandler) ShowAPIToken(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	// Get tokens from cookies (set during admin creation)
	var apiToken, generatedPassword, username string
	if ck, err := r.Cookie("setup_api_token"); err == nil {
		apiToken = ck.Value
	}
	if ck, err := r.Cookie("setup_generated_password"); err == nil {
		generatedPassword = ck.Value
	}
	if ck, err := r.Cookie("setup_username"); err == nil {
		username = ck.Value
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_api_token.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":             Translate(r, "setup.api_token") + " - " + title,
		"APIToken":          apiToken,
		"GeneratedPassword": generatedPassword,
		"Username":          username,
	}))
}

// ProcessAPIToken handles acknowledgment of API token (Step 2 → Step 3)
func (h *SetupHandler) ProcessAPIToken(w http.ResponseWriter, r *http.Request) {
	// Clear the sensitive cookies
	isHTTPS := util.TrustedIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{Name: "setup_api_token", Value: "", MaxAge: -1, Path: "/", Secure: isHTTPS, HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "setup_generated_password", Value: "", MaxAge: -1, Path: "/", Secure: isHTTPS, HttpOnly: true})

	// Redirect to server configuration (Step 3)
	basePath := strings.TrimSuffix(r.URL.Path, "/api-token")
	http.Redirect(w, r, basePath+"/config", http.StatusFound)
}

// ShowServerConfig shows the server configuration page (Step 3)
// AI.md: Setup wizard Step 3 - Server Configuration
func (h *SetupHandler) ShowServerConfig(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
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
	if r.URL.Query().Get("skip") == "true" {
		basePath := strings.TrimSuffix(r.URL.Path, "/config")
		http.Redirect(w, r, basePath+"/security", http.StatusFound)
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_server_config.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":           Translate(r, "setup.server_configuration") + " - " + title,
		"DefaultAppName":  title,
		"DefaultDomain":   defaultDomain,
		"DefaultMode":     defaultMode,
		"DefaultTimezone": defaultTimezone,
	}))
}

// ProcessServerConfig handles server configuration submission (Step 3 → Step 4)
func (h *SetupHandler) ProcessServerConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/setup_server_config.tmpl", util.TemplateData(r, map[string]interface{}{
			"Title": Translate(r, "setup.server_configuration"), "error": Translate(r, "errors.setup.invalid_form_data")}))
		return
	}

	input := struct {
		AppName  string
		Domain   string
		Mode     string
		Timezone string
	}{
		AppName:  r.FormValue("app_name"),
		Domain:   r.FormValue("domain"),
		Mode:     r.FormValue("mode"),
		Timezone: r.FormValue("timezone"),
	}

	// Save settings to the canonical YAML configuration files.
	settings := map[string]string{
		"server.branding.title": input.AppName,
		"server.domain":         input.Domain,
		"mode":                  input.Mode,
		"server.timezone":       input.Timezone,
	}

	for key, value := range settings {
		if value != "" {
			if err := (&model.SettingsModel{}).SetString(key, value); err != nil {
				log.Printf("WARNING: ProcessServerConfig: failed to save setting %s: %v", key, err)
			}
		}
	}

	// Redirect to security settings (Step 4)
	basePath := strings.TrimSuffix(r.URL.Path, "/config")
	http.Redirect(w, r, basePath+"/security", http.StatusFound)
}

// ShowSecurity shows the security settings page (Step 4)
// AI.md: Setup wizard Step 4 - Security Settings
func (h *SetupHandler) ShowSecurity(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	// Check for skip parameter
	if r.URL.Query().Get("skip") == "true" {
		basePath := strings.TrimSuffix(r.URL.Path, "/security")
		http.Redirect(w, r, basePath+"/services", http.StatusFound)
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_security.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title": Translate(r, "setup.security_settings") + " - " + title,
	}))
}

// ProcessSecurity handles security settings submission (Step 4 → Step 5)
func (h *SetupHandler) ProcessSecurity(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/setup_security.tmpl", util.TemplateData(r, map[string]interface{}{
			"Title": Translate(r, "setup.security_settings"), "error": Translate(r, "errors.setup.invalid_form_data")}))
		return
	}

	input := struct {
		BackupPassword        string
		BackupPasswordConfirm string
		Enable2FA             bool
		TOTPCode              string
	}{
		BackupPassword:        r.FormValue("backup_password"),
		BackupPasswordConfirm: r.FormValue("backup_password_confirm"),
		Enable2FA:             config.IsTruthy(r.FormValue("enable_2fa")),
		TOTPCode:              r.FormValue("totp_code"),
	}
	_ = input.Enable2FA
	_ = input.TOTPCode

	// Validate backup password match
	if input.BackupPassword != "" && input.BackupPassword != input.BackupPasswordConfirm {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/setup_security.tmpl", util.TemplateData(r, map[string]interface{}{
			"Title": Translate(r, "setup.security_settings"), "error": Translate(r, "errors.setup.backup_passwords_do_not_match")}))
		return
	}

	// Save backup encryption password if provided
	if input.BackupPassword != "" {
		cfg := config.GetGlobalConfig()
		if cfg == nil || cfg.Server.Security.EncryptionKey == "" {
			InternalError(w, r, Translate(r, "errors.setup.database_error"))
			return
		}
		sealedPassword, err := util.EncryptAtRest(cfg.Server.Security.EncryptionKey, input.BackupPassword)
		if err != nil {
			InternalError(w, r, Translate(r, "errors.setup.database_error"))
			return
		}
		if err := config.WriteSecret("server.backup.encryption_password", sealedPassword); err != nil {
			InternalError(w, r, Translate(r, "errors.setup.database_error"))
			return
		}
		_ = (&model.SettingsModel{}).DeleteSetting("backup.encryption_password")
		cfg.Server.Maintenance.Backup.Encryption.Enabled = true
		if err := config.SaveConfig(cfg); err != nil {
			InternalError(w, r, Translate(r, "errors.setup.database_error"))
			return
		}
	}

	// Handle 2FA setup (if enabled, this would involve TOTP verification)
	// For now, we skip this as it requires more complex flow

	// Redirect to optional services (Step 5)
	basePath := strings.TrimSuffix(r.URL.Path, "/security")
	http.Redirect(w, r, basePath+"/services", http.StatusFound)
}

// ShowServices shows the optional services page (Step 5)
// AI.md: Setup wizard Step 5 - Optional Services
func (h *SetupHandler) ShowServices(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
	title := "Weather"
	if cfg != nil && cfg.Server.Branding.Title != "" {
		title = cfg.Server.Branding.Title
	}

	// Check for skip parameter
	if r.URL.Query().Get("skip") == "true" {
		basePath := strings.TrimSuffix(r.URL.Path, "/services")
		http.Redirect(w, r, basePath+"/complete", http.StatusFound)
		return
	}

	// Check if Tor is available
	torAvailable := util.IsTorAvailable()

	// Get admin email for SSL contact
	var adminEmail string
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT email FROM admins LIMIT 1").Scan(&adminEmail); err != nil && err != sql.ErrNoRows {
		InternalError(w, r, Translate(r, "errors.common.internal_error"))
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_services.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":        Translate(r, "setup.optional_services") + " - " + title,
		"TorAvailable": torAvailable,
		"AdminEmail":   adminEmail,
	}))
}

// ProcessServices handles optional services submission (Step 5 → Step 6)
func (h *SetupHandler) ProcessServices(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/setup_services.tmpl", util.TemplateData(r, map[string]interface{}{
			"Title": Translate(r, "setup.optional_services"), "error": Translate(r, "errors.setup.invalid_form_data")}))
		return
	}

	input := struct {
		EnableSSL        bool
		SSLDomain        string
		SSLEmail         string
		EnableMultiUser  bool
		RegistrationMode string
	}{
		EnableSSL:        config.IsTruthy(r.FormValue("enable_ssl")),
		SSLDomain:        r.FormValue("ssl_domain"),
		SSLEmail:         r.FormValue("ssl_email"),
		EnableMultiUser:  config.IsTruthy(r.FormValue("enable_multiuser")),
		RegistrationMode: r.FormValue("registration_mode"),
	}

	settings := make(map[string]string)
	if input.EnableSSL {
		settings["ssl.enabled"] = "true"
		if input.SSLDomain != "" {
			settings["ssl.domain"] = input.SSLDomain
		}
		if input.SSLEmail != "" {
			settings["ssl.email"] = input.SSLEmail
		}
	}
	if input.EnableMultiUser {
		settings["features.multiuser"] = "true"
		if input.RegistrationMode != "" {
			settings["users.registration_mode"] = input.RegistrationMode
		}
	}
	for key, value := range settings {
		if err := (&model.SettingsModel{}).SetString(key, value); err != nil {
			InternalError(w, r, Translate(r, "errors.common.internal_error"))
			return
		}
	}

	// Redirect to complete (Step 6)
	basePath := strings.TrimSuffix(r.URL.Path, "/services")
	http.Redirect(w, r, basePath+"/complete", http.StatusFound)
}

// CompleteSetup shows the setup completion page
// AI.md: Setup is complete when Primary Admin is created
func (h *SetupHandler) CompleteSetup(w http.ResponseWriter, r *http.Request) {
	// Mark setup as complete in server.yml.
	if err := (&model.SettingsModel{}).SetBool("setup.completed", true); err != nil {
		log.Printf("WARNING: CompleteSetup: failed to mark setup complete: %v", err)
	}

	// Get the admin path from config (derive from current URL)
	// Current URL is /server/{admin_path}/setup/complete
	reqPath := r.URL.Path
	parts := strings.Split(reqPath, "/")
	adminPath := "/server/administration" // default
	if len(parts) > 2 && parts[1] == "server" && parts[2] != "" {
		adminPath = "/server/" + parts[2]
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/setup_complete.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":     Translate(r, "setup.setup_complete"),
		"AdminPath": adminPath,
	}))
}

// GetSetupStatus returns the current setup status as a healthz endpoint
func (h *SetupHandler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.CurrentConfig()
	adminPath := "/server/administration"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}

	// Check if a primary admin exists
	var adminCount int
	err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM admins").Scan(&adminCount)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"status":  "error",
			"message": Translate(r, "setup.status.failed_to_check_admin_status"),
		})
		return
	}

	// No admin = setup not started
	if adminCount == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":      "not_started",
			"step":        0,
			"total_steps": 2,
			"message":     Translate(r, "setup.status.setup_not_started"),
			"next_action": Translate(r, "setup.status.create_primary_admin_account"),
			"next_route":  adminPath,
			"is_complete": false,
		})
		return
	}

	// Check if setup is completed
	setupComplete, _, setupConfigErr := config.ReadSetting("setup.completed")

	// If setup.completed doesn't exist or is not "true", continue the setup wizard
	if setupConfigErr != nil || !config.IsTruthy(fmt.Sprint(setupComplete)) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":      "admin_created",
			"step":        1,
			"total_steps": 2,
			"message":     Translate(r, "setup.status.primary_admin_created_setup_wizard_still_in_progress"),
			"next_action": Translate(r, "setup.status.continue_server_setup"),
			"next_route":  adminPath + "/setup/api-token",
			"is_complete": false,
		})
		return
	}

	// Setup is complete
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "completed",
		"step":        2,
		"total_steps": 2,
		"message":     Translate(r, "setup.status.setup_completed_successfully"),
		"next_action": Translate(r, "setup.status.access_admin_dashboard"),
		"next_route":  adminPath,
		"is_complete": true,
	})
}
