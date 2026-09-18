package scheduler

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/server/service"
	_ "modernc.org/sqlite"
)

// setupNotificationCleanupDBs mirrors the in-memory SQLite pattern used in
// src/server/service/notification_service_test.go: separate user/server DBs,
// explicit DDL for the tables CleanupExpired() touches.
func setupNotificationCleanupDBs(t *testing.T) (userDB, serverDB *sql.DB) {
	t.Helper()

	counter := atomic.AddInt64(&dbCounter, 1)
	name := fmt.Sprintf("file:%s_%d", t.Name(), counter)

	var err error
	userDB, err = sql.Open("sqlite", name+"_user?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open user db: %v", err)
	}
	serverDB, err = sql.Open("sqlite", name+"_server?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}

	if _, err := userDB.Exec(`CREATE TABLE user_notifications (
		id TEXT PRIMARY KEY,
		user_id INTEGER,
		type TEXT,
		display TEXT,
		title TEXT,
		message TEXT,
		action_json TEXT,
		read BOOLEAN DEFAULT 0,
		dismissed BOOLEAN DEFAULT 0,
		created_at DATETIME,
		expires_at DATETIME
	)`); err != nil {
		t.Fatalf("create user_notifications: %v", err)
	}

	if _, err := serverDB.Exec(`CREATE TABLE server_admin_notifications (
		id TEXT PRIMARY KEY,
		admin_id INTEGER,
		type TEXT,
		display TEXT,
		title TEXT,
		message TEXT,
		action_json TEXT,
		read BOOLEAN DEFAULT 0,
		dismissed BOOLEAN DEFAULT 0,
		created_at DATETIME,
		expires_at DATETIME
	)`); err != nil {
		t.Fatalf("create server_admin_notifications: %v", err)
	}

	t.Cleanup(func() {
		userDB.Close()
		serverDB.Close()
	})

	return userDB, serverDB
}

func newTestNotificationCleaner(t *testing.T) (*NotificationCleaner, *sql.DB, *sql.DB) {
	t.Helper()
	userDB, serverDB := setupNotificationCleanupDBs(t)
	svc := service.NewNotificationService(userDB, serverDB, service.NewWebSocketHub())
	return NewNotificationCleaner(svc), userDB, serverDB
}

