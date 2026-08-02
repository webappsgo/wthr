package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newNotificationsTestDB opens a fresh in-memory SQLite database with a
// hand-rolled schema matching the tables notifications.go and
// notification_preferences.go actually query. This is deliberately NOT the
// real production ServerSchema/UsersSchema: those schemas do not define a
// "notifications", "user_notification_preferences" (in this shape), or
// "notification_subscriptions" table at all (see BUG #2/#2b/#5 in the final
// report). Per the task's test-writing rules, hand-rolling here is the only
// way to exercise the handlers' own SQL at all; it does not paper over the
// production mismatch, which is covered separately by dedicated bug tests.
func newNotificationsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:handler_notif_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open notifications db: %v", err)
	}
	schema := `
	CREATE TABLE notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		link TEXT,
		read INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE user_notification_preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		channel_type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		priority INTEGER NOT NULL DEFAULT 0,
		quiet_hours_start TEXT,
		quiet_hours_end TEXT,
		config TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, channel_type)
	);
	CREATE TABLE notification_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		subscription_type TEXT NOT NULL,
		subscription_category TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		config TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, subscription_type, subscription_category)
	);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("apply notifications test schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// setUserIDInt mirrors what these two handler files actually read
// (c.GetInt("user_id")), which is a DIFFERENT context key/type than what the
// real auth middleware sets (middleware.UserContextKey = "user", holding a
// *models.User; see BUG #1 in the final report). Tests here deliberately set
// the key the handler code reads, so success/ownership/validation paths can
// be exercised; the auth-bypass subtests below set NO key at all, mirroring
// a real unauthenticated (or genuinely authenticated-but-mismatched) request.
func setUserIDInt(c *gin.Context, id int) {
	c.Set("user_id", id)
}

// --- notifications.go -------------------------------------------------

func TestNotificationHandler_ListNotifications(t *testing.T) {
	t.Run("success returns only the caller's notifications, newest first", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES
			(1, 'info', 'A', 'first', 0), (1, 'info', 'B', 'second', 1), (2, 'info', 'other user', 'x', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications", "")
		setUserIDInt(c, 1)

		h.ListNotifications(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":2`) {
			t.Errorf("body = %s, want total:2 (only user 1's rows)", w.Body.String())
		}
	})

	t.Run("BUG: no authenticated user should return 401, not 200 scoped to user_id=0", func(t *testing.T) {
		// ListNotifications (and every other method in notifications.go) reads
		// userID := c.GetInt("user_id"). gin's GetInt silently returns 0, with
		// no error, when the key is absent - there is no auth check at all.
		// On top of that, the real auth middleware (src/server/middleware/auth.go)
		// never sets a "user_id" key in the first place; it sets UserContextKey
		// ("user") to a *models.User. So even a genuinely logged-in request
		// would be treated as user_id=0 here, not rejected and not correctly
		// scoped. This is BUG #1 in the final report. This subtest asserts the
		// CORRECT behavior (401, request rejected) and is expected to fail
		// against the current implementation.
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES (0, 'info', 'ghost', 'x', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications", "")
		// Deliberately NOT calling setUserIDInt - mirrors a request with no
		// session at all.

		h.ListNotifications(c)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 (BUG: unauthenticated requests are silently scoped to user_id=0 and return 200 instead of being rejected); body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_GetUnreadCount(t *testing.T) {
	db := newNotificationsTestDB(t)
	h := &NotificationHandler{DB: db}

	if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES
		(5, 'info', 'a', 'x', 0), (5, 'info', 'b', 'x', 0), (5, 'info', 'c', 'x', 1)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c, w := newTestContextJSON(t, http.MethodGet, "/api/notifications/unread-count", "")
	setUserIDInt(c, 5)

	h.GetUnreadCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"unread_count":2`) {
		t.Errorf("body = %s, want unread_count:2", w.Body.String())
	}
}

func TestNotificationHandler_MarkAsRead(t *testing.T) {
	t.Run("success marks the owner's own notification as read", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES (1, 'info', 'a', 'x', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/1/read", "")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.MarkAsRead(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var readInt int
		if err := db.QueryRow("SELECT read FROM notifications WHERE id = 1").Scan(&readInt); err != nil {
			t.Fatalf("query: %v", err)
		}
		if readInt != 1 {
			t.Errorf("read = %d, want 1", readInt)
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/999/read", "")
		c.Params = gin.Params{{Key: "id", Value: "999"}}
		setUserIDInt(c, 1)

		h.MarkAsRead(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("notification owned by another user returns 403", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES (2, 'info', 'a', 'x', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/1/read", "")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.MarkAsRead(c)

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationHandler_MarkAllAsRead(t *testing.T) {
	db := newNotificationsTestDB(t)
	h := &NotificationHandler{DB: db}
	if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES
		(1, 'info', 'a', 'x', 0), (1, 'info', 'b', 'x', 0), (2, 'info', 'other', 'x', 0)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c, w := newTestContextJSON(t, http.MethodPost, "/api/notifications/read-all", "")
	setUserIDInt(c, 1)

	h.MarkAllAsRead(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var stillUnreadForUser2 int
	if err := db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = 2 AND read = 0").Scan(&stillUnreadForUser2); err != nil {
		t.Fatalf("query: %v", err)
	}
	if stillUnreadForUser2 != 1 {
		t.Errorf("other user's unread count = %d, want 1 (unaffected)", stillUnreadForUser2)
	}
}

func TestNotificationHandler_DeleteNotification(t *testing.T) {
	t.Run("success deletes the owner's own notification", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES (1, 'info', 'a', 'x', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/notifications/1", "")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.DeleteNotification(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM notifications WHERE id = 1").Scan(&count); err != nil {
			t.Fatalf("query: %v", err)
		}
		if count != 0 {
			t.Errorf("row still present after delete")
		}
	})

	t.Run("notification owned by another user returns 403 and is not deleted", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}
		if _, err := db.Exec(`INSERT INTO notifications (user_id, type, title, message, read) VALUES (2, 'info', 'a', 'x', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/notifications/1", "")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.DeleteNotification(c)

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM notifications WHERE id = 1").Scan(&count); err != nil {
			t.Fatalf("query: %v", err)
		}
		if count != 1 {
			t.Errorf("row was deleted despite ownership mismatch")
		}
	})
}

func TestNotificationHandler_CreateNotification(t *testing.T) {
	t.Run("success inserts a row with the given fields", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		if err := h.CreateNotification(7, "info", "Title", "Message", "https://example.com"); err != nil {
			t.Fatalf("CreateNotification: %v", err)
		}

		var userID int
		var link sql.NullString
		if err := db.QueryRow("SELECT user_id, link FROM notifications WHERE title = 'Title'").Scan(&userID, &link); err != nil {
			t.Fatalf("query: %v", err)
		}
		if userID != 7 {
			t.Errorf("user_id = %d, want 7", userID)
		}
		if !link.Valid || link.String != "https://example.com" {
			t.Errorf("link = %+v, want https://example.com", link)
		}
	})

	t.Run("empty link is stored as NULL, not an empty string", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := &NotificationHandler{DB: db}

		if err := h.CreateNotification(7, "info", "NoLink", "Message", ""); err != nil {
			t.Fatalf("CreateNotification: %v", err)
		}

		var link sql.NullString
		if err := db.QueryRow("SELECT link FROM notifications WHERE title = 'NoLink'").Scan(&link); err != nil {
			t.Fatalf("query: %v", err)
		}
		if link.Valid {
			t.Errorf("link = %q, want NULL for empty input", link.String)
		}
	})
}

