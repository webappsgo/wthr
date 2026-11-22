package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// operatorTokenRegeneratedEvent is the audit event of AI.md PART 11 "Operator Token".
const operatorTokenRegeneratedEvent = "security.operator_token_regenerated"

// AdminOperatorTokenHandler manages the operator token on the admin security page.
type AdminOperatorTokenHandler struct{}

// NewAdminOperatorTokenHandler creates the handler.
func NewAdminOperatorTokenHandler() *AdminOperatorTokenHandler {
	return &AdminOperatorTokenHandler{}
}

// securityPageData is the data of the admin security page; extra carries the one-time
// new token after a regeneration.
func securityPageData(r *http.Request, extra map[string]interface{}) map[string]interface{} {
	token := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		token = cfg.Server.Token
	}
	data := map[string]interface{}{
		"title":               Translate(r, "admin.security.title"),
		"page":                "security",
		"breadcrumb":          Translate(r, "admin.security.title"),
		"OperatorTokenMasked": middleware.MaskOperatorToken(token),
		"Settings":            securitySettingsValues(),
	}
	for key, value := range extra {
		data[key] = value
	}
	return data
}

func securitySettingsValues() map[string]string {
	settings := &model.SettingsModel{DB: database.GetServerDB()}
	keys := []string{
		"security.csp", "security.hsts", "security.x_frame_options", "security.x_content_type", "security.referrer_policy", "security.permissions_policy",
		"security.cors_enabled", "security.cors_origins", "security.cors_methods", "security.cors_headers", "security.cors_credentials", "security.cors_max_age",
		"rate_limit.enabled", "rate_limit.global", "rate_limit.per_ip", "rate_limit.api", "rate_limit.admin",
		"security.session_timeout", "security.max_login_attempts", "security.lockout_duration", "security.password_min_length",
	}
	values := map[string]string{
		"security.csp":  "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:;",
		"security.hsts": "max-age=31536000; includeSubDomains", "security.x_frame_options": "DENY", "security.x_content_type": "nosniff",
		"security.referrer_policy": "strict-origin-when-cross-origin", "security.permissions_policy": "geolocation=(), microphone=(), camera=()",
		"security.cors_enabled": "false", "security.cors_origins": "*", "security.cors_methods": "GET, POST, PUT, DELETE, OPTIONS", "security.cors_headers": "Content-Type, Authorization, X-Requested-With", "security.cors_credentials": "false", "security.cors_max_age": "3600",
		"rate_limit.enabled": "true", "rate_limit.global": "100", "rate_limit.per_ip": "60", "rate_limit.api": "120", "rate_limit.admin": "300",
		"security.session_timeout": "2592000", "security.max_login_attempts": "5", "security.lockout_duration": "30", "security.password_min_length": "8",
	}
	for _, key := range keys {
		if value, err := settings.GetSetting(key); err == nil && value != nil {
			values[key] = value.Value
		}
	}
	return values
}

// ShowSecurityPage renders the admin security page with the masked operator token.
// GET /server/{admin_path}/security
func (h *AdminOperatorTokenHandler) ShowSecurityPage(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_security.tmpl", AdminTemplateData(r, securityPageData(r, map[string]interface{}{
		"flash": TakeFlash(w, r),
	})))
}

// GetOperatorToken returns the masked token, never the value.
// GET /api/{api_version}/server/{admin_path}/security/operator-token
func (h *AdminOperatorTokenHandler) GetOperatorToken(w http.ResponseWriter, r *http.Request) {
	token := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		token = cfg.Server.Token
	}
	RespondData(w, r, map[string]interface{}{
		"masked":     middleware.MaskOperatorToken(token),
		"configured": token != "",
	})
}

// confirmAdminPassword re-prompts the current admin's password, the sensitive-operation
// flow of AI.md PART 5. It writes the failure response itself and returns false.
func confirmAdminPassword(w http.ResponseWriter, r *http.Request, password string) (adminID int64, ok bool) {
	rawID, exists := reqctx.GetValue(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return 0, false
	}
	idValue, isInt := rawID.(int)
	if !isInt {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return 0, false
	}
	adminID = int64(idValue)

	var hash string
	err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect,
		"SELECT password_hash FROM admins WHERE id = ?", adminID).Scan(&hash)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.admins.failed_to_verify_password"))
		return 0, false
	}
	if valid, _ := model.VerifyPassword(password, hash); !valid {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.current_password_is_incorrect"))
		return 0, false
	}
	return adminID, true
}

// regenerateOperatorToken replaces server.token: it generates the new value, saves it to
// server.yml (mode 0600), makes it the only value the gate accepts and writes the audit event.
func regenerateOperatorToken(r *http.Request, adminID int64) (string, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return "", http.ErrNotSupported
	}
	token, err := middleware.NewOperatorToken()
	if err != nil {
		return "", err
	}

	previous := cfg.Server.Token
	cfg.Server.Token = token
	if err := config.SaveConfig(cfg); err != nil {
		cfg.Server.Token = previous
		return "", err
	}
	middleware.SetOperatorToken(token)

	if auditLogger, exists := reqctx.GetValue(r.Context(), "auditLogger"); exists {
		if logger, isLogger := auditLogger.(*service.AuditLogger); isLogger {
			_ = logger.LogSuccess(operatorTokenRegeneratedEvent, "security", "admin", strconv.FormatInt(adminID, 10),
				util.TrustedGetClientIP(r), map[string]interface{}{"endpoint": r.URL.Path})
		}
	}
	return token, nil
}

// RegenerateOperatorToken replaces the operator token after the admin confirms their password.
// The new value is shown exactly once, in this response; the previous value stops working at
// once. A browser form re-renders the security page with the value, a JSON body returns it.
// POST /server/{admin_path}/security/operator-token, POST /api/{api_version}/server/{admin_path}/security/operator-token
func (h *AdminOperatorTokenHandler) RegenerateOperatorToken(w http.ResponseWriter, r *http.Request) {
	password := ""
	isForm := wantsFormSubmission(r)
	if isForm {
		if err := r.ParseForm(); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
			return
		}
		password = r.PostForm.Get("current_password")
	} else {
		var body struct {
			CurrentPassword string `json:"current_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
			return
		}
		password = body.CurrentPassword
	}
	if strings.TrimSpace(password) == "" {
		if isForm {
			SetFlash(w, r, "error", "flash.operator_token_password_required")
			http.Redirect(w, r, adminPagePath(r, "/security"), http.StatusSeeOther)
			return
		}
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	adminID, ok := confirmAdminPassword(w, r, password)
	if !ok {
		return
	}
	token, err := regenerateOperatorToken(r, adminID)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.security.operator_token_failed"))
		return
	}

	if isForm {
		w.Header().Set("Cache-Control", "no-store")
		middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_security.tmpl", AdminTemplateData(r, securityPageData(r, map[string]interface{}{
			"NewOperatorToken": token,
		})))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	RespondSuccess(w, r, Translate(r, "admin.security.operator_token_regenerated_message"), map[string]interface{}{"token": token})
}
