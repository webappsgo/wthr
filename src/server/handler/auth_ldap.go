// Package handler provides HTTP handlers for LDAP authentication per AI.md PART 11.
package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
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
func (h *LDAPAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LDAPLoginRequest
	if !DecodeAndValidate(w, r, &req) {
		return
	}

	ldapCfg := h.LDAPService.Config()
	if !ldapCfg.Enabled {
		RespondError(w, r, http.StatusServiceUnavailable, ErrServiceUnavail, Translate(r, "errors.auth.ldap.ldap_authentication_is_not_enabled"))
		return
	}

	email, displayName, err := h.LDAPService.Authenticate(req.Username, req.Password)
	if err != nil {
		if err.Error() == "invalid credentials" {
			Unauthorized(w, r, Translate(r, "errors.auth.ldap.invalid_credentials"))
			return
		}
		RespondError(w, r, http.StatusServiceUnavailable, ErrServiceUnavail, Translate(r, "errors.auth.ldap.ldap_service_unavailable"))
		return
	}

	usersDB := database.GetUsersDB()

	var userID int64
	err = database.QueryRowContext(context.Background(), usersDB, database.TimeoutSimpleSelect,
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
			InternalError(w, r, Translate(r, "errors.auth.ldap.failed_to_create_account"))
			return
		}
		pwHash, hashErr := models.HashPassword(randToken)
		if hashErr != nil {
			InternalError(w, r, Translate(r, "errors.auth.ldap.failed_to_create_account"))
			return
		}
		// created_at/updated_at are bound as canonical UTC text rather than
		// produced by SQL's CURRENT_TIMESTAMP, which yields a different type and
		// zone on PostgreSQL, MySQL and SQL Server than it does on SQLite.
		now := dbtime.FormatSQLTimestamp(time.Now())
		result, insertErr := database.ExecContext(context.Background(), usersDB, database.TimeoutWrite, `
			INSERT INTO user_accounts (username, email, display_name, password_hash, role, is_active, email_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'user', 1, 1, ?, ?)
		`, req.Username, email, displayName, pwHash, now, now)
		if insertErr != nil {
			InternalError(w, r, Translate(r, "errors.auth.ldap.failed_to_create_account"))
			return
		}
		userID, _ = result.LastInsertId()
	} else if err != nil {
		InternalError(w, r, Translate(r, "errors.auth.ldap.database_error"))
		return
	}

	sessionModel := &models.UserSessionModel{DB: usersDB}
	session, sessionErr := sessionModel.CreateSession(userID, util.TrustedGetClientIP(r), r.UserAgent(), 24*time.Hour)
	if sessionErr != nil {
		InternalError(w, r, Translate(r, "errors.auth.ldap.failed_to_create_session"))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.SessionID,
		MaxAge:   86400,
		Path:     "/",
		Secure:   r.TLS != nil,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
