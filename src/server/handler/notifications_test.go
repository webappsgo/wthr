package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/server/model"
)

// newNotificationsTestDB builds the fixture for every test in this file by
// executing the real database.UsersSchema, which is where the
// user_notifications table this handler reads actually lives. A hand-rolled
// CREATE TABLE is what previously hid the fact that the handler queried a
// table production never creates, so the real schema constant is the only
// acceptable fixture here.
func newNotificationsTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db := newTestUsersDB(t)
	seedNotificationUsers(t, db, 1, 2)

	return db
}

// seedNotificationUsers inserts the user_accounts parent rows that
// user_notifications.user_id references. username, email and password_hash are
// all NOT NULL in the real DDL, so every seeded account supplies them.
func seedNotificationUsers(t *testing.T, db *sql.DB, ids ...int) {
	t.Helper()

	for _, id := range ids {
		suffix := strconv.Itoa(id)
		_, err := db.Exec(`INSERT INTO user_accounts (id, username, email, password_hash) VALUES (?, ?, ?, ?)`,
			id, "user"+suffix, "user"+suffix+"@example.test", "argon2id$placeholder")
		if err != nil {
			t.Fatalf("seed user_accounts id=%d: %v", id, err)
		}
	}
}

// seedNotification inserts one user_notifications row with an explicit ULID
// primary key and a canonical UTC created_at, matching how
// model.UserNotificationModel.Create writes rows in production.
func seedNotification(t *testing.T, db *sql.DB, id string, userID int, notifType, display, title string, actionJSON interface{}, read, dismissed bool, createdAt time.Time) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO user_notifications (id, user_id, type, display, title, message, action_json, read, dismissed, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, notifType, display, title, title+" body", actionJSON, read, dismissed, dbtime.FormatSQLTimestamp(createdAt))
	if err != nil {
		t.Fatalf("seed user_notifications id=%s: %v", id, err)
	}
}

// notificationListResponse mirrors the ListNotifications payload so tests can
// assert on decoded values instead of matching raw JSON substrings.
type notificationListResponse struct {
	Notifications []Notification `json:"notifications"`
	Total         int            `json:"total"`
	Page          int            `json:"page"`
	Limit         int            `json:"limit"`
}

// decodeNotificationList decodes a ListNotifications response body.
func decodeNotificationList(t *testing.T, body []byte) notificationListResponse {
	t.Helper()

	var payload notificationListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode list response: %v (body=%s)", err, body)
	}

	return payload
}

// setUserIDInt sets the context key notifications.go reads to identify the
// caller. The unauthenticated subtests deliberately leave it unset.
func setUserIDInt(c *gin.Context, id int) {
	c.Set("user_id", id)
}

// readNotificationRow returns the stored read/dismissed flags and action_json
// of one row, for asserting the effect of a write.
func readNotificationRow(t *testing.T, db *sql.DB, id string) (read bool, dismissed bool, action sql.NullString) {
	t.Helper()

	err := db.QueryRow(`SELECT read, dismissed, action_json FROM user_notifications WHERE id = ?`, id).
		Scan(&read, &dismissed, &action)
	if err != nil {
		t.Fatalf("read back notification %s: %v", id, err)
	}

	return read, dismissed, action
}

