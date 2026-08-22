package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/database"
)

// newNotificationPreferencesTestHandler wires a NotificationPreferencesHandler
// against a fresh in-memory DB carrying the real database.UsersSchema — the
// same DDL initUsersDB applies to users.db, which is the database main.go
// hands this handler. Per-channel delivery preferences live in
// user_notification_channel_preferences; the separate, one-row-per-user
// user_notification_preferences table holds WebUI display preferences and is
// owned by model.NotificationPreferencesModel.
func newNotificationPreferencesTestHandler(t *testing.T) *NotificationPreferencesHandler {
	t.Helper()
	db := newNotificationPreferencesTestDB(t)
	return &NotificationPreferencesHandler{DB: db}
}

// newNotificationPreferencesTestDB opens a fresh in-memory users database.
func newNotificationPreferencesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:handler_notif_prefs_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open notification preferences db: %v", err)
	}
	if _, err := db.Exec(database.UsersSchema); err != nil {
		db.Close()
		t.Fatalf("apply UsersSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// assertPreferencesErrorShape asserts the canonical AI.md PART 14 error body
// {"ok": false, "error": "CODE", "message": "..."} and rejects the legacy
// bare {"error": "text"} shape this handler used before the conversion.
func assertPreferencesErrorShape(t *testing.T, w *httptest.ResponseRecorder, wantCode string) {
	t.Helper()

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error body: %v; body=%s", err, w.Body.String())
	}
	if resp.OK {
		t.Errorf("ok = true, want false; body=%s", w.Body.String())
	}
	if resp.Error != wantCode {
		t.Errorf("error = %q, want %q; body=%s", resp.Error, wantCode, w.Body.String())
	}
	if resp.Message == "" {
		t.Errorf("message is empty; body=%s", w.Body.String())
	}
}

// assertPreferencesSuccessShape asserts the canonical AI.md PART 14 action
// response {"ok": true, "data": {"message": "..."}} with an optional id.
func assertPreferencesSuccessShape(t *testing.T, w *httptest.ResponseRecorder, wantID bool) {
	t.Helper()

	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal success body: %v; body=%s", err, w.Body.String())
	}
	if !resp.OK {
		t.Errorf("ok = false, want true; body=%s", w.Body.String())
	}
	if msg, _ := resp.Data["message"].(string); msg == "" {
		t.Errorf("data.message is empty; body=%s", w.Body.String())
	}
	if wantID {
		if _, ok := resp.Data["id"]; !ok {
			t.Errorf("data.id missing; body=%s", w.Body.String())
		}
	}
}

