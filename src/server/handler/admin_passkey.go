package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	models "github.com/casapps/wthr/src/server/model"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// logAdminPasskeyAudit writes an admin.passkey_added or admin.passkey_removed
// event to server_audit_log (server.db). Errors are non-fatal — the passkey
// operation has already completed; logging failure must not roll it back.
func logAdminPasskeyAudit(db *sql.DB, action string, adminID, passkeyID int64, passkeyName, clientIP, userAgent string) {
	details, _ := json.Marshal(map[string]string{"name": passkeyName})
	_, err := db.Exec(`
		INSERT INTO server_audit_log
			(ulid, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, 'admin', ?, ?, 'admin_passkey', ?, ?, ?, ?, 'success')
	`,
		ulid.Make().String(),
		fmt.Sprintf("%d", adminID),
		action,
		fmt.Sprintf("%d", passkeyID),
		string(details),
		clientIP,
		userAgent,
	)
	if err != nil {
		// Non-fatal: the passkey operation already completed.
		_ = err
	}
}

// adminSessionCookieName is the canonical cookie name for full admin sessions.
// Mirrors the literal already in use in HandleLogin (auth.go) and
// RequireAdminAuth (middleware/admin_auth.go) — kept here as a constant so
// the passkey-login finalize path issues the same cookie shape.
const adminSessionCookieName = "admin_session"

// AdminPasskeyHandler hosts the gin REST routes for admin-side WebAuthn
// passkey management (registration, listing, and deletion).
type AdminPasskeyHandler struct {
	DB *sql.DB
}

func NewAdminPasskeyHandler(db *sql.DB) *AdminPasskeyHandler {
	return &AdminPasskeyHandler{DB: db}
}

func (h *AdminPasskeyHandler) loadAdminFromContext(c *gin.Context) (*models.Admin, bool) {
	value, exists := c.Get("admin_id")
	if !exists {
		return nil, false
	}

	var adminID int64
	switch v := value.(type) {
	case int:
		if v <= 0 {
			return nil, false
		}
		adminID = int64(v)
	case int64:
		if v <= 0 {
			return nil, false
		}
		adminID = v
	default:
		return nil, false
	}

	adminModel := &models.AdminModel{DB: h.DB}
	admin, err := adminModel.GetByID(adminID)
	if err != nil || admin == nil {
		return nil, false
	}
	return admin, true
}

func adminPasskeyEnvelope(c *gin.Context) PasskeyEnvelope {
	return PasskeyEnvelope{
		Host:  c.Request.Host,
		HTTPS: requestUsesHTTPS(c),
	}
}

// ListPasskeys handles GET /api/{api_version}/{admin_path}/profile/security/passkeys.
func (h *AdminPasskeyHandler) ListPasskeys(c *gin.Context) {
	admin, ok := h.loadAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	summaries, err := ListAdminPasskeys(h.DB, admin.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load passkeys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"passkeys": summaries,
	})
}

// adminPasskeyRegistrationStartRequest mirrors passkeyRegistrationStartRequest
// (user-side) — admins also need to confirm their password before
// registering a new passkey.
type adminPasskeyRegistrationStartRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// RegisterPasskey handles POST /api/{api_version}/{admin_path}/profile/security/passkeys.
// Two-phase: a request body without `response` starts the ceremony and
// returns the WebAuthn options; a request body with `response` finishes it.
func (h *AdminPasskeyHandler) RegisterPasskey(c *gin.Context) {
	admin, ok := h.loadAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	if _, hasResponse := envelope["response"]; hasResponse {
		// Finish: caller must include the ceremony_token they were given.
		var finish struct {
			CeremonyToken string `json:"ceremony_token"`
		}
		if err := json.Unmarshal(body, &finish); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
			return
		}
		if strings.TrimSpace(finish.CeremonyToken) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "ceremony_token is required"})
			return
		}

		result, err := FinishAdminPasskeyRegistrationToken(h.DB, admin, adminPasskeyEnvelope(c), finish.CeremonyToken, body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
			return
		}

		// Audit: admin.passkey_added per AI.md PART 11 shape.
		logAdminPasskeyAudit(h.DB, "admin.passkey_added", admin.ID, result.Passkey.ID,
			result.Passkey.Name, c.ClientIP(), c.Request.UserAgent())

		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"message": "Passkey registered successfully",
			"passkey": result.Passkey,
		})
		return
	}

	// Begin: parse start request, verify password, return options + token.
	var req adminPasskeyRegistrationStartRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	result, err := BeginAdminPasskeyRegistrationToken(h.DB, admin, adminPasskeyEnvelope(c), req.Name, req.Password)
	if err != nil {
		// Map password-related errors to 401 to match the user-side handler.
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "invalid password") {
			status = http.StatusUnauthorized
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":             true,
		"ceremony_token": result.CeremonyToken,
		"options":        result.Options,
	})
}

