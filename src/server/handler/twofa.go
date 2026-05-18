package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/casapps/wthr/src/server/middleware"
	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/util"

	"github.com/gin-gonic/gin"
)

type TwoFactorHandler struct {
	DB *sql.DB
}

type TwoFactorStatusResponse struct {
	Enabled           bool `json:"enabled"`
	RecoveryKeysCount int  `json:"recovery_keys_count"`
}

type TwoFactorSetupResponse struct {
	Secret    string `json:"secret"`
	QRCode    string `json:"qr_code"`
	ManualURL string `json:"manual_url"`
	Account   string `json:"account"`
	Issuer    string `json:"issuer"`
}

type RecoveryKeysResponse struct {
	Message      string   `json:"message"`
	RecoveryKeys []string `json:"recovery_keys"`
}

// LoadCurrentUserTwoFactorStatus returns the same payload used by GET /api/v1/users/security/2fa.
func LoadCurrentUserTwoFactorStatus(db *sql.DB, user *models.User) (*TwoFactorStatusResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.loadCurrentUserTwoFactorStatus(user)
}

// PrepareCurrentUserTwoFactorSetup returns the same setup payload used by GET /api/v1/users/security/2fa/setup.
func PrepareCurrentUserTwoFactorSetup(db *sql.DB, user *models.User) (*TwoFactorSetupResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.prepareCurrentUserTwoFactorSetup(user)
}

// EnableCurrentUserTwoFactor applies the same 2FA enable flow used by POST /api/v1/users/security/2fa/enable.
func EnableCurrentUserTwoFactor(db *sql.DB, user *models.User, secret string, code string) (*RecoveryKeysResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.enableCurrentUserTwoFactor(user, secret, code)
}

// DisableCurrentUserTwoFactor applies the same 2FA disable flow used by POST /api/v1/users/security/2fa/disable.
func DisableCurrentUserTwoFactor(db *sql.DB, user *models.User, password string) error {
	h := &TwoFactorHandler{DB: db}
	return h.disableCurrentUserTwoFactor(user, password)
}

// VerifyCurrentUserTwoFactorCode applies the same verification flow used by POST /api/v1/users/security/2fa/verify.
func VerifyCurrentUserTwoFactorCode(db *sql.DB, user *models.User, code string) error {
	h := &TwoFactorHandler{DB: db}
	return h.verifyCurrentUserTwoFactorCode(user, code)
}

// RegenerateCurrentUserRecoveryKeys applies the same recovery-key regeneration flow used by POST /api/v1/users/security/recovery/regenerate.
func RegenerateCurrentUserRecoveryKeys(db *sql.DB, user *models.User, code string) (*RecoveryKeysResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.regenerateCurrentUserRecoveryKeys(user, code)
}

func (h *TwoFactorHandler) loadCurrentUserTwoFactorStatus(user *models.User) (*TwoFactorStatusResponse, error) {
	recoveryKeysCount := 0
	if user.TwoFactorEnabled {
		recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
		count, err := recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
		if err != nil {
			return nil, err
		}
		recoveryKeysCount = count
	}

	return &TwoFactorStatusResponse{
		Enabled:           user.TwoFactorEnabled,
		RecoveryKeysCount: recoveryKeysCount,
	}, nil
}

func (h *TwoFactorHandler) prepareCurrentUserTwoFactorSetup(user *models.User) (*TwoFactorSetupResponse, error) {
	if user.TwoFactorEnabled {
		return nil, fmt.Errorf("two-factor authentication is already enabled")
	}

	secret, qrCodeDataURL, err := utils.GenerateTOTPSecret(user.Email, "Weather")
	if err != nil {
		return nil, fmt.Errorf("failed to generate 2FA secret")
	}

	return &TwoFactorSetupResponse{
		Secret:    secret,
		QRCode:    qrCodeDataURL,
		ManualURL: utils.GenerateOTPAuthURL(user.Email, secret, "Weather"),
		Account:   user.Email,
		Issuer:    "Weather",
	}, nil
}

