package models

import (
	"testing"
)

// TestUserNotificationModel_GetUnreadDismissDelete covers the read-list
// filter, dismiss ownership check, delete ownership check, and MarkAllAsRead.
func TestUserNotificationModel_GetUnreadDismissDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	model := &UserNotificationModel{DB: db}

	a, err := model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "A", "msg a", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	b, err := model.Create(1, NotificationTypeWarning, NotificationDisplayBanner, "B", "msg b", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("GetUnread returns both before any action", func(t *testing.T) {
		unread, err := model.GetUnread(1)
		if err != nil {
			t.Fatalf("GetUnread() error = %v", err)
		}
		if len(unread) != 2 {
			t.Fatalf("GetUnread() = %d, want 2", len(unread))
		}
	})

	t.Run("Dismiss removes from unread list but not from storage", func(t *testing.T) {
		if err := model.Dismiss(a.ID, 1); err != nil {
			t.Fatalf("Dismiss() error = %v", err)
		}
		unread, err := model.GetUnread(1)
		if err != nil {
			t.Fatalf("GetUnread() error = %v", err)
		}
		if len(unread) != 1 {
			t.Fatalf("GetUnread() after dismiss = %d, want 1", len(unread))
		}
		if _, err := model.GetByID(a.ID); err != nil {
			t.Errorf("dismissed notification should still exist, GetByID() error = %v", err)
		}
	})

	t.Run("Dismiss wrong owner errors", func(t *testing.T) {
		if err := model.Dismiss(b.ID, 999); err == nil {
			t.Error("Dismiss() expected error for wrong owner")
		}
	})

	t.Run("Dismiss unknown id errors", func(t *testing.T) {
		if err := model.Dismiss("bogus-id", 1); err == nil {
			t.Error("Dismiss() expected error for unknown id")
		}
	})

	t.Run("MarkAllAsRead", func(t *testing.T) {
		if err := model.MarkAllAsRead(1); err != nil {
			t.Fatalf("MarkAllAsRead() error = %v", err)
		}
		count, err := model.GetUnreadCount(1)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUnreadCount() after MarkAllAsRead = %d, want 0", count)
		}
	})

	t.Run("Delete wrong owner errors", func(t *testing.T) {
		if err := model.Delete(b.ID, 999); err == nil {
			t.Error("Delete() expected error for wrong owner")
		}
	})

	t.Run("Delete happy path", func(t *testing.T) {
		if err := model.Delete(b.ID, 1); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := model.GetByID(b.ID); err == nil {
			t.Error("GetByID() expected error after Delete")
		}
	})
}

