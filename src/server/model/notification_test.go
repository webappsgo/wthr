package models

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// dbCounter ensures unique database names when a test calls setupTestDB multiple times
var dbCounter int64

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *sql.DB {
	// Using file:NAME?mode=memory&cache=shared ensures all connections share the same in-memory database
	// This is required because sql.DB uses connection pooling, and with plain :memory:
	// each connection would get its own separate database
	// We use unique names per test + counter to ensure test isolation even when called multiple times
	testName := t.Name()
	counter := atomic.AddInt64(&dbCounter, 1)
	dbName := fmt.Sprintf("file:%s_model_%d?mode=memory&cache=shared", testName, counter)
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create user_notifications table
	_, err = db.Exec(`
		CREATE TABLE user_notifications (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL CHECK(type IN ('success', 'info', 'warning', 'error', 'security')),
			display TEXT NOT NULL CHECK(display IN ('toast', 'banner', 'center')) DEFAULT 'toast',
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			action_json TEXT,
			read BOOLEAN DEFAULT 0,
			dismissed BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create user_notifications table: %v", err)
	}

	// Create server_admin_notifications table
	_, err = db.Exec(`
		CREATE TABLE server_admin_notifications (
			id TEXT PRIMARY KEY,
			admin_id INTEGER NOT NULL,
			type TEXT NOT NULL CHECK(type IN ('success', 'info', 'warning', 'error', 'security')),
			display TEXT NOT NULL CHECK(display IN ('toast', 'banner', 'center')) DEFAULT 'toast',
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			action_json TEXT,
			read BOOLEAN DEFAULT 0,
			dismissed BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create server_admin_notifications table: %v", err)
	}

	// Create user_notification_preferences table
	_, err = db.Exec(`
		CREATE TABLE user_notification_preferences (
			user_id INTEGER PRIMARY KEY,
			enable_toast BOOLEAN DEFAULT 1,
			enable_banner BOOLEAN DEFAULT 1,
			enable_center BOOLEAN DEFAULT 1,
			enable_sound BOOLEAN DEFAULT 0,
			toast_duration_success INTEGER DEFAULT 5,
			toast_duration_info INTEGER DEFAULT 5,
			toast_duration_warning INTEGER DEFAULT 10,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create user_notification_preferences table: %v", err)
	}

	// Create server_admin_notification_preferences table
	_, err = db.Exec(`
		CREATE TABLE server_admin_notification_preferences (
			admin_id INTEGER PRIMARY KEY,
			enable_toast BOOLEAN DEFAULT 1,
			enable_banner BOOLEAN DEFAULT 1,
			enable_center BOOLEAN DEFAULT 1,
			enable_sound BOOLEAN DEFAULT 0,
			toast_duration_success INTEGER DEFAULT 5,
			toast_duration_info INTEGER DEFAULT 5,
			toast_duration_warning INTEGER DEFAULT 10,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create server_admin_notification_preferences table: %v", err)
	}

	return db
}

func TestUserNotificationModel_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	tests := []struct {
		name      string
		userID    int
		notifType NotificationType
		display   NotificationDisplay
		title     string
		message   string
		action    *NotificationAction
		wantErr   bool
	}{
		{
			name:      "Create success notification",
			userID:    1,
			notifType: NotificationTypeSuccess,
			display:   NotificationDisplayToast,
			title:     "Test Success",
			message:   "This is a test success notification",
			action:    nil,
			wantErr:   false,
		},
		{
			name:      "Create warning notification with action",
			userID:    1,
			notifType: NotificationTypeWarning,
			display:   NotificationDisplayBanner,
			title:     "Test Warning",
			message:   "This is a test warning notification",
			action:    &NotificationAction{Label: "View Details", URL: "/details"},
			wantErr:   false,
		},
		{
			name:      "Create error notification",
			userID:    2,
			notifType: NotificationTypeError,
			display:   NotificationDisplayCenter,
			title:     "Test Error",
			message:   "This is a test error notification",
			action:    nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notif, err := model.Create(tt.userID, tt.notifType, tt.display, tt.title, tt.message, tt.action)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if notif == nil {
					t.Fatal("Create() returned nil notification")
				}
				if notif.ID == "" {
					t.Error("Create() notification ID is empty")
				}
				if notif.UserID == nil || *notif.UserID != tt.userID {
					t.Errorf("Create() userID = %v, want %v", notif.UserID, tt.userID)
				}
				if notif.Type != tt.notifType {
					t.Errorf("Create() type = %v, want %v", notif.Type, tt.notifType)
				}
				if notif.Display != tt.display {
					t.Errorf("Create() display = %v, want %v", notif.Display, tt.display)
				}
				if notif.Title != tt.title {
					t.Errorf("Create() title = %v, want %v", notif.Title, tt.title)
				}
				if notif.Message != tt.message {
					t.Errorf("Create() message = %v, want %v", notif.Message, tt.message)
				}
				if notif.Read {
					t.Error("Create() notification should be unread by default")
				}
				if notif.Dismissed {
					t.Error("Create() notification should not be dismissed by default")
				}
			}
		})
	}
}

