package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
)

// newNotificationChannelsTestDB opens a fresh in-memory SQLite database with
// the real database.ServerSchema applied verbatim — the same DDL initServerDB
// applies to server.db in production, so every column these tests exercise is
// the column the running server has. No DDL is written by hand here: a
// hand-rolled table that shadows a real one hides the "no such column" failures
// that would fire in production.
func newNotificationChannelsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:handler_notif_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open notification channels db: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		db.Close()
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newNotificationChannelsTestHandler wires a NotificationChannelHandler
// against a fresh in-memory DB carrying the real server schema. SMTP-dependent handlers
// (TestChannel's "email" branch, AutoDetectSMTP) are deliberately not
// exercised here — SendTestEmail/AutoDetect perform real SMTP/network I/O,
// which this package's testing rules treat as untestable in a unit test.
func newNotificationChannelsTestHandler(t *testing.T) *NotificationChannelHandler {
	t.Helper()
	db := newNotificationChannelsTestDB(t)
	return &NotificationChannelHandler{
		DB:             db,
		ChannelManager: service.NewChannelManager(db),
		SMTP:           service.NewSMTPService(db),
	}
}

// insertTestNotificationChannel inserts a single row directly, bypassing
// InitializeChannels, so tests can control exactly which channels exist. The
// row goes into server_notification_channels, the one table the handler,
// service.ChannelManager and smtp.go all read. Timestamps are written through
// dbtime so the fixture holds the canonical UTC layout production writes rather
// than the driver's local-zone rendering of a bound time.Time.
func insertTestNotificationChannel(t *testing.T, db *sql.DB, channelType, channelName string, enabled bool, state string) {
	t.Helper()
	now := dbtime.FormatSQLTimestamp(time.Now())
	_, err := db.Exec(`
		INSERT INTO server_notification_channels
		(channel_type, channel_name, enabled, state, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, '{}', ?, ?)
	`, channelType, channelName, enabled, state, now, now)
	if err != nil {
		t.Fatalf("insert test channel: %v", err)
	}
}

func TestNotificationChannelHandlerListChannels(t *testing.T) {
	t.Run("empty table returns empty list", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels", "")

		h.ListChannels(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":0`) {
			t.Errorf("expected total:0, got: %s", w.Body.String())
		}
	})

	t.Run("returns inserted channels sorted by name", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		insertTestNotificationChannel(t, h.DB, "slack", "Slack", true, "enabled")
		insertTestNotificationChannel(t, h.DB, "discord", "Discord", false, "disabled")

		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels", "")
		h.ListChannels(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"total":2`) {
			t.Errorf("expected total:2, got: %s", body)
		}
		if strings.Index(body, "Discord") > strings.Index(body, "Slack") {
			t.Errorf("expected Discord (alphabetically first) before Slack, got: %s", body)
		}
	})
}

func TestNotificationChannelHandlerGetChannel(t *testing.T) {
	t.Run("unknown channel returns 404", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels/slack", "")
		r = withURLParam(r, "type", "slack")

		h.GetChannel(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})

	t.Run("existing channel returns its fields", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		insertTestNotificationChannel(t, h.DB, "slack", "Slack", true, "enabled")

		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels/slack", "")
		r = withURLParam(r, "type", "slack")
		h.GetChannel(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"channel_name":"Slack"`) {
			t.Errorf("expected channel_name Slack, got: %s", w.Body.String())
		}
	})
}

func TestNotificationChannelHandlerUpdateChannel(t *testing.T) {
	t.Run("malformed body returns 400", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodPut, "/server/admin/config/channels/slack", "not json")
		r = withURLParam(r, "type", "slack")

		h.UpdateChannel(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid body updates enabled and config", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		insertTestNotificationChannel(t, h.DB, "slack", "Slack", false, "disabled")

		r, w := newAPIRequest(t, http.MethodPut, "/server/admin/config/channels/slack", map[string]interface{}{
			"enabled": true,
			"config":  map[string]interface{}{"webhook_url": "https://example.com/hook"},
		})
		r = withURLParam(r, "type", "slack")

		h.UpdateChannel(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}

		var enabled bool
		var config string
		if err := h.DB.QueryRow("SELECT enabled, config FROM server_notification_channels WHERE channel_type = ?", "slack").Scan(&enabled, &config); err != nil {
			t.Fatalf("query updated row: %v", err)
		}
		if !enabled {
			t.Errorf("expected enabled = true after update")
		}
		if !strings.Contains(config, "webhook_url") {
			t.Errorf("expected config to contain webhook_url, got: %s", config)
		}
	})
}