func (h *TwoFactorHandler) enableCurrentUserTwoFactor(user *models.User, secret string, code string) (*RecoveryKeysResponse, error) {
	if user.TwoFactorEnabled {
		return nil, fmt.Errorf("two-factor authentication is already enabled")
	}

	valid, err := utils.VerifyTOTP(secret, code)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid verification code")
	}

	userModel := &models.UserModel{DB: h.DB}
	if err := userModel.EnableTwoFactor(user.ID, secret); err != nil {
		return nil, fmt.Errorf("failed to enable two-factor authentication")
	}

	recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
	recoveryKeys, err := recoveryKeyModel.GenerateRecoveryKeys(int(user.ID))
	if err != nil {
		userModel.DisableTwoFactor(user.ID)
		return nil, fmt.Errorf("failed to generate recovery keys")
	}

	return &RecoveryKeysResponse{
		Message:      "Two-factor authentication enabled successfully",
		RecoveryKeys: recoveryKeys,
	}, nil
}

func (h *TwoFactorHandler) disableCurrentUserTwoFactor(user *models.User, password string) error {
	if !user.TwoFactorEnabled {
		return fmt.Errorf("two-factor authentication is not enabled")
	}

	userModel := &models.UserModel{DB: h.DB}
	if !userModel.CheckPassword(user, password) {
		return fmt.Errorf("invalid password")
	}

	if err := userModel.DisableTwoFactor(user.ID); err != nil {
		return fmt.Errorf("failed to disable two-factor authentication")
	}

	recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
	recoveryKeyModel.DeleteAllForUser(int(user.ID))
	return nil
}

func (h *TwoFactorHandler) verifyCurrentUserTwoFactorCode(user *models.User, code string) error {
	if !user.TwoFactorEnabled {
		return fmt.Errorf("two-factor authentication is not enabled")
	}

	valid, err := utils.VerifyTOTP(user.TwoFactorSecret, code)
	if err != nil || !valid {
		return fmt.Errorf("invalid verification code")
	}

	return nil
}

func (h *TwoFactorHandler) regenerateCurrentUserRecoveryKeys(user *models.User, code string) (*RecoveryKeysResponse, error) {
	if !user.TwoFactorEnabled {
		return nil, fmt.Errorf("two-factor authentication is not enabled")
	}

	valid, err := utils.VerifyTOTP(user.TwoFactorSecret, code)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid verification code")
	}

	recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
	recoveryKeys, err := recoveryKeyModel.GenerateRecoveryKeys(int(user.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to generate recovery keys")
	}

	return &RecoveryKeysResponse{
		Message:      "Recovery keys regenerated successfully",
		RecoveryKeys: recoveryKeys,
	}, nil
}

// GetTwoFactorStatus returns the 2FA status for the authenticated user (API endpoint)
// @Summary Get 2FA status
// @Description Get the current user's two-factor authentication status.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "2FA status"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/2fa [get]
func (h *TwoFactorHandler) GetTwoFactorStatus(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	status, err := h.loadCurrentUserTwoFactorStatus(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get two-factor status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":                  true,
		"enabled":             status.Enabled,
		"recovery_keys_count": status.RecoveryKeysCount,
	})
}

// ShowSecurityPage renders the security settings page
func (h *TwoFactorHandler) ShowSecurityPage(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/login")
		return
	}

	// Get recovery keys count if 2FA is enabled
	var recoveryKeysCount int
	if user.TwoFactorEnabled {
		recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
		recoveryKeysCount, _ = recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
	}

	passkeyModel := &models.UserPasskeyModel{DB: h.DB}
	passkeys, _ := passkeyModel.ListByUserID(user.ID)
	if len(passkeys) > 0 && recoveryKeysCount == 0 {
		recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
		recoveryKeysCount, _ = recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
	}

	NegotiateResponse(c, "page/user/security.tmpl", utils.TemplateData(c, gin.H{
		"title":             "Security Settings",
		"user":              user,
		"recoveryKeysCount": recoveryKeysCount,
		"passkeys":          passkeys,
		"hasPasskeys":       len(passkeys) > 0,
	}))
}

