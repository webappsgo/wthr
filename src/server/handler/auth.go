package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

type AuthHandler struct {
	DB *sql.DB
}

type CurrentUserProfileResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone,omitempty"`
	Role     string `json:"role"`
}

type UpdateCurrentUserProfileRequest struct {
	DisplayName string `json:"display_name"`
	Phone       string `json:"phone"`
}

// LoginRequest represents login request payload
type LoginRequest struct {
	// Can be username, email, or phone
	Identifier string `json:"identifier" binding:"required"`
	Password   string `json:"password" binding:"required"`
	// Two-factor authentication code (TOTP or recovery key)
	TwoFactorCode string `json:"two_factor_code"`
	// Set to true if using a recovery key instead of TOTP
	UseRecoveryKey bool `json:"use_recovery_key"`
}

// RegisterRequest represents registration request payload
type RegisterRequest struct {
	Username        string `json:"username" binding:"required"`
	Email           string `json:"email" binding:"required,email"`
	Password        string `json:"password" binding:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

// ShowLoginPage renders the login page
func (h *AuthHandler) ShowLoginPage(w http.ResponseWriter, r *http.Request) {
	// Check if already authenticated as admin (admin_session cookie)
	cfg := config.GetGlobalConfig()
	adminPath := "/server/" + cfg.GetAdminPath()
	adminSessionCookie, err := r.Cookie("admin_session")
	if err == nil && adminSessionCookie.Value != "" {
		adminSessionID := adminSessionCookie.Value
		// Validate admin session exists in database.
		// expires_at is scanned as a raw driver value and compared in Go rather
		// than tested with SQLite's datetime(): a row holding a non-UTC or
		// non-canonical layout makes datetime() return NULL, so the predicate
		// never matches and a live session is treated as expired. The Go
		// comparison also works on PostgreSQL and MySQL, which have no
		// datetime('now'). dbtime.IsAfter reports false for NULL and for
		// unparseable values, so an uninterpretable session stays invalid.
		var adminID int
		var storedExpiresAt interface{}
		err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
			SELECT admin_id, expires_at FROM server_admin_sessions
			WHERE id = ?
		`, adminSessionID).Scan(&adminID, &storedExpiresAt)
		if err == nil && dbtime.IsAfter(storedExpiresAt, time.Now()) {
			http.Redirect(w, r, adminPath, http.StatusFound)
			return
		}
	}

	// Check if already authenticated as user (weather_session cookie)
	if middleware.IsAuthenticated(r) {
		http.Redirect(w, r, "/users/dashboard", http.StatusFound)
		return
	}

	NegotiateResponse(w, r, "page/login.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":               "Login",
		"verified":            r.URL.Query().Get("verified") == "1",
		"pendingVerification": r.URL.Query().Get("pending_verification") == "1",
		"registrationPublic":  isPublicRegistrationEnabled(),
	}))
}

// ShowRegisterPage renders the registration page
func (h *AuthHandler) ShowRegisterPage(w http.ResponseWriter, r *http.Request) {
	if !isPublicRegistrationEnabled() {
		NegotiateErrorResponse(w, r, http.StatusNotFound, "page/error.tmpl", ErrNotFound, "Registration is not available", util.TemplateData(r, map[string]interface{}{
			"title": "Not Found",
		}))
		return
	}

	// Check if already authenticated
	if middleware.IsAuthenticated(r) {
		http.Redirect(w, r, "/users/dashboard", http.StatusFound)
		return
	}

	NegotiateResponse(w, r, "page/register.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": "Register",
	}))
}