func TestUserNotificationModel_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	// Create a test notification
	created, err := model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test", "Test message", nil)
	if err != nil {
		t.Fatalf("Failed to create test notification: %v", err)
	}

	tests := []struct {
		name    string
		id      string
		userID  int
		wantErr bool
	}{
		{
			name:    "Get existing notification",
			id:      created.ID,
			userID:  1,
			wantErr: false,
		},
		{
			name:    "Get non-existent notification",
			id:      "non-existent-id",
			userID:  1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notif, err := model.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if notif == nil {
					t.Fatal("GetByID() returned nil notification")
				}
				if notif.ID != tt.id {
					t.Errorf("GetByID() id = %v, want %v", notif.ID, tt.id)
				}
				if notif.UserID != nil && *notif.UserID != tt.userID {
					t.Errorf("GetByID() userID = %v, want %v", *notif.UserID, tt.userID)
				}
			}
		})
	}
}

func TestUserNotificationModel_MarkAsRead(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	// Create a test notification
	created, err := model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test", "Test message", nil)
	if err != nil {
		t.Fatalf("Failed to create test notification: %v", err)
	}

	// Mark as read
	err = model.MarkAsRead(created.ID, 1)
	if err != nil {
		t.Fatalf("MarkAsRead() error = %v", err)
	}

	// Verify it's marked as read
	notif, err := model.GetByID(created.ID)
	if err != nil {
		t.Fatalf("Failed to get notification: %v", err)
	}

	if !notif.Read {
		t.Error("MarkAsRead() notification should be marked as read")
	}
}

func TestUserNotificationModel_GetUnreadCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	// Create multiple notifications
	_, _ = model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test 1", "Message 1", nil)
	_, _ = model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test 2", "Message 2", nil)
	created3, _ := model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test 3", "Message 3", nil)

	// Mark one as read
	_ = model.MarkAsRead(created3.ID, 1)

	// Get unread count
	count, err := model.GetUnreadCount(1)
	if err != nil {
		t.Fatalf("GetUnreadCount() error = %v", err)
	}

	if count != 2 {
		t.Errorf("GetUnreadCount() = %v, want 2", count)
	}
}

func TestUserNotificationModel_CleanupExpired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	// Create a notification and manually set it as expired
	_, err := model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test", "Test message", nil)
	if err != nil {
		t.Fatalf("Failed to create test notification: %v", err)
	}

	// Update to make it expired (30+ days old)
	expiredDate := time.Now().AddDate(0, 0, -31)
	_, err = db.Exec(`UPDATE user_notifications SET expires_at = ?`, expiredDate)
	if err != nil {
		t.Fatalf("Failed to update expiration: %v", err)
	}

	// Run cleanup
	deleted, err := model.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("CleanupExpired() deleted = %v, want 1", deleted)
	}

	// Verify notification was deleted
	count, _ := model.GetUnreadCount(1)
	if count != 0 {
		t.Errorf("After cleanup, count should be 0, got %v", count)
	}
}

func TestUserNotificationModel_EnforceLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	// Create 5 notifications
	for i := 1; i <= 5; i++ {
		_, err := model.Create(1, NotificationTypeInfo, NotificationDisplayToast, "Test", "Test message", nil)
		if err != nil {
			t.Fatalf("Failed to create notification %d: %v", i, err)
		}
		// Ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Enforce limit of 3
	deleted, err := model.EnforceLimit(1, 3)
	if err != nil {
		t.Fatalf("EnforceLimit() error = %v", err)
	}

	if deleted != 2 {
		t.Errorf("EnforceLimit() deleted = %v, want 2", deleted)
	}

	// Verify only 3 notifications remain
	notifications, err := model.GetByUserID(1, 100, 0)
	if err != nil {
		t.Fatalf("Failed to get notifications: %v", err)
	}

	if len(notifications) != 3 {
		t.Errorf("After EnforceLimit, count = %v, want 3", len(notifications))
	}
}

