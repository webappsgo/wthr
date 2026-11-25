package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

type AuthHandler struct {
	DB   *database.Store
	SAML *SAMLAuthHandler
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
	adminSessionCookie, err := r.Cookie(middleware.AdminSessionCookieName())
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
		_, storedExpiresAt, err := model.LookupActiveAdminSession(database.GetServerStore(), database.GetUsersStore(), model.HashAdminSessionToken(adminSessionID))
		if err == nil && dbtime.IsAfter(storedExpiresAt, time.Now()) {
			http.Redirect(w, r, adminPath, http.StatusFound)
			return
		}
	}

	// Check if already authenticated as user (user session cookie)
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
// Per spec: checks admins FIRST, then users
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

	// Step 1: Check admins FIRST
	adminModel := &model.AdminModel{Store: database.GetUsersStore()}
	admin, adminErr := adminModel.VerifyCredentials(req.Identifier, req.Password)

	if adminErr == nil && admin != nil {
		// Admin password verified. If the admin has registered passkeys,
		// hold them in a pending state and require a passkey challenge
		// before issuing the admin_session cookie. Per AI.md PART 17 line
		// 28679 a passkey can be used as a primary login or as 2FA — for
		// admins we use it strictly as a 2nd factor here.
		hasPasskeys, hpkErr := AdminHasPasskeys(database.GetUsersStore(), admin.ID)
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
					"message":            Translate(r, "errors.auth.passkey_verification_required"),
					"type":               "admin",
					"requires_passkey":   true,
					"session_token":      pendingToken,
					"redirect":           adminPath,
					"challenge_endpoint": cfg.GetAPIPath() + "/server/auth/admin/passkey/challenge",
					"verify_endpoint":    cfg.GetAPIPath() + "/server/auth/admin/passkey/verify",
				})
			} else {
				// Non-JSON callers (HTML form login) get redirected to a
				// challenge page; the pending token is propagated via a
				// short-lived cookie so the in-browser JS can reach it.
				isHTTPS := util.TrustedIsHTTPS(r)
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

		// TOTP admins must prove possession before an admin session is issued.
		// If passkeys are also registered, the passkey challenge above remains
		// the selected second factor for this login attempt.
		mfaStatus, mfaErr := model.AdminTOTPStatusFor(database.GetUsersStore(), admin.ID)
		if mfaErr != nil {
			respondWithError(w, r, http.StatusInternalServerError, "Failed to load admin MFA status")
			return
		}
		if mfaStatus.Enabled {
			if req.TwoFactorCode == "" {
				if strings.Contains(contentType, "application/json") {
					writeAuthJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": Translate(r, "errors.auth.two_factor_required"), "require_2fa": true, "type": "admin"})
				} else {
					middleware.RenderHTML(w, r, http.StatusOK, "page/login.tmpl", util.TemplateData(r, map[string]interface{}{
						"title": Translate(r, "login.2fa_required_title"), "require_2fa": true, "identifier": req.Identifier,
						"error": Translate(r, "errors.auth.two_factor_required"),
					}))
				}
				return
			}
			valid, verifyErr := VerifyAdminTOTP(database.GetUsersStore(), admin.ID, req.TwoFactorCode, req.UseRecoveryKey)
			if verifyErr != nil || !valid {
				respondWithError(w, r, http.StatusUnauthorized, Translate(r, "errors.user.public.invalid_two_factor_code"))
				return
			}
		}

		// No passkeys registered — issue a full admin session. 30 days default.
		cfg := config.GetGlobalConfig()
		adminPath := "/server/" + cfg.GetAdminPath()

		adminSessionModel := &model.AdminSessionModel{Store: database.GetServerStore()}
		duration := middleware.SessionSettingsFor(middleware.SessionAccountAdmin).MaxAge
		adminSession, err := adminSessionModel.CreateSession(admin.ID, util.TrustedGetClientIP(r), r.UserAgent(), duration)
		if err != nil {
			respondWithError(w, r, http.StatusInternalServerError, "Failed to create session")
			return
		}

		if err := adminModel.UpdateLastLogin(admin.ID); err != nil {
			log.Printf("WARNING: failed to update last login for admin %d: %v", admin.ID, err)
		}

		http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountAdmin, adminSession.SessionID, time.Duration(int(duration.Seconds()))*time.Second))

		if strings.Contains(contentType, "application/json") {
			writeAuthJSON(w, http.StatusOK, map[string]interface{}{
				"message":  Translate(r, "success.auth.login"),
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

	// Step 2: Check users
	userModel := &model.UserModel{Store: h.DB}
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
			recoveryKeyModel := &model.RecoveryKeyModel{Store: h.DB}
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
	sessionModel := &model.SessionModel{Store: h.DB}
	session, err := sessionModel.CreateSession(user.ID, sessionTimeout)
	if err != nil {
		respondWithError(w, r, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Set the user session cookie (user sessions only)
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountUser, session.ID, time.Duration(sessionTimeout)*time.Second))

	// Respond based on request type
	if strings.Contains(contentType, "application/json") {
		writeAuthJSON(w, http.StatusOK, map[string]interface{}{
			"message": Translate(r, "success.auth.login"),
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

	userModel := &model.UserModel{Store: h.DB}

	// Normalize username first (per AI.md PART 22: validation accepts lowercase only)
	username := util.NormalizeUsername(req.Username)

	// Validate username
	if err := util.ValidateUsername(username); err != nil {
		respondWithError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// All users created via /register are regular users.
	// Admin accounts are created through the /{admin_path}/setup wizard on first run.
	role := "user"

	// Create user
	user, err := userModel.CreateUserAccount(username, req.Email, req.Password, role)
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
				"message":               Translate(r, "success.auth.registration_verify_email"),
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
	sessionModel := &model.SessionModel{Store: h.DB}
	session, err := sessionModel.CreateSession(user.ID, sessionTimeout)
	if err != nil {
		respondWithError(w, r, http.StatusInternalServerError, "User created but failed to login")
		return
	}

	// Set session cookie with proper security settings per AI.md PART 11
	// Secure: auto (based on TLS), HttpOnly: true, SameSite: Lax
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountUser, session.ID, time.Duration(sessionTimeout)*time.Second))

	// Respond based on request type
	if strings.Contains(contentType, "application/json") {
		writeAuthJSON(w, http.StatusCreated, map[string]interface{}{
			"message": Translate(r, "success.auth.registration"),
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
		if redirect == "" || strings.HasPrefix(redirect, "/server/administration") {
			redirect = "/users"
		}
		http.Redirect(w, r, redirect, http.StatusFound)
	}
}

// HandleLogout processes logout requests
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if h.SAML != nil && h.SAML.TryStartSAMLLogout(w, r) {
		return
	}
	// Get session from context
	session, exists := middleware.GetCurrentSession(r)
	if exists {
		sessionModel := &model.SessionModel{Store: h.DB}
		if err := sessionModel.DeleteSession(session.ID); err != nil {
			log.Printf("WARNING: logout: failed to delete user session: %v", err)
		}
	}

	// AI.md PART 17: one logout endpoint ends a Server Admin session as well
	adminSessionEnded := false
	if cookie, err := r.Cookie(middleware.AdminSessionCookieName()); err == nil && cookie.Value != "" {
		adminSessionModel := &model.AdminSessionModel{Store: database.GetServerStore()}
		if err := adminSessionModel.DeleteSession(cookie.Value); err != nil {
			log.Printf("WARNING: logout: failed to delete admin session: %v", err)
		}
		http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountAdmin))
		adminSessionEnded = true
	}

	// Clear session cookie
	http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountUser))

	// Respond based on request type
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "application/json") {
		writeAuthJSON(w, http.StatusOK, map[string]interface{}{"message": Translate(r, "success.auth.logout")})
	} else {
		if adminSessionEnded {
			http.Redirect(w, r, "/server/auth/login", http.StatusFound)
			return
		}
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

	writeAuthJSON(w, http.StatusOK, map[string]interface{}{"message": Translate(r, "success.auth.profile_updated")})
}

// LoadCurrentUserProfile returns the same current-user payload used by GET /api/v1/users.
func LoadCurrentUserProfile(db *database.Store, userID int64) (*CurrentUserProfileResponse, error) {
	userModel := &model.UserModel{Store: db}
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
func UpdateCurrentUserProfile(db *database.Store, userID int64, req *UpdateCurrentUserProfileRequest) error {
	if req == nil {
		return fmt.Errorf("invalid request")
	}

	userModel := &model.UserModel{Store: db}
	return userModel.UpdateProfile(userID, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Phone))
}

// Helper functions

func (h *AuthHandler) getSessionTimeout() (int, error) {
	value, exists, err := config.ReadSetting("auth.session_timeout")
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, sql.ErrNoRows
	}
	timeoutStr := fmt.Sprint(value)

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
	return config.IsMultiUserEnabled() && config.IsRegistrationOpen()
}

func requiresEmailVerification() bool {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return false
	}
	if !cfg.Users.Registration.RequireEmailVerification {
		return false
	}

	// AI.md PART 18 (SMTP Requirement): with no working SMTP server, email
	// functionality is completely disabled and email verification is skipped
	// entirely (addresses are auto-verified), so registration must not strand
	// a new account on a verification step that can never complete.
	smtpService := service.SharedSMTPService(database.GetServerStore())
	if err := smtpService.LoadConfig(); err != nil {
		return false
	}
	return smtpService.IsEnabled()
}

func createUserEmailVerification(userID int64, email string) (*model.UserEmailVerification, error) {
	verificationModel := &model.UserEmailVerificationModel{Store: database.GetUsersStore()}
	return verificationModel.CreateVerification(userID, email)
}