// HandleLogin processes login requests
// Per spec: checks server_admin_credentials FIRST, then user_accounts
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	// Support both JSON and form data
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if !DecodeAndValidate(w, r, &req) {
			return
		}
	} else {
		// Accept both "identifier" and legacy "email" field names
		req.Identifier = r.PostFormValue("identifier")
		if req.Identifier == "" {
			// Backward compatibility
			req.Identifier = r.PostFormValue("email")
		}
		req.Password = r.PostFormValue("password")
		req.TwoFactorCode = r.PostFormValue("two_factor_code")
		req.UseRecoveryKey = r.PostFormValue("use_recovery_key") == "true"
	}

	// Trim whitespace from non-password fields
	req.Identifier = strings.TrimSpace(req.Identifier)
	req.TwoFactorCode = strings.TrimSpace(req.TwoFactorCode)

	// Passwords cannot start or end with whitespace
	if req.Password != strings.TrimSpace(req.Password) {
		respondWithError(w, r, http.StatusBadRequest, "Password cannot start or end with whitespace")
		return
	}

	// Identifier and password are required before any credential lookup
	if req.Identifier == "" || req.Password == "" {
		respondWithError(w, r, http.StatusBadRequest, "Identifier and password are required")
		return
	}

	// Step 1: Check server_admin_credentials FIRST
	adminModel := &model.AdminModel{DB: database.GetServerDB()}
	admin, adminErr := adminModel.VerifyCredentials(req.Identifier, req.Password)

	if adminErr == nil && admin != nil {
		// Admin password verified. If the admin has registered passkeys,
		// hold them in a pending state and require a passkey challenge
		// before issuing the admin_session cookie. Per AI.md PART 17 line
		// 28679 a passkey can be used as a primary login or as 2FA — for
		// admins we use it strictly as a 2nd factor here.
		serverDB := database.GetServerDB()
		hasPasskeys, hpkErr := AdminHasPasskeys(serverDB, admin.ID)
		if hpkErr != nil {
			respondWithError(w, r, http.StatusInternalServerError, "Failed to load passkey status")
			return
		}

		if hasPasskeys {
			pendingToken, perr := CreateAdminPendingSession(admin.ID, util.TrustedGetClientIP(r), r.UserAgent())
			if perr != nil {
				respondWithError(w, r, http.StatusInternalServerError, "Failed to create pending session")
				return
			}

			cfg := config.GetGlobalConfig()
			adminPath := "/server/" + cfg.GetAdminPath()

			if strings.Contains(contentType, "application/json") {
				writeAuthJSON(w, http.StatusOK, map[string]interface{}{
					"message":            "Passkey verification required",
					"type":               "admin",
					"requires_passkey":   true,
					"session_token":      pendingToken,
					"redirect":           adminPath,
					"challenge_endpoint": "/api/v1/server/auth/admin/passkey/challenge",
					"verify_endpoint":    "/api/v1/server/auth/admin/passkey/verify",
				})
			} else {
				// Non-JSON callers (HTML form login) get redirected to a
				// challenge page; the pending token is propagated via a
				// short-lived cookie so the in-browser JS can reach it.
				isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
				http.SetCookie(w, &http.Cookie{
					Name:     "admin_passkey_pending",
					Value:    pendingToken,
					Path:     "/",
					MaxAge:   int(time.Until(time.Now().Add(15 * time.Minute)).Seconds()),
					HttpOnly: true,
					Secure:   isHTTPS,
					SameSite: http.SameSiteLaxMode,
				})
				http.Redirect(w, r, adminPath+"/passkey", http.StatusFound)
			}
			return
		}

		// No passkeys registered — issue a full admin session. 30 days default.
		cfg := config.GetGlobalConfig()
		adminPath := "/server/" + cfg.GetAdminPath()

		adminSessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
		duration := 30 * 24 * time.Hour
		adminSession, err := adminSessionModel.CreateSession(admin.ID, util.TrustedGetClientIP(r), r.UserAgent(), duration)
		if err != nil {
			respondWithError(w, r, http.StatusInternalServerError, "Failed to create session")
			return
		}

		adminModel.UpdateLastLogin(admin.ID)

		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

		http.SetCookie(w, &http.Cookie{
			Name:     "admin_session",
			Value:    adminSession.SessionID,
			Path:     "/",
			MaxAge:   int(duration.Seconds()),
			HttpOnly: true,
			Secure:   isHTTPS,
			SameSite: http.SameSiteLaxMode,
		})

		if strings.Contains(contentType, "application/json") {
			writeAuthJSON(w, http.StatusOK, map[string]interface{}{
				"message":  "Login successful",
				"type":     "admin",
				"redirect": adminPath,
				"admin": map[string]interface{}{
					"id":       admin.ID,
					"username": admin.Username,
					"email":    admin.Email,
				},
			})
		} else {
			http.Redirect(w, r, adminPath, http.StatusFound)
		}
		return
	}

	// Step 2: Check user_accounts
	userModel := &model.UserModel{DB: h.DB}
	user, err := userModel.GetByIdentifier(req.Identifier)
	if err != nil {
		respondWithError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if !userModel.CheckPassword(user, req.Password) {
		respondWithError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if requiresEmailVerification() && !user.EmailVerified {
		respondWithError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
		// If 2FA code not provided, return specific response
		if req.TwoFactorCode == "" {
			if strings.Contains(contentType, "application/json") {
				writeAuthJSON(w, http.StatusUnauthorized, map[string]interface{}{
					"error":       "Two-factor authentication required",
					"require_2fa": true,
					"user_id":     user.ID,
				})
			} else {
				// Render login page with 2FA prompt
				middleware.RenderHTML(w, r, http.StatusOK, "page/login.tmpl", util.TemplateData(r, map[string]interface{}{
					"title":       "Login - Two-Factor Required",
					"require_2fa": true,
					"identifier":  req.Identifier,
				}))
			}
			return
		}

		// Verify 2FA code (TOTP or recovery key)
		var verified bool
		if req.UseRecoveryKey {
			// Verify recovery key
			recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
			verified, err = recoveryKeyModel.VerifyAndUseRecoveryKey(int(user.ID), req.TwoFactorCode)
			if err != nil {
				respondWithError(w, r, http.StatusInternalServerError, "Failed to verify recovery key")
				return
			}
		} else {
			// Verify TOTP code
			var secret string
			secret, err = model.DecryptTwoFactorSecret(user.TwoFactorSecret)
			if err != nil {
				respondWithError(w, r, http.StatusInternalServerError, "Failed to verify 2FA code")
				return
			}
			verified, err = util.VerifyTOTP(secret, req.TwoFactorCode)
			if err != nil {
				respondWithError(w, r, http.StatusInternalServerError, "Failed to verify 2FA code")
				return
			}
		}

		if !verified {
			respondWithError(w, r, http.StatusUnauthorized, "Invalid two-factor authentication code")
			return
		}
	}

	// Get session timeout from settings
	sessionTimeout, err := h.getSessionTimeout()
	if err != nil {
		// Default 30 days
		sessionTimeout = 2592000
	}

	// Create user session
	sessionModel := &model.SessionModel{DB: h.DB}
	session, err := sessionModel.Create(user.ID, sessionTimeout)
	if err != nil {
		respondWithError(w, r, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Set weather_session cookie (user sessions only)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   sessionTimeout,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Respond based on request type
	if strings.Contains(contentType, "application/json") {
		writeAuthJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Login successful",
			"type":    "user",
			"user": map[string]interface{}{
				"id":       user.ID,
				"username": user.Username,
				"email":    user.Email,
				"role":     user.Role,
			},
		})
	} else {
		// Check for redirect parameter
		redirect := r.URL.Query().Get("redirect")
		if redirect != "" && strings.HasPrefix(redirect, "/") {
			http.Redirect(w, r, redirect, http.StatusFound)
		} else {
			http.Redirect(w, r, "/users/dashboard", http.StatusFound)
		}
	}
}

// HandleRegister processes registration requests
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if !isPublicRegistrationEnabled() {
		respondWithError(w, r, http.StatusNotFound, "Registration is not available")
		return
	}

	var req RegisterRequest

	// Support both JSON and form data
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if !DecodeAndValidate(w, r, &req) {
			return
		}
	} else {
		req.Username = r.PostFormValue("username")
		req.Email = r.PostFormValue("email")
		req.Password = r.PostFormValue("password")
		req.ConfirmPassword = r.PostFormValue("confirm_password")
	}

	// Trim whitespace from non-password fields
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	// Passwords cannot start or end with whitespace
	if req.Password != strings.TrimSpace(req.Password) {
		respondWithError(w, r, http.StatusBadRequest, "Password cannot start or end with whitespace")
		return
	}

	// Validate passwords match
	if req.Password != req.ConfirmPassword {
		respondWithError(w, r, http.StatusBadRequest, "Passwords do not match")
		return
	}

	if err := util.ValidateEmail(req.Email); err != nil {
		respondWithError(w, r, http.StatusBadRequest, "Please enter a valid email address")
		return
	}

	userModel := &model.UserModel{DB: h.DB}

	// Validate username
	if err := util.ValidateUsername(req.Username); err != nil {
		respondWithError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Normalize username
	username := util.NormalizeUsername(req.Username)

	// All users created via /register are regular users.
	// Admin accounts are created through the /{admin_path}/config/setup wizard on first run.
	role := "user"

	// Create user
	user, err := userModel.Create(username, req.Email, req.Password, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respondWithError(w, r, http.StatusBadRequest, "Unable to complete registration. [Forgot credentials?](/server/auth/password/forgot)")
			return
		}
		respondWithError(w, r, http.StatusInternalServerError, "Failed to create account. Please try again later.")
		return
	}

	if requiresEmailVerification() {
		if _, err := createUserEmailVerification(user.ID, user.Email); err != nil {
			respondWithError(w, r, http.StatusInternalServerError, "Failed to start email verification")
			return
		}

		if strings.Contains(contentType, "application/json") {
			writeAuthJSON(w, http.StatusCreated, map[string]interface{}{
				"message":               "Registration successful. Please verify your email before logging in.",
				"verification_required": true,
				"user": map[string]interface{}{
					"id":       user.ID,
					"username": user.Username,
					"email":    user.Email,
					"role":     user.Role,
				},
			})
			return
		}

		http.Redirect(w, r, "/server/auth/login?pending_verification=1", http.StatusFound)
		return
	}

	// Get session timeout from settings
	sessionTimeout, err := h.getSessionTimeout()
	if err != nil {
		// Default 30 days
		sessionTimeout = 2592000
	}

	// Auto-login after registration
	sessionModel := &model.SessionModel{DB: h.DB}
	session, err := sessionModel.Create(user.ID, sessionTimeout)
	if err != nil {
		respondWithError(w, r, http.StatusInternalServerError, "User created but failed to login")
		return
	}

	// Set session cookie with proper security settings per AI.md PART 11
	// Secure: auto (based on TLS), HttpOnly: true, SameSite: Lax
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   sessionTimeout,
		HttpOnly: true,
		// Secure: auto-detect based on TLS (AI.md: secure: auto)
		Secure: r.TLS != nil,
		// SameSite: Lax per AI.md session configuration
		SameSite: http.SameSiteLaxMode,
	})

	// Respond based on request type
	if strings.Contains(contentType, "application/json") {
		writeAuthJSON(w, http.StatusCreated, map[string]interface{}{
			"message": "Registration successful",
			"user": map[string]interface{}{
				"id":       user.ID,
				"username": user.Username,
				"email":    user.Email,
				"role":     user.Role,
			},
			"redirect": "/users",
		})
	} else {
		// Honor ?redirect= param, but never redirect to admin routes
		redirect := r.URL.Query().Get("redirect")
		if redirect == "" || strings.HasPrefix(redirect, "/server/admin") {
			redirect = "/users"
		}
		http.Redirect(w, r, redirect, http.StatusFound)
	}
}

