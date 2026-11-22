package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/service"
)

// AdminPGPKeyHandler manages the authenticated administrator's personal report key.
type AdminPGPKeyHandler struct{ DB *sql.DB }

// Save validates and stores the current administrator's armored public key.
func (h AdminPGPKeyHandler) Save(w http.ResponseWriter, r *http.Request) {
	adminID := AdminID(r)
	if adminID <= 0 || h.DB == nil {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}
	key, err := service.ValidatePublicKey(strings.TrimSpace(req.PublicKey))
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.profile.invalid_pgp_key"))
		return
	}
	if _, err = h.DB.ExecContext(r.Context(), `UPDATE admins SET personal_pgp_public_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, key, adminID); err != nil {
		InternalError(w, r, Translate(r, "errors.generic.internal_error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "registered": true})
}

// Remove deletes the current administrator's personal report key.
func (h AdminPGPKeyHandler) Remove(w http.ResponseWriter, r *http.Request) {
	adminID := AdminID(r)
	if adminID <= 0 || h.DB == nil {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE admins SET personal_pgp_public_key = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, adminID); err != nil {
		InternalError(w, r, Translate(r, "errors.generic.internal_error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "registered": false})
}

// SaveForm registers the administrator's report key through a no-JavaScript HTML form.
func (h AdminPGPKeyHandler) SaveForm(w http.ResponseWriter, r *http.Request) {
	adminID := AdminID(r)
	if adminID <= 0 || h.DB == nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		redirectPGPProfile(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		redirectPGPProfile(w, r)
		return
	}
	key, err := service.ValidatePublicKey(strings.TrimSpace(r.PostFormValue("public_key")))
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		redirectPGPProfile(w, r)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE admins SET personal_pgp_public_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, key, adminID); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
	} else {
		SetFlash(w, r, "success", "flash.settings_saved")
	}
	redirectPGPProfile(w, r)
}

// RemoveForm removes the administrator's report key through a no-JavaScript HTML form.
func (h AdminPGPKeyHandler) RemoveForm(w http.ResponseWriter, r *http.Request) {
	adminID := AdminID(r)
	if adminID <= 0 || h.DB == nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		redirectPGPProfile(w, r)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE admins SET personal_pgp_public_key = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, adminID); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
	} else {
		SetFlash(w, r, "success", "flash.settings_saved")
	}
	redirectPGPProfile(w, r)
}

func redirectPGPProfile(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/remove")
	path = strings.TrimSuffix(path, "/profile/security/pgp") + "/profile"
	http.Redirect(w, r, path, http.StatusSeeOther)
}
