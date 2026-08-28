package handler

import (
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"

	"github.com/webappsgo/wthr/src/util"
)

// AdminUsersHandler handles user management settings
type AdminUsersHandler struct {
	ConfigPath string
}

// ShowUserSettings displays user management settings page
func (h *AdminUsersHandler) ShowUserSettings(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_users.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.users.user_management_settings"),
	}))
}

// UpdateUserSettings updates user management settings in server.yml
func (h *AdminUsersHandler) UpdateUserSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled                              bool   `json:"enabled"`
		RegistrationMode                     string `json:"registration_mode"`
		RegistrationRequireEmailVerification bool   `json:"registration_require_email_verification"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	// Registration modes per AI.md config-rules.md: open/invite/admin_only/
	// disabled. Legacy values (public/private) are normalised to their
	// current equivalents rather than rejected, so already-stored configs
	// and older API clients keep working.
	switch req.RegistrationMode {
	case "public":
		req.RegistrationMode = "open"
	case "private":
		req.RegistrationMode = "invite"
	case "open", "invite", "admin_only", "disabled":
	default:
		BadRequest(w, r, Translate(r, "errors.admin.users.invalid_registration_mode"))
		return
	}

	// Update server.yml
	updates := map[string]interface{}{
		"users.enabled":                                 req.Enabled,
		"users.registration.mode":                       req.RegistrationMode,
		"users.registration.require_email_verification": req.RegistrationRequireEmailVerification,
	}

	if err := util.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		InternalError(w, r, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
