package handler

import (
	"net/http"

	"github.com/webappsgo/wthr/src/util"
	"github.com/gin-gonic/gin"
)

// AdminUsersHandler handles user management settings
type AdminUsersHandler struct {
	ConfigPath string
}

// ShowUserSettings displays user management settings page
func (h *AdminUsersHandler) ShowUserSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "admin-users.tmpl", gin.H{
		"title": "User Management Settings",
	})
}

// UpdateUserSettings updates user management settings in server.yml
func (h *AdminUsersHandler) UpdateUserSettings(c *gin.Context) {
	var req struct {
		Enabled                      bool   `json:"enabled"`
		RegistrationMode             string `json:"registration_mode"`
		RegistrationRequireEmailVerification bool `json:"registration_require_email_verification"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.RegistrationMode {
	case "public", "private", "disabled":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid registration mode"})
		return
	}

	// Update server.yml
	updates := map[string]interface{}{
		"users.enabled":                                req.Enabled,
		"users.registration.mode":                      req.RegistrationMode,
		"users.registration.require_email_verification": req.RegistrationRequireEmailVerification,
	}

	if err := utils.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
