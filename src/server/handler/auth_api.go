// Package handler provides auth API handlers per AI.md PART 33
package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
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
	Message string               `json:"message"`
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
		Email:    util.MaskEmail(user.Email),
		Role:     user.Role,
	}
}

func validateAuthUser(user *models.User) error {
	if !user.IsActive {
		return fmt.Errorf("account is disabled")
	}
	if user.IsBanned {
		return fmt.Errorf("account is suspended")
	}
	if requiresEmailVerification() && !user.EmailVerified {
		return fmt.Errorf("invalid credentials")
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

func requestUsesHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setUserSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   requestUsesHTTPS(r),
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
		return nil, fmt.Errorf("password cannot start or end with whitespace")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.GetByIdentifier(req.Identifier)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if !userModel.CheckPassword(user, req.Password) {
		return nil, fmt.Errorf("invalid credentials")
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
				return nil, fmt.Errorf("invalid two-factor code")
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
	secret, err := models.DecryptTwoFactorSecret(user.TwoFactorSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid two-factor code")
	}

	verified, err := util.VerifyTOTP(secret, code)
	if err != nil || !verified {
		return nil, fmt.Errorf("invalid two-factor code")
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
		return nil, fmt.Errorf("invalid recovery key")
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
		return nil, fmt.Errorf("invalid session token")
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
		return nil, fmt.Errorf("invalid session token")
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
		return nil, fmt.Errorf("registration is not available")
	}
	if !config.IsRegistrationPublic() {
		return nil, fmt.Errorf("public registration is not available")
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if err := util.ValidateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := util.ValidateEmail(req.Email); err != nil {
		return nil, err
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.Create(util.NormalizeUsername(req.Username), req.Email, req.Password, "user")
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("username or email already exists")
		}
		return nil, fmt.Errorf("failed to create account")
	}

	response := &AuthRegisterResponse{
		User: buildAuthUserSummary(user),
	}

	if requiresEmailVerification() {
		if _, err := createUserEmailVerification(user.ID, user.Email); err != nil {
			return nil, fmt.Errorf("failed to start email verification")
		}
		response.VerificationRequired = true
		return response, nil
	}

	sessionResponse, err := createFullAuthSession(db, user)
	if err != nil {
		return nil, fmt.Errorf("account created but failed to login")
	}
	response.Token = sessionResponse.Token

	return response, nil
}

func LogoutCurrentUserSession(db *sql.DB, session *models.Session) error {
	if session == nil {
		return fmt.Errorf("session authentication required")
	}

	sessionModel := &models.SessionModel{DB: db}
	if err := sessionModel.Delete(session.ID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

func RefreshCurrentUserSession(db *sql.DB, session *models.Session, user *models.User) (*AuthLoginResponse, error) {
	if session == nil || user == nil {
		return nil, fmt.Errorf("session authentication required")
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

	// Expiry is compared in Go, not in SQL. expires_at is stored as canonical
	// UTC text while the old predicate bound a raw time.Time, which the SQLite
	// driver serializes as time.Time.String() in the host's LOCAL zone — the
	// two encodings compared lexicographically, so the filter was wrong across
	// zones. dbtime.IsAfter fails closed: a NULL or unparseable stored value
	// is treated as expired.
	//
	// The lookup is by SHA-256 hash, not by the raw token. Every row in this
	// table is written by UserEmailVerificationModel.CreateVerification, which
	// stores models.HashAPIToken(token) — PART 11 forbids keeping a usable
	// token at rest. Querying the raw value here matched nothing, so no
	// emailed verification link could ever be redeemed through this endpoint.
	var storedExpiresAt interface{}
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
		SELECT id, user_id, email, expires_at
		FROM user_email_verifications
		WHERE token = ?
	`, models.HashAPIToken(strings.TrimSpace(req.Token))).Scan(&verification.ID, &verification.UserID, &verification.Email, &storedExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid or expired verification token")
	}
	if !dbtime.IsAfter(storedExpiresAt, time.Now()) {
		return fmt.Errorf("invalid or expired verification token")
	}
	if parsed, ok := dbtime.ParseStoredTimestamp(storedExpiresAt); ok {
		verification.ExpiresAt = parsed
	}

	// updated_at is bound as canonical UTC text rather than as a raw time.Time,
	// which the SQLite driver would serialize in the host's LOCAL zone.
	_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		UPDATE user_accounts
		SET email_verified = 1, updated_at = ?
		WHERE id = ?
	`, dbtime.FormatSQLTimestamp(time.Now()), verification.UserID)
	if err != nil {
		return fmt.Errorf("failed to verify email")
	}

	_, _ = database.ExecContext(context.Background(), db, database.TimeoutWrite, `DELETE FROM user_email_verifications WHERE id = ?`, verification.ID)
	return nil
}

// passwordResetGoroutineDone is a test-only synchronization hook, invoked
// (if set) after the async password-reset goroutine below finishes, win or
// lose. Production code never sets it. Tests that exercise
// RequestAPIUserPasswordReset set it to await the goroutine's completion
// before tearing down their test DB, instead of racing a leaked goroutine
// against a closed connection (AI.md PART 29).
var passwordResetGoroutineDone func()

// SetPasswordResetGoroutineDoneHookForTesting sets (or clears, with nil) the
// test-only synchronization hook invoked after RequestAPIUserPasswordReset's
// async goroutine finishes. Only test code in other packages should call
// this — production code never does.
func SetPasswordResetGoroutineDoneHookForTesting(hook func()) {
	passwordResetGoroutineDone = hook
}

func RequestAPIUserPasswordReset(db *sql.DB, req *APIPasswordForgotRequest, resetContext *APIPasswordResetContext) error {
	email := strings.TrimSpace(req.Email)
	if err := util.ValidateEmail(email); err != nil {
		return fmt.Errorf("invalid email format")
	}

	clientIP := ""
	fullHost := ""
	if resetContext != nil {
		clientIP = strings.TrimSpace(resetContext.ClientIP)
		fullHost = strings.TrimSpace(resetContext.FullHost)
	}

	go func(emailAddress string, requestIP string, baseURL string) {
		if passwordResetGoroutineDone != nil {
			defer passwordResetGoroutineDone()
		}

		var user struct {
			ID    int64
			Email string
		}

		err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
			SELECT id, email FROM user_accounts WHERE email = ? AND is_active = 1
		`, emailAddress).Scan(&user.ID, &user.Email)
		if err != nil {
			return
		}

		token, err := models.GenerateSecureToken(32)
		if err != nil {
			return
		}

		// created_at/expires_at are bound as canonical UTC text. Binding a raw
		// time.Time makes the SQLite driver serialize it as time.Time.String()
		// in the host's LOCAL zone, which no reader can compare against a UTC
		// instant without guessing the writer's offset.
		//
		// Only the SHA-256 hash is stored. The raw token goes out in the email
		// link and is never persisted, so a leaked copy of the users database
		// cannot be replayed to seize accounts. This also matches
		// UserPasswordResetModel.CreateReset, the other writer of this table —
		// storing the raw value here left rows the model's reader could not
		// match and vice versa.
		issuedAt := time.Now()
		_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
			INSERT INTO user_password_resets (user_id, token, ip_address, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?)
		`, user.ID, models.HashAPIToken(token), requestIP, dbtime.FormatSQLTimestamp(issuedAt), dbtime.FormatSQLTimestamp(issuedAt.Add(1*time.Hour)))
		if err != nil {
			return
		}

		resetURL := fmt.Sprintf("%s/auth/password/reset?token=%s", baseURL, token)

		// The SMTP service reads server_config and server_notification_channels,
		// both declared in database.ServerSchema, so it must be given the SERVER
		// handle. The `db` in scope here is the users handle - every caller passes
		// one, and it is what the user_password_resets insert above writes to -
		// so passing it made the service look for its configuration in the wrong
		// database and silently find no SMTP settings at all.
		smtpService := service.NewSMTPService(database.GetServerDB())
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
		return fmt.Errorf("invalid request format")
	}

	var reset struct {
		ID        int64
		UserID    int64
		ExpiresAt time.Time
	}

	// Expiry is compared in Go, not in SQL — see VerifyAPIUserEmail above for
	// why the SQL-side predicate compared two incompatible text encodings.
	// dbtime.IsAfter fails closed: a NULL or unparseable stored value is
	// treated as expired.
	//
	// Looked up by SHA-256 hash, matching what both writers of this table
	// store; see RequestAPIUserPasswordReset above.
	var storedExpiresAt interface{}
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, `
		SELECT id, user_id, expires_at
		FROM user_password_resets
		WHERE token = ?
	`, models.HashAPIToken(strings.TrimSpace(req.Token))).Scan(&reset.ID, &reset.UserID, &storedExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid or expired reset token")
	}
	if !dbtime.IsAfter(storedExpiresAt, time.Now()) {
		return fmt.Errorf("invalid or expired reset token")
	}
	if parsed, ok := dbtime.ParseStoredTimestamp(storedExpiresAt); ok {
		reset.ExpiresAt = parsed
	}

	hashedPassword, err := util.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("failed to process password")
	}

	// updated_at is bound as canonical UTC text rather than as a raw time.Time,
	// which the SQLite driver would serialize in the host's LOCAL zone.
	_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		UPDATE user_accounts
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, hashedPassword, dbtime.FormatSQLTimestamp(time.Now()), reset.UserID)
	if err != nil {
		return fmt.Errorf("failed to reset password")
	}

	_, _ = database.ExecContext(context.Background(), db, database.TimeoutWrite, `DELETE FROM user_password_resets WHERE id = ?`, reset.ID)
	_, _ = database.ExecContext(context.Background(), db, database.TimeoutWrite, `DELETE FROM user_sessions WHERE user_id = ?`, reset.UserID)
	return nil
}