func TestNotificationPreferencesModel_GetUserPreferences(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	serverDB := setupTestDB(t)
	defer serverDB.Close()

	model := &NotificationPreferencesModel{UserDB: db, ServerDB: serverDB}

	// Get default preferences (no record exists)
	prefs, err := model.GetUserPreferences(1)
	if err != nil {
		t.Fatalf("GetUserPreferences() error = %v", err)
	}

	// Check defaults
	if !prefs.EnableToast {
		t.Error("Default EnableToast should be true")
	}
	if !prefs.EnableBanner {
		t.Error("Default EnableBanner should be true")
	}
	if !prefs.EnableCenter {
		t.Error("Default EnableCenter should be true")
	}
	if prefs.EnableSound {
		t.Error("Default EnableSound should be false")
	}
	if prefs.ToastDurationSuccess != 5 {
		t.Errorf("Default ToastDurationSuccess = %v, want 5", prefs.ToastDurationSuccess)
	}
	if prefs.ToastDurationInfo != 5 {
		t.Errorf("Default ToastDurationInfo = %v, want 5", prefs.ToastDurationInfo)
	}
	if prefs.ToastDurationWarning != 10 {
		t.Errorf("Default ToastDurationWarning = %v, want 10", prefs.ToastDurationWarning)
	}
}

func TestNotificationPreferencesModel_UpdateUserPreferences(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	serverDB := setupTestDB(t)
	defer serverDB.Close()

	model := &NotificationPreferencesModel{UserDB: db, ServerDB: serverDB}

	// Update preferences
	newPrefs := &NotificationPreferences{
		EnableToast:          false,
		EnableBanner:         true,
		EnableCenter:         true,
		EnableSound:          true,
		ToastDurationSuccess: 3,
		ToastDurationInfo:    7,
		ToastDurationWarning: 15,
	}

	err := model.UpdateUserPreferences(1, newPrefs)
	if err != nil {
		t.Fatalf("UpdateUserPreferences() error = %v", err)
	}

	// Get updated preferences
	prefs, err := model.GetUserPreferences(1)
	if err != nil {
		t.Fatalf("GetUserPreferences() error = %v", err)
	}

	if prefs.EnableToast {
		t.Error("EnableToast should be false after update")
	}
	if !prefs.EnableSound {
		t.Error("EnableSound should be true after update")
	}
	if prefs.ToastDurationSuccess != 3 {
		t.Errorf("ToastDurationSuccess = %v, want 3", prefs.ToastDurationSuccess)
	}
}

func TestAdminNotificationModel_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &AdminNotificationModel{DB: db}

	notif, err := model.Create(1, NotificationTypeSuccess, NotificationDisplayToast, "Admin Test", "Admin message", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if notif == nil {
		t.Fatal("Create() returned nil notification")
	}
	if notif.ID == "" {
		t.Error("Create() notification ID is empty")
	}
	if notif.AdminID == nil || *notif.AdminID != 1 {
		t.Errorf("Create() adminID = %v, want 1", notif.AdminID)
	}
}

func TestUserNotificationModel_GetStatistics(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}

	// Create notifications of different types
	_, _ = model.Create(1, NotificationTypeSuccess, NotificationDisplayToast, "Success", "Message", nil)
	_, _ = model.Create(1, NotificationTypeInfo, NotificationDisplayBanner, "Info", "Message", nil)
	created3, _ := model.Create(1, NotificationTypeWarning, NotificationDisplayCenter, "Warning", "Message", nil)

	// Mark one as read
	_ = model.MarkAsRead(created3.ID, 1)

	// Get statistics
	stats, err := model.GetStatistics(1)
	if err != nil {
		t.Fatalf("GetStatistics() error = %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %v, want 3", stats.Total)
	}
	if stats.Unread != 2 {
		t.Errorf("Unread = %v, want 2", stats.Unread)
	}
	if stats.Read != 1 {
		t.Errorf("Read = %v, want 1", stats.Read)
	}
	if stats.ByType["success"] != 1 {
		t.Errorf("ByType[success] = %v, want 1", stats.ByType["success"])
	}
	if stats.ByType["info"] != 1 {
		t.Errorf("ByType[info] = %v, want 1", stats.ByType["info"])
	}
	if stats.ByType["warning"] != 1 {
		t.Errorf("ByType[warning] = %v, want 1", stats.ByType["warning"])
	}
	if stats.ByDisplay["toast"] != 1 {
		t.Errorf("ByDisplay[toast] = %v, want 1", stats.ByDisplay["toast"])
	}
	if stats.ByDisplay["banner"] != 1 {
		t.Errorf("ByDisplay[banner] = %v, want 1", stats.ByDisplay["banner"])
	}
	if stats.ByDisplay["center"] != 1 {
		t.Errorf("ByDisplay[center] = %v, want 1", stats.ByDisplay["center"])
	}
}