// TestAdminNotificationModel_FullLifecycle mirrors the user model's
// lifecycle coverage for the admin-scoped table, since the two
// implementations are separate code paths that could silently diverge.
func TestAdminNotificationModel_FullLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	model := &AdminNotificationModel{DB: db}

	created, err := model.Create(5, NotificationTypeError, NotificationDisplayCenter, "Admin Alert", "msg", &NotificationAction{Label: "View", URL: "/x"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Action == nil || created.Action.Label != "View" {
		t.Errorf("Create() action = %+v, want Label=View", created.Action)
	}

	t.Run("GetByID", func(t *testing.T) {
		got, err := model.GetByID(created.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.AdminID == nil || *got.AdminID != 5 {
			t.Errorf("GetByID() adminID = %v, want 5", got.AdminID)
		}
	})

	t.Run("GetByID not found", func(t *testing.T) {
		if _, err := model.GetByID("missing"); err == nil {
			t.Error("GetByID() expected error for missing id")
		}
	})

	t.Run("GetByAdminID", func(t *testing.T) {
		list, err := model.GetByAdminID(5, 10, 0)
		if err != nil {
			t.Fatalf("GetByAdminID() error = %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("GetByAdminID() = %d, want 1", len(list))
		}
	})

	t.Run("GetUnread and GetUnreadCount", func(t *testing.T) {
		unread, err := model.GetUnread(5)
		if err != nil {
			t.Fatalf("GetUnread() error = %v", err)
		}
		if len(unread) != 1 {
			t.Fatalf("GetUnread() = %d, want 1", len(unread))
		}
		count, err := model.GetUnreadCount(5)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 1 {
			t.Errorf("GetUnreadCount() = %d, want 1", count)
		}
	})

	t.Run("MarkAsRead wrong owner errors", func(t *testing.T) {
		if err := model.MarkAsRead(created.ID, 999); err == nil {
			t.Error("MarkAsRead() expected error for wrong owner")
		}
	})

	t.Run("MarkAsRead happy path", func(t *testing.T) {
		if err := model.MarkAsRead(created.ID, 5); err != nil {
			t.Fatalf("MarkAsRead() error = %v", err)
		}
		count, err := model.GetUnreadCount(5)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUnreadCount() after MarkAsRead = %d, want 0", count)
		}
	})

	second, err := model.Create(5, NotificationTypeInfo, NotificationDisplayToast, "Second", "msg", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("MarkAllAsRead", func(t *testing.T) {
		if err := model.MarkAllAsRead(5); err != nil {
			t.Fatalf("MarkAllAsRead() error = %v", err)
		}
		count, err := model.GetUnreadCount(5)
		if err != nil {
			t.Fatalf("GetUnreadCount() error = %v", err)
		}
		if count != 0 {
			t.Errorf("GetUnreadCount() after MarkAllAsRead = %d, want 0", count)
		}
	})

	t.Run("Dismiss", func(t *testing.T) {
		if err := model.Dismiss(second.ID, 5); err != nil {
			t.Fatalf("Dismiss() error = %v", err)
		}
	})

	t.Run("Dismiss wrong owner errors", func(t *testing.T) {
		if err := model.Dismiss(created.ID, 999); err == nil {
			t.Error("Dismiss() expected error for wrong owner")
		}
	})

	t.Run("Delete wrong owner errors", func(t *testing.T) {
		if err := model.Delete(created.ID, 999); err == nil {
			t.Error("Delete() expected error for wrong owner")
		}
	})

	t.Run("Delete happy path", func(t *testing.T) {
		if err := model.Delete(created.ID, 5); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := model.GetByID(created.ID); err == nil {
			t.Error("GetByID() expected error after Delete")
		}
	})

	t.Run("CleanupExpired removes nothing for non-expired rows", func(t *testing.T) {
		affected, err := model.CleanupExpired()
		if err != nil {
			t.Fatalf("CleanupExpired() error = %v", err)
		}
		if affected != 0 {
			t.Errorf("CleanupExpired() = %d, want 0 (nothing expired yet)", affected)
		}
	})

	t.Run("EnforceLimit trims to newest N", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			if _, err := model.Create(7, NotificationTypeInfo, NotificationDisplayToast, "n", "m", nil); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}
		affected, err := model.EnforceLimit(7, 2)
		if err != nil {
			t.Fatalf("EnforceLimit() error = %v", err)
		}
		if affected != 3 {
			t.Errorf("EnforceLimit() removed %d, want 3", affected)
		}
		remaining, err := model.GetByAdminID(7, 100, 0)
		if err != nil {
			t.Fatalf("GetByAdminID() error = %v", err)
		}
		if len(remaining) != 2 {
			t.Errorf("GetByAdminID() after EnforceLimit = %d, want 2", len(remaining))
		}
	})

	t.Run("GetStatistics aggregates by type and display", func(t *testing.T) {
		stats, err := model.GetStatistics(7)
		if err != nil {
			t.Fatalf("GetStatistics() error = %v", err)
		}
		if stats.Total != 2 {
			t.Errorf("GetStatistics() Total = %d, want 2", stats.Total)
		}
		if stats.Unread != 2 {
			t.Errorf("GetStatistics() Unread = %d, want 2", stats.Unread)
		}
		if stats.ByType[NotificationTypeInfo] != 2 {
			t.Errorf("GetStatistics() ByType[info] = %d, want 2", stats.ByType[NotificationTypeInfo])
		}
		if stats.ByDisplay[NotificationDisplayToast] != 2 {
			t.Errorf("GetStatistics() ByDisplay[toast] = %d, want 2", stats.ByDisplay[NotificationDisplayToast])
		}
	})
}

// TestNotificationPreferencesModel_AdminPreferences mirrors the existing
// user-preferences coverage for the admin-scoped preference storage.
func TestNotificationPreferencesModel_AdminPreferences(t *testing.T) {
	userDB := setupTestDB(t)
	defer userDB.Close()
	serverDB := setupTestDB(t)
	defer serverDB.Close()
	model := &NotificationPreferencesModel{UserDB: userDB, ServerDB: serverDB}

	t.Run("defaults when no record exists", func(t *testing.T) {
		prefs, err := model.GetAdminPreferences(1)
		if err != nil {
			t.Fatalf("GetAdminPreferences() error = %v", err)
		}
		if !prefs.EnableToast || !prefs.EnableBanner || !prefs.EnableCenter {
			t.Error("defaults should enable toast/banner/center")
		}
		if prefs.EnableSound {
			t.Error("default EnableSound should be false")
		}
	})

	t.Run("update and read back", func(t *testing.T) {
		newPrefs := &NotificationPreferences{
			EnableToast:          false,
			EnableBanner:         false,
			EnableCenter:         true,
			EnableSound:          true,
			ToastDurationSuccess: 1,
			ToastDurationInfo:    2,
			ToastDurationWarning: 3,
		}
		if err := model.UpdateAdminPreferences(1, newPrefs); err != nil {
			t.Fatalf("UpdateAdminPreferences() error = %v", err)
		}
		prefs, err := model.GetAdminPreferences(1)
		if err != nil {
			t.Fatalf("GetAdminPreferences() error = %v", err)
		}
		if prefs.EnableToast || prefs.EnableBanner {
			t.Error("EnableToast/EnableBanner should be false after update")
		}
		if !prefs.EnableSound {
			t.Error("EnableSound should be true after update")
		}
		if prefs.ToastDurationWarning != 3 {
			t.Errorf("ToastDurationWarning = %d, want 3", prefs.ToastDurationWarning)
		}
	})

	t.Run("update again upserts rather than duplicating", func(t *testing.T) {
		newPrefs := &NotificationPreferences{EnableToast: true, ToastDurationSuccess: 9}
		if err := model.UpdateAdminPreferences(1, newPrefs); err != nil {
			t.Fatalf("UpdateAdminPreferences() second call error = %v", err)
		}
		var count int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_admin_notification_preferences WHERE admin_id = ?", 1).Scan(&count); err != nil {
			t.Fatalf("query count: %v", err)
		}
		if count != 1 {
			t.Errorf("row count = %d, want 1 (upsert, not insert)", count)
		}
	})
}
