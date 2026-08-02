package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
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
func (h *AdminsHandler) ListAdmins(c *gin.Context) {
	admins, err := h.AdminModel.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch admins"})
		return
	}

	// Get total count
	count, _ := h.AdminModel.GetCount()

	c.JSON(http.StatusOK, gin.H{
		"ok":     true,
		"count":  count,
		"admins": admins,
	})
}

// GetAdminCount returns the number of admins
// TEMPLATE.md Part 31: Admins can see count but not full details
func (h *AdminsHandler) GetAdminCount(c *gin.Context) {
	count, err := h.AdminModel.GetCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count admins"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":    true,
		"count": count,
	})
}

// CreateInvite creates a new admin invitation
// TEMPLATE.md Part 31: 15-minute invite tokens
func (h *AdminsHandler) CreateInvite(c *gin.Context) {
	var request struct {
		Email     string `json:"email" binding:"required,email"`
		ExpiresIn string `json:"expires_in"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Get current admin ID from session (set by RequireAdminAuth middleware)
	adminIDInterface, exists := c.Get("admin_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	currentAdminID := adminIDInterface.(int)

	invite, expiresIn, err := h.InviteService.CreateInvite(request.Email, currentAdminID, request.ExpiresIn)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"message":    "Invitation created successfully",
		"token":      invite.Token,
		"email":      invite.InvitedEmail,
		"expires_at": invite.ExpiresAt,
		"expires_in": expiresIn,
	})
}

// AcceptInvite processes an invitation acceptance
func (h *AdminsHandler) AcceptInvite(c *gin.Context) {
	var request struct {
		Token    string `json:"token" binding:"required"`
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	admin, err := h.InviteService.AcceptInvite(request.Token, request.Username, request.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Failed to accept invite",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Admin account created successfully",
		"admin": gin.H{
			"id":       admin.ID,
			"username": admin.Username,
			"email":    admin.Email,
		},
	})
}

// GetPendingInvites returns all active invites
func (h *AdminsHandler) GetPendingInvites(c *gin.Context) {
	invites, err := h.InviteService.GetPendingInvites()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invites"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"count":   len(invites),
		"invites": invites,
	})
}

// RevokeInvite revokes a pending invitation
func (h *AdminsHandler) RevokeInvite(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
		return
	}

	if err := h.InviteService.RevokeInvite(token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to revoke invite",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Invite revoked successfully",
	})
}

// UpdateAdmin updates an admin's information
func (h *AdminsHandler) UpdateAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
		return
	}

	var request struct {
		Username string `json:"username" binding:"required,min=3"`
		Email    string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := h.AdminModel.Update(id, request.Username, request.Email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to update admin",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Admin updated successfully",
	})
}

// DeleteAdmin removes an admin account
func (h *AdminsHandler) DeleteAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
		return
	}

	// Prevent deletion of self (basic protection)
	// Get current admin ID from session (set by RequireAdminAuth middleware)
	adminIDInterface, exists := c.Get("admin_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	currentAdminID := int64(adminIDInterface.(int))
	if id == currentAdminID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete your own account"})
		return
	}

	if err := h.AdminModel.Delete(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Failed to delete admin",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Admin deleted successfully",
	})
}

// ChangePassword updates an admin's password
func (h *AdminsHandler) ChangePassword(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
		return
	}

	var request struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Verify current password before allowing change (security requirement)
	// Get current admin from session
	adminIDInterface, exists := c.Get("admin_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	currentAdminID := int64(adminIDInterface.(int))

	// Verify current password
	if request.CurrentPassword != "" {
		var currentHash string
		err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT password FROM admins WHERE id = ?", currentAdminID).Scan(&currentHash)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify password"})
			return
		}

		valid, _ := model.VerifyPassword(request.CurrentPassword, currentHash)
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
			return
		}
	}

	// Hash new password with Argon2id
	passwordHash, err := util.HashPassword(request.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if err := h.AdminModel.UpdatePassword(id, passwordHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to update password",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Password updated successfully",
	})
}

// ShowAdminsPage renders the admin management page
func (h *AdminsHandler) ShowAdminsPage(c *gin.Context) {
	admins, _ := h.AdminModel.GetAll()
	count, _ := h.AdminModel.GetCount()
	pendingInvites, _ := h.InviteService.GetPendingInvites()

	c.HTML(http.StatusOK, "admin/admin_admins.tmpl", util.TemplateData(c, gin.H{
		"title":           "Admin Management",
		"page":            "admins",
		"admins":          admins,
		"admin_count":     count,
		"pending_invites": pendingInvites,
	}))
}

// ShowInviteAcceptPage renders the invite acceptance page
func (h *AdminsHandler) ShowInviteAcceptPage(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "error.tmpl", util.TemplateData(c, gin.H{
			"title":   "Invalid Invite",
			"message": "No invite token provided",
		}))
		return
	}

	// Verify token
	invite, err := h.InviteService.VerifyInvite(token)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.tmpl", util.TemplateData(c, gin.H{
			"title":   "Invalid Invite",
			"message": err.Error(),
		}))
		return
	}

	c.HTML(http.StatusOK, "admin/admin-invite-accept.tmpl", util.TemplateData(c, gin.H{
		"title":   "Accept Admin Invitation",
		"token":   token,
		"email":   invite.InvitedEmail,
		"expires": invite.ExpiresAt,
	}))
}
