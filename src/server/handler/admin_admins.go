package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// AdminsHandler handles admin account management
// TEMPLATE.md Part 31: Multiple admin accounts support
type AdminsHandler struct {
	DB            *sql.DB
	AdminModel    *model.AdminModel
	InviteService *service.AdminInviteService
}

// NewAdminsHandler creates a new admins handler
func NewAdminsHandler(db *sql.DB, inviteService *service.AdminInviteService) *AdminsHandler {
	return &AdminsHandler{
		DB:            db,
		AdminModel:    &model.AdminModel{DB: db},
		InviteService: inviteService,
	}
}

// ListAdmins returns all admins (privacy-safe: no passwords)
// TEMPLATE.md Part 31: Admin privacy - can only see count and basic info
func (h *AdminsHandler) ListAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.AdminModel.GetAll()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.admins.failed_to_fetch_admins"))
		return
	}

	// Get total count
	count, _ := h.AdminModel.GetCount()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"count":  count,
		"admins": admins,
	})
}

// GetAdminCount returns the number of admins
// TEMPLATE.md Part 31: Admins can see count but not full details
func (h *AdminsHandler) GetAdminCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.AdminModel.GetCount()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.admins.failed_to_count_admins"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"count": count,
	})
}

// CreateInvite creates a new admin invitation
// TEMPLATE.md Part 31: 15-minute invite tokens
func (h *AdminsHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email     string `json:"email"`
		ExpiresIn string `json:"expires_in"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// binding:"required,email" equivalent
	if request.Email == "" || util.ValidateEmail(request.Email) != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// Get current admin ID from session (set by RequireAdminAuth middleware)
	adminIDInterface, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}
	currentAdminID := adminIDInterface.(int)

	invite, expiresIn, err := h.InviteService.CreateInvite(request.Email, currentAdminID, request.ExpiresIn)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"message":    Translate(r, "success.admin.admins.invitation_created_successfully"),
		"token":      invite.Token,
		"email":      invite.InvitedEmail,
		"expires_at": invite.ExpiresAt,
		"expires_in": expiresIn,
	})
}

// AcceptInvite processes an invitation acceptance
func (h *AdminsHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// binding:"required" / "required,min=3" / "required,min=8" equivalents
	if request.Token == "" || len(request.Username) < 3 || len(request.Password) < 8 {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	admin, err := h.InviteService.AcceptInvite(request.Token, request.Username, request.Password)
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.failed_to_accept_invite"), map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.admins.admin_account_created_successfully"),
		"admin": map[string]interface{}{
			"id":       admin.ID,
			"username": admin.Username,
			"email":    admin.Email,
		},
	})
}

// GetPendingInvites returns all active invites
func (h *AdminsHandler) GetPendingInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := h.InviteService.GetPendingInvites()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.admins.failed_to_fetch_invites"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"count":   len(invites),
		"invites": invites,
	})
}

// RevokeInvite revokes a pending invitation
func (h *AdminsHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		BadRequest(w, r, Translate(r, "errors.admin.admins.token_is_required"))
		return
	}

	if err := h.InviteService.RevokeInvite(token); err != nil {
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, Translate(r, "errors.admin.admins.failed_to_revoke_invite"), map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.admins.invite_revoked_successfully"),
	})
}

// UpdateAdmin updates an admin's information
func (h *AdminsHandler) UpdateAdmin(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_admin_id"))
		return
	}

	var request struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// binding:"required,min=3" / "required,email" equivalents
	if len(request.Username) < 3 || request.Email == "" || util.ValidateEmail(request.Email) != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	if err := h.AdminModel.Update(id, request.Username, request.Email); err != nil {
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, Translate(r, "errors.admin.admins.failed_to_update_admin"), map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.admins.admin_updated_successfully"),
	})
}

// DeleteAdmin removes an admin account
func (h *AdminsHandler) DeleteAdmin(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_admin_id"))
		return
	}

	// Prevent deletion of self (basic protection)
	// Get current admin ID from session (set by RequireAdminAuth middleware)
	adminIDInterface, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}
	currentAdminID := int64(adminIDInterface.(int))
	if id == currentAdminID {
		BadRequest(w, r, Translate(r, "errors.admin.admins.cannot_delete_your_own_account"))
		return
	}

	if err := h.AdminModel.Delete(id); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.failed_to_delete_admin"), map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.admins.admin_deleted_successfully"),
	})
}

// ChangePassword updates an admin's password
func (h *AdminsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_admin_id"))
		return
	}

	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// binding:"required" (current_password) / "required,min=8" (new_password) equivalents.
	// current_password being binding:"required" meant gin's ShouldBindJSON rejected an
	// empty value BEFORE the handler body ran, making the "if request.CurrentPassword != ""
	// branch below dead code as originally written - preserved verbatim here.
	if request.CurrentPassword == "" || len(request.NewPassword) < 8 {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// Verify current password before allowing change (security requirement)
	// Get current admin from session
	adminIDInterface, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}
	currentAdminID := int64(adminIDInterface.(int))

	// Verify current password
	if request.CurrentPassword != "" {
		var currentHash string
		err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT password FROM admins WHERE id = ?", currentAdminID).Scan(&currentHash)
		if err != nil {
			InternalError(w, r, Translate(r, "errors.admin.admins.failed_to_verify_password"))
			return
		}

		valid, _ := model.VerifyPassword(request.CurrentPassword, currentHash)
		if !valid {
			Unauthorized(w, r, Translate(r, "errors.admin.admins.current_password_is_incorrect"))
			return
		}
	}

	// Hash new password with Argon2id
	passwordHash, err := util.HashPassword(request.NewPassword)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.admins.failed_to_hash_password"))
		return
	}

	if err := h.AdminModel.UpdatePassword(id, passwordHash); err != nil {
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, Translate(r, "errors.admin.admins.failed_to_update_password"), map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.admins.password_updated_successfully"),
	})
}

// ShowAdminsPage renders the admin management page
func (h *AdminsHandler) ShowAdminsPage(w http.ResponseWriter, r *http.Request) {
	admins, _ := h.AdminModel.GetAll()
	count, _ := h.AdminModel.GetCount()
	pendingInvites, _ := h.InviteService.GetPendingInvites()

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_admins.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":           Translate(r, "admin.admins.admin_management"),
		"page":            "admins",
		"admins":          admins,
		"admin_count":     count,
		"pending_invites": pendingInvites,
	}))
}

// ShowInviteAcceptPage renders the invite acceptance page
func (h *AdminsHandler) ShowInviteAcceptPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":   Translate(r, "errors.admin.admins.invalid_invite"),
			"message": Translate(r, "errors.admin.admins.no_invite_token_provided"),
		}))
		return
	}

	// Verify token
	invite, err := h.InviteService.VerifyInvite(token)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":   Translate(r, "errors.admin.admins.invalid_invite"),
			"message": err.Error(),
		}))
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin-invite-accept.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":   Translate(r, "admin.admins.accept_admin_invitation"),
		"token":   token,
		"email":   invite.InvitedEmail,
		"expires": invite.ExpiresAt,
	}))
}
