package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// newNotificationAPIHandlers wires a real NotificationService against
// in-memory databases carrying the actual production schemas, since
// (unlike notifications.go/notification_preferences.go) notification_api.go
// goes through the service/model layer whose SQL matches ServerSchema and
// UsersSchema exactly.
func newNotificationAPIHandlers(t *testing.T) *NotificationAPIHandlers {
	t.Helper()
	usersDB := newTestUsersDB(t)
	serverDB := newTestServerDB(t)
	svc := service.NewNotificationService(usersDB, serverDB, service.NewWebSocketHub())
	return NewNotificationAPIHandlers(svc, nil)
}

// setUserIDContext / setAdminIDContext mirror exactly what notification_api.go
// reads (c.Get("user_id") / c.Get("admin_id")) so the success paths are
// reachable; see the context-key-mismatch subtest below for what real
// middleware actually sets.
func setUserIDContext(c *gin.Context, id int) {
	c.Set("user_id", id)
}

func setAdminIDContext(c *gin.Context, id int) {
	c.Set("admin_id", id)
}

// GetUserNotifications covers the success path (service call reaches the
// real user_notifications table with no rows) and the auth-required path.
func TestNotificationAPIHandlers_GetUserNotifications(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("success with no notifications yet", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications", "")
		setUserIDContext(c, 1)

		h.GetUserNotifications(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"count":0`) {
			t.Errorf("body = %s, want count:0", w.Body.String())
		}
	})

	t.Run("missing user_id context returns 401", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications", "")

		h.GetUserNotifications(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// This directly demonstrates BUG #7: real auth middleware (see
// src/server/middleware/auth.go) stores the authenticated user under
// middleware.UserContextKey ("user"), and never calls
// c.Set("user_id", ...) anywhere in production code. A genuinely
// authenticated request therefore still gets 401 from every user-facing
// notification API endpoint.
func TestNotificationAPIHandlers_GetUserNotifications_RealMiddlewareContextKeyMismatch(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications", "")

	// Mirrors what real auth middleware actually does: sets "user" to a
	// value, never "user_id" to an int.
	c.Set("user", struct{ ID int }{ID: 1})

	h.GetUserNotifications(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (BUG #7: handler reads \"user_id\" but middleware sets \"user\"); body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_GetUserUnreadNotifications(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("success", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications/unread", "")
		setUserIDContext(c, 1)

		h.GetUserUnreadNotifications(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing auth returns 401", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications/unread", "")

		h.GetUserUnreadNotifications(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationAPIHandlers_GetUserUnreadCount(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications/count", "")
	setUserIDContext(c, 1)

	h.GetUserUnreadCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"count":0`) {
		t.Errorf("body = %s, want count:0", w.Body.String())
	}
}

func TestNotificationAPIHandlers_GetUserNotificationStats(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications/stats", "")
	setUserIDContext(c, 1)

	h.GetUserNotificationStats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// MarkUserNotificationRead exercises both the not-found path (empty DB) and
// the missing-auth path.
func TestNotificationAPIHandlers_MarkUserNotificationRead(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("unknown id returns 404", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/does-not-exist/read", "")
		setUserIDContext(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "does-not-exist"}}

		h.MarkUserNotificationRead(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing auth returns 401 before touching id", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/x/read", "")
		c.Params = gin.Params{{Key: "id", Value: "x"}}

		h.MarkUserNotificationRead(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationAPIHandlers_MarkAllUserNotificationsRead(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/read", "")
	setUserIDContext(c, 1)

	h.MarkAllUserNotificationsRead(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_DismissUserNotification(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/x/dismiss", "")
	setUserIDContext(c, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}

	h.DismissUserNotification(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown notification; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_DeleteUserNotification(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/user/notifications/x", "")
	setUserIDContext(c, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}

	h.DeleteUserNotification(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown notification; body=%s", w.Code, w.Body.String())
	}
}

// GetUserNotificationPreferences returns library defaults when no row
// exists yet; UpdateUserNotificationPreferences round-trips them, including
// the toast-duration validation boundaries.
func TestNotificationAPIHandlers_UserPreferences(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("get returns defaults with no row present", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications/preferences", "")
		setUserIDContext(c, 1)

		h.GetUserNotificationPreferences(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"enable_toast":true`) {
			t.Errorf("body = %s, want default enable_toast:true", w.Body.String())
		}
	})

	t.Run("update success", func(t *testing.T) {
		body := model.NotificationPreferences{
			EnableToast:          true,
			EnableBanner:         false,
			EnableCenter:         true,
			EnableSound:          true,
			ToastDurationSuccess: 5,
			ToastDurationInfo:    5,
			ToastDurationWarning: 10,
		}
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/preferences", body)
		setUserIDContext(c, 1)

		h.UpdateUserNotificationPreferences(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects toast duration outside 1-60", func(t *testing.T) {
		body := model.NotificationPreferences{
			ToastDurationSuccess: 999,
			ToastDurationInfo:    5,
			ToastDurationWarning: 10,
		}
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/preferences", body)
		setUserIDContext(c, 1)

		h.UpdateUserNotificationPreferences(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects malformed JSON body", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/user/notifications/preferences", "{not-json")
		setUserIDContext(c, 1)

		h.UpdateUserNotificationPreferences(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// Admin-side endpoints use "admin_id", which real admin_auth middleware does
// set (unlike "user_id"), so these are both a functional-coverage pass and a
// contrast case showing the admin surface is NOT affected by BUG #7.
func TestNotificationAPIHandlers_GetAdminNotifications(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("success with no notifications yet", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/admin/admin/notifications", "")
		setAdminIDContext(c, 1)

		h.GetAdminNotifications(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing admin_id returns 401", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/admin/admin/notifications", "")

		h.GetAdminNotifications(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestNotificationAPIHandlers_GetAdminUnreadCount(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/admin/admin/notifications/count", "")
	setAdminIDContext(c, 1)

	h.GetAdminUnreadCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_GetAdminNotificationStats(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/admin/admin/notifications/stats", "")
	setAdminIDContext(c, 1)

	h.GetAdminNotificationStats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_MarkAdminNotificationRead(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/admin/notifications/x/read", "")
	setAdminIDContext(c, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}

	h.MarkAdminNotificationRead(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown notification; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_MarkAllAdminNotificationsRead(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/admin/notifications/read", "")
	setAdminIDContext(c, 1)

	h.MarkAllAdminNotificationsRead(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_DismissAdminNotification(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/admin/notifications/x/dismiss", "")
	setAdminIDContext(c, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}

	h.DismissAdminNotification(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown notification; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_DeleteAdminNotification(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/server/admin/admin/notifications/x", "")
	setAdminIDContext(c, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}

	h.DeleteAdminNotification(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown notification; body=%s", w.Code, w.Body.String())
	}
}

func TestNotificationAPIHandlers_AdminPreferences(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("get returns defaults", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/admin/admin/notifications/preferences", "")
		setAdminIDContext(c, 1)

		h.GetAdminNotificationPreferences(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update success", func(t *testing.T) {
		body := model.NotificationPreferences{
			EnableToast:          true,
			EnableBanner:         true,
			EnableCenter:         true,
			ToastDurationSuccess: 5,
			ToastDurationInfo:    5,
			ToastDurationWarning: 10,
		}
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/admin/notifications/preferences", body)
		setAdminIDContext(c, 1)

		h.UpdateAdminNotificationPreferences(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects out-of-range toast duration", func(t *testing.T) {
		body := model.NotificationPreferences{
			ToastDurationSuccess: 0,
			ToastDurationInfo:    5,
			ToastDurationWarning: 10,
		}
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/admin/notifications/preferences", body)
		setAdminIDContext(c, 1)

		h.UpdateAdminNotificationPreferences(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// SendTestNotification validates the request body and the two enum fields
// before reaching the service layer.
func TestNotificationAPIHandlers_SendTestNotification(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("success", func(t *testing.T) {
		body := map[string]interface{}{
			"type":    "info",
			"display": "toast",
			"title":   "Hello",
			"message": "World",
		}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/admin/notifications/send", body)
		setAdminIDContext(c, 1)

		h.SendTestNotification(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing auth returns 401", func(t *testing.T) {
		body := map[string]interface{}{
			"type": "info", "display": "toast", "title": "t", "message": "m",
		}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/admin/notifications/send", body)

		h.SendTestNotification(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing required field returns 400", func(t *testing.T) {
		body := map[string]interface{}{
			"type": "info", "display": "toast", "message": "m",
		}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/admin/notifications/send", body)
		setAdminIDContext(c, 1)

		h.SendTestNotification(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid notification type returns 400", func(t *testing.T) {
		body := map[string]interface{}{
			"type": "bogus", "display": "toast", "title": "t", "message": "m",
		}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/admin/notifications/send", body)
		setAdminIDContext(c, 1)

		h.SendTestNotification(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid display type returns 400", func(t *testing.T) {
		body := map[string]interface{}{
			"type": "info", "display": "bogus", "title": "t", "message": "m",
		}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/admin/notifications/send", body)
		setAdminIDContext(c, 1)

		h.SendTestNotification(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// HandleWebSocketConnection requires auth before attempting the protocol
// upgrade; this is reachable without a real WS client since the auth check
// runs first and short-circuits.
func TestNotificationAPIHandlers_HandleWebSocketConnection_Unauthorized(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	c, w := newTestContextJSON(t, http.MethodGet, "/ws/notifications", "")

	h.HandleWebSocketConnection(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
}

// Method aliases must delegate to the same underlying handler and behavior.
func TestNotificationAPIHandlers_Aliases(t *testing.T) {
	h := newNotificationAPIHandlers(t)

	t.Run("GetUserStats delegates and requires auth", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/user/notifications/stats", "")
		h.GetUserStats(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("GetAdminStats delegates and requires auth", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/server/admin/admin/notifications/stats", "")
		h.GetAdminStats(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// RegisterNotificationAPIRoutes is exercised end-to-end through a real
// gin.Engine to confirm the route table wires up without panicking and
// that an unauthenticated request through the full router still yields 401.
func TestRegisterNotificationAPIRoutes(t *testing.T) {
	h := newNotificationAPIHandlers(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterNotificationAPIRoutes(router, h, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/notifications", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no user_id set on a bare router); body=%s", w.Code, w.Body.String())
	}
}
