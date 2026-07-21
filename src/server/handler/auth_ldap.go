// Package handler provides HTTP handlers for LDAP authentication per AI.md PART 11.
package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
)

// LDAPAuthHandler handles LDAP-based user authentication.
type LDAPAuthHandler struct {
	DB          *sql.DB
	LDAPService *service.LDAPService
}

// LDAPLoginRequest is the POST body for /auth/ldap.
type LDAPLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login authenticates a user via LDAP and creates a session.
//
// @Summary LDAP login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LDAPLoginRequest true "LDAP credentials"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /auth/ldap [post]
func (h *LDAPAuthHandler) Login(c *gin.Context) {
	var req LDAPLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	ldapCfg := h.LDAPService.Config()
	if !ldapCfg.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "LDAP authentication is not enabled"})
		return
	}

	email, displayName, err := h.LDAPService.Authenticate(req.Username, req.Password)
	if err != nil {
		if err.Error() == "invalid credentials" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "LDAP service unavailable"})
		return
	}

	usersDB := database.GetUsersDB()

	var userID int64
	err = usersDB.QueryRow(
		"SELECT id FROM user_accounts WHERE username = ? AND is_active = 1 AND is_banned = 0",
		req.Username,
	).Scan(&userID)

	if err == sql.ErrNoRows {
		if email == "" {
			email = req.Username + "@ldap.local"
		}
		if displayName == "" {
			displayName = req.Username
		}
		randToken, tokenErr := models.GenerateSecureToken(32)
		if tokenErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
			return
		}
		pwHash, hashErr := models.HashPassword(randToken)
		if hashErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
			return
		}
		result, insertErr := usersDB.Exec(`
			INSERT INTO user_accounts (username, email, display_name, password_hash, role, is_active, email_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'user', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, req.Username, email, displayName, pwHash)
		if insertErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
			return
		}
		userID, _ = result.LastInsertId()
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	sessionModel := &models.UserSessionModel{DB: usersDB}
	session, sessionErr := sessionModel.CreateSession(userID, c.ClientIP(), c.Request.UserAgent(), 24*time.Hour)
	if sessionErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.SetCookie(middleware.SessionCookieName, session.SessionID, 86400, "/", "", c.Request.TLS != nil, true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