func insertNotification(t *testing.T, db *sql.DB, table, idCol, ownerCol string, id string, owner int, expiresAt time.Time) {
	t.Helper()
	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, type, display, title, message, created_at, expires_at)
		 VALUES (?, ?, 'system', 'toast', 't', 'm', ?, ?)`,
		table, idCol, ownerCol,
	)
	if _, err := db.Exec(query, id, owner, time.Now(), expiresAt); err != nil {
		t.Fatalf("insert into %s: %v", table, err)
	}
}

func TestCleanupExpiredNotifications(t *testing.T) {
	t.Run("no notifications is a no-op success", func(t *testing.T) {
		cleaner, _, _ := newTestNotificationCleaner(t)
		if err := cleaner.CleanupExpiredNotifications(); err != nil {
			t.Fatalf("CleanupExpiredNotifications() error: %v", err)
		}
	})

	t.Run("mixed expired and active rows: only expired ones are deleted", func(t *testing.T) {
		cleaner, userDB, serverDB := newTestNotificationCleaner(t)

		expired := time.Now().Add(-1 * time.Hour)
		active := time.Now().Add(1 * time.Hour)

		insertNotification(t, userDB, "user_notifications", "id", "user_id", "u1", 1, expired)
		insertNotification(t, userDB, "user_notifications", "id", "user_id", "u2", 1, active)
		insertNotification(t, serverDB, "server_admin_notifications", "id", "admin_id", "a1", 1, expired)
		insertNotification(t, serverDB, "server_admin_notifications", "id", "admin_id", "a2", 1, active)

		if err := cleaner.CleanupExpiredNotifications(); err != nil {
			t.Fatalf("CleanupExpiredNotifications() error: %v", err)
		}

		var userRemaining, adminRemaining int
		if err := userDB.QueryRow("SELECT COUNT(*) FROM user_notifications").Scan(&userRemaining); err != nil {
			t.Fatalf("count user notifications: %v", err)
		}
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_admin_notifications").Scan(&adminRemaining); err != nil {
			t.Fatalf("count admin notifications: %v", err)
		}
		if userRemaining != 1 {
			t.Errorf("remaining user notifications = %d, want 1", userRemaining)
		}
		if adminRemaining != 1 {
			t.Errorf("remaining admin notifications = %d, want 1", adminRemaining)
		}
	})

	t.Run("expires_at exactly now is deleted (<=  boundary)", func(t *testing.T) {
		cleaner, userDB, _ := newTestNotificationCleaner(t)
		now := time.Now()
		insertNotification(t, userDB, "user_notifications", "id", "user_id", "boundary", 1, now)

		// Give the DELETE ... <= ? comparison a moment of slack so "now" at insert
		// time is guaranteed to be <= "now" at cleanup time.
		time.Sleep(5 * time.Millisecond)

		if err := cleaner.CleanupExpiredNotifications(); err != nil {
			t.Fatalf("CleanupExpiredNotifications() error: %v", err)
		}

		var remaining int
		if err := userDB.QueryRow("SELECT COUNT(*) FROM user_notifications").Scan(&remaining); err != nil {
			t.Fatalf("count: %v", err)
		}
		if remaining != 0 {
			t.Errorf("remaining = %d, want 0 (row at the expiry boundary should be deleted)", remaining)
		}
	})
}

func TestEnforceLimits(t *testing.T) {
	t.Run("no notifications is a no-op success", func(t *testing.T) {
		cleaner, _, _ := newTestNotificationCleaner(t)
		if err := cleaner.EnforceLimits(); err != nil {
			t.Errorf("EnforceLimits() = %v, want nil", err)
		}
	})

	t.Run("trims each recipient down to the retention cap", func(t *testing.T) {
		cleaner, userDB, serverDB := newTestNotificationCleaner(t)

		active := time.Now().Add(1 * time.Hour)
		overflow := maxNotificationsPerRecipient + 5
		for i := 0; i < overflow; i++ {
			insertNotification(t, userDB, "user_notifications", "id", "user_id", fmt.Sprintf("u%d", i), 1, active)
			insertNotification(t, serverDB, "server_admin_notifications", "id", "admin_id", fmt.Sprintf("a%d", i), 1, active)
		}
		// A second recipient must be trimmed independently, not skipped.
		insertNotification(t, userDB, "user_notifications", "id", "user_id", "other", 2, active)

		if err := cleaner.EnforceLimits(); err != nil {
			t.Fatalf("EnforceLimits() error: %v", err)
		}

		var user1, user2, admin1 int
		if err := userDB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id = 1").Scan(&user1); err != nil {
			t.Fatalf("count user 1: %v", err)
		}
		if err := userDB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id = 2").Scan(&user2); err != nil {
			t.Fatalf("count user 2: %v", err)
		}
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_admin_notifications WHERE admin_id = 1").Scan(&admin1); err != nil {
			t.Fatalf("count admin 1: %v", err)
		}

		if user1 != maxNotificationsPerRecipient {
			t.Errorf("user 1 notifications = %d, want %d", user1, maxNotificationsPerRecipient)
		}
		if admin1 != maxNotificationsPerRecipient {
			t.Errorf("admin 1 notifications = %d, want %d", admin1, maxNotificationsPerRecipient)
		}
		if user2 != 1 {
			t.Errorf("user 2 notifications = %d, want 1 (under the cap, must be untouched)", user2)
		}
	})
}

// --- ScheduleNotificationCleanup / ScheduleNotificationLimitEnforcement --------------
//
// Both register their job with the built-in scheduler (AI.md PART 19 allows no
// other scheduling mechanism), so the assertions cover registration under the
// expected task id rather than any goroutine/ticker behavior.

func TestScheduleNotificationCleanup_RegistersTask(t *testing.T) {
	s := NewScheduler(nil)
	cleaner, _, _ := newTestNotificationCleaner(t)

	s.ScheduleNotificationCleanup(cleaner, "02:00")

	task, ok := s.tasks[notificationCleanupTaskName]
	if !ok {
		t.Fatalf("task %q was not registered with the scheduler", notificationCleanupTaskName)
	}
	if task.Schedule != "0 2 * * *" {
		t.Errorf("schedule = %q, want %q", task.Schedule, "0 2 * * *")
	}
}

func TestScheduleNotificationLimitEnforcement_RegistersTask(t *testing.T) {
	s := NewScheduler(nil)
	cleaner, _, _ := newTestNotificationCleaner(t)

	s.ScheduleNotificationLimitEnforcement(cleaner, "03:30")

	task, ok := s.tasks[notificationLimitTaskName]
	if !ok {
		t.Fatalf("task %q was not registered with the scheduler", notificationLimitTaskName)
	}
	if task.Schedule != "30 3 * * *" {
		t.Errorf("schedule = %q, want %q", task.Schedule, "30 3 * * *")
	}
}

func TestDailyCronFromClockTime(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "02:00", want: "0 2 * * *"},
		{in: "23:59", want: "59 23 * * *"},
		{in: "0:05", want: "5 0 * * *"},
		{in: "24:00", wantErr: true},
		{in: "12:60", wantErr: true},
		{in: "1200", wantErr: true},
		{in: "ab:cd", wantErr: true},
	}

	for _, tc := range tests {
		got, err := dailyCronFromClockTime(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("dailyCronFromClockTime(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("dailyCronFromClockTime(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("dailyCronFromClockTime(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