func TestNotificationHandler_ListNotifications(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("returns only the caller's notifications, newest first", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		seedNotification(t, db, "01JOLDEST0000000000000000", 1, "info", "center", "older", nil, false, false, base)
		seedNotification(t, db, "01JNEWEST0000000000000000", 1, "warning", "toast", "newer", nil, true, false, base.Add(time.Hour))
		seedNotification(t, db, "01JOTHERUSER000000000000", 2, "info", "toast", "not mine", nil, false, false, base.Add(2*time.Hour))

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications", "")
		setUserIDInt(c, 1)

		h.ListNotifications(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		payload := decodeNotificationList(t, w.Body.Bytes())
		if payload.Total != 2 {
			t.Errorf("total = %d, want 2 (only user 1's rows)", payload.Total)
		}
		if len(payload.Notifications) != 2 {
			t.Fatalf("len(notifications) = %d, want 2", len(payload.Notifications))
		}
		if payload.Notifications[0].ID != "01JNEWEST0000000000000000" {
			t.Errorf("first id = %q, want the newest row", payload.Notifications[0].ID)
		}
		if !payload.Notifications[0].CreatedAt.Equal(base.Add(time.Hour)) {
			t.Errorf("created_at = %v, want %v", payload.Notifications[0].CreatedAt, base.Add(time.Hour))
		}
		if payload.Notifications[1].Display != "center" {
			t.Errorf("display = %q, want the stored value", payload.Notifications[1].Display)
		}
	})

	t.Run("exposes the action url stored in action_json as link", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		seedNotification(t, db, "01JLINKED0000000000000000", 1, "info", "toast", "linked",
			`{"label":"Open","url":"/users/alerts"}`, false, false, base)
		seedNotification(t, db, "01JBROKEN0000000000000000", 1, "info", "toast", "broken action",
			`not json`, false, false, base.Add(-time.Hour))

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications", "")
		setUserIDInt(c, 1)

		h.ListNotifications(c)

		payload := decodeNotificationList(t, w.Body.Bytes())
		if len(payload.Notifications) != 2 {
			t.Fatalf("len(notifications) = %d, want 2", len(payload.Notifications))
		}
		if payload.Notifications[0].Link != "/users/alerts" {
			t.Errorf("link = %q, want /users/alerts", payload.Notifications[0].Link)
		}
		if payload.Notifications[1].Link != "" {
			t.Errorf("link = %q, want empty for an unparseable action_json", payload.Notifications[1].Link)
		}
	})

	t.Run("unread filter excludes read and dismissed rows", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		seedNotification(t, db, "01JUNREAD0000000000000000", 1, "info", "toast", "unread", nil, false, false, base)
		seedNotification(t, db, "01JALREADYREAD0000000000", 1, "info", "toast", "read", nil, true, false, base)
		seedNotification(t, db, "01JDISMISSED000000000000", 1, "info", "toast", "dismissed", nil, false, true, base)

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications?unread=true", "")
		setUserIDInt(c, 1)

		h.ListNotifications(c)

		payload := decodeNotificationList(t, w.Body.Bytes())
		if payload.Total != 1 {
			t.Errorf("total = %d, want 1", payload.Total)
		}
		if len(payload.Notifications) != 1 || payload.Notifications[0].ID != "01JUNREAD0000000000000000" {
			t.Errorf("notifications = %+v, want only the unread, undismissed row", payload.Notifications)
		}
	})

	t.Run("clamps out-of-range pagination values", func(t *testing.T) {
		cases := []struct {
			name      string
			query     string
			wantPage  int
			wantLimit int
		}{
			{"negative page", "?page=-5", 1, defaultNotificationPageSize},
			{"zero limit", "?limit=0", 1, defaultNotificationPageSize},
			{"limit above cap", "?limit=100000", 1, defaultNotificationPageSize},
			{"non-numeric limit", "?limit=all", 1, defaultNotificationPageSize},
			{"accepted values", "?page=2&limit=5", 2, 5},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				db := newNotificationsTestDB(t)
				h := &NotificationHandler{DB: db}

				c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications"+tc.query, "")
				setUserIDInt(c, 1)

				h.ListNotifications(c)

				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
				}

				payload := decodeNotificationList(t, w.Body.Bytes())
				if payload.Page != tc.wantPage {
					t.Errorf("page = %d, want %d", payload.Page, tc.wantPage)
				}
				if payload.Limit != tc.wantLimit {
					t.Errorf("limit = %d, want %d", payload.Limit, tc.wantLimit)
				}
			})
		}
	})

	t.Run("rejects an unauthenticated request", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications", "")

		h.ListNotifications(c)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_GetUnreadCount(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("counts only unread, undismissed rows owned by the caller", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		seedNotification(t, db, "01JCOUNTA0000000000000000", 1, "info", "toast", "a", nil, false, false, base)
		seedNotification(t, db, "01JCOUNTB0000000000000000", 1, "info", "toast", "b", nil, false, false, base)
		seedNotification(t, db, "01JCOUNTC0000000000000000", 1, "info", "toast", "c", nil, true, false, base)
		seedNotification(t, db, "01JCOUNTD0000000000000000", 1, "info", "toast", "d", nil, false, true, base)
		seedNotification(t, db, "01JCOUNTE0000000000000000", 2, "info", "toast", "e", nil, false, false, base)

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications/unread-count", "")
		setUserIDInt(c, 1)

		h.GetUnreadCount(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var payload struct {
			UnreadCount int `json:"unread_count"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.UnreadCount != 2 {
			t.Errorf("unread_count = %d, want 2", payload.UnreadCount)
		}
	})

	t.Run("rejects an unauthenticated request", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications/unread-count", "")

		h.GetUnreadCount(c)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_MarkAsRead(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("marks the caller's own notification read", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		seedNotification(t, db, "01JMARKOWN000000000000000", 1, "info", "toast", "mine", nil, false, false, base)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/01JMARKOWN000000000000000/read", "")
		setUserIDInt(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "01JMARKOWN000000000000000"}}

		h.MarkAsRead(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if read, _, _ := readNotificationRow(t, db, "01JMARKOWN000000000000000"); !read {
			t.Error("read = false, want true after MarkAsRead")
		}
	})

	t.Run("returns 404 for an unknown id", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/missing/read", "")
		setUserIDInt(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "01JDOESNOTEXIST000000000"}}

		h.MarkAsRead(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("refuses another user's notification and leaves it unread", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		seedNotification(t, db, "01JMARKTHEIRS000000000000", 2, "info", "toast", "theirs", nil, false, false, base)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/01JMARKTHEIRS000000000000/read", "")
		setUserIDInt(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "01JMARKTHEIRS000000000000"}}

		h.MarkAsRead(c)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
		if read, _, _ := readNotificationRow(t, db, "01JMARKTHEIRS000000000000"); read {
			t.Error("read = true, want the other user's row left untouched")
		}
	})

	t.Run("rejects an unauthenticated request", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/x/read", "")
		c.Params = gin.Params{{Key: "id", Value: "01JANY000000000000000000"}}

		h.MarkAsRead(c)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_MarkAllAsRead(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("marks every row of the caller and nobody else's", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		seedNotification(t, db, "01JALLA00000000000000000", 1, "info", "toast", "a", nil, false, false, base)
		seedNotification(t, db, "01JALLB00000000000000000", 1, "info", "toast", "b", nil, false, false, base)
		seedNotification(t, db, "01JALLC00000000000000000", 2, "info", "toast", "c", nil, false, false, base)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/read-all", "")
		setUserIDInt(c, 1)

		h.MarkAllAsRead(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var unreadForUser1 int
		if err := db.QueryRow(`SELECT COUNT(*) FROM user_notifications WHERE user_id = 1 AND read = 0`).Scan(&unreadForUser1); err != nil {
			t.Fatalf("count user 1: %v", err)
		}
		if unreadForUser1 != 0 {
			t.Errorf("user 1 unread = %d, want 0", unreadForUser1)
		}

		if read, _, _ := readNotificationRow(t, db, "01JALLC00000000000000000"); read {
			t.Error("user 2's row was marked read, want untouched")
		}
	})

	t.Run("rejects an unauthenticated request", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/read-all", "")

		h.MarkAllAsRead(c)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_DeleteNotification(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("deletes the caller's own notification", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		seedNotification(t, db, "01JDELMINE000000000000000", 1, "info", "toast", "mine", nil, false, false, base)

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/notifications/01JDELMINE000000000000000", "")
		setUserIDInt(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "01JDELMINE000000000000000"}}

		h.DeleteNotification(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var remaining int
		if err := db.QueryRow(`SELECT COUNT(*) FROM user_notifications WHERE id = ?`, "01JDELMINE000000000000000").Scan(&remaining); err != nil {
			t.Fatalf("count: %v", err)
		}
		if remaining != 0 {
			t.Errorf("remaining = %d, want 0", remaining)
		}
	})

	t.Run("refuses another user's notification and keeps the row", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		seedNotification(t, db, "01JDELTHEIRS0000000000000", 2, "info", "toast", "theirs", nil, false, false, base)

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/notifications/01JDELTHEIRS0000000000000", "")
		setUserIDInt(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "01JDELTHEIRS0000000000000"}}

		h.DeleteNotification(c)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}

		var remaining int
		if err := db.QueryRow(`SELECT COUNT(*) FROM user_notifications WHERE id = ?`, "01JDELTHEIRS0000000000000").Scan(&remaining); err != nil {
			t.Fatalf("count: %v", err)
		}
		if remaining != 1 {
			t.Errorf("remaining = %d, want 1 (row must survive a rejected delete)", remaining)
		}
	})

	t.Run("returns 404 for an unknown id", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/notifications/missing", "")
		setUserIDInt(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "01JDOESNOTEXIST000000000"}}

		h.DeleteNotification(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects an unauthenticated request", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/notifications/x", "")
		c.Params = gin.Params{{Key: "id", Value: "01JANY000000000000000000"}}

		h.DeleteNotification(c)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_CreateNotification(t *testing.T) {
	t.Run("writes a canonical user_notifications row", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		before := time.Now().UTC().Add(-time.Second)
		if err := h.CreateNotification(1, "security", "New sign-in", "A new device signed in", "/users/sessions"); err != nil {
			t.Fatalf("CreateNotification: %v", err)
		}

		var id, notifType, display, title, message string
		var actionJSON sql.NullString
		var read, dismissed bool
		var storedCreatedAt, storedExpiresAt interface{}
		err := db.QueryRow(`
			SELECT id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
			FROM user_notifications WHERE user_id = ?
		`, 1).Scan(&id, &notifType, &display, &title, &message, &actionJSON, &read, &dismissed, &storedCreatedAt, &storedExpiresAt)
		if err != nil {
			t.Fatalf("read back inserted row: %v", err)
		}

		if len(id) != 26 {
			t.Errorf("id = %q, want a 26-character ULID", id)
		}
		if notifType != "security" {
			t.Errorf("type = %q, want security", notifType)
		}
		if display != string(model.NotificationDisplayToast) {
			t.Errorf("display = %q, want toast", display)
		}
		if title != "New sign-in" || message != "A new device signed in" {
			t.Errorf("title/message = %q/%q, want the supplied values", title, message)
		}
		if read || dismissed {
			t.Errorf("read/dismissed = %v/%v, want false/false", read, dismissed)
		}

		created, ok := dbtime.ParseStoredTimestamp(storedCreatedAt)
		if !ok {
			t.Fatalf("created_at = %v, want a parseable canonical timestamp", storedCreatedAt)
		}
		if created.Before(before) {
			t.Errorf("created_at = %v, want at or after %v", created, before)
		}
		expires, ok := dbtime.ParseStoredTimestamp(storedExpiresAt)
		if !ok {
			t.Fatalf("expires_at = %v, want a parseable canonical timestamp", storedExpiresAt)
		}
		if !expires.After(created) {
			t.Errorf("expires_at = %v, want later than created_at %v", expires, created)
		}

		if !actionJSON.Valid {
			t.Fatal("action_json is NULL, want the link stored as an action")
		}
		var action model.NotificationAction
		if err := json.Unmarshal([]byte(actionJSON.String), &action); err != nil {
			t.Fatalf("decode action_json: %v", err)
		}
		if action.URL != "/users/sessions" {
			t.Errorf("action url = %q, want /users/sessions", action.URL)
		}
		if action.Label != "New sign-in" {
			t.Errorf("action label = %q, want the notification title", action.Label)
		}
	})

	t.Run("stores no action when the link is empty", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		if err := h.CreateNotification(1, "info", "Plain", "No link here", ""); err != nil {
			t.Fatalf("CreateNotification: %v", err)
		}

		var actionJSON sql.NullString
		if err := db.QueryRow(`SELECT action_json FROM user_notifications WHERE user_id = ?`, 1).Scan(&actionJSON); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if actionJSON.Valid && actionJSON.String != "" {
			t.Errorf("action_json = %q, want NULL/empty", actionJSON.String)
		}
	})

	t.Run("rejects a type the CHECK constraint would refuse", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		if err := h.CreateNotification(1, "critical", "Bad", "Not a valid type", ""); err == nil {
			t.Fatal("CreateNotification returned nil, want an error for an invalid type")
		}

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM user_notifications`).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("rows = %d, want 0 after a rejected type", count)
		}
	})

	t.Run("accepts every type the CHECK constraint allows", func(t *testing.T) {
		types := []string{"success", "info", "warning", "error", "security"}

		for _, notifType := range types {
			t.Run(notifType, func(t *testing.T) {
				db := newNotificationsTestDB(t)
				h := &NotificationHandler{DB: db}

				if err := h.CreateNotification(1, notifType, "T", "M", ""); err != nil {
					t.Fatalf("CreateNotification(%s): %v", notifType, err)
				}
			})
		}
	})
}
