package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
)

// encryptionKeyRotatedEvent is the audit event of AI.md PART 11 "Server Encryption Key".
const encryptionKeyRotatedEvent = "security.encryption_key_rotated"

// newEncryptionKeyRotator builds the rotator over the live databases.
func newEncryptionKeyRotator() *service.EncryptionKeyRotator {
	return &service.EncryptionKeyRotator{ServerDB: database.GetServerDB(), UsersDB: database.GetUsersDB()}
}

// encryptionKeyView is what the admin panel and API may show about the key: that it is
// configured, its version and the state of a rotation in progress, never any key material.
type encryptionKeyView struct {
	Configured     bool   `json:"configured"`
	Version        int    `json:"version"`
	GraceActive    bool   `json:"grace_active"`
	GraceUntil     string `json:"grace_until,omitempty"`
	PendingRows    int    `json:"pending_rows"`
	GraceDaysTotal int    `json:"grace_days"`
}

// currentEncryptionKeyView reads the key state.
func currentEncryptionKeyView() encryptionKeyView {
	view := encryptionKeyView{GraceDaysTotal: int(service.EncryptionKeyGrace / (24 * time.Hour))}
	cfg := config.GetGlobalConfig()
	if cfg == nil || cfg.Server.Security.EncryptionKey == "" {
		return view
	}
	view.Configured = true
	view.Version = cfg.Server.Security.EncryptionKeyVersion

	rotator := newEncryptionKeyRotator()
	if rotator.ServerDB == nil {
		return view
	}
	if until, ok := rotator.GraceUntil(); ok {
		view.GraceActive = true
		view.GraceUntil = until.Format("2006-01-02")
		if pending, err := rotator.PendingRows(); err == nil {
			view.PendingRows = pending
		}
	}
	return view
}

// AdminEncryptionKeyHandler manages server.security.encryption_key on the admin security pages.
type AdminEncryptionKeyHandler struct{}

// NewAdminEncryptionKeyHandler creates the handler.
func NewAdminEncryptionKeyHandler() *AdminEncryptionKeyHandler {
	return &AdminEncryptionKeyHandler{}
}

// ShowEncryptionPage renders the encryption key page.
// GET /server/{admin_path}/security/encryption
func (h *AdminEncryptionKeyHandler) ShowEncryptionPage(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_security_encryption.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":      Translate(r, "admin.security_encryption.title"),
		"page":       "security",
		"breadcrumb": Translate(r, "admin.security_encryption.title"),
		"flash":      TakeFlash(w, r),
		"Key":        currentEncryptionKeyView(),
	}))
}

// GetEncryptionKey returns the key state without any key material.
// GET /api/{api_version}/server/{admin_path}/security/encryption
func (h *AdminEncryptionKeyHandler) GetEncryptionKey(w http.ResponseWriter, r *http.Request) {
	RespondData(w, r, currentEncryptionKeyView())
}

// RotateEncryptionKey rotates the key after the admin confirms their password: the previous key
// stays valid for 30 days, every dependent row is re-encrypted, and the action is audited.
// POST /server/{admin_path}/security/encryption, POST /api/{api_version}/server/{admin_path}/security/encryption
func (h *AdminEncryptionKeyHandler) RotateEncryptionKey(w http.ResponseWriter, r *http.Request) {
	isForm := wantsFormSubmission(r)
	password := ""
	if isForm {
		if err := r.ParseForm(); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
			return
		}
		password = r.PostForm.Get("password")
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

	back := adminPagePath(r, adminSecurityPath+"/encryption")
	if strings.TrimSpace(password) == "" || !verifyAdminPassword(r, password) {
		logSecurityAudit(r, "security.encryption_key_rotation_denied", "encryption_key", "password re-prompt failed")
		if isForm {
			SetFlash(w, r, "error", "flash.encryption_password_invalid")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		Unauthorized(w, r, Translate(r, "errors.admin.admins.current_password_is_incorrect"))
		return
	}

	err := newEncryptionKeyRotator().Rotate()
	if err == nil {
		logSecurityAudit(r, encryptionKeyRotatedEvent, "encryption_key", "")
	}

	if isForm {
		switch {
		case err == nil:
			SetFlash(w, r, "success", "flash.encryption_rotated")
		case errors.Is(err, service.ErrEncryptionRotationPending):
			SetFlash(w, r, "error", "flash.encryption_rotation_pending")
		default:
			SetFlash(w, r, "error", "flash.encryption_rotation_failed")
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	switch {
	case err == nil:
		RespondSuccess(w, r, Translate(r, "admin.security_encryption.rotated_message"), map[string]interface{}{"key": currentEncryptionKeyView()})
	case errors.Is(err, service.ErrEncryptionRotationPending):
		RespondError(w, r, http.StatusConflict, "CONFLICT", Translate(r, "admin.security_encryption.rotation_pending"))
	default:
		InternalError(w, r, Translate(r, "admin.security_encryption.rotation_failed"))
	}
}
