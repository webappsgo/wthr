package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newNotificationPreferencesTestHandler wires a NotificationPreferencesHandler
// against a fresh legacy-schema in-memory DB. Like notification_channels.go,
// this handler's SQL (user_notification_preferences with channel_type/
// priority/config columns, notification_subscriptions) matches only
// database.Schema — not database.ServerSchema/UsersSchema, which define a
// completely different, incompatible user_notification_preferences table
// (enable_toast/enable_banner/... columns, no channel_type). See the final
// report: this is the same production bug class as notification_channels.go.
func newNotificationPreferencesTestHandler(t *testing.T) *NotificationPreferencesHandler {
	t.Helper()
	db := newNotificationChannelsTestDB(t)
	return &NotificationPreferencesHandler{DB: db}
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
			INSERT INTO user_notification_preferences
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
	})

	t.Run("valid update persists fields", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		res, err := h.DB.Exec(`
			INSERT INTO user_notification_preferences
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

		var enabled bool
		var priority int
		if err := h.DB.QueryRow("SELECT enabled, priority FROM user_notification_preferences WHERE id = ?", id).Scan(&enabled, &priority); err != nil {
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
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/preferences", "not json")
		setUserIDContext(c, 1)

		h.CreatePreference(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
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

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_notification_preferences WHERE user_id = 1 AND channel_type = 'sms'").Scan(&count); err != nil {
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

		var count, priority int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_notification_preferences WHERE user_id = 2 AND channel_type = 'push'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected upsert to keep 1 row, got %d", count)
		}
		if err := h.DB.QueryRow("SELECT priority FROM user_notification_preferences WHERE user_id = 2 AND channel_type = 'push'").Scan(&priority); err != nil {
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
	})

	t.Run("existing row is deleted", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		res, err := h.DB.Exec(`
			INSERT INTO user_notification_preferences
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

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_notification_preferences WHERE id = ?", id).Scan(&count); err != nil {
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

		var enabled bool
		if err := h.DB.QueryRow("SELECT enabled FROM notification_subscriptions WHERE id = ?", id).Scan(&enabled); err != nil {
			t.Fatalf("query updated row: %v", err)
		}
		if !enabled {
			t.Errorf("expected enabled = true after update")
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
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationPreferencesTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/notifications/subscriptions", "not json")
		setUserIDContext(c, 1)

		h.CreateSubscription(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
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

		var count int
		if err := h.DB.QueryRow("SELECT COUNT(*) FROM notification_subscriptions WHERE user_id = 1 AND subscription_type = 'weather' AND subscription_category = 'severe'").Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row, got %d", count)
		}
	})
}