func TestNotificationChannelHandlerEnableDisableChannel(t *testing.T) {
	h := newNotificationChannelsTestHandler(t)
	insertTestNotificationChannel(t, h.DB, "discord", "Discord", false, "disabled")

	r, w := newAPIRequest(t, http.MethodPost, "/server/admin/config/channels/discord/enable", "")
	r = withURLParam(r, "type", "discord")
	h.EnableChannel(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var state string
	var enabled bool
	if err := h.DB.QueryRow("SELECT enabled, state FROM server_notification_channels WHERE channel_type = ?", "discord").Scan(&enabled, &state); err != nil {
		t.Fatalf("query after enable: %v", err)
	}
	if !enabled || state != "enabled" {
		t.Errorf("after EnableChannel: enabled=%v state=%q, want true/enabled", enabled, state)
	}

	r2, w2 := newAPIRequest(t, http.MethodPost, "/server/admin/config/channels/discord/disable", "")
	r2 = withURLParam(r2, "type", "discord")
	h.DisableChannel(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200: %s", w2.Code, w2.Body.String())
	}

	if err := h.DB.QueryRow("SELECT enabled, state FROM server_notification_channels WHERE channel_type = ?", "discord").Scan(&enabled, &state); err != nil {
		t.Fatalf("query after disable: %v", err)
	}
	if enabled || state != "disabled" {
		t.Errorf("after DisableChannel: enabled=%v state=%q, want false/disabled", enabled, state)
	}
}

func TestNotificationChannelHandlerTestChannel(t *testing.T) {
	t.Run("missing recipient returns 400", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodPost, "/server/admin/config/channels/slack/test", map[string]interface{}{})
		r = withURLParam(r, "type", "slack")

		h.TestChannel(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unregistered channel type returns 500", func(t *testing.T) {
		// No Go NotificationChannel implementation is ever registered for
		// "slack" in this test (only "email" is registered in production,
		// via main.go), so ChannelManager.GetChannel always fails here.
		h := newNotificationChannelsTestHandler(t)
		insertTestNotificationChannel(t, h.DB, "slack", "Slack", true, "enabled")

		r, w := newAPIRequest(t, http.MethodPost, "/server/admin/config/channels/slack/test", map[string]interface{}{
			"recipient": "someone",
		})
		r = withURLParam(r, "type", "slack")

		h.TestChannel(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationChannelHandlerGetChannelStats(t *testing.T) {
	t.Run("unknown channel returns 404", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels/slack/stats", "")
		r = withURLParam(r, "type", "slack")

		h.GetChannelStats(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	})

	t.Run("existing channel returns stats", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		insertTestNotificationChannel(t, h.DB, "slack", "Slack", true, "enabled")

		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels/slack/stats", "")
		r = withURLParam(r, "type", "slack")
		h.GetChannelStats(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationChannelHandlerListSMTPProviders(t *testing.T) {
	t.Run("no category returns all providers", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/smtp/providers", "")

		h.ListSMTPProviders(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"grouped"`) {
			t.Errorf("expected grouped key in response, got: %s", w.Body.String())
		}
	})

	t.Run("filters by category", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/smtp/providers?category=transactional", "")

		h.ListSMTPProviders(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		all := service.ListProvidersByCategory("transactional")
		if !strings.Contains(w.Body.String(), fmt.Sprintf(`"total":%d`, len(all))) {
			t.Errorf("expected total %d for category filter, got: %s", len(all), w.Body.String())
		}
	})
}

func TestNotificationChannelHandlerInitializeChannels(t *testing.T) {
	h := newNotificationChannelsTestHandler(t)
	r, w := newAPIRequest(t, http.MethodPost, "/server/admin/config/channels/initialize", "")

	h.InitializeChannels(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var count int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM server_notification_channels").Scan(&count); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if count != len(service.ChannelRegistry) {
		t.Errorf("expected %d channels initialized, got %d", len(service.ChannelRegistry), count)
	}

	// Calling again must not duplicate rows (existence check short-circuits).
	r2, w2 := newAPIRequest(t, http.MethodPost, "/server/admin/config/channels/initialize", "")
	h.InitializeChannels(w2, r2)
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM server_notification_channels").Scan(&count); err != nil {
		t.Fatalf("count channels after re-init: %v", err)
	}
	if count != len(service.ChannelRegistry) {
		t.Errorf("expected re-initialize to be a no-op, still want %d, got %d", len(service.ChannelRegistry), count)
	}
}

func TestNotificationChannelHandlerGetChannelDefinitions(t *testing.T) {
	t.Run("no category returns all definitions", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels/definitions", "")

		h.GetChannelDefinitions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), fmt.Sprintf(`"total":%d`, len(service.ChannelRegistry))) {
			t.Errorf("expected total %d, got: %s", len(service.ChannelRegistry), w.Body.String())
		}
	})

	t.Run("filters by category", func(t *testing.T) {
		h := newNotificationChannelsTestHandler(t)
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/config/channels/definitions?category=sms", "")

		h.GetChannelDefinitions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"twilio"`) {
			t.Errorf("expected sms category to include twilio, got: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), `"slack"`) {
			t.Errorf("expected sms category filter to exclude slack, got: %s", w.Body.String())
		}
	})
}

func TestNotificationChannelHandlerGetQueueStats(t *testing.T) {
	h := newNotificationChannelsTestHandler(t)

	_, err := h.DB.Exec(`
		INSERT INTO notification_queue
		(user_id, channel_type, subject, body, state, priority, created_at, updated_at)
		VALUES (1, 'email', 'sub', 'body', 'delivered', 5, ?, ?)
	`, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("insert queue row: %v", err)
	}
	_, err = h.DB.Exec(`
		INSERT INTO notification_queue
		(user_id, channel_type, subject, body, state, priority, created_at, updated_at)
		VALUES (1, 'slack', 'sub', 'body', 'failed', 5, ?, ?)
	`, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("insert queue row: %v", err)
	}

	r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/queue/stats", "")
	h.GetQueueStats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"total":2`) {
		t.Errorf("expected total:2, got: %s", body)
	}
	if !strings.Contains(body, `"delivered":1`) {
		t.Errorf("expected delivered:1, got: %s", body)
	}
	if !strings.Contains(body, `"failed":1`) {
		t.Errorf("expected failed:1, got: %s", body)
	}
}

func TestNotificationChannelHandlerGetNotificationHistory(t *testing.T) {
	h := newNotificationChannelsTestHandler(t)

	_, err := h.DB.Exec(`
		INSERT INTO notification_history
		(queue_id, user_id, channel_type, status, subject, created_at)
		VALUES (NULL, 1, 'email', 'sent', 'hello', ?)
	`, time.Now())
	if err != nil {
		t.Fatalf("insert history row: %v", err)
	}
	_, err = h.DB.Exec(`
		INSERT INTO notification_history
		(queue_id, user_id, channel_type, status, subject, created_at)
		VALUES (NULL, 1, 'slack', 'failed', 'oops', ?)
	`, time.Now())
	if err != nil {
		t.Fatalf("insert history row: %v", err)
	}

	t.Run("no filters returns all", func(t *testing.T) {
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/history", "")
		h.GetNotificationHistory(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":2`) {
			t.Errorf("expected total:2, got: %s", w.Body.String())
		}
	})

	t.Run("filters by channel", func(t *testing.T) {
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/history?channel=slack", "")
		h.GetNotificationHistory(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"total":1`) {
			t.Errorf("expected total:1 for channel filter, got: %s", body)
		}
		if !strings.Contains(body, `"channel_type":"slack"`) {
			t.Errorf("expected slack entry, got: %s", body)
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/history?status=failed", "")
		h.GetNotificationHistory(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":1`) {
			t.Errorf("expected total:1 for status filter, got: %s", w.Body.String())
		}
	})

	t.Run("respects limit query param", func(t *testing.T) {
		r, w := newAPIRequest(t, http.MethodGet, "/server/admin/notifications/history?limit=1", "")
		h.GetNotificationHistory(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"total":1`) {
			t.Errorf("expected total:1 for limit=1, got: %s", w.Body.String())
		}
	})
}