// --- notification_preferences.go ---------------------------------------

func TestNotificationPreferencesHandler_GetUserPreferences(t *testing.T) {
	db := newNotificationsTestDB(t)
	h := NewNotificationPreferencesHandler(db)

	if _, err := db.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled, priority) VALUES
		(1, 'email', 1, 5), (1, 'sms', 0, 1), (2, 'email', 1, 9)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c, w := newTestContextJSON(t, http.MethodGet, "/api/preferences", "")
	setUserIDInt(c, 1)

	h.GetUserPreferences(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"total":2`) {
		t.Errorf("body = %s, want total:2 (only user 1's rows)", w.Body.String())
	}
}

func TestNotificationPreferencesHandler_CreatePreference(t *testing.T) {
	t.Run("success creates a preference row", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/preferences", map[string]interface{}{
			"channel_type": "email",
			"enabled":      true,
			"priority":     3,
		})
		setUserIDInt(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM user_notification_preferences WHERE user_id = 1 AND channel_type = 'email'").Scan(&count); err != nil {
			t.Fatalf("query: %v", err)
		}
		if count != 1 {
			t.Errorf("row not created, count = %d", count)
		}
	})

	t.Run("missing required channel_type returns 400", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/preferences", map[string]interface{}{
			"enabled": true,
		})
		setUserIDInt(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/preferences", "{not json")
		setUserIDInt(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationPreferencesHandler_UpdatePreference(t *testing.T) {
	t.Run("success updates the caller's own preference", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)
		if _, err := db.Exec(`INSERT INTO user_notification_preferences (id, user_id, channel_type, enabled, priority) VALUES (1, 1, 'email', 0, 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPut, "/api/preferences/1", map[string]interface{}{
			"enabled":  true,
			"priority": 8,
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.UpdatePreference(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var enabled int
		if err := db.QueryRow("SELECT enabled FROM user_notification_preferences WHERE id = 1").Scan(&enabled); err != nil {
			t.Fatalf("query: %v", err)
		}
		if enabled != 1 {
			t.Errorf("enabled = %d, want 1", enabled)
		}
	})

	t.Run("BUG: non-numeric id should be rejected with 400, not silently treated as 0", func(t *testing.T) {
		// prefID, _ := strconv.Atoi(c.Param("id")) discards the parse error, so a
		// non-numeric :id silently becomes prefID=0. The subsequent UPDATE ...
		// WHERE id = ? AND user_id = ? then matches zero rows, but the handler
		// never checks RowsAffected() and always returns 200 "Preference
		// updated successfully" regardless of whether anything was actually
		// updated. Expected behavior: a non-numeric id should be a 400, and a
		// no-op update should not claim success. This is BUG #4 in the final
		// report (notification_preferences.go:81,96-104). This subtest asserts
		// the CORRECT behavior (400) and is expected to fail against the
		// current implementation.
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)
		if _, err := db.Exec(`INSERT INTO user_notification_preferences (id, user_id, channel_type, enabled, priority) VALUES (1, 1, 'email', 0, 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPut, "/api/preferences/notanumber", map[string]interface{}{
			"enabled": true,
		})
		c.Params = gin.Params{{Key: "id", Value: "notanumber"}}
		setUserIDInt(c, 1)

		h.UpdatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (BUG: non-numeric id is silently treated as 0 instead of being rejected); body=%s", w.Code, w.Body.String())
		}
		var enabled int
		if err := db.QueryRow("SELECT enabled FROM user_notification_preferences WHERE id = 1").Scan(&enabled); err != nil {
			t.Fatalf("query: %v", err)
		}
		if enabled != 0 {
			t.Errorf("enabled = %d, want unchanged 0 (the real row was never touched)", enabled)
		}
	})
}

func TestNotificationPreferencesHandler_DeletePreference(t *testing.T) {
	t.Run("success deletes the caller's own preference", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)
		if _, err := db.Exec(`INSERT INTO user_notification_preferences (id, user_id, channel_type, enabled, priority) VALUES (1, 1, 'email', 0, 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/preferences/1", "")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.DeletePreference(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM user_notification_preferences WHERE id = 1").Scan(&count); err != nil {
			t.Fatalf("query: %v", err)
		}
		if count != 0 {
			t.Errorf("row still present after delete")
		}
	})

	t.Run("BUG: unknown id should return 404, not 200 success", func(t *testing.T) {
		// Same missing-RowsAffected()-check pattern as UpdatePreference: the
		// DELETE matches zero rows for an id that doesn't exist (or belongs to
		// another user), but the handler always returns 200 "Preference
		// deleted successfully" regardless. This is BUG #4 in the final
		// report (notification_preferences.go:158-172). This subtest asserts
		// the CORRECT behavior (404) and is expected to fail against the
		// current implementation.
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/preferences/999", "")
		c.Params = gin.Params{{Key: "id", Value: "999"}}
		setUserIDInt(c, 1)

		h.DeletePreference(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (BUG: DELETE never checks RowsAffected, always claims success); body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationPreferencesHandler_Subscriptions(t *testing.T) {
	t.Run("GetSubscriptions success returns only the caller's subscriptions", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)
		if _, err := db.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, subscription_category, enabled) VALUES
			(1, 'weather', 'alerts', 1), (2, 'weather', 'alerts', 1)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodGet, "/api/subscriptions", "")
		setUserIDInt(c, 1)

		h.GetSubscriptions(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":1`) {
			t.Errorf("body = %s, want total:1 (only user 1's row)", w.Body.String())
		}
	})

	t.Run("CreateSubscription success inserts a row", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/subscriptions", map[string]interface{}{
			"subscription_type":     "weather",
			"subscription_category": "alerts",
			"enabled":               true,
		})
		setUserIDInt(c, 1)

		h.CreateSubscription(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM notification_subscriptions WHERE user_id = 1").Scan(&count); err != nil {
			t.Fatalf("query: %v", err)
		}
		if count != 1 {
			t.Errorf("row not created")
		}
	})

	t.Run("CreateSubscription missing required fields returns 400", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)

		c, w := newTestContextJSON(t, http.MethodPost, "/api/subscriptions", map[string]interface{}{
			"enabled": true,
		})
		setUserIDInt(c, 1)

		h.CreateSubscription(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("UpdateSubscription success updates the caller's own subscription", func(t *testing.T) {
		db := newNotificationsTestDB(t)
		h := NewNotificationPreferencesHandler(db)
		if _, err := db.Exec(`INSERT INTO notification_subscriptions (id, user_id, subscription_type, subscription_category, enabled) VALUES (1, 1, 'weather', 'alerts', 0)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPut, "/api/subscriptions/1", map[string]interface{}{
			"enabled": true,
		})
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		setUserIDInt(c, 1)

		h.UpdateSubscription(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var enabled int
		if err := db.QueryRow("SELECT enabled FROM notification_subscriptions WHERE id = 1").Scan(&enabled); err != nil {
			t.Fatalf("query: %v", err)
		}
		if enabled != 1 {
			t.Errorf("enabled = %d, want 1", enabled)
		}
	})
}