func ValidateAPIUserInvite(db *sql.DB, token string) (*UserInviteValidationResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("token required")
	}

	inviteModel := &models.UserInviteModel{DB: db}
	invite, err := inviteModel.VerifyInvite(token)
	if err != nil {
		return nil, err
	}

	return &UserInviteValidationResponse{
		Username:  invite.Username,
		Email:     util.MaskEmail(invite.Email),
		Role:      invite.Role,
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

func CompleteAPIUserInvite(db *sql.DB, token string, username string, password string) (*UserInviteCompletionResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("token required")
	}

	inviteModel := &models.UserInviteModel{DB: db}
	invite, err := inviteModel.VerifyInvite(token)
	if err != nil {
		return nil, err
	}

	username = util.NormalizeUsername(username)
	if err := util.ValidateUsername(username); err != nil {
		return nil, err
	}

	if invite.Username != "" && username != util.NormalizeUsername(invite.Username) {
		return nil, fmt.Errorf("invite username does not match")
	}

	if invite.Email == "" {
		return nil, fmt.Errorf("invite is missing an email address")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.Create(username, invite.Email, password, invite.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to create account")
	}

	// updated_at is bound as canonical UTC text rather than produced by SQL's
	// CURRENT_TIMESTAMP, which yields a different type and zone on PostgreSQL,
	// MySQL and SQL Server than it does on SQLite.
	if _, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `UPDATE user_accounts SET email_verified = 1, updated_at = ? WHERE id = ?`, dbtime.FormatSQLTimestamp(time.Now()), user.ID); err != nil {
		return nil, fmt.Errorf("failed to finalize account")
	}

	if err := inviteModel.MarkUsed(token, user.ID); err != nil {
		return nil, fmt.Errorf("failed to finalize invite")
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
		return nil, fmt.Errorf("token required")
	}

	inviteService := service.NewAdminInviteService(database.GetServerDB(), "", nil)
	invite, err := inviteService.VerifyInvite(token)
	if err != nil {
		return nil, err
	}

	return &ServerInviteValidationResponse{
		Email:     util.MaskEmail(invite.InvitedEmail),
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

func CompleteAPIServerInvite(token string, username string, password string) (*ServerInviteCompletionResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("token required")
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
			Email:    util.MaskEmail(admin.Email),
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
func (h *AuthAPIHandler) HandleAPILogin(w http.ResponseWriter, r *http.Request) {
	var req APILoginRequest
	if !DecodeAndValidate(w, r, &req) {
		return
	}

	response, err := LoginAPIUser(h.DB, &req, util.GetClientIP(r))
	if err != nil {
		status := http.StatusUnauthorized
		switch err.Error() {
		case "password cannot start or end with whitespace":
			status = http.StatusBadRequest
		case "account is disabled", "account is suspended":
			status = http.StatusForbidden
		case "failed to create session":
			status = http.StatusInternalServerError
		}

		if strings.Contains(err.Error(), "failed to create") || strings.Contains(err.Error(), "failed to store pending session") {
			status = http.StatusInternalServerError
		}

		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIRegister(w http.ResponseWriter, r *http.Request) {
	var req APIRegisterRequest
	if !DecodeAndValidate(w, r, &req) {
		return
	}

	response, err := RegisterAPIUser(h.DB, &req)
	if err != nil {
		status := http.StatusInternalServerError
		switch err.Error() {
		case "registration is not available", "public registration is not available":
			status = http.StatusNotFound
		case "username or email already exists":
			status = http.StatusConflict
		case "password must be at least 8 characters":
			status = http.StatusBadRequest
		default:
			if strings.HasPrefix(err.Error(), "Username") || strings.HasPrefix(err.Error(), "Email") {
				status = http.StatusBadRequest
			}
		}

		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPILogout(w http.ResponseWriter, r *http.Request) {
	session, ok := middleware.GetCurrentSession(r)
	if !ok || session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"ok":    false,
			"error": "session authentication required",
		})
		return
	}

	if err := LogoutCurrentUserSession(h.DB, session); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPI2FA(w http.ResponseWriter, r *http.Request) {
	var req API2FARequest

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	response, err := CompleteAPIUserTwoFactor(h.DB, &req, util.GetClientIP(r))
	if err != nil {
		status := http.StatusUnauthorized
		if err.Error() == "Invalid request format" {
			status = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "failed to create session") {
			status = http.StatusInternalServerError
		}

		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIRecoveryUse(w http.ResponseWriter, r *http.Request) {
	var req APIRecoveryUseRequest

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	response, err := UseAPIUserRecoveryKey(h.DB, &req, util.GetClientIP(r))
	if err != nil {
		status := http.StatusUnauthorized
		if strings.Contains(err.Error(), "failed to create session") || strings.Contains(err.Error(), "failed to load remaining recovery keys") {
			status = http.StatusInternalServerError
		}

		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIRefresh(w http.ResponseWriter, r *http.Request) {
	session, ok := middleware.GetCurrentSession(r)
	if !ok || session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"ok":    false,
			"error": "session authentication required",
		})
		return
	}

	user, ok := middleware.GetCurrentUser(r)
	if !ok || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"ok":    false,
			"error": "Not authenticated",
		})
		return
	}

	response, err := RefreshCurrentUserSession(h.DB, session, user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req APIVerifyEmailRequest

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	if err := VerifyAPIUserEmail(h.DB, &req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "failed to verify email" {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIPasswordForgot(w http.ResponseWriter, r *http.Request) {
	var req APIPasswordForgotRequest

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	if err := RequestAPIUserPasswordReset(h.DB, &req, &APIPasswordResetContext{
		ClientIP: util.GetClientIP(r),
		FullHost: util.GetHostInfo(r).FullHost,
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	// Always return success to prevent email enumeration
	// Per AI.md security requirements
	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req APIPasswordResetRequest

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	if err := ResetAPIUserPassword(h.DB, &req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "failed to process password" || err.Error() == "failed to reset password" {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIUserInviteValidate(w http.ResponseWriter, r *http.Request) {
	response, err := ValidateAPIUserInvite(h.DB, chi.URLParam(r, "token"))
	if err != nil {
		status := http.StatusGone
		if err.Error() == "Token required" {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIUserInviteComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	response, err := CompleteAPIUserInvite(h.DB, chi.URLParam(r, "token"), req.Username, req.Password)
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
		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if strings.TrimSpace(response.Message) != "" && response.Token == "" && response.User == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"message": response.Message,
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIServerInviteValidate(w http.ResponseWriter, r *http.Request) {
	response, err := ValidateAPIServerInvite(chi.URLParam(r, "token"))
	if err != nil {
		status := http.StatusGone
		if err.Error() == "Token required" {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AuthAPIHandler) HandleAPIServerInviteComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	response, err := CompleteAPIServerInvite(chi.URLParam(r, "token"), req.Username, req.Password)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "Token required" {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"ok":      true,
		"message": response.Message,
		"data": map[string]interface{}{
			"admin": response.Admin,
		},
	})
}
