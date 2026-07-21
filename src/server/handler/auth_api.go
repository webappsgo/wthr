// Package handler provides auth API handlers per AI.md PART 33
package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

// AuthAPIHandler handles auth API endpoints per AI.md PART 33
type AuthAPIHandler struct {
	DB *sql.DB
}

// NewAuthAPIHandler creates a new auth API handler
func NewAuthAPIHandler(db *sql.DB) *AuthAPIHandler {
	return &AuthAPIHandler{DB: db}
}

// APILoginRequest represents login API request per AI.md PART 33
type APILoginRequest struct {
	// Can be username, email, user_id, or phone
	Identifier    string `json:"identifier" binding:"required"`
	Password      string `json:"password" binding:"required"`
	TwoFactorCode string `json:"two_factor_code,omitempty"`
	RecoveryKey   string `json:"recovery_key,omitempty"`
}

// APIRegisterRequest represents registration API request per AI.md PART 33
type APIRegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

type API2FARequest struct {
	SessionToken  string `json:"session_token" binding:"required"`
	TwoFactorCode string `json:"two_factor_code" binding:"required"`
}

type APIRecoveryUseRequest struct {
	SessionToken string `json:"session_token" binding:"required"`
	RecoveryKey  string `json:"recovery_key" binding:"required"`
}

type APIVerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type APIPasswordForgotRequest struct {
	Email string `json:"email" binding:"required"`
}

type APIPasswordResetRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

type APIPasswordResetContext struct {
	ClientIP string
	FullHost string
}

type UserInviteValidationResponse struct {
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ServerInviteValidationResponse struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
}

type UserInviteCompletionResponse struct {
	Message string           `json:"message,omitempty"`
	Token   string           `json:"token,omitempty"`
	User    *AuthUserSummary `json:"user,omitempty"`
}

type InvitedAdminSummary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type ServerInviteCompletionResponse struct {
	Message string              `json:"message"`
	Admin   *InvitedAdminSummary `json:"admin"`
}

type AuthUserSummary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type AuthLoginResponse struct {
	RequiresTwoFactor bool             `json:"requires_2fa"`
	SessionToken      string           `json:"session_token,omitempty"`
	Token             string           `json:"token,omitempty"`
	User              *AuthUserSummary `json:"user,omitempty"`
	ExpiresAt         *time.Time       `json:"expires_at,omitempty"`
	RemainingKeys     *int             `json:"remaining_keys,omitempty"`
}

type AuthRegisterResponse struct {
	VerificationRequired bool             `json:"verification_required"`
	Token                string           `json:"token,omitempty"`
	User                 *AuthUserSummary `json:"user"`
}

const (
	authSessionTTLSeconds        = 30 * 24 * 60 * 60
	authPendingSessionTTLSeconds = 15 * 60
	authPendingStageTwoFactor    = "pending_2fa"
)

func buildAuthUserSummary(user *models.User) *AuthUserSummary {
	return &AuthUserSummary{
		ID:       user.ID,
		Username: user.Username,
		Email:    utils.MaskEmail(user.Email),
		Role:     user.Role,
	}
}

func validateAuthUser(user *models.User) error {
	if !user.IsActive {
		return fmt.Errorf("Account is disabled")
	}
	if user.IsBanned {
		return fmt.Errorf("Account is suspended")
	}
	if requiresEmailVerification() && !user.EmailVerified {
		return fmt.Errorf("Invalid credentials")
	}
	return nil
}

func createFullAuthSession(db *sql.DB, user *models.User) (*AuthLoginResponse, error) {
	sessionModel := &models.SessionModel{DB: db}
	session, err := sessionModel.Create(user.ID, authSessionTTLSeconds)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &AuthLoginResponse{
		Token:     session.ID,
		User:      buildAuthUserSummary(user),
		ExpiresAt: &session.ExpiresAt,
	}, nil
}