// SetupTwoFactor generates a TOTP secret and QR code for setup
// @Summary Setup 2FA
// @Description Generate a TOTP secret and QR code to begin 2FA enrollment.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "TOTP setup info with QR code"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/2fa/setup [get]
func (h *TwoFactorHandler) SetupTwoFactor(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	setup, err := h.prepareCurrentUserTwoFactorSetup(user)
	if err != nil {
		if err.Error() == "two-factor authentication is already enabled" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Two-factor authentication is already enabled"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"secret":     setup.Secret,
		"qr_code":    setup.QRCode,
		"manual_url": setup.ManualURL,
		"account":    setup.Account,
		"issuer":     setup.Issuer,
	})
}

// EnableTwoFactor verifies the TOTP code and enables 2FA for the user
// @Summary Enable 2FA
// @Description Verify TOTP code and enable two-factor authentication. Returns recovery keys.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object true "TOTP secret and verification code"
// @Success 200 {object} map[string]interface{} "2FA enabled, recovery keys returned"
// @Failure 400 {object} map[string]interface{} "Invalid code"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/2fa/enable [post]
func (h *TwoFactorHandler) EnableTwoFactor(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		Secret string `json:"secret" binding:"required"`
		Code   string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	response, err := h.enableCurrentUserTwoFactor(user, req.Secret, req.Code)
	if err != nil {
		if err.Error() == "two-factor authentication is already enabled" || err.Error() == "invalid verification code" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       response.Message,
		"recovery_keys": response.RecoveryKeys,
	})
}

// DisableTwoFactor disables 2FA for the user
// @Summary Disable 2FA
// @Description Disable two-factor authentication. Requires current password.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object true "Current password"
// @Success 200 {object} map[string]interface{} "2FA disabled"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Wrong password or not authenticated"
// @Router /api/v1/users/security/2fa/disable [post]
func (h *TwoFactorHandler) DisableTwoFactor(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := h.disableCurrentUserTwoFactor(user, req.Password); err != nil {
		switch err.Error() {
		case "two-factor authentication is not enabled":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		case "invalid password":
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Two-factor authentication disabled successfully",
	})
}

// VerifyTwoFactorCode verifies a TOTP code for an authenticated user
// This is used during sensitive operations, not during login
// @Summary Verify TOTP code
// @Description Verify a TOTP code for elevated trust within a session.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object true "TOTP code"
// @Success 200 {object} map[string]interface{} "Code valid"
// @Failure 400 {object} map[string]interface{} "Invalid code"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/2fa/verify [post]
func (h *TwoFactorHandler) VerifyTwoFactorCode(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := h.verifyCurrentUserTwoFactorCode(user, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Code verified successfully",
	})
}

// RegenerateRecoveryKeys generates new recovery keys for a user
// @Summary Regenerate recovery keys
// @Description Invalidate existing recovery keys and generate new ones. Requires TOTP code.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object true "TOTP code"
// @Success 200 {object} map[string]interface{} "New recovery keys"
// @Failure 400 {object} map[string]interface{} "Invalid code"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/recovery/regenerate [post]
func (h *TwoFactorHandler) RegenerateRecoveryKeys(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	response, err := h.regenerateCurrentUserRecoveryKeys(user, req.Code)
	if err != nil {
		if err.Error() == "two-factor authentication is not enabled" || err.Error() == "invalid verification code" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       response.Message,
		"recovery_keys": response.RecoveryKeys,
	})
}

// respondWith2FAError sends appropriate error response based on content type
func respondWith2FAError(c *gin.Context, statusCode int, message string) {
	contentType := c.GetHeader("Content-Type")
	acceptHeader := c.GetHeader("Accept")

	if strings.Contains(contentType, "application/json") || strings.Contains(acceptHeader, "application/json") {
		c.JSON(statusCode, gin.H{"error": message})
	} else {
		c.HTML(statusCode, "page/error.tmpl", gin.H{
			"error": message,
		})
	}
}
