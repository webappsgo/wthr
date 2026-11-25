package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"

	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// AdminTOTPHandler manages each administrator's own TOTP and recovery codes.
type AdminTOTPHandler struct{ DB *database.Store }

// NewAdminTOTPHandler creates an admin MFA handler using the admins database.
func NewAdminTOTPHandler(db *database.Store) *AdminTOTPHandler { return &AdminTOTPHandler{DB: db} }

func (h *AdminTOTPHandler) currentAdmin(r *http.Request) (*model.Admin, bool) {
	raw, ok := reqctx.GetValue(r.Context(), "admin_id")
	if !ok {
		return nil, false
	}
	var id int64
	switch v := raw.(type) {
	case int:
		id = int64(v)
	case int64:
		id = v
	default:
		return nil, false
	}
	if id <= 0 {
		return nil, false
	}
	admin, err := (&model.AdminModel{Store: h.DB}).GetByID(id)
	return admin, err == nil && admin != nil
}

// ShowSecurityPage renders the signed-in administrator's MFA profile page.
func (h *AdminTOTPHandler) ShowSecurityPage(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.currentAdmin(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusSeeOther)
		return
	}
	status, err := model.AdminTOTPStatusFor(h.DB, admin.ID)
	if err != nil {
		http.Error(w, Translate(r, "errors.admin.totp.status_failed"), http.StatusInternalServerError)
		return
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_totp.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.totp.title"), "page": "profile-security", "breadcrumb": Translate(r, "admin.totp.title"), "totp": status,
	}))
}

// Status returns the current administrator MFA state without exposing secrets.
func (h *AdminTOTPHandler) Status(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.currentAdmin(r)
	if !ok {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.passkey.not_authenticated"), nil)
		return
	}
	status, err := model.AdminTOTPStatusFor(h.DB, admin.ID)
	if err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.status_failed"), nil)
		return
	}
	totpJSON(w, http.StatusOK, true, "", map[string]interface{}{"enabled": status.Enabled, "recovery_keys_remaining": status.RecoveryKeysCount})
}

// SetupTOTP stages a new encrypted TOTP secret after current-password confirmation.
func (h *AdminTOTPHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.currentAdmin(r)
	if !ok {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.passkey.not_authenticated"), nil)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decodeAdminTOTPRequest(w, r, &req) {
		return
	}
	if passwordHasSurroundingWhitespace(req.Password) {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.admin.totp.password_invalid"), nil)
		return
	}
	if valid, _ := model.VerifyPassword(req.Password, admin.PasswordHash); !valid {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.admin.totp.password_invalid"), nil)
		return
	}
	status, err := model.AdminTOTPStatusFor(h.DB, admin.ID)
	if err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.status_failed"), nil)
		return
	}
	if status.Enabled {
		totpJSON(w, http.StatusConflict, false, Translate(r, "errors.admin.totp.already_enabled"), nil)
		return
	}
	secret, qr, err := util.GenerateTOTPSecret(admin.Username, "Weather Admin")
	if err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.setup_failed"), nil)
		return
	}
	if err := model.StorePendingAdminTOTPSecret(h.DB, admin.ID, secret); err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.setup_failed"), nil)
		return
	}
	logAdminTOTPEvent(r, admin.ID, "admin.totp_setup_started")
	setup := map[string]interface{}{"secret": secret, "qr_code": qr, "manual_url": util.GenerateOTPAuthURL(admin.Username, secret, "Weather Admin")}
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		h.renderSecurity(w, r, admin, true, setup, nil, "")
		return
	}
	totpJSON(w, http.StatusOK, true, "", setup)
}

// EnableTOTP confirms possession of the staged secret and returns recovery codes once.
func (h *AdminTOTPHandler) EnableTOTP(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.currentAdmin(r)
	if !ok {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.passkey.not_authenticated"), nil)
		return
	}
	var req struct {
		Code          string `json:"code"`
		RecoverySaved bool   `json:"recovery_saved"`
	}
	if !decodeAdminTOTPRequest(w, r, &req) {
		return
	}
	if !req.RecoverySaved {
		totpJSON(w, http.StatusBadRequest, false, Translate(r, "errors.admin.totp.recovery_ack_required"), nil)
		return
	}
	secret, err := model.PendingAdminTOTPSecret(h.DB, admin.ID)
	if err != nil {
		totpJSON(w, http.StatusConflict, false, Translate(r, "errors.admin.totp.setup_missing"), nil)
		return
	}
	valid, err := util.VerifyTOTP(secret, strings.TrimSpace(req.Code))
	if err != nil || !valid {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.admin.totp.code_invalid"), nil)
		return
	}
	keys, err := model.EnableAdminTOTP(h.DB, admin.ID, secret)
	if err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.enable_failed"), nil)
		return
	}
	logAdminTOTPEvent(r, admin.ID, "admin.totp_enabled")
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		h.renderSecurity(w, r, admin, false, nil, keys, Translate(r, "success.admin.totp.enabled"))
		return
	}
	totpJSON(w, http.StatusOK, true, "", map[string]interface{}{"recovery_keys": keys})
}

