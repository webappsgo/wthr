package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
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
func (h *AuthHandler) ShowLoginPage(c *gin.Context) {
	// Check if already authenticated as admin (admin_session cookie)
	cfg := config.GetGlobalConfig()
	adminPath := "/server/" + cfg.GetAdminPath()
	adminSessionID, err := c.Cookie("admin_session")
	if err == nil && adminSessionID != "" {
		// Validate admin session exists in database
		var adminID int
		err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
			SELECT admin_id FROM server_admin_sessions
			WHERE id = ? AND datetime(expires_at) > datetime('now')
		`, adminSessionID).Scan(&adminID)
		if err == nil {
			c.Redirect(http.StatusFound, adminPath)
			return
		}
	}

	// Check if already authenticated as user (weather_session cookie)
	if middleware.IsAuthenticated(c) {
		c.Redirect(http.StatusFound, "/users/dashboard")
		return
	}

	NegotiateResponse(c, "page/login.tmpl", utils.TemplateData(c, gin.H{
		"title":               "Login",
		"verified":            c.Query("verified") == "1",
		"pendingVerification": c.Query("pending_verification") == "1",
		"registrationPublic":  isPublicRegistrationEnabled(),
	}))
}

// ShowRegisterPage renders the registration page
func (h *AuthHandler) ShowRegisterPage(c *gin.Context) {
	if !isPublicRegistrationEnabled() {
		NegotiateErrorResponse(c, http.StatusNotFound, "page/error.tmpl", ErrNotFound, "Registration is not available", utils.TemplateData(c, gin.H{
			"title": "Not Found",
		}))
		return
	}

	// Check if already authenticated
	if middleware.IsAuthenticated(c) {
		c.Redirect(http.StatusFound, "/users/dashboard")
		return
	}

	NegotiateResponse(c, "page/register.tmpl", utils.TemplateData(c, gin.H{
		"title": "Register",
	}))
}

// HandleLogin processes login requests
// Per spec: checks server_admin_credentials FIRST, then user_accounts
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	var req LoginRequest

	// Support both JSON and form data
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
	} else {
		// Accept both "identifier" and legacy "email" field names
		req.Identifier = c.PostForm("identifier")
		if req.Identifier == "" {
			// Backward compatibility
			req.Identifier = c.PostForm("email")
		}
		req.Password = c.PostForm("password")
		req.TwoFactorCode = c.PostForm("two_factor_code")
		req.UseRecoveryKey = c.PostForm("use_recovery_key") == "true"
	}

	// Trim whitespace from non-password fields
	req.Identifier = strings.TrimSpace(req.Identifier)
	req.TwoFactorCode = strings.TrimSpace(req.TwoFactorCode)

	// Passwords cannot start or end with whitespace
	if req.Password != strings.TrimSpace(req.Password) {
		respondWithError(c, http.StatusBadRequest, "Password cannot start or end with whitespace")
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
			respondWithError(c, http.StatusInternalServerError, "Failed to load passkey status")
			return
		}

		if hasPasskeys {
			pendingToken, perr := CreateAdminPendingSession(admin.ID, c.ClientIP(), c.Request.UserAgent())
			if perr != nil {
				respondWithError(c, http.StatusInternalServerError, "Failed to create pending session")
				return
			}

			cfg := config.GetGlobalConfig()
			adminPath := "/server/" + cfg.GetAdminPath()

			if strings.Contains(contentType, "application/json") {
				c.JSON(http.StatusOK, gin.H{
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
				isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
				http.SetCookie(c.Writer, &http.Cookie{
					Name:     "admin_passkey_pending",
					Value:    pendingToken,
					Path:     "/",
					MaxAge:   int(time.Until(time.Now().Add(15 * time.Minute)).Seconds()),
					HttpOnly: true,
					Secure:   isHTTPS,
					SameSite: http.SameSiteLaxMode,
				})
				c.Redirect(http.StatusFound, adminPath+"/passkey")
			}
			return
		}

		// No passkeys registered — issue a full admin session. 30 days default.
		cfg := config.GetGlobalConfig()
		adminPath := "/server/" + cfg.GetAdminPath()

		adminSessionModel := &model.AdminSessionModel{DB: database.GetServerDB()}
		duration := 30 * 24 * time.Hour
		adminSession, err := adminSessionModel.CreateSession(admin.ID, c.ClientIP(), c.Request.UserAgent(), duration)
		if err != nil {
			respondWithError(c, http.StatusInternalServerError, "Failed to create session")
			return
		}

		adminModel.UpdateLastLogin(admin.ID)

		isHTTPS := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

		http.SetCookie(c.Writer, &http.Cookie{
			Name:     "admin_session",
			Value:    adminSession.SessionID,
			Path:     "/",
			MaxAge:   int(duration.Seconds()),
			HttpOnly: true,
			Secure:   isHTTPS,
			SameSite: http.SameSiteLaxMode,
		})

		if strings.Contains(contentType, "application/json") {
			c.JSON(http.StatusOK, gin.H{
				"message":  "Login successful",
				"type":     "admin",
				"redirect": adminPath,
				"admin": gin.H{
					"id":       admin.ID,
					"username": admin.Username,
					"email":    admin.Email,
				},
			})
		} else {
			c.Redirect(http.StatusFound, adminPath)
		}
		return
	}

	// Step 2: Check user_accounts
	userModel := &model.UserModel{DB: h.DB}
	user, err := userModel.GetByIdentifier(req.Identifier)
	if err != nil {
		respondWithError(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if !userModel.CheckPassword(user, req.Password) {
		respondWithError(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if requiresEmailVerification() && !user.EmailVerified {
		respondWithError(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
		// If 2FA code not provided, return specific response
		if req.TwoFactorCode == "" {
			if strings.Contains(contentType, "application/json") {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":       "Two-factor authentication required",
					"require_2fa": true,
					"user_id":     user.ID,
				})
			} else {
				// Render login page with 2FA prompt
				c.HTML(http.StatusOK, "page/login.tmpl", utils.TemplateData(c, gin.H{
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
				respondWithError(c, http.StatusInternalServerError, "Failed to verify recovery key")
				return
			}
		} else {
			// Verify TOTP code
			var secret string
			secret, err = model.DecryptTwoFactorSecret(user.TwoFactorSecret)
			if err != nil {
				respondWithError(c, http.StatusInternalServerError, "Failed to verify 2FA code")
				return
			}
			verified, err = utils.VerifyTOTP(secret, req.TwoFactorCode)
			if err != nil {
				respondWithError(c, http.StatusInternalServerError, "Failed to verify 2FA code")
				return
			}
		}

		if !verified {
			respondWithError(c, http.StatusUnauthorized, "Invalid two-factor authentication code")
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
		respondWithError(c, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Set weather_session cookie (user sessions only)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   sessionTimeout,
		HttpOnly: true,
		Secure:   c.Request.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Respond based on request type
	if strings.Contains(contentType, "application/json") {
		c.JSON(http.StatusOK, gin.H{
			"message": "Login successful",
			"type":    "user",
			"user": gin.H{
				"id":       user.ID,
				"username": user.Username,
				"email":    user.Email,
				"role":     user.Role,
			},
		})
	} else {
		// Check for redirect parameter
		redirect := c.Query("redirect")
		if redirect != "" && strings.HasPrefix(redirect, "/") {
			c.Redirect(http.StatusFound, redirect)
		} else {
			c.Redirect(http.StatusFound, "/users/dashboard")
		}
	}
}

// HandleRegister processes registration requests
func (h *AuthHandler) HandleRegister(c *gin.Context) {
	if !isPublicRegistrationEnabled() {
		respondWithError(c, http.StatusNotFound, "Registration is not available")
		return
	}

	var req RegisterRequest

	// Support both JSON and form data
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
	} else {
		req.Username = c.PostForm("username")
		req.Email = c.PostForm("email")
		req.Password = c.PostForm("password")
		req.ConfirmPassword = c.PostForm("confirm_password")
	}

	// Trim whitespace from non-password fields
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	// Passwords cannot start or end with whitespace
	if req.Password != strings.TrimSpace(req.Password) {
		respondWithError(c, http.StatusBadRequest, "Password cannot start or end with whitespace")
		return
	}

	// Validate passwords match
	if req.Password != req.ConfirmPassword {
		respondWithError(c, http.StatusBadRequest, "Passwords do not match")
		return
	}

	if err := utils.ValidateEmail(req.Email); err != nil {
		respondWithError(c, http.StatusBadRequest, "Please enter a valid email address")
		return
	}

	userModel := &model.UserModel{DB: h.DB}

	// Validate username
	if err := utils.ValidateUsername(req.Username); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}

	// Normalize username
	username := utils.NormalizeUsername(req.Username)

	// All users created via /register are regular users.
	// Admin accounts are created through the /{admin_path}/server/setup wizard on first run.
	role := "user"

	// Create user
	user, err := userModel.Create(username, req.Email, req.Password, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respondWithError(c, http.StatusBadRequest, "Unable to complete registration. [Forgot credentials?](/server/auth/password/forgot)")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "Failed to create account. Please try again later.")
		return
	}

	if requiresEmailVerification() {
		if _, err := createUserEmailVerification(user.ID, user.Email); err != nil {
			respondWithError(c, http.StatusInternalServerError, "Failed to start email verification")
			return
		}

		if strings.Contains(contentType, "application/json") {
			c.JSON(http.StatusCreated, gin.H{
				"message":               "Registration successful. Please verify your email before logging in.",
				"verification_required": true,
				"user": gin.H{
					"id":       user.ID,
					"username": user.Username,
					"email":    user.Email,
					"role":     user.Role,
				},
			})
			return
		}

		c.Redirect(http.StatusFound, "/server/auth/login?pending_verification=1")
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
		respondWithError(c, http.StatusInternalServerError, "User created but failed to login")
		return
	}

	// Set session cookie with proper security settings per AI.md PART 11
	// Secure: auto (based on TLS), HttpOnly: true, SameSite: Lax
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   sessionTimeout,
		HttpOnly: true,
		// Secure: auto-detect based on TLS (AI.md: secure: auto)
		Secure: c.Request.TLS != nil,
		// SameSite: Lax per AI.md session configuration
		SameSite: http.SameSiteLaxMode,
	})

	// Respond based on request type
	if strings.Contains(contentType, "application/json") {
		c.JSON(http.StatusCreated, gin.H{
			"message": "Registration successful",
			"user": gin.H{
				"id":       user.ID,
				"username": user.Username,
				"email":    user.Email,
				"role":     user.Role,
			},
			"redirect": "/users",
		})
	} else {
		// Honor ?redirect= param, but never redirect to admin routes
		redirect := c.Query("redirect")
		if redirect == "" || strings.HasPrefix(redirect, "/server/admin") {
			redirect = "/users"
		}
		c.Redirect(http.StatusFound, redirect)
	}
}

// HandleLogout processes logout requests
func (h *AuthHandler) HandleLogout(c *gin.Context) {
	// Get session from context
	session, exists := middleware.GetCurrentSession(c)
	if exists {
		sessionModel := &model.SessionModel{DB: h.DB}
		sessionModel.Delete(session.ID)
	}

	// Clear session cookie
	c.SetCookie(
		middleware.SessionCookieName,
		"",
		-1,
		"/",
		"",
		false,
		true,
	)

	// Respond based on request type
	acceptHeader := c.GetHeader("Accept")
	if strings.Contains(acceptHeader, "application/json") {
		c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
	} else {
		c.Redirect(http.StatusFound, "/")
	}
}

// GetCurrentUser returns current user info
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	response, err := LoadCurrentUserProfile(h.DB, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load current user"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// UpdateProfile updates user profile (display name and phone only)
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req UpdateCurrentUserProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := UpdateCurrentUserProfile(h.DB, user.ID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
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

func respondWithError(c *gin.Context, statusCode int, message string) {
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/json") {
		c.JSON(statusCode, gin.H{"error": message})
	} else {
		// For form submissions, render inline error on same page
		// Get the current path to determine which template to render
		path := c.Request.URL.Path

		if strings.Contains(path, "login") {
			c.HTML(statusCode, "page/login.tmpl", utils.TemplateData(c, gin.H{
				"title":              "Login",
				"error":              message,
				"registrationPublic": isPublicRegistrationEnabled(),
			}))
		} else if strings.Contains(path, "register") {
			c.HTML(statusCode, "page/register.tmpl", utils.TemplateData(c, gin.H{
				"title": "Register",
				"error": message,
			}))
		} else {
			// Fallback to error page for other cases
			c.HTML(statusCode, "page/error.tmpl", gin.H{
				"error": message,
			})
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
