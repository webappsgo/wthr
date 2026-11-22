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
	config := readAdminConfig(h.ConfigPath)
	registrationMode := adminConfigString(config, "users.registration.mode")
	if registrationMode == "" {
		registrationMode = "open"
	}
	data := map[string]interface{}{
		"title": Translate(r, "admin.users.user_management_settings"),
		"settings": map[string]interface{}{
			"enabled":                    adminConfigBool(config, "users.enabled"),
			"registration_mode":          registrationMode,
			"require_email_verification": adminConfigBool(config, "users.registration.require_email_verification"),
		},
		"flash": TakeFlash(w, r),
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_users.tmpl", AdminTemplateData(r, data))
}

// UpdateUserSettingsForm handles a session-authenticated HTML form submission.
func (h *AdminUsersHandler) UpdateUserSettingsForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/users/settings"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	mode := r.FormValue("registration_mode")
	if mode != "open" && mode != "private" {
		redirectAdminForm(w, r, adminPagePath(r, "/users/settings"), "flash.settings_saved", "flash.settings_save_failed", errInvalidAdminForm)
		return
	}
	updates := map[string]interface{}{
		"users.enabled":                                 r.FormValue("enabled") == "true",
		"users.registration.mode":                       mode,
		"users.registration.require_email_verification": r.FormValue("registration_require_email_verification") == "true",
	}
	err := util.UpdateYAMLConfig(h.ConfigPath, updates)
	redirectAdminForm(w, r, adminPagePath(r, "/users/settings"), "flash.settings_saved", "flash.settings_save_failed", err)
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

	// Registration modes per AI.md PART 34: "open" (default) and "private".
	// Any other value is rejected so an unsupported mode is never persisted.
	switch req.RegistrationMode {
	case "open", "private":
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