// HandleLogout processes logout requests
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, exists := middleware.GetCurrentSession(r)
	if exists {
		sessionModel := &model.SessionModel{DB: h.DB}
		sessionModel.Delete(session.ID)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		Domain:   "",
		Secure:   false,
		HttpOnly: true,
	})

	// Respond based on request type
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "application/json") {
		writeAuthJSON(w, http.StatusOK, map[string]interface{}{"message": "Logged out successfully"})
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// GetCurrentUser returns current user info
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	response, err := LoadCurrentUserProfile(h.DB, user.ID)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load current user"})
		return
	}

	writeAuthJSON(w, http.StatusOK, response)
}

// UpdateProfile updates user profile (display name and phone only)
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	var req UpdateCurrentUserProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}

	if err := UpdateCurrentUserProfile(h.DB, user.ID, &req); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to update profile"})
		return
	}

	writeAuthJSON(w, http.StatusOK, map[string]interface{}{"message": "Profile updated successfully"})
}

// LoadCurrentUserProfile returns the same current-user payload used by GET /api/v1/users.
func LoadCurrentUserProfile(db *sql.DB, userID int64) (*CurrentUserProfileResponse, error) {
	userModel := &model.UserModel{DB: db}
	user, err := userModel.GetByID(userID)
	if err != nil {
		return nil, err
	}

	return &CurrentUserProfileResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Phone:    user.Phone,
		Role:     user.Role,
	}, nil
}