func requestUsesHTTPS(c *gin.Context) bool {
	return c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func setUserSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   requestUsesHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func userHasPasskeys(db *sql.DB, userID int64) (bool, error) {
	passkeyModel := &models.UserPasskeyModel{DB: db}
	return passkeyModel.HasPasskeys(userID)
}

func createPendingTwoFactorSession(db *sql.DB, userID int64) (*models.Session, error) {
	sessionModel := &models.SessionModel{DB: db}
	session, err := sessionModel.Create(userID, authPendingSessionTTLSeconds)
	if err != nil {
		return nil, fmt.Errorf("failed to create pending session: %w", err)
	}

	if err := sessionModel.UpdateData(session.ID, map[string]interface{}{
		"auth_stage":        authPendingStageTwoFactor,
		"requires_2fa":      true,
		"temporary_session": true,
	}); err != nil {
		_ = sessionModel.Delete(session.ID)
		return nil, fmt.Errorf("failed to store pending session: %w", err)
	}

	session.Data = map[string]interface{}{
		"auth_stage":        authPendingStageTwoFactor,
		"requires_2fa":      true,
		"temporary_session": true,
	}
	return session, nil
}

func loadPendingTwoFactorSession(db *sql.DB, sessionToken string) (*models.Session, error) {
	sessionModel := &models.SessionModel{DB: db}
	session, err := sessionModel.GetByID(strings.TrimSpace(sessionToken))
	if err != nil {
		return nil, fmt.Errorf("invalid session token")
	}
	if session.Data == nil {
		return nil, fmt.Errorf("invalid session token")
	}

	authStage, _ := session.Data["auth_stage"].(string)
	requiresTwoFactor, _ := session.Data["requires_2fa"].(bool)
	if authStage != authPendingStageTwoFactor || !requiresTwoFactor {
		return nil, fmt.Errorf("invalid session token")
	}

	return session, nil
}

func LoginAPIUser(db *sql.DB, req *APILoginRequest, clientIP string) (*AuthLoginResponse, error) {
	req.Identifier = strings.TrimSpace(req.Identifier)

	if req.Password != strings.TrimSpace(req.Password) {
		return nil, fmt.Errorf("Password cannot start or end with whitespace")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.GetByIdentifier(req.Identifier)
	if err != nil {
		return nil, fmt.Errorf("Invalid credentials")
	}

	if !userModel.CheckPassword(user, req.Password) {
		return nil, fmt.Errorf("Invalid credentials")
	}

	if err := validateAuthUser(user); err != nil {
		return nil, err
	}

	hasPasskeys, err := userHasPasskeys(db, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load passkey status: %w", err)
	}

	if user.TwoFactorEnabled || hasPasskeys {
		if req.RecoveryKey != "" {
			return loginWithRecoveryKey(db, user, req.RecoveryKey, clientIP)
		}
		if req.TwoFactorCode != "" {
			if !user.TwoFactorEnabled {
				return nil, fmt.Errorf("Invalid two-factor code")
			}
			return loginWithTwoFactorCode(db, user, req.TwoFactorCode, clientIP)
		}

		pendingSession, err := createPendingTwoFactorSession(db, user.ID)
		if err != nil {
			return nil, err
		}

		return &AuthLoginResponse{
			RequiresTwoFactor: true,
			SessionToken:      pendingSession.ID,
		}, nil
	}

	response, err := createFullAuthSession(db, user)
	if err != nil {
		return nil, err
	}
	_ = userModel.UpdateLastLogin(user.ID, clientIP)
	return response, nil
}

func loginWithTwoFactorCode(db *sql.DB, user *models.User, code string, clientIP string) (*AuthLoginResponse, error) {
	verified, err := utils.VerifyTOTP(user.TwoFactorSecret, code)
	if err != nil || !verified {
		return nil, fmt.Errorf("Invalid two-factor code")
	}

	response, err := createFullAuthSession(db, user)
	if err != nil {
		return nil, err
	}

	userModel := &models.UserModel{DB: db}
	_ = userModel.UpdateLastLogin(user.ID, clientIP)
	return response, nil
}

func loginWithRecoveryKey(db *sql.DB, user *models.User, recoveryKey string, clientIP string) (*AuthLoginResponse, error) {
	recoveryKeyModel := &models.RecoveryKeyModel{DB: db}
	verified, err := recoveryKeyModel.VerifyAndUseRecoveryKey(int(user.ID), recoveryKey)
	if err != nil || !verified {
		return nil, fmt.Errorf("Invalid recovery key")
	}

	response, err := createFullAuthSession(db, user)
	if err != nil {
		return nil, err
	}

	remainingKeys, err := recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to load remaining recovery keys: %w", err)
	}
	response.RemainingKeys = &remainingKeys

	userModel := &models.UserModel{DB: db}
	_ = userModel.UpdateLastLogin(user.ID, clientIP)
	return response, nil
}

func CompleteAPIUserTwoFactor(db *sql.DB, req *API2FARequest, clientIP string) (*AuthLoginResponse, error) {
	pendingSession, err := loadPendingTwoFactorSession(db, req.SessionToken)
	if err != nil {
		return nil, err
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.GetByID(int64(pendingSession.UserID))
	if err != nil {
		return nil, fmt.Errorf("Invalid session token")
	}

	if err := validateAuthUser(user); err != nil {
		return nil, err
	}

	response, err := loginWithTwoFactorCode(db, user, req.TwoFactorCode, clientIP)
	if err != nil {
		return nil, err
	}

	sessionModel := &models.SessionModel{DB: db}
	_ = sessionModel.Delete(pendingSession.ID)

	return response, nil
}

func UseAPIUserRecoveryKey(db *sql.DB, req *APIRecoveryUseRequest, clientIP string) (*AuthLoginResponse, error) {
	pendingSession, err := loadPendingTwoFactorSession(db, req.SessionToken)
	if err != nil {
		return nil, err
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.GetByID(int64(pendingSession.UserID))
	if err != nil {
		return nil, fmt.Errorf("Invalid session token")
	}

	if err := validateAuthUser(user); err != nil {
		return nil, err
	}

	response, err := loginWithRecoveryKey(db, user, req.RecoveryKey, clientIP)
	if err != nil {
		return nil, err
	}

	sessionModel := &models.SessionModel{DB: db}
	_ = sessionModel.Delete(pendingSession.ID)

	return response, nil
}

func RegisterAPIUser(db *sql.DB, req *APIRegisterRequest) (*AuthRegisterResponse, error) {
	if !config.IsMultiUserEnabled() {
		return nil, fmt.Errorf("Registration is not available")
	}
	if !config.IsRegistrationPublic() {
		return nil, fmt.Errorf("Public registration is not available")
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if err := utils.ValidateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := utils.ValidateEmail(req.Email); err != nil {
		return nil, err
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("Password must be at least 8 characters")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.Create(utils.NormalizeUsername(req.Username), req.Email, req.Password, "user")
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("Username or email already exists")
		}
		return nil, fmt.Errorf("Failed to create account")
	}

	response := &AuthRegisterResponse{
		User: buildAuthUserSummary(user),
	}

	if requiresEmailVerification() {
		if _, err := createUserEmailVerification(user.ID, user.Email); err != nil {
			return nil, fmt.Errorf("Failed to start email verification")
		}
		response.VerificationRequired = true
		return response, nil
	}

	sessionResponse, err := createFullAuthSession(db, user)
	if err != nil {
		return nil, fmt.Errorf("Account created but failed to login")
	}
	response.Token = sessionResponse.Token

	return response, nil
}

func LogoutCurrentUserSession(db *sql.DB, session *models.Session) error {
	if session == nil {
		return fmt.Errorf("Session authentication required")
	}

	sessionModel := &models.SessionModel{DB: db}
	if err := sessionModel.Delete(session.ID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

func RefreshCurrentUserSession(db *sql.DB, session *models.Session, user *models.User) (*AuthLoginResponse, error) {
	if session == nil || user == nil {
		return nil, fmt.Errorf("Session authentication required")
	}

	sessionModel := &models.SessionModel{DB: db}
	if err := sessionModel.Delete(session.ID); err != nil {
		return nil, fmt.Errorf("failed to refresh session")
	}

	response, err := createFullAuthSession(db, user)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh session")
	}

	return response, nil
}

func VerifyAPIUserEmail(db *sql.DB, req *APIVerifyEmailRequest) error {
	var verification struct {
		ID        int64
		UserID    int64
		Email     string
		ExpiresAt time.Time
	}

	err := db.QueryRow(`
		SELECT id, user_id, email, expires_at
		FROM user_email_verifications
		WHERE token = ? AND expires_at > ?
	`, strings.TrimSpace(req.Token), time.Now()).Scan(&verification.ID, &verification.UserID, &verification.Email, &verification.ExpiresAt)
	if err != nil {
		return fmt.Errorf("Invalid or expired verification token")
	}

	_, err = db.Exec(`
		UPDATE user_accounts
		SET email_verified = 1, updated_at = ?
		WHERE id = ?
	`, time.Now(), verification.UserID)
	if err != nil {
		return fmt.Errorf("Failed to verify email")
	}

	_, _ = db.Exec(`DELETE FROM user_email_verifications WHERE id = ?`, verification.ID)
	return nil
}

func RequestAPIUserPasswordReset(db *sql.DB, req *APIPasswordForgotRequest, resetContext *APIPasswordResetContext) error {
	email := strings.TrimSpace(req.Email)
	if err := utils.ValidateEmail(email); err != nil {
		return fmt.Errorf("Invalid email format")
	}

	clientIP := ""
	fullHost := ""
	if resetContext != nil {
		clientIP = strings.TrimSpace(resetContext.ClientIP)
		fullHost = strings.TrimSpace(resetContext.FullHost)
	}

	go func(emailAddress string, requestIP string, baseURL string) {
		var user struct {
			ID    int64
			Email string
		}

		err := db.QueryRow(`
			SELECT id, email FROM user_accounts WHERE email = ? AND is_active = 1
		`, emailAddress).Scan(&user.ID, &user.Email)
		if err != nil {
			return
		}

		token, err := models.GenerateSecureToken(32)
		if err != nil {
			return
		}

		_, err = db.Exec(`
			INSERT INTO user_password_resets (user_id, token, ip_address, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?)
		`, user.ID, token, requestIP, time.Now(), time.Now().Add(1*time.Hour))
		if err != nil {
			return
		}

		resetURL := fmt.Sprintf("%s/auth/password/reset?token=%s", baseURL, token)
		smtpService := service.NewSMTPService(db)
		if err := smtpService.LoadConfig(); err == nil {
			subject := "Password Reset Request"
			body := fmt.Sprintf(`
<html>
<body>
	<h2>Password Reset Request</h2>
	<p>You requested a password reset for your account.</p>
	<p>Click the link below to reset your password (expires in 1 hour):</p>
	<p><a href="%s">%s</a></p>
	<p>If you did not request this reset, please ignore this email.</p>
	<p><em>Sent at %s</em></p>
</body>
</html>
			`, resetURL, resetURL, time.Now().Format(time.RFC1123))

			_ = smtpService.SendEmail(user.Email, subject, body)
		}
	}(email, clientIP, fullHost)

	return nil
}

func ResetAPIUserPassword(db *sql.DB, req *APIPasswordResetRequest) error {
	if len(req.Password) < 8 {
		return fmt.Errorf("Invalid request format")
	}

	var reset struct {
		ID        int64
		UserID    int64
		ExpiresAt time.Time
	}

	err := db.QueryRow(`
		SELECT id, user_id, expires_at
		FROM user_password_resets
		WHERE token = ? AND expires_at > ?
	`, strings.TrimSpace(req.Token), time.Now()).Scan(&reset.ID, &reset.UserID, &reset.ExpiresAt)
	if err != nil {
		return fmt.Errorf("Invalid or expired reset token")
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("Failed to process password")
	}

	_, err = db.Exec(`
		UPDATE user_accounts
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, hashedPassword, time.Now(), reset.UserID)
	if err != nil {
		return fmt.Errorf("Failed to reset password")
	}

	_, _ = db.Exec(`DELETE FROM user_password_resets WHERE id = ?`, reset.ID)
	_, _ = db.Exec(`DELETE FROM user_sessions WHERE user_id = ?`, reset.UserID)
	return nil
}

func ValidateAPIUserInvite(db *sql.DB, token string) (*UserInviteValidationResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("Token required")
	}

	inviteModel := &models.UserInviteModel{DB: db}
	invite, err := inviteModel.VerifyInvite(token)
	if err != nil {
		return nil, err
	}

	return &UserInviteValidationResponse{
		Username:  invite.Username,
		Email:     utils.MaskEmail(invite.Email),
		Role:      invite.Role,
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

func CompleteAPIUserInvite(db *sql.DB, token string, username string, password string) (*UserInviteCompletionResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("Token required")
	}

	inviteModel := &models.UserInviteModel{DB: db}
	invite, err := inviteModel.VerifyInvite(token)
	if err != nil {
		return nil, err
	}

	username = utils.NormalizeUsername(username)
	if err := utils.ValidateUsername(username); err != nil {
		return nil, err
	}

	if invite.Username != "" && username != utils.NormalizeUsername(invite.Username) {
		return nil, fmt.Errorf("Invite username does not match")
	}

	if invite.Email == "" {
		return nil, fmt.Errorf("Invite is missing an email address")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.Create(username, invite.Email, password, invite.Role)
	if err != nil {
		return nil, fmt.Errorf("Failed to create account")
	}

	if _, err := db.Exec(`UPDATE user_accounts SET email_verified = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, user.ID); err != nil {
		return nil, fmt.Errorf("Failed to finalize account")
	}

	if err := inviteModel.MarkUsed(token, user.ID); err != nil {
		return nil, fmt.Errorf("Failed to finalize invite")
	}

	sessionModel := &models.SessionModel{DB: db}
	session, err := sessionModel.Create(user.ID, authSessionTTLSeconds)
	if err != nil {
		return &UserInviteCompletionResponse{
			Message: "Account created. Please log in.",
		}, nil
	}

	return &UserInviteCompletionResponse{
		Token: session.ID,
		User:  buildAuthUserSummary(user),
	}, nil
}

func ValidateAPIServerInvite(token string) (*ServerInviteValidationResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("Token required")
	}

	inviteService := service.NewAdminInviteService(database.GetServerDB(), "", nil)
	invite, err := inviteService.VerifyInvite(token)
	if err != nil {
		return nil, err
	}

	return &ServerInviteValidationResponse{
		Email:     utils.MaskEmail(invite.InvitedEmail),
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

func CompleteAPIServerInvite(token string, username string, password string) (*ServerInviteCompletionResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("Token required")
	}

	inviteService := service.NewAdminInviteService(database.GetServerDB(), "", nil)
	admin, err := inviteService.AcceptInvite(token, username, password)
	if err != nil {
		return nil, err
	}

	return &ServerInviteCompletionResponse{
		Message: "Admin account created successfully. Please log in.",
		Admin: &InvitedAdminSummary{
			ID:       admin.ID,
			Username: admin.Username,
			Email:    utils.MaskEmail(admin.Email),
		},
	}, nil
}

// HandleAPILogin handles POST /api/v1/server/auth/login per AI.md PART 33
// @Summary Login
// @Description Authenticate with username/email and password. Returns session token or pending-2fa token.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body APILoginRequest true "Login credentials"
// @Success 200 {object} map[string]interface{} "Login successful — session_token returned"
// @Success 202 {object} map[string]interface{} "2FA required — pending session token returned"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Invalid credentials"
// @Failure 403 {object} map[string]interface{} "Account disabled or suspended"
// @Router /api/v1/server/auth/login [post]
func (h *AuthAPIHandler) HandleAPILogin(c *gin.Context) {
	var req APILoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	response, err := LoginAPIUser(h.DB, &req, c.ClientIP())
	if err != nil {
		status := http.StatusUnauthorized
		switch err.Error() {
		case "Password cannot start or end with whitespace":
			status = http.StatusBadRequest
		case "Account is disabled", "Account is suspended":
			status = http.StatusForbidden
		case "failed to create session":
			status = http.StatusInternalServerError
		}

		if strings.Contains(err.Error(), "failed to create") || strings.Contains(err.Error(), "failed to store pending session") {
			status = http.StatusInternalServerError
		}

		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIRegister handles POST /api/v1/server/auth/register per AI.md PART 33
// @Summary Register
// @Description Register a new user account. Requires open registration mode.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body APIRegisterRequest true "Registration details"
// @Success 201 {object} map[string]interface{} "Account created"
// @Failure 400 {object} map[string]interface{} "Validation error"
// @Failure 404 {object} map[string]interface{} "Registration disabled"
// @Failure 409 {object} map[string]interface{} "Username or email already exists"
// @Router /api/v1/server/auth/register [post]
func (h *AuthAPIHandler) HandleAPIRegister(c *gin.Context) {
	var req APIRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	response, err := RegisterAPIUser(h.DB, &req)
	if err != nil {
		status := http.StatusInternalServerError
		switch err.Error() {
		case "Registration is not available", "Public registration is not available":
			status = http.StatusNotFound
		case "Username or email already exists":
			status = http.StatusConflict
		case "Password must be at least 8 characters":
			status = http.StatusBadRequest
		default:
			if strings.HasPrefix(err.Error(), "Username") || strings.HasPrefix(err.Error(), "Email") {
				status = http.StatusBadRequest
			}
		}

		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPILogout handles POST /api/v1/server/auth/logout per AI.md PART 33
// @Summary Logout
// @Description Invalidate the current session token.
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "Logged out"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/server/auth/logout [post]
func (h *AuthAPIHandler) HandleAPILogout(c *gin.Context) {
	session, ok := middleware.GetCurrentSession(c)
	if !ok || session == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "Session authentication required",
		})
		return
	}

	if err := LogoutCurrentUserSession(h.DB, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Logged out successfully",
	})
}

// HandleAPI2FA handles POST /api/v1/server/auth/2fa per AI.md PART 33
// @Summary Complete 2FA
// @Description Complete login with a TOTP code when 2FA is required.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body API2FARequest true "2FA request with pending session token and TOTP code"
// @Success 200 {object} map[string]interface{} "Session token returned"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Invalid code or expired session"
// @Router /api/v1/server/auth/2fa [post]
func (h *AuthAPIHandler) HandleAPI2FA(c *gin.Context) {
	var req API2FARequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	response, err := CompleteAPIUserTwoFactor(h.DB, &req, c.ClientIP())
	if err != nil {
		status := http.StatusUnauthorized
		if err.Error() == "Invalid request format" {
			status = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "failed to create session") {
			status = http.StatusInternalServerError
		}

		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIRecoveryUse handles POST /api/v1/server/auth/recovery/use per AI.md PART 33
// @Summary Login with recovery key
// @Description Complete login using a one-time recovery key when 2FA device is unavailable.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body APIRecoveryUseRequest true "Pending session token and recovery key"
// @Success 200 {object} map[string]interface{} "Session token returned"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Invalid key or expired session"
// @Router /api/v1/server/auth/recovery/use [post]
func (h *AuthAPIHandler) HandleAPIRecoveryUse(c *gin.Context) {
	var req APIRecoveryUseRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	response, err := UseAPIUserRecoveryKey(h.DB, &req, c.ClientIP())
	if err != nil {
		status := http.StatusUnauthorized
		if strings.Contains(err.Error(), "failed to create session") || strings.Contains(err.Error(), "failed to load remaining recovery keys") {
			status = http.StatusInternalServerError
		}

		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIRefresh handles POST /api/v1/server/auth/refresh per AI.md PART 33
// @Summary Refresh session
// @Description Extend the current session expiry and return a refreshed token.
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "Refreshed session token"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/server/auth/refresh [post]
func (h *AuthAPIHandler) HandleAPIRefresh(c *gin.Context) {
	session, ok := middleware.GetCurrentSession(c)
	if !ok || session == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "Session authentication required",
		})
		return
	}

	user, ok := middleware.GetCurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "Not authenticated",
		})
		return
	}

	response, err := RefreshCurrentUserSession(h.DB, session, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIVerifyEmail handles POST /api/v1/server/auth/verify per AI.md PART 33
// @Summary Verify email
// @Description Verify a user's email address using the verification code sent by email.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body APIVerifyEmailRequest true "Verification code"
// @Success 200 {object} map[string]interface{} "Email verified"
// @Failure 400 {object} map[string]interface{} "Bad request or invalid code"
// @Router /api/v1/server/auth/verify [post]
func (h *AuthAPIHandler) HandleAPIVerifyEmail(c *gin.Context) {
	var req APIVerifyEmailRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	if err := VerifyAPIUserEmail(h.DB, &req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "Failed to verify email" {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Email verified successfully",
	})
}

// HandleAPIPasswordForgot handles POST /api/v1/server/auth/password/forgot per AI.md PART 33
// @Summary Request password reset
// @Description Send a password reset email. Always returns 200 to prevent enumeration.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body APIPasswordForgotRequest true "Email address"
// @Success 200 {object} map[string]interface{} "Reset email sent (or silently skipped)"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Router /api/v1/server/auth/password/forgot [post]
func (h *AuthAPIHandler) HandleAPIPasswordForgot(c *gin.Context) {
	var req APIPasswordForgotRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid email format",
		})
		return
	}

	if err := RequestAPIUserPasswordReset(h.DB, &req, &APIPasswordResetContext{
		ClientIP: c.ClientIP(),
		FullHost: utils.GetHostInfo(c).FullHost,
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	// Always return success to prevent email enumeration
	// Per AI.md security requirements
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "If an account exists with that email, a password reset link will be sent",
	})
}

// HandleAPIPasswordReset handles POST /api/v1/server/auth/password/reset per AI.md PART 33
// @Summary Reset password
// @Description Complete password reset using the token from the reset email.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body APIPasswordResetRequest true "Reset token and new password"
// @Success 200 {object} map[string]interface{} "Password reset successful"
// @Failure 400 {object} map[string]interface{} "Bad request or expired token"
// @Router /api/v1/server/auth/password/reset [post]
func (h *AuthAPIHandler) HandleAPIPasswordReset(c *gin.Context) {
	var req APIPasswordResetRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	if err := ResetAPIUserPassword(h.DB, &req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "Failed to process password" || err.Error() == "Failed to reset password" {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Password reset successfully. Please log in with your new password.",
	})
}

// HandleAPIUserInviteValidate handles GET /api/v1/server/auth/invite/user/{token} per AI.md PART 33
// @Summary Validate user invite
// @Description Validate a user invite token before completion.
// @Tags Auth
// @Produce json
// @Param token path string true "Invite token"
// @Success 200 {object} map[string]interface{} "Invite is valid"
// @Failure 400 {object} map[string]interface{} "Token required"
// @Failure 410 {object} map[string]interface{} "Token expired or invalid"
// @Router /api/v1/server/auth/invite/user/{token} [get]
func (h *AuthAPIHandler) HandleAPIUserInviteValidate(c *gin.Context) {
	response, err := ValidateAPIUserInvite(h.DB, c.Param("token"))
	if err != nil {
		status := http.StatusGone
		if err.Error() == "Token required" {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIUserInviteComplete handles POST /api/v1/server/auth/invite/user/{token} per AI.md PART 33
// @Summary Complete user invite
// @Description Accept a user invite and create the account.
// @Tags Auth
// @Accept json
// @Produce json
// @Param token path string true "Invite token"
// @Param body body object true "Username and password"
// @Success 201 {object} map[string]interface{} "Account created"
// @Failure 400 {object} map[string]interface{} "Validation error"
// @Failure 410 {object} map[string]interface{} "Token expired or invalid"
// @Router /api/v1/server/auth/invite/user/{token} [post]
func (h *AuthAPIHandler) HandleAPIUserInviteComplete(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	response, err := CompleteAPIUserInvite(h.DB, c.Param("token"), req.Username, req.Password)
	if err != nil {
		status := http.StatusBadRequest
		switch err.Error() {
		case "Token required", "Invite username does not match", "Invite is missing an email address":
			status = http.StatusBadRequest
		case "Failed to create account", "Failed to finalize account", "Failed to finalize invite":
			status = http.StatusInternalServerError
		default:
			if strings.Contains(err.Error(), "invite token") {
				status = http.StatusGone
			}
		}
		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if strings.TrimSpace(response.Message) != "" && response.Token == "" && response.User == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"message": response.Message,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIServerInviteValidate handles GET /api/v1/server/auth/invite/server/{token}.
// @Summary Validate admin invite
// @Description Validate a server-admin invite token before completion.
// @Tags Auth
// @Produce json
// @Param token path string true "Invite token"
// @Success 200 {object} map[string]interface{} "Invite is valid"
// @Failure 400 {object} map[string]interface{} "Token required"
// @Failure 410 {object} map[string]interface{} "Token expired or invalid"
// @Router /api/v1/server/auth/invite/server/{token} [get]
func (h *AuthAPIHandler) HandleAPIServerInviteValidate(c *gin.Context) {
	response, err := ValidateAPIServerInvite(c.Param("token"))
	if err != nil {
		status := http.StatusGone
		if err.Error() == "Token required" {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": response,
	})
}

// HandleAPIServerInviteComplete handles POST /api/v1/server/auth/invite/server/{token}.
// @Summary Complete admin invite
// @Description Accept a server-admin invite and create the admin account.
// @Tags Auth
// @Accept json
// @Produce json
// @Param token path string true "Invite token"
// @Param body body object true "Username and password"
// @Success 201 {object} map[string]interface{} "Admin account created"
// @Failure 400 {object} map[string]interface{} "Validation error"
// @Failure 410 {object} map[string]interface{} "Token expired or invalid"
// @Router /api/v1/server/auth/invite/server/{token} [post]
func (h *AuthAPIHandler) HandleAPIServerInviteComplete(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":    false,
			"error": "Invalid request format",
		})
		return
	}

	response, err := CompleteAPIServerInvite(c.Param("token"), req.Username, req.Password)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "Token required" {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"ok":      true,
		"message": response.Message,
		"data": gin.H{
			"admin": response.Admin,
		},
	})
}
