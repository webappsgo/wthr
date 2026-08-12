package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/util"
)

// AdminUsersHandler handles user management settings
type AdminUsersHandler struct {
	ConfigPath string
}

// ShowUserSettings displays user management settings page
func (h *AdminUsersHandler) ShowUserSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "admin/admin_users.tmpl", gin.H{
		"title": "User Management Settings",
	})
}

// UpdateUserSettings updates user management settings in server.yml
func (h *AdminUsersHandler) UpdateUserSettings(c *gin.Context) {
	var req struct {
		Enabled                              bool   `json:"enabled"`
		RegistrationMode                     string `json:"registration_mode"`
		RegistrationRequireEmailVerification bool   `json:"registration_require_email_verification"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid registration mode"})
		return
	}

	// Update server.yml
	updates := map[string]interface{}{
		"users.enabled":                                 req.Enabled,
		"users.registration.mode":                       req.RegistrationMode,
		"users.registration.require_email_verification": req.RegistrationRequireEmailVerification,
	}

	if err := util.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
