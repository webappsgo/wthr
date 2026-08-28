package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"

	"github.com/go-chi/chi/v5"
)

// NotificationHandler serves the user notification center. DB must be the
// users database handle (database.GetUsersDB() / DualDB.Users): the
// user_notifications table this handler reads lives in database.UsersSchema,
// not database.ServerSchema.
type NotificationHandler struct {
	DB *sql.DB
}

// defaultNotificationPageSize is the page size used when the request omits
// "limit" or asks for a value outside the accepted range.
const defaultNotificationPageSize = 20

// maxNotificationPageSize caps how many rows one page may request, so a
// hostile "limit" cannot turn a listing into a full-table read.
const maxNotificationPageSize = 100

// Notification is the API projection of one user_notifications row. The
// primary key is a ULID string, not an integer, and the optional link is
// carried inside the table's action_json column rather than a "link" column.
type Notification struct {
	ID        string    `json:"id"`
	UserID    int       `json:"user_id"`
	Type      string    `json:"type"`
	Display   string    `json:"display"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Link      string    `json:"link,omitempty"`
	Read      bool      `json:"read"`
	Dismissed bool      `json:"dismissed"`
	CreatedAt time.Time `json:"created_at"`
}

// notificationColumns is the explicit column list every read in this file
// selects, in scan order. Naming the columns keeps the scan stable if the
// table gains columns later.
const notificationColumns = "id, user_id, type, display, title, message, action_json, read, dismissed, created_at"

// unreadNotificationPredicate narrows a user-scoped query to notifications
// that still need the user's attention. A dismissed notification is no longer
// "unread" anywhere else in the app (see model.UserNotificationModel.GetUnread),
// so both the unread listing filter and the unread count exclude dismissed rows.
const unreadNotificationPredicate = " AND read = 0 AND dismissed = 0"

// authedUserID returns the authenticated user's id from context. Unlike
// reqctx.GetInt, which silently returns 0 when the key is absent, this
// reports whether a valid caller was actually authenticated so handlers can
// reject unauthenticated requests instead of silently scoping them to
// user_id=0.
func authedUserID(r *http.Request) (int, bool) {
	val, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		return 0, false
	}
	id, ok := val.(int)
	if !ok {
		return 0, false
	}
	return id, true
}

// notificationLinkFromAction extracts the action URL stored in action_json.
// A NULL, empty or unparseable payload yields an empty link rather than an
// error: a malformed action must not hide the notification itself.
func notificationLinkFromAction(actionJSON sql.NullString) string {
	if !actionJSON.Valid || actionJSON.String == "" {
		return ""
	}

	var action model.NotificationAction
	if err := json.Unmarshal([]byte(actionJSON.String), &action); err != nil {
		return ""
	}

	return action.URL
}

// scanNotification reads one user_notifications row in notificationColumns
// order. created_at is scanned untyped and parsed with dbtime so rows written
// in the canonical UTC layout and rows written by an older local-zone writer
// both resolve to the same absolute instant.
func scanNotification(scan func(dest ...interface{}) error) (Notification, error) {
	var n Notification
	var actionJSON sql.NullString
	var storedCreatedAt interface{}

	err := scan(&n.ID, &n.UserID, &n.Type, &n.Display, &n.Title, &n.Message,
		&actionJSON, &n.Read, &n.Dismissed, &storedCreatedAt)
	if err != nil {
		return Notification{}, err
	}

	n.Link = notificationLinkFromAction(actionJSON)
	if parsed, ok := dbtime.ParseStoredTimestamp(storedCreatedAt); ok {
		n.CreatedAt = parsed
	}

	return n, nil
}

// ListNotifications returns all notifications for the current user
func (h *NotificationHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := authedUserID(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.notifications.core.unauthorized"))
		return
	}

	// Get pagination params. Both values are clamped: a non-numeric, zero or
	// negative "limit" would otherwise reach SQLite as LIMIT -1 (no limit at
	// all) and a page below 1 would produce a negative OFFSET.
	pageParam := r.URL.Query().Get("page")
	if pageParam == "" {
		pageParam = "1"
	}
	page, _ := strconv.Atoi(pageParam)
	if page < 1 {
		page = 1
	}
	limitParam := r.URL.Query().Get("limit")
	if limitParam == "" {
		limitParam = strconv.Itoa(defaultNotificationPageSize)
	}
	limit, _ := strconv.Atoi(limitParam)
	if limit < 1 || limit > maxNotificationPageSize {
		limit = defaultNotificationPageSize
	}
	offset := (page - 1) * limit

	// Get unread filter
	unreadParam := r.URL.Query().Get("unread")
	if unreadParam == "" {
		unreadParam = "false"
	}
	unreadOnly := unreadParam == "true"

	// Build query
	query := "SELECT " + notificationColumns + " FROM user_notifications WHERE user_id = ?"
	args := []interface{}{userID}

	if unreadOnly {
		query += unreadNotificationPredicate
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, query, args...)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_fetch_notifications"))
		return
	}
	defer rows.Close()

	notifications := []Notification{}
	for rows.Next() {
		n, scanErr := scanNotification(rows.Scan)
		if scanErr != nil {
			InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_fetch_notifications"))
			return
		}

		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_fetch_notifications"))
		return
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM user_notifications WHERE user_id = ?"
	countArgs := []interface{}{userID}
	if unreadOnly {
		countQuery += unreadNotificationPredicate
	}
	if err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, countQuery, countArgs...).Scan(&total); err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_fetch_notifications"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
		"total":         total,
		"page":          page,
		"limit":         limit,
	})
}

// GetUnreadCount returns the count of unread notifications
func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := authedUserID(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.notifications.core.unauthorized"))
		return
	}

	var count int
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM user_notifications WHERE user_id = ?"+unreadNotificationPredicate, userID).Scan(&count)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_get_unread_count"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"unread_count": count,
	})
}

// MarkAsRead marks a notification as read
func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := authedUserID(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.notifications.core.unauthorized"))
		return
	}
	notificationID := chi.URLParam(r, "id")

	// Verify ownership
	var ownerID int
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect,
		"SELECT user_id FROM user_notifications WHERE id = ?", notificationID).Scan(&ownerID)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.notifications.core.notification_not_found"))
		return
	}

	if ownerID != userID {
		Forbidden(w, r, Translate(r, "errors.locations.access_denied"))
		return
	}

	// Mark as read. user_id is repeated in the WHERE clause so the write stays
	// scoped to the owner even if the row changed between the check above and
	// this statement.
	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite,
		"UPDATE user_notifications SET read = 1 WHERE id = ? AND user_id = ?", notificationID, userID)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_mark_notification_as_read"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// MarkAllAsRead marks all notifications as read for the current user
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := authedUserID(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.notifications.core.unauthorized"))
		return
	}

	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite,
		"UPDATE user_notifications SET read = 1 WHERE user_id = ? AND read = 0", userID)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_mark_notifications_as_read"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// DeleteNotification deletes a notification
func (h *NotificationHandler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := authedUserID(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.notifications.core.unauthorized"))
		return
	}
	notificationID := chi.URLParam(r, "id")

	// Verify ownership
	var ownerID int
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect,
		"SELECT user_id FROM user_notifications WHERE id = ?", notificationID).Scan(&ownerID)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.notifications.core.notification_not_found"))
		return
	}

	if ownerID != userID {
		Forbidden(w, r, Translate(r, "errors.locations.access_denied"))
		return
	}

	// Delete, scoped to the owner for the same reason as MarkAsRead.
	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite,
		"DELETE FROM user_notifications WHERE id = ? AND user_id = ?", notificationID, userID)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.core.failed_to_delete_notification"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// isValidNotificationType reports whether notifType is one of the five values
// the user_notifications.type CHECK constraint accepts. Writing anything else
// aborts the INSERT at the database level, so it is rejected here first.
func isValidNotificationType(notifType string) bool {
	switch model.NotificationType(notifType) {
	case model.NotificationTypeSuccess,
		model.NotificationTypeInfo,
		model.NotificationTypeWarning,
		model.NotificationTypeError,
		model.NotificationTypeSecurity:
		return true
	}

	return false
}

// CreateNotification creates a new notification (internal use).
// The insert itself is delegated to model.UserNotificationModel.Create, which
// owns the canonical row shape for this table: a generated ULID primary key,
// a CHECK-valid display value, a canonical UTC created_at and a 30-day
// expires_at. link is stored inside action_json, the only column the real
// table has for one; the caller-supplied title doubles as the action label so
// no untranslated string is invented here.
func (h *NotificationHandler) CreateNotification(userID int, notifType, title, message, link string) error {
	if !isValidNotificationType(notifType) {
		return fmt.Errorf("invalid notification type %q", notifType)
	}

	var action *model.NotificationAction
	if link != "" {
		action = &model.NotificationAction{Label: title, URL: link}
	}

	notificationModel := &model.UserNotificationModel{DB: h.DB}
	_, err := notificationModel.Create(userID, model.NotificationType(notifType),
		model.NotificationDisplayToast, title, message, action)

	return err
}

// ShowNotificationsPage renders the notifications page
func (h *NotificationHandler) ShowNotificationsPage(w http.ResponseWriter, r *http.Request) {
	userRole := reqctx.GetString(r.Context(), "user_role")

	NegotiateResponse(w, r, "page/notifications.tmpl", util.TemplateData(r, map[string]interface{}{
		"IsAdmin": userRole == "admin",
		"title":   Translate(r, "notifications.notifications_page_title"),
		"page":    "notifications",
	}))
}
