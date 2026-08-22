package model

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
)

// dbCounter ensures unique database names when a test calls setupTestDB multiple times
var dbCounter int64

// setupTestDB creates an in-memory SQLite database for testing
// Both real schema constants are applied verbatim - the same DDL the server
// runs against users.db and server.db - so these tests exercise the columns,
// CHECK constraints and defaults production actually has. Hand-rolled CREATE
// TABLE statements are deliberately absent: a fixture table that shadows a real
// one hides "no such column" failures that would only surface in production.
// The two schemas share no table name, and every statement in them is
// idempotent, so applying both to one connection is safe.
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

	if _, err := db.Exec(database.UsersSchema); err != nil {
		db.Close()
		t.Fatalf("Failed to apply UsersSchema: %v", err)
	}

	if _, err := db.Exec(database.ServerSchema); err != nil {
		db.Close()
		t.Fatalf("Failed to apply ServerSchema: %v", err)
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

// insertNotificationExpiryFixture writes one notification row with expires_at
// set to an exact piece of text, so a test can reproduce a value written by an
// older local-zone writer without depending on the host's own timezone.
func insertNotificationExpiryFixture(t *testing.T, db *sql.DB, table, ownerColumn, id string, ownerID int, expiresAt string) {
	t.Helper()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, %s, type, display, title, message, action_json, read, dismissed, created_at, expires_at)
		VALUES (?, ?, 'info', 'toast', 'Fixture', 'Fixture message', NULL, 0, 0, ?, ?)
	`, table, ownerColumn)

	if _, err := db.Exec(query, id, ownerID, sqlTimestamp(time.Now().Add(-time.Minute)), expiresAt); err != nil {
		t.Fatalf("insert %s fixture %s: %v", table, id, err)
	}
}

// notificationExpiryCase describes one stored expires_at value and whether the
// row it belongs to is still live at the instant the test runs.
type notificationExpiryCase struct {
	name      string
	expiresAt func(now time.Time) string
	wantLive  bool
}

// notificationExpiryCases covers every encoding a notifications table can hold.
// The two zone cases are the regression the SQL "expires_at > ?" comparison
// could not survive: a row one hour in the future rendered in a -11:00 zone has
// wall-clock TEXT that sorts BEFORE the UTC threshold, so the old lexicographic
// test hid a live notification, and a row one hour in the past rendered in a
// +13:00 zone sorts AFTER it, so the old test kept serving an expired one. The
// offsets are fixed rather than derived from time.Local, so the contradiction
// between text order and instant order holds on any host timezone. The
// unparseable case pins the fail-closed rule: the old text comparison ranked
// "not-a-timestamp" above any digit and returned the row.
var notificationExpiryCases = []notificationExpiryCase{
	{
		name:      "live-utc",
		expiresAt: func(now time.Time) string { return sqlTimestamp(now.Add(time.Hour)) },
		wantLive:  true,
	},
	{
		name:      "expired-utc",
		expiresAt: func(now time.Time) string { return sqlTimestamp(now.Add(-time.Hour)) },
		wantLive:  false,
	},
	{
		name: "live-west-zone-text-reads-as-past",
		expiresAt: func(now time.Time) string {
			return now.Add(time.Hour).In(adminZoneWest).Format(localLayout)
		},
		wantLive: true,
	},
	{
		name: "expired-east-zone-text-reads-as-future",
		expiresAt: func(now time.Time) string {
			return now.Add(-time.Hour).In(adminZoneEast).Format(localLayout)
		},
		wantLive: false,
	},
	{
		name:      "unparseable-fails-closed",
		expiresAt: func(now time.Time) string { return "not-a-timestamp" },
		wantLive:  false,
	},
}

// liveCount converts the case's expectation into the row count every read is
// expected to report.
func (c notificationExpiryCase) liveCount() int {
	if c.wantLive {
		return 1
	}

	return 0
}

func TestUserNotificationModelExpiryAcrossStoredLayouts(t *testing.T) {
	for _, tc := range notificationExpiryCases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()

			model := &UserNotificationModel{DB: db}
			insertNotificationExpiryFixture(t, db, "user_notifications", "user_id", "fixture-user-1", 42, tc.expiresAt(time.Now().UTC()))

			want := tc.liveCount()

			listed, err := model.GetByUserID(42, 50, 0)
			if err != nil {
				t.Fatalf("GetByUserID: %v", err)
			}
			if len(listed) != want {
				t.Errorf("GetByUserID returned %d rows, want %d", len(listed), want)
			}

			unread, err := model.GetUnread(42)
			if err != nil {
				t.Fatalf("GetUnread: %v", err)
			}
			if len(unread) != want {
				t.Errorf("GetUnread returned %d rows, want %d", len(unread), want)
			}

			count, err := model.GetUnreadCount(42)
			if err != nil {
				t.Fatalf("GetUnreadCount: %v", err)
			}
			if count != want {
				t.Errorf("GetUnreadCount = %d, want %d", count, want)
			}

			stats, err := model.GetStatistics(42)
			if err != nil {
				t.Fatalf("GetStatistics: %v", err)
			}
			if stats.Total != want {
				t.Errorf("GetStatistics Total = %d, want %d", stats.Total, want)
			}
			if stats.Unread != want {
				t.Errorf("GetStatistics Unread = %d, want %d", stats.Unread, want)
			}
			if stats.ByType[NotificationTypeInfo] != want {
				t.Errorf("GetStatistics ByType[info] = %d, want %d", stats.ByType[NotificationTypeInfo], want)
			}
			if stats.ByDisplay[NotificationDisplayToast] != want {
				t.Errorf("GetStatistics ByDisplay[toast] = %d, want %d", stats.ByDisplay[NotificationDisplayToast], want)
			}
		})
	}
}

func TestAdminNotificationModelExpiryAcrossStoredLayouts(t *testing.T) {
	for _, tc := range notificationExpiryCases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()

			model := &AdminNotificationModel{DB: db}
			insertNotificationExpiryFixture(t, db, "server_admin_notifications", "admin_id", "fixture-admin-1", 7, tc.expiresAt(time.Now().UTC()))

			want := tc.liveCount()

			listed, err := model.GetByAdminID(7, 50, 0)
			if err != nil {
				t.Fatalf("GetByAdminID: %v", err)
			}
			if len(listed) != want {
				t.Errorf("GetByAdminID returned %d rows, want %d", len(listed), want)
			}

			unread, err := model.GetUnread(7)
			if err != nil {
				t.Fatalf("GetUnread: %v", err)
			}
			if len(unread) != want {
				t.Errorf("GetUnread returned %d rows, want %d", len(unread), want)
			}

			count, err := model.GetUnreadCount(7)
			if err != nil {
				t.Fatalf("GetUnreadCount: %v", err)
			}
			if count != want {
				t.Errorf("GetUnreadCount = %d, want %d", count, want)
			}

			stats, err := model.GetStatistics(7)
			if err != nil {
				t.Fatalf("GetStatistics: %v", err)
			}
			if stats.Total != want {
				t.Errorf("GetStatistics Total = %d, want %d", stats.Total, want)
			}
			if stats.Unread != want {
				t.Errorf("GetStatistics Unread = %d, want %d", stats.Unread, want)
			}
			if stats.ByType[NotificationTypeInfo] != want {
				t.Errorf("GetStatistics ByType[info] = %d, want %d", stats.ByType[NotificationTypeInfo], want)
			}
			if stats.ByDisplay[NotificationDisplayToast] != want {
				t.Errorf("GetStatistics ByDisplay[toast] = %d, want %d", stats.ByDisplay[NotificationDisplayToast], want)
			}
		})
	}
}

// TestNotificationPaginationSkipsExpiredRowsBeforeWindowing pins the semantics
// of moving LIMIT/OFFSET out of SQL and into Go. The expired row sits between
// two live rows in created_at order, so an SQL LIMIT applied before the expiry
// test would have consumed a slot on a row the caller must never see.
func TestNotificationPaginationSkipsExpiredRowsBeforeWindowing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := &UserNotificationModel{DB: db}
	now := time.Now().UTC()

	// created_at descending is the listing order, so "newest" is inserted with
	// the latest created_at; the ids below are ordered newest to oldest.
	rows := []struct {
		id        string
		createdAt time.Time
		expiresAt string
	}{
		{"page-1", now.Add(-1 * time.Minute), sqlTimestamp(now.Add(time.Hour))},
		{"page-2", now.Add(-2 * time.Minute), sqlTimestamp(now.Add(-time.Hour))},
		{"page-3", now.Add(-3 * time.Minute), now.Add(time.Hour).In(adminZoneWest).Format(localLayout)},
		{"page-4", now.Add(-4 * time.Minute), sqlTimestamp(now.Add(time.Hour))},
	}

	for _, row := range rows {
		_, err := db.Exec(`
			INSERT INTO user_notifications (id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at)
			VALUES (?, 9, 'info', 'toast', 'Fixture', 'Fixture message', NULL, 0, 0, ?, ?)
		`, row.id, sqlTimestamp(row.createdAt), row.expiresAt)
		if err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	first, err := model.GetByUserID(9, 2, 0)
	if err != nil {
		t.Fatalf("GetByUserID first page: %v", err)
	}
	if len(first) != 2 || first[0].ID != "page-1" || first[1].ID != "page-3" {
		t.Fatalf("first page = %v, want [page-1 page-3]", notificationIDs(first))
	}

	second, err := model.GetByUserID(9, 2, 2)
	if err != nil {
		t.Fatalf("GetByUserID second page: %v", err)
	}
	if len(second) != 1 || second[0].ID != "page-4" {
		t.Fatalf("second page = %v, want [page-4]", notificationIDs(second))
	}

	none, err := model.GetByUserID(9, 0, 0)
	if err != nil {
		t.Fatalf("GetByUserID zero limit: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("zero limit returned %v, want no rows", notificationIDs(none))
	}

	all, err := model.GetByUserID(9, noNotificationLimit, 0)
	if err != nil {
		t.Fatalf("GetByUserID negative limit: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("negative limit returned %v, want all 3 live rows", notificationIDs(all))
	}
}

// notificationIDs renders a result set as its IDs for readable failures.
func notificationIDs(notifications []*Notification) []string {
	ids := make([]string, 0, len(notifications))
	for _, notif := range notifications {
		ids = append(ids, notif.ID)
	}

	return ids
}
