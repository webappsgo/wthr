package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

// logAdminPasskeyAudit writes an admin.passkey_added or admin.passkey_removed
// event to server_audit_log (server.db). Errors are non-fatal — the passkey
// operation has already completed; logging failure must not roll it back.
func logAdminPasskeyAudit(db *sql.DB, action string, adminID, passkeyID int64, passkeyName, clientIP, userAgent string) {
	details, _ := json.Marshal(map[string]string{"name": passkeyName})

	// timestamp is bound explicitly rather than left to the column's
	// DEFAULT CURRENT_TIMESTAMP. The default is an implicit producer no
	// application-side discipline reaches: on PostgreSQL and MySQL it renders
	// in the server's own zone and type, which would leave this one column
	// holding two layouts that no ORDER BY or range comparison can reconcile.
	// The other two writers of server_audit_log (src/server/middleware/audit.go
	// and src/scheduler/scheduler.go) already bind canonical UTC text.
	_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		INSERT INTO server_audit_log
			(ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, ?, 'admin', ?, ?, 'admin_passkey', ?, ?, ?, ?, 'success')
	`,
		ulid.Make().String(),
		dbtime.FormatSQLTimestamp(time.Now()),
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

// AdminPasskeyHandler hosts the chi REST routes for admin-side WebAuthn
// passkey management (registration, listing, and deletion).
type AdminPasskeyHandler struct {
	DB *sql.DB
}

func NewAdminPasskeyHandler(db *sql.DB) *AdminPasskeyHandler {
	return &AdminPasskeyHandler{DB: db}
}

func (h *AdminPasskeyHandler) loadAdminFromContext(r *http.Request) (*models.Admin, bool) {
	value, exists := reqctx.Get(r.Context(), "admin_id")
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

func adminPasskeyEnvelope(r *http.Request) PasskeyEnvelope {
	return PasskeyEnvelope{
		Host:  r.Host,
		HTTPS: requestUsesHTTPS(r),
	}
}

// ListPasskeys handles GET /api/{api_version}/server/{admin_path}/{admin_username}/profile/security/passkeys.
func (h *AdminPasskeyHandler) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.loadAdminFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "Not authenticated"})
		return
	}

	summaries, err := ListAdminPasskeys(h.DB, admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to load passkeys"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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

// RegisterPasskey handles POST /api/{api_version}/server/{admin_path}/{admin_username}/profile/security/passkeys.
// Two-phase: a request body without `response` starts the ceremony and
// returns the WebAuthn options; a request body with `response` finishes it.
func (h *AdminPasskeyHandler) RegisterPasskey(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.loadAdminFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "Not authenticated"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
		return
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
		return
	}

	if _, hasResponse := envelope["response"]; hasResponse {
		// Finish: caller must include the ceremony_token they were given.
		var finish struct {
			CeremonyToken string `json:"ceremony_token"`
		}
		if err := json.Unmarshal(body, &finish); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
			return
		}
		if strings.TrimSpace(finish.CeremonyToken) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "ceremony_token is required"})
			return
		}

		result, err := FinishAdminPasskeyRegistrationToken(h.DB, admin, adminPasskeyEnvelope(r), finish.CeremonyToken, body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}

		// Audit: admin.passkey_added per AI.md PART 11 shape.
		logAdminPasskeyAudit(h.DB, "admin.passkey_added", admin.ID, result.Passkey.ID,
			result.Passkey.Name, util.TrustedGetClientIP(r), r.UserAgent())

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"message": "Passkey registered successfully",
			"passkey": result.Passkey,
		})
		return
	}

	// Begin: parse start request, verify password, return options + token.
	var req adminPasskeyRegistrationStartRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
		return
	}

	result, err := BeginAdminPasskeyRegistrationToken(h.DB, admin, adminPasskeyEnvelope(r), req.Name, req.Password)
	if err != nil {
		// Map password-related errors to 401 to match the user-side handler.
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "invalid password") {
			status = http.StatusUnauthorized
		}
		writeJSON(w, status, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *AdminPasskeyHandler) BeginPasskeyChallenge(w http.ResponseWriter, r *http.Request) {
	var req adminPasskeyChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.SessionToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "session_token is required"})
		return
	}

	result, err := BeginAdminPasskeyLoginToken(h.DB, adminPasskeyEnvelope(r), req.SessionToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":             true,
		"ceremony_token": result.CeremonyToken,
		"options":        result.Options,
	})
}

// VerifyPasskey handles
// POST /api/{api_version}/auth/admin/passkey/verify. Public route — the
// ceremony token (issued by BeginPasskeyChallenge above) authenticates the
// request. Sets the admin_session cookie on success.
func (h *AdminPasskeyHandler) VerifyPasskey(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
		return
	}

	var envelope struct {
		CeremonyToken string `json:"ceremony_token"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(envelope.CeremonyToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "ceremony_token is required"})
		return
	}

	// 30 days, mirroring HandleLogin's full-session duration.
	const adminSessionDuration = 30 * 24 * time.Hour

	result, err := FinishAdminPasskeyLoginToken(
		h.DB,
		adminPasskeyEnvelope(r),
		envelope.CeremonyToken,
		body,
		util.TrustedGetClientIP(r),
		r.UserAgent(),
		adminSessionDuration,
	)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	isHTTPS := requestUsesHTTPS(r)
	maxAge := int(time.Until(result.ExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    result.SessionID,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   isHTTPS,
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "Passkey authentication successful",
		"admin": map[string]interface{}{
			"id":       result.Admin.ID,
			"username": result.Admin.Username,
			"email":    result.Admin.Email,
		},
		"expires_at": result.ExpiresAt,
	})
}

// DeletePasskey handles DELETE /api/{api_version}/server/{admin_path}/{admin_username}/profile/security/passkeys/:passkey_id.
func (h *AdminPasskeyHandler) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.loadAdminFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "Not authenticated"})
		return
	}

	passkeyID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "passkey_id")), 10, 64)
	if err != nil || passkeyID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "Invalid passkey id"})
		return
	}

	// Fetch the passkey name before deletion so the audit log entry is meaningful.
	var passkeyName string
	_ = database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect,
		"SELECT name FROM server_admin_passkeys WHERE id = ? AND admin_id = ?",
		passkeyID, admin.ID,
	).Scan(&passkeyName)

	if err := DeleteAdminPasskey(h.DB, admin.ID, passkeyID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "passkey not found" {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	// Audit: admin.passkey_removed per AI.md PART 11 shape.
	logAdminPasskeyAudit(h.DB, "admin.passkey_removed", admin.ID, passkeyID,
		passkeyName, util.TrustedGetClientIP(r), r.UserAgent())

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "Passkey deleted successfully",
	})
}
