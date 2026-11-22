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
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// ShowAdminModerationPage renders the user list and applies search/status
// filters on the server so the page remains useful without JavaScript.
func ShowAdminModerationPage(w http.ResponseWriter, r *http.Request) {
	users, err := (&model.UserModel{DB: database.GetUsersDB()}).GetAll()
	if err != nil {
		renderModerationError(w, r)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	filtered := make([]*model.User, 0, len(users))
	for _, user := range users {
		if query != "" && !strings.Contains(strings.ToLower(user.Username+" "+user.Email+" "+user.DisplayName), strings.ToLower(query)) {
			continue
		}
		if status == "active" && (!user.IsActive || user.IsBanned) || status == "disabled" && user.IsActive && !user.IsBanned {
			continue
		}
		filtered = append(filtered, user)
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_moderation.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.moderation.title"), "page": "moderation-users", "users": filtered,
		"query": query, "status": status, "flash": TakeFlash(w, r),
	}))
}

// AdminModerationImpersonateUser starts a separate, short-lived user session.
// The existing admin session remains in its own cookie and is never copied to
// user request context or the user session record.
func AdminModerationImpersonateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !middleware.ValidateCSRFRequest(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}
	adminID := AdminID(r)
	if adminID <= 0 {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		http.NotFound(w, r)
		return
	}
	user, err := (&model.UserModel{DB: database.GetUsersDB()}).GetByID(userID)
	if err != nil || !user.IsActive || user.IsBanned || user.Role == "admin" {
		http.NotFound(w, r)
		return
	}
	const lifetime = 15 * time.Minute
	sessions := &model.SessionModel{DB: database.GetUsersDB()}
	session, err := sessions.CreateSession(user.ID, int(lifetime.Seconds()))
	if err != nil {
		http.Error(w, "Unable to start impersonation", http.StatusInternalServerError)
		return
	}
	if err := sessions.UpdateData(session.ID, map[string]interface{}{
		"admin_impersonation": true,
		"admin_id":            adminID,
		"target_user_id":      user.ID,
		"started_at":          time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		if deleteErr := sessions.DeleteSession(session.ID); deleteErr != nil {
			util.LogError("impersonation start: unable to remove partial user session for target %d: %v", user.ID, deleteErr)
		}
		http.Error(w, "Unable to start impersonation", http.StatusInternalServerError)
		return
	}
	if err := writeImpersonationAudit(r, adminID, user.ID, "start", "success"); err != nil {
		if deleteErr := sessions.DeleteSession(session.ID); deleteErr != nil {
			util.LogError("impersonation start: unable to remove unaudited user session for target %d: %v", user.ID, deleteErr)
		}
		util.LogError("audit: unable to record impersonation start for admin %d and user %d: %v", adminID, user.ID, err)
		http.Error(w, "Unable to record impersonation", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountUser, session.ID, lifetime))
	http.Redirect(w, r, "/users/dashboard", http.StatusSeeOther)
}

// StopUserImpersonation revokes only the marked user session and redirects to
// the still-authenticated admin panel. The separate admin cookie is untouched.
func StopUserImpersonation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !middleware.ValidateCSRFRequest(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}
	cookie, err := r.Cookie(middleware.UserSessionCookieName())
	if err != nil || cookie.Value == "" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	sessions := &model.SessionModel{DB: database.GetUsersDB()}
	session, err := sessions.GetByID(cookie.Value)
	if err != nil || !isAdminImpersonationSession(session) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	adminID, adminOK := numericSessionValue(session.Data["admin_id"])
	targetID, targetOK := numericSessionValue(session.Data["target_user_id"])
	if !adminOK || !targetOK || targetID != int64(session.UserID) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := sessions.DeleteSession(cookie.Value); err != nil {
		util.LogError("impersonation stop: unable to revoke user session for target %d: %v", targetID, err)
		http.Error(w, "Unable to stop impersonation", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountUser))
	if err := writeImpersonationAudit(r, adminID, targetID, "stop", "success"); err != nil {
		util.LogError("audit: unable to record impersonation stop for admin %d and user %d: %v", adminID, targetID, err)
	}
	adminPath := "/server/administration"
	if cfg, cfgErr := config.CurrentConfig(); cfgErr == nil && cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}
	http.Redirect(w, r, adminPath, http.StatusSeeOther)
}