// UpdateCurrentUserProfile applies the same profile update used by PATCH /api/v1/users.
func UpdateCurrentUserProfile(db *sql.DB, userID int64, req *UpdateCurrentUserProfileRequest) error {
	if req == nil {
		return fmt.Errorf("invalid request")
	}

	userModel := &model.UserModel{DB: db}
	return userModel.UpdateProfile(userID, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Phone))
}

// Helper functions

func (h *AuthHandler) getSessionTimeout() (int, error) {
	var timeoutStr string

	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = ?", "auth.session_timeout").Scan(&timeoutStr)
	if err != nil {
		return 0, err
	}

	timeout, err := strconv.Atoi(timeoutStr)
	if err != nil {
		return 0, err
	}

	return timeout, nil
}

// writeAuthJSON writes the non-canonical ad-hoc {"error": "..."}/{"message": "..."}
// shapes this handler has always used, preserved verbatim (this predates the
// canonical {"ok","error":CODE,"message"} response format and is not upgraded
// here — a mechanical framework conversion must not change response bodies).
func writeAuthJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondWithError(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		writeAuthJSON(w, statusCode, map[string]interface{}{"error": message})
	} else {
		// For form submissions, render inline error on same page
		// Get the current path to determine which template to render
		path := r.URL.Path

		if strings.Contains(path, "login") {
			middleware.RenderHTML(w, r, statusCode, "page/login.tmpl", util.TemplateData(r, map[string]interface{}{
				"title":              "Login",
				"error":              message,
				"registrationPublic": isPublicRegistrationEnabled(),
			}))
		} else if strings.Contains(path, "register") {
			middleware.RenderHTML(w, r, statusCode, "page/register.tmpl", util.TemplateData(r, map[string]interface{}{
				"title": "Register",
				"error": message,
			}))
		} else {
			// Fallback to error page for other cases
			middleware.RenderHTML(w, r, statusCode, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
				"error": message,
			}))
		}
	}
}

func isPublicRegistrationEnabled() bool {
	return config.IsMultiUserEnabled() && config.IsRegistrationPublic()
}

func requiresEmailVerification() bool {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return false
	}

	return cfg.Users.Registration.RequireEmailVerification
}

func createUserEmailVerification(userID int64, email string) (*model.UserEmailVerification, error) {
	verificationModel := &model.UserEmailVerificationModel{DB: database.GetUsersDB()}
	return verificationModel.CreateVerification(userID, email)
}