// adminPasskeyChallengeRequest is the body shape for
// POST /api/{api_version}/auth/admin/passkey/challenge.
type adminPasskeyChallengeRequest struct {
	SessionToken string `json:"session_token"`
}

// BeginPasskeyChallenge handles
// POST /api/{api_version}/auth/admin/passkey/challenge. Public route — the
// pending-session token (issued by HandleLogin when an admin with passkeys
// has just verified their password) authenticates the request.
func (h *AdminPasskeyHandler) BeginPasskeyChallenge(c *gin.Context) {
	var req adminPasskeyChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.SessionToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "session_token is required"})
		return
	}

	result, err := BeginAdminPasskeyLoginToken(h.DB, adminPasskeyEnvelope(c), req.SessionToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":             true,
		"ceremony_token": result.CeremonyToken,
		"options":        result.Options,
	})
}

// VerifyPasskey handles
// POST /api/{api_version}/auth/admin/passkey/verify. Public route — the
// ceremony token (issued by BeginPasskeyChallenge above) authenticates the
// request. Sets the admin_session cookie on success.
func (h *AdminPasskeyHandler) VerifyPasskey(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	var envelope struct {
		CeremonyToken string `json:"ceremony_token"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(envelope.CeremonyToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "ceremony_token is required"})
		return
	}

	// 30 days, mirroring HandleLogin's full-session duration.
	const adminSessionDuration = 30 * 24 * time.Hour

	result, err := FinishAdminPasskeyLoginToken(
		h.DB,
		adminPasskeyEnvelope(c),
		envelope.CeremonyToken,
		body,
		c.ClientIP(),
		c.Request.UserAgent(),
		adminSessionDuration,
	)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": err.Error()})
		return
	}

	isHTTPS := requestUsesHTTPS(c)
	maxAge := int(time.Until(result.ExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetCookie(adminSessionCookieName, result.SessionID, maxAge, "/", "", isHTTPS, true)

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Passkey authentication successful",
		"admin": gin.H{
			"id":       result.Admin.ID,
			"username": result.Admin.Username,
			"email":    result.Admin.Email,
		},
		"expires_at": result.ExpiresAt,
	})
}

// DeletePasskey handles DELETE /api/{api_version}/{admin_path}/profile/security/passkeys/:passkey_id.
func (h *AdminPasskeyHandler) DeletePasskey(c *gin.Context) {
	admin, ok := h.loadAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	passkeyID, err := strconv.ParseInt(strings.TrimSpace(c.Param("passkey_id")), 10, 64)
	if err != nil || passkeyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid passkey id"})
		return
	}

	// Fetch the passkey name before deletion so the audit log entry is meaningful.
	var passkeyName string
	_ = h.DB.QueryRow(
		"SELECT name FROM server_admin_passkeys WHERE id = ? AND admin_id = ?",
		passkeyID, admin.ID,
	).Scan(&passkeyName)

	if err := DeleteAdminPasskey(h.DB, admin.ID, passkeyID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "passkey not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// Audit: admin.passkey_removed per AI.md PART 11 shape.
	logAdminPasskeyAudit(h.DB, "admin.passkey_removed", admin.ID, passkeyID,
		passkeyName, c.ClientIP(), c.Request.UserAgent())

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Passkey deleted successfully",
	})
}
