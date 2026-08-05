package handler

import (
	"net/http"
	"testing"
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

	h.GetNotifications(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_GetUnreadNotifications_Unauthorized verifies
// the same "user_id" guard on GetUnreadNotifications.
func TestUserNotificationHandlers_GetUnreadNotifications_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/unread")

	h.GetUnreadNotifications(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_GetUnreadCount_Unauthorized verifies the
// same "user_id" guard on GetUnreadCount.
func TestUserNotificationHandlers_GetUnreadCount_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/unread/count")

	h.GetUnreadCount(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUserNotificationHandlers_MarkAllAsRead_Unauthorized verifies the same
// "user_id" guard on a write-path method (MarkAllAsRead).
func TestUserNotificationHandlers_MarkAllAsRead_Unauthorized(t *testing.T) {
	h := &UserNotificationHandlers{}
	c, w := newAPITestContext("/api/v1/users/notifications/read-all")

	h.MarkAllAsRead(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}
