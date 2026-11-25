package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

type adminTokenRow struct {
	ID         int
	UserID     int
	Email      string
	Name       string
	Prefix     string
	CreatedAt  string
	LastUsedAt string
	ExpiresAt  string
}

// ShowAdminTokensPage renders the admin's API-token registry for browsers.
func ShowAdminTokensPage(w http.ResponseWriter, r *http.Request) {
	renderAdminTokensPage(w, r, "")
}

func renderAdminTokensPage(w http.ResponseWriter, r *http.Request, newToken string) {
	rows, err := database.QueryContext(context.Background(), database.GetUsersStore(), database.TimeoutComplexSelect, `
		SELECT t.id, t.owner_id, u.email, t.name, t.token_prefix, t.created_at, t.last_used_at, t.expires_at
		FROM tokens t LEFT JOIN users u ON t.owner_id = u.id
		WHERE t.owner_type = 'user' ORDER BY t.created_at DESC
	`)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
			"title": Translate(r, "admin.tokens.title"), "message": Translate(r, "errors.admin.core.failed_to_fetch_tokens"),
		}))
		return
	}
	defer func() { _ = rows.Close() }()
	tokens := make([]adminTokenRow, 0)
	for rows.Next() {
		var token adminTokenRow
		var email sql.NullString
		var created, used, expires interface{}
		if err := rows.Scan(&token.ID, &token.UserID, &email, &token.Name, &token.Prefix, &created, &used, &expires); err != nil {
			middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
				"title": Translate(r, "admin.tokens.title"), "message": Translate(r, "errors.admin.core.failed_to_fetch_tokens"),
			}))
			return
		}
		token.Email = email.String
		if parsed, ok := dbtime.ParseStoredTimestamp(created); ok {
			token.CreatedAt = parsed.Format("2006-01-02 15:04 UTC")
		}
		if parsed, ok := dbtime.ParseStoredTimestamp(used); ok {
			token.LastUsedAt = parsed.Format("2006-01-02 15:04 UTC")
		}
		if parsed, ok := dbtime.ParseStoredTimestamp(expires); ok {
			token.ExpiresAt = parsed.Format("2006-01-02 15:04 UTC")
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
			"title": Translate(r, "admin.tokens.title"), "message": Translate(r, "errors.admin.core.failed_to_fetch_tokens"),
		}))
		return
	}
	users, err := (&model.UserModel{Store: database.GetUsersStore()}).GetAll()
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
			"title": Translate(r, "admin.tokens.title"), "message": Translate(r, "errors.admin.core.failed_to_fetch_users"),
		}))
		return
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/tokens.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.tokens.title"), "page": "tokens", "tokens": tokens, "users": users,
		"new_token": newToken, "flash": TakeFlash(w, r),
	}))
}

// GenerateAdminTokenForm creates a token for a selected user. The token is
// shown in the POST response only; its plaintext is never stored or placed in
// a URL.
func GenerateAdminTokenForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !middleware.ValidateCSRFRequest(r) {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
		return
	}
	userID, err := strconv.Atoi(r.FormValue("user_id"))
	name := strings.TrimSpace(r.FormValue("name"))
	if err != nil || userID <= 0 || name == "" || len(name) > 100 {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
		return
	}
	user, err := (&model.UserModel{Store: database.GetUsersStore()}).GetByID(int64(userID))
	if err != nil || user == nil || !user.IsActive || user.IsBanned {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
		return
	}
	tokenModel := &model.TokenModel{Store: database.GetUsersStore()}
	token, err := tokenModel.CreateAPIToken(userID, name)
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
		return
	}
	if err := writeAdminTokenAudit(r, AdminID(r), userID, token.ID, "token.created"); err != nil {
		if deleteErr := tokenModel.DeleteAPIToken(token.ID); deleteErr != nil {
			util.LogError("token create: audit failed and token %d could not be rolled back: %v", token.ID, deleteErr)
		}
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	middleware.RenderHTML(w, r, http.StatusOK, "admin/token_created.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.tokens.new_token_heading"), "page": "tokens", "new_token": token.Token,
	}))
}

// RevokeAdminTokenForm revokes a user API token and returns to the token list.
func RevokeAdminTokenForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err == nil && id > 0 {
		err = (&model.TokenModel{Store: database.GetUsersStore()}).DeleteAPIToken(id)
	}
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
	} else {
		if err := writeAdminTokenAudit(r, AdminID(r), 0, id, "token.revoked"); err != nil {
			util.LogError("audit: token %d was revoked but the audit entry could not be recorded: %v", id, err)
		}
		SetFlash(w, r, "success", "flash.settings_saved")
	}
	http.Redirect(w, r, adminPagePath(r, "/security/tokens"), http.StatusSeeOther)
}

func writeAdminTokenAudit(r *http.Request, adminID int64, userID, tokenID int, action string) error {
	details, err := json.Marshal(map[string]interface{}{"user_id": userID, "token_id": tokenID})
	if err != nil {
		return err
	}
	_, err = database.ExecContext(context.Background(), database.GetServerStore(), database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, source, status)
		VALUES (?, ?, 'admin', ?, ?, 'token', ?, ?, ?, ?, 'web', 'success')
	`, ulid.Make().String(), dbtime.FormatSQLTimestamp(time.Now()), strconv.FormatInt(adminID, 10), action,
		strconv.Itoa(tokenID), string(details), util.TrustedGetClientIP(r), r.UserAgent())
	return err
}