func TestNotificationPreferencesHandlerGetUserPreferences(t *testing.T) {
	t.Run("no rows returns empty list", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newAPITestContext("/notifications/preferences")
		setUserIDContext(c, 1)

		h.GetUserPreferences(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":0`) {
			t.Errorf("expected total:0, got: %s", w.Body.String())
		}
	})

	t.Run("returns inserted preference with config", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		_, err := h.DB.Exec(`
			INSERT INTO user_notification_channel_preferences
			(user_id, channel_type, enabled, priority, config, created_at, updated_at)
			VALUES (1, 'email', 1, 7, '{"foo":"bar"}', ?, ?)
		`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("insert preference: %v", err)
		}

		c, w := newAPITestContext("/notifications/preferences")
		setUserIDContext(c, 1)

		h.GetUserPreferences(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"total":1`) {
			t.Errorf("expected total:1, got: %s", body)
		}
		if !strings.Contains(body, `"foo":"bar"`) {
			t.Errorf("expected decoded config, got: %s", body)
		}
	})

	t.Run("query failure returns canonical DATABASE_ERROR", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		// Closing the pool makes the SELECT fail deterministically without
		// touching the schema or needing a live database server.
		if err := h.DB.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}

		c, w := newAPITestContext("/notifications/preferences")
		setUserIDContext(c, 1)

		h.GetUserPreferences(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrDatabaseError)
	})
}

func TestNotificationPreferencesHandlerUpdatePreference(t *testing.T) {
	t.Run("non-numeric id returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/preferences/x", map[string]interface{}{})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "x"}}

		h.UpdatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/preferences/1", "not json")
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("unknown preference returns 404", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/preferences/999", map[string]interface{}{
			"enabled": true, "priority": 3,
		})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.UpdatePreference(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrNotFound)
	})

	t.Run("valid update persists fields", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		res, err := h.DB.Exec(`
			INSERT INTO user_notification_channel_preferences
			(user_id, channel_type, enabled, priority, created_at, updated_at)
			VALUES (1, 'email', 0, 5, ?, ?)
		`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("insert preference: %v", err)
		}
		id, _ := res.LastInsertId()

		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/preferences/1", map[string]interface{}{
			"enabled": true, "priority": 9,
		})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdatePreference(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		assertPreferencesSuccessShape(t, w, false)

		var enabled bool
		var priority int
		if err := h.DB.QueryRow("SELECT enabled, priority FROM user_notification_channel_preferences WHERE id = ?", id).Scan(&enabled, &priority); err != nil {
			t.Fatalf("query updated row: %v", err)
		}
		if !enabled || priority != 9 {
			t.Errorf("enabled=%v priority=%d, want true/9", enabled, priority)
		}
	})
}

func TestNotificationPreferencesHandlerCreatePreference(t *testing.T) {
	t.Run("missing channel_type returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/preferences", map[string]interface{}{})
		setUserIDContext(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/preferences", "not json")
		setUserIDContext(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("valid body creates row", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/preferences", map[string]interface{}{
			"channel_type": "sms", "enabled": true, "priority": 4,
		})
		setUserIDContext(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		assertPreferencesSuccessShape(t, w, true)

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_notification_channel_preferences WHERE user_id = 1 AND channel_type = 'sms'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row, got %d", count)
		}
	})

	t.Run("duplicate channel_type upserts instead of duplicating", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c1, _ := newTestContextJSON(t, http.MethodPost, "/notifications/preferences", map[string]interface{}{
			"channel_type": "push", "enabled": false, "priority": 1,
		})
		setUserIDContext(c1, 2)
		h.CreatePreference(c1)

		c2, w2 := newTestContextJSON(t, http.MethodPost, "/notifications/preferences", map[string]interface{}{
			"channel_type": "push", "enabled": true, "priority": 8,
		})
		setUserIDContext(c2, 2)
		h.CreatePreference(c2)

		if w2.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w2.Code, w2.Body.String())
		}
		assertPreferencesSuccessShape(t, w2, true)

		var count, priority int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_notification_channel_preferences WHERE user_id = 2 AND channel_type = 'push'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected upsert to keep 1 row, got %d", count)
		}
		if err := h.DB.QueryRow("SELECT priority FROM user_notification_channel_preferences WHERE user_id = 2 AND channel_type = 'push'").Scan(&priority); err != nil {
			t.Fatalf("query priority: %v", err)
		}
		if priority != 8 {
			t.Errorf("expected upsert to update priority to 8, got %d", priority)
		}
	})
}

func TestNotificationPreferencesHandlerDeletePreference(t *testing.T) {
	t.Run("non-numeric id returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newAPITestContext("/notifications/preferences/x")
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "x"}}

		h.DeletePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newAPITestContext("/notifications/preferences/999")
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.DeletePreference(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrNotFound)
	})

	t.Run("existing row is deleted", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		res, err := h.DB.Exec(`
			INSERT INTO user_notification_channel_preferences
			(user_id, channel_type, enabled, priority, created_at, updated_at)
			VALUES (1, 'email', 1, 5, ?, ?)
		`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("insert preference: %v", err)
		}
		id, _ := res.LastInsertId()

		c, w := newAPITestContext("/notifications/preferences/1")
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.DeletePreference(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		assertPreferencesSuccessShape(t, w, false)

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_notification_channel_preferences WHERE id = ?", id).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected row deleted, still present")
		}
	})
}

func TestNotificationPreferencesHandlerGetSubscriptions(t *testing.T) {
	t.Run("no rows returns empty list", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newAPITestContext("/notifications/subscriptions")
		setUserIDContext(c, 1)

		h.GetSubscriptions(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":0`) {
			t.Errorf("expected total:0, got: %s", w.Body.String())
		}
	})

	t.Run("returns inserted subscription", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		_, err := h.DB.Exec(`
			INSERT INTO notification_subscriptions
			(user_id, subscription_type, subscription_category, enabled, config, created_at, updated_at)
			VALUES (1, 'weather', 'severe', 1, '{"radius":10}', ?, ?)
		`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("insert subscription: %v", err)
		}

		c, w := newAPITestContext("/notifications/subscriptions")
		setUserIDContext(c, 1)

		h.GetSubscriptions(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"total":1`) {
			t.Errorf("expected total:1, got: %s", body)
		}
		if !strings.Contains(body, `"radius":10`) {
			t.Errorf("expected decoded config, got: %s", body)
		}
	})

	t.Run("query failure returns canonical DATABASE_ERROR", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		if err := h.DB.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}

		c, w := newAPITestContext("/notifications/subscriptions")
		setUserIDContext(c, 1)

		h.GetSubscriptions(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrDatabaseError)
	})
}

func TestNotificationPreferencesHandlerUpdateSubscription(t *testing.T) {
	t.Run("non-numeric id returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/subscriptions/x", map[string]interface{}{})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "x"}}

		h.UpdateSubscription(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/subscriptions/1", "not json")
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateSubscription(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("valid body updates row", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		res, err := h.DB.Exec(`
			INSERT INTO notification_subscriptions
			(user_id, subscription_type, subscription_category, enabled, created_at, updated_at)
			VALUES (1, 'weather', 'severe', 0, ?, ?)
		`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("insert subscription: %v", err)
		}
		id, _ := res.LastInsertId()

		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/subscriptions/1", map[string]interface{}{
			"enabled": true,
		})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateSubscription(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		assertPreferencesSuccessShape(t, w, false)

		var enabled bool
		if err := h.DB.QueryRow("SELECT enabled FROM notification_subscriptions WHERE id = ?", id).Scan(&enabled); err != nil {
			t.Fatalf("query updated row: %v", err)
		}
		if !enabled {
			t.Errorf("expected enabled = true after update")
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/subscriptions/4242", map[string]interface{}{
			"enabled": true,
		})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "4242"}}

		h.UpdateSubscription(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrNotFound)
	})

	t.Run("subscription owned by another user returns 404", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		res, err := h.DB.Exec(`
			INSERT INTO notification_subscriptions
			(user_id, subscription_type, subscription_category, enabled, created_at, updated_at)
			VALUES (2, 'weather', 'severe', 0, ?, ?)
		`, time.Now(), time.Now())
		if err != nil {
			t.Fatalf("insert subscription: %v", err)
		}
		id, _ := res.LastInsertId()

		c, w := newTestContextJSON(t, http.MethodPut, "/notifications/subscriptions/1", map[string]interface{}{
			"enabled": true,
		})
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(id, 10)}}

		h.UpdateSubscription(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrNotFound)

		var enabled bool
		if err := h.DB.QueryRow("SELECT enabled FROM notification_subscriptions WHERE id = ?", id).Scan(&enabled); err != nil {
			t.Fatalf("query row: %v", err)
		}
		if enabled {
			t.Errorf("another user's subscription must not be modified")
		}
	})
}

func TestNotificationPreferencesHandlerCreateSubscription(t *testing.T) {
	t.Run("missing required fields returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/subscriptions", map[string]interface{}{
			"subscription_type": "weather",
		})
		setUserIDContext(c, 1)

		h.CreateSubscription(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/subscriptions", "not json")
		setUserIDContext(c, 1)

		h.CreateSubscription(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
		assertPreferencesErrorShape(t, w, ErrInvalidInput)
	})

	t.Run("valid body creates row", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/subscriptions", map[string]interface{}{
			"subscription_type":     "weather",
			"subscription_category": "severe",
			"enabled":               true,
		})
		setUserIDContext(c, 1)

		h.CreateSubscription(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		assertPreferencesSuccessShape(t, w, true)

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM notification_subscriptions WHERE user_id = 1 AND subscription_type = 'weather' AND subscription_category = 'severe'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row, got %d", count)
		}
	})
}