// RegenerateRecoveryKeys replaces all codes after a valid current TOTP code.
func (h *AdminTOTPHandler) RegenerateRecoveryKeys(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.currentAdmin(r)
	if !ok {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.passkey.not_authenticated"), nil)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if !decodeAdminTOTPRequest(w, r, &req) {
		return
	}
	valid, err := model.VerifyAdminTOTP(h.DB, admin.ID, req.Code, false)
	if err != nil || !valid {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.admin.totp.code_invalid"), nil)
		return
	}
	keys, err := model.RegenerateAdminRecoveryKeys(h.DB, admin.ID)
	if err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.recovery_failed"), nil)
		return
	}
	logAdminTOTPEvent(r, admin.ID, "admin.totp_recovery_regenerated")
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		h.renderSecurity(w, r, admin, false, nil, keys, Translate(r, "success.admin.totp.recovery_regenerated"))
		return
	}
	totpJSON(w, http.StatusOK, true, "", map[string]interface{}{"recovery_keys": keys})
}

// DisableTOTP requires a valid TOTP code or an unused recovery key.
func (h *AdminTOTPHandler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.currentAdmin(r)
	if !ok {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.passkey.not_authenticated"), nil)
		return
	}
	var req struct {
		Code           string `json:"code"`
		UseRecoveryKey bool   `json:"use_recovery_key"`
	}
	if !decodeAdminTOTPRequest(w, r, &req) {
		return
	}
	valid, err := model.VerifyAdminTOTP(h.DB, admin.ID, req.Code, req.UseRecoveryKey)
	if err != nil || !valid {
		totpJSON(w, http.StatusUnauthorized, false, Translate(r, "errors.admin.totp.code_invalid"), nil)
		return
	}
	if err := model.DisableAdminTOTP(h.DB, admin.ID); err != nil {
		totpJSON(w, http.StatusInternalServerError, false, Translate(r, "errors.admin.totp.disable_failed"), nil)
		return
	}
	logAdminTOTPEvent(r, admin.ID, "admin.totp_disabled")
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/totp/disable"), http.StatusSeeOther)
		return
	}
	totpJSON(w, http.StatusOK, true, Translate(r, "success.admin.totp.disabled"), nil)
}

func logAdminTOTPEvent(r *http.Request, adminID int64, action string) {
	db := database.GetServerStore()
	if db == nil {
		log.Printf("audit log unavailable: action=%s admin_id=%d", action, adminID)
		return
	}
	_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `INSERT INTO audit_log (ulid,timestamp,actor_type,actor_id,action,resource_type,resource_id,ip_address,user_agent,status) VALUES (?,?,'admin',?,?,'admin_totp',?,?,?,'success')`, ulid.Make().String(), dbtime.FormatSQLTimestamp(time.Now()), fmt.Sprintf("%d", adminID), action, fmt.Sprintf("%d", adminID), util.TrustedGetClientIP(r), r.UserAgent())
	if err != nil {
		log.Printf("audit log write failed: action=%s admin_id=%d error=%v", action, adminID, err)
	}
}

func (h *AdminTOTPHandler) renderSecurity(w http.ResponseWriter, r *http.Request, admin *model.Admin, showSetup bool, setup map[string]interface{}, keys []string, message string) {
	status, err := model.AdminTOTPStatusFor(h.DB, admin.ID)
	if err != nil {
		http.Error(w, Translate(r, "errors.admin.totp.status_failed"), http.StatusInternalServerError)
		return
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_totp.tmpl", AdminTemplateData(r, map[string]interface{}{"title": Translate(r, "admin.totp.title"), "page": "profile-security", "breadcrumb": Translate(r, "admin.totp.title"), "totp": status, "show_setup": showSetup, "setup": setup, "recovery_keys": keys, "success": message}))
}

// VerifyAdminTOTP is the login-flow verification entry point.
func VerifyAdminTOTP(store *database.Store, adminID int64, code string, useRecoveryKey bool) (bool, error) {
	return model.VerifyAdminTOTP(store, adminID, code, useRecoveryKey)
}

func decodeAdminTOTPRequest(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
			totpJSON(w, http.StatusBadRequest, false, Translate(r, "errors.admin.totp.invalid_request"), nil)
			return false
		}
		return true
	}
	if err := r.ParseForm(); err != nil {
		totpJSON(w, http.StatusBadRequest, false, Translate(r, "errors.admin.totp.invalid_request"), nil)
		return false
	}
	switch req := dst.(type) {
	case *struct {
		Password string `json:"password"`
	}:
		req.Password = r.Form.Get("password")
	case *struct {
		Code          string `json:"code"`
		RecoverySaved bool   `json:"recovery_saved"`
	}:
		req.Code = r.Form.Get("code")
		req.RecoverySaved = r.Form.Get("recovery_saved") == "true" || r.Form.Get("recovery_saved") == "on"
	case *struct {
		Code string `json:"code"`
	}:
		req.Code = r.Form.Get("code")
	case *struct {
		Code           string `json:"code"`
		UseRecoveryKey bool   `json:"use_recovery_key"`
	}:
		req.Code = r.Form.Get("code")
		req.UseRecoveryKey = r.Form.Get("use_recovery_key") == "true" || r.Form.Get("use_recovery_key") == "on"
	}
	return true
}

func totpJSON(w http.ResponseWriter, status int, ok bool, message string, data interface{}) {
	payload := map[string]interface{}{"ok": ok}
	if message != "" {
		payload["message"] = message
		if !ok {
			payload["error"] = message
		}
	}
	if data != nil {
		for k, v := range data.(map[string]interface{}) {
			payload[k] = v
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
