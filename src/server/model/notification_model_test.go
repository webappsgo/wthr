package models

import (
	"testing"
	"time"
)

// TestNotificationModel_GetUnreadCount covers the distinct NotificationModel
// type (separate from UserNotificationModel), including the
// read/dismissed/expired exclusion filters and the NULL-expiry case.
func TestNotificationModel_GetUnreadCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	model := &NotificationModel{DB: db}

	insert := func(id string, read, dismissed bool, expiresAt interface{}) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO user_notifications (id, user_id, type, display, title, message, read, dismissed, expires_at)
			 VALUES (?, 1, 'info', 'toast', 't', 'm', ?, ?, ?)`,
			id, read, dismissed, expiresAt,
		); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	t.Run("zero on empty table", func(t *testing.T) {
		count, err := model.GetUnreadCount(1)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUnreadCount() = %d, want 0", count)
		}
	})

	insert("unread-future", false, false, time.Now().Add(24*time.Hour))
	insert("unread-null-expiry", false, false, nil)
	insert("already-read", true, false, time.Now().Add(24*time.Hour))
	insert("dismissed", false, true, time.Now().Add(24*time.Hour))
	insert("expired", false, false, time.Now().Add(-24*time.Hour))

	t.Run("counts only unread, non-dismissed, non-expired", func(t *testing.T) {
		count, err := model.GetUnreadCount(1)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 2 {
			t.Errorf("GetUnreadCount() = %d, want 2", count)
		}
	})

	t.Run("scoped to requested user only", func(t *testing.T) {
		count, err := model.GetUnreadCount(2)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUnreadCount() for other user = %d, want 0", count)
		}
	})
}