func isAdminImpersonationSession(session *model.Session) bool {
	return session != nil && session.Data != nil && session.Data["admin_impersonation"] == true
}

func numericSessionValue(value interface{}) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), number > 0
	case int64:
		return number, number > 0
	case float64:
		return int64(number), number > 0 && number == float64(int64(number))
	default:
		return 0, false
	}
}

func writeImpersonationAudit(r *http.Request, adminID, targetID int64, phase, status string) error {
	details, err := json.Marshal(map[string]interface{}{"phase": phase, "admin_id": adminID, "target_user_id": targetID})
	if err != nil {
		return err
	}
	_, err = database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, source, status)
		VALUES (?, ?, 'admin', ?, ?, 'user', ?, ?, ?, ?, 'web', ?)
	`, ulid.Make().String(), dbtime.FormatSQLTimestamp(time.Now()), strconv.FormatInt(adminID, 10),
		"admin.user.impersonate", strconv.FormatInt(targetID, 10), string(details),
		util.TrustedGetClientIP(r), r.UserAgent(), status)
	return err
}

// ShowAdminModerationUserPage renders the selected user's profile, security
// state, active session metadata, and recent activity.
func ShowAdminModerationUserPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	users := &model.UserModel{DB: database.GetUsersDB()}
	user, err := users.GetByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	activities, err := (&model.UserActivityLogModel{DB: database.GetUsersDB()}).GetActivities(id, 0, 25)
	if err != nil {
		renderModerationError(w, r)
		return
	}
	sessions, err := moderationUserSessions(id)
	if err != nil {
		renderModerationError(w, r)
		return
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_user_detail.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.user_detail.title"), "page": "moderation-users", "user": user,
		"activities": activities, "sessions": sessions, "flash": TakeFlash(w, r),
	}))
}

func moderationUserSessions(userID int64) ([]map[string]interface{}, error) {
	rows, err := database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutComplexSelect,
		"SELECT id, ip_address, user_agent, created_at, expires_at, last_used_at FROM user_sessions WHERE user_id = ? ORDER BY last_used_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var sessions []map[string]interface{}
	for rows.Next() {
		var id int64
		var ip, agent sql.NullString
		var created, expires, used interface{}
		if err := rows.Scan(&id, &ip, &agent, &created, &expires, &used); err != nil {
			return nil, err
		}
		sessions = append(sessions, map[string]interface{}{"id": id, "ip_address": ip.String, "user_agent": agent.String, "created_at": created, "expires_at": expires, "last_used_at": used})
	}
	return sessions, rows.Err()
}

// AdminModerationUserAction handles session-authenticated HTML form actions.
func AdminModerationUserAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/moderation/users"), http.StatusSeeOther)
		return
	}
	users := &model.UserModel{DB: database.GetUsersDB()}
	target := adminPagePath(r, "/moderation/users/"+strconv.FormatInt(id, 10))
	action := r.FormValue("action")
	switch action {
	case "disable", "enable":
		if err := users.SetActive(id, action == "enable"); err != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
		} else {
			SetFlash(w, r, "success", "flash.settings_saved")
		}
	case "delete":
		if err := users.DeleteUserAccount(id); err != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
			break
		}
		SetFlash(w, r, "success", "flash.settings_saved")
		target = adminPagePath(r, "/moderation/users")
	case "revoke_sessions":
		if err := (&model.UserSessionModel{DB: database.GetUsersDB()}).DeleteAllSessionsForUser(id); err != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
		} else {
			SetFlash(w, r, "success", "flash.settings_saved")
		}
	default:
		SetFlash(w, r, "error", "flash.settings_save_failed")
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func renderModerationError(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.moderation.title"), "message": Translate(r, "errors.admin.core.failed_to_fetch_users"),
	}))
}
