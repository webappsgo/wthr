package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
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
func LoadCurrentUserTwoFactorStatus(db *sql.DB, user *model.User) (*TwoFactorStatusResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.loadCurrentUserTwoFactorStatus(user)
}

// PrepareCurrentUserTwoFactorSetup returns the same setup payload used by GET /api/v1/users/security/2fa/setup.
func PrepareCurrentUserTwoFactorSetup(db *sql.DB, user *model.User) (*TwoFactorSetupResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.prepareCurrentUserTwoFactorSetup(user)
}

// EnableCurrentUserTwoFactor applies the same 2FA enable flow used by POST /api/v1/users/security/2fa/enable.
func EnableCurrentUserTwoFactor(db *sql.DB, user *model.User, secret string, code string) (*RecoveryKeysResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.enableCurrentUserTwoFactor(user, secret, code)
}

// DisableCurrentUserTwoFactor applies the same 2FA disable flow used by POST /api/v1/users/security/2fa/disable.
func DisableCurrentUserTwoFactor(db *sql.DB, user *model.User, password string) error {
	h := &TwoFactorHandler{DB: db}
	return h.disableCurrentUserTwoFactor(user, password)
}

// VerifyCurrentUserTwoFactorCode applies the same verification flow used by POST /api/v1/users/security/2fa/verify.
func VerifyCurrentUserTwoFactorCode(db *sql.DB, user *model.User, code string) error {
	h := &TwoFactorHandler{DB: db}
	return h.verifyCurrentUserTwoFactorCode(user, code)
}

// RegenerateCurrentUserRecoveryKeys applies the same recovery-key regeneration flow used by POST /api/v1/users/security/recovery/regenerate.
func RegenerateCurrentUserRecoveryKeys(db *sql.DB, user *model.User, code string) (*RecoveryKeysResponse, error) {
	h := &TwoFactorHandler{DB: db}
	return h.regenerateCurrentUserRecoveryKeys(user, code)
}

func (h *TwoFactorHandler) loadCurrentUserTwoFactorStatus(user *model.User) (*TwoFactorStatusResponse, error) {
	recoveryKeysCount := 0
	if user.TwoFactorEnabled {
		recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
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

func (h *TwoFactorHandler) prepareCurrentUserTwoFactorSetup(user *model.User) (*TwoFactorSetupResponse, error) {
	if user.TwoFactorEnabled {
		return nil, fmt.Errorf("two-factor authentication is already enabled")
	}

	secret, qrCodeDataURL, err := util.GenerateTOTPSecret(user.Email, "Weather")
	if err != nil {
		return nil, fmt.Errorf("failed to generate 2FA secret")
	}

	return &TwoFactorSetupResponse{
		Secret:    secret,
		QRCode:    qrCodeDataURL,
		ManualURL: util.GenerateOTPAuthURL(user.Email, secret, "Weather"),
		Account:   user.Email,
		Issuer:    "Weather",
	}, nil
}

func (h *TwoFactorHandler) enableCurrentUserTwoFactor(user *model.User, secret string, code string) (*RecoveryKeysResponse, error) {
	if user.TwoFactorEnabled {
		return nil, fmt.Errorf("two-factor authentication is already enabled")
	}

	valid, err := util.VerifyTOTP(secret, code)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid verification code")
	}

	userModel := &model.UserModel{DB: h.DB}
	if err := userModel.EnableTwoFactor(user.ID, secret); err != nil {
		return nil, fmt.Errorf("failed to enable two-factor authentication")
	}

	recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
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

func (h *TwoFactorHandler) disableCurrentUserTwoFactor(user *model.User, password string) error {
	if !user.TwoFactorEnabled {
		return fmt.Errorf("two-factor authentication is not enabled")
	}

	userModel := &model.UserModel{DB: h.DB}
	if !userModel.CheckPassword(user, password) {
		return fmt.Errorf("invalid password")
	}

	if err := userModel.DisableTwoFactor(user.ID); err != nil {
		return fmt.Errorf("failed to disable two-factor authentication")
	}

	recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
	recoveryKeyModel.DeleteAllForUser(int(user.ID))
	return nil
}

func (h *TwoFactorHandler) verifyCurrentUserTwoFactorCode(user *model.User, code string) error {
	if !user.TwoFactorEnabled {
		return fmt.Errorf("two-factor authentication is not enabled")
	}

	secret, err := model.DecryptTwoFactorSecret(user.TwoFactorSecret)
	if err != nil {
		return fmt.Errorf("invalid verification code")
	}

	valid, err := util.VerifyTOTP(secret, code)
	if err != nil || !valid {
		return fmt.Errorf("invalid verification code")
	}

	return nil
}

func (h *TwoFactorHandler) regenerateCurrentUserRecoveryKeys(user *model.User, code string) (*RecoveryKeysResponse, error) {
	if !user.TwoFactorEnabled {
		return nil, fmt.Errorf("two-factor authentication is not enabled")
	}

	secret, err := model.DecryptTwoFactorSecret(user.TwoFactorSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid verification code")
	}

	valid, err := util.VerifyTOTP(secret, code)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid verification code")
	}

	recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
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
func (h *TwoFactorHandler) GetTwoFactorStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "Not authenticated"})
		return
	}

	status, err := h.loadCurrentUserTwoFactorStatus(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to get two-factor status"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                  true,
		"enabled":             status.Enabled,
		"recovery_keys_count": status.RecoveryKeysCount,
	})
}

// ShowSecurityPage renders the security settings page
func (h *TwoFactorHandler) ShowSecurityPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Get recovery keys count if 2FA is enabled
	var recoveryKeysCount int
	if user.TwoFactorEnabled {
		recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
		recoveryKeysCount, _ = recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
	}

	passkeyModel := &model.UserPasskeyModel{DB: h.DB}
	passkeys, _ := passkeyModel.ListByUserID(user.ID)
	if len(passkeys) > 0 && recoveryKeysCount == 0 {
		recoveryKeyModel := &model.RecoveryKeyModel{DB: h.DB}
		recoveryKeysCount, _ = recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
	}

	NegotiateResponse(w, r, "page/user/security.tmpl", util.TemplateData(r, map[string]interface{}{
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
func (h *TwoFactorHandler) SetupTwoFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	setup, err := h.prepareCurrentUserTwoFactorSetup(user)
	if err != nil {
		if err.Error() == "two-factor authentication is already enabled" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Two-factor authentication is already enabled"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *TwoFactorHandler) EnableTwoFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Secret == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request"})
		return
	}

	response, err := h.enableCurrentUserTwoFactor(user, req.Secret, req.Code)
	if err != nil {
		if err.Error() == "two-factor authentication is already enabled" || err.Error() == "invalid verification code" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *TwoFactorHandler) DisableTwoFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request"})
		return
	}

	if err := h.disableCurrentUserTwoFactor(user, req.Password); err != nil {
		switch err.Error() {
		case "two-factor authentication is not enabled":
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		case "invalid password":
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": err.Error()})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *TwoFactorHandler) VerifyTwoFactorCode(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request"})
		return
	}

	if err := h.verifyCurrentUserTwoFactorCode(user, req.Code); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *TwoFactorHandler) RegenerateRecoveryKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Not authenticated"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request"})
		return
	}

	response, err := h.regenerateCurrentUserRecoveryKeys(user, req.Code)
	if err != nil {
		if err.Error() == "two-factor authentication is not enabled" || err.Error() == "invalid verification code" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":       response.Message,
		"recovery_keys": response.RecoveryKeys,
	})
}
