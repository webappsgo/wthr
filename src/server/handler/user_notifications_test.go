package handler

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestNewUserNotificationHandlers verifies the constructor wires the
// NotificationService field as passed.
func TestNewUserNotificationHandlers(t *testing.T) {
	h := NewUserNotificationHandlers(nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.NotificationService != nil {
		t.Errorf("expected NotificationService to be nil (as passed), got %v", h.NotificationService)
	}
}

// TestUserNotificationHandlers_GetNotifications_Unauthorized verifies the
// shared "user_id" context-missing guard returns 401 before the
// (nil) NotificationService is ever consulted.
func TestUserNotificationHandlers_GetNotifications_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications")

	h.GetNotifications(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_GetUnreadNotifications_Unauthorized verifies
// the same "user_id" guard on GetUnreadNotifications.
func TestUserNotificationHandlers_GetUnreadNotifications_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/unread")

	h.GetUnreadNotifications(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_GetUnreadCount_Unauthorized verifies the
// same "user_id" guard on GetUnreadCount.
func TestUserNotificationHandlers_GetUnreadCount_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/unread/count")

	h.GetUnreadCount(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_MarkAllAsRead_Unauthorized verifies the same
// "user_id" guard on a write-path method (MarkAllAsRead).
func TestUserNotificationHandlers_MarkAllAsRead_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/read-all")

	h.MarkAllAsRead(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_GetStatistics_Unauthorized verifies the same
// "user_id" guard on GetStatistics.
func TestUserNotificationHandlers_GetStatistics_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/stats")

	h.GetStatistics(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_MarkAsRead_Unauthorized verifies the same
// "user_id" guard on MarkAsRead.
func TestUserNotificationHandlers_MarkAsRead_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/1/read")

	h.MarkAsRead(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_Dismiss_Unauthorized verifies the same
// "user_id" guard on Dismiss.
func TestUserNotificationHandlers_Dismiss_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/1/dismiss")

	h.Dismiss(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_Delete_Unauthorized verifies the same
// "user_id" guard on Delete.
func TestUserNotificationHandlers_Delete_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/1")

	h.Delete(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_GetPreferences_Unauthorized verifies the same
// "user_id" guard on GetPreferences.
func TestUserNotificationHandlers_GetPreferences_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/preferences")

	h.GetPreferences(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_UpdatePreferences_Unauthorized verifies the
// same "user_id" guard on UpdatePreferences, checked before the request body
// is ever decoded.
func TestUserNotificationHandlers_UpdatePreferences_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/preferences")

	h.UpdatePreferences(w, c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterUserNotificationRoutes verifies route registration does not
// panic and wires the "/notifications" route group onto the given router.
func TestRegisterUserNotificationRoutes(t *testing.T) {
	h := &UserNotificationHandlers{}
	r := chi.NewRouter()

	RegisterUserNotificationRoutes(r, h)

	req, w := newAPITestContext("/notifications")
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected /notifications to be registered, got 404")
	}
}
