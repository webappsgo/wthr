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
	// EnforceLimits is currently a no-op (see notification_cleanup.go comments:
	// "This would need to get all user IDs and admin IDs from the database...
	// For now, we'll just log that the task ran"). This test documents the
	// current always-nil behavior; it is not exercising real limit enforcement
	// because none exists yet at this layer.
	cleaner, _, _ := newTestNotificationCleaner(t)
	if err := cleaner.EnforceLimits(); err != nil {
		t.Errorf("EnforceLimits() = %v, want nil", err)
	}
}

// --- ScheduleNotificationCleanup / ScheduleNotificationLimitEnforcement --------------
//
// Both schedulers are exercised only for their synchronous, non-blocking half
// (delay calculation, logging, launching the background goroutine). The
// goroutine's post-Sleep body (which calls cleaner.CleanupExpiredNotifications()/
// EnforceLimits() and then loops on a 24h ticker) is intentionally never
// reached in-test: the target time is picked far enough in the future that
// the real time.Sleep cannot elapse before the test process exits.

func TestScheduleNotificationCleanup_ReturnsImmediately(t *testing.T) {
	s := NewScheduler(nil)
	cleaner, _, _ := newTestNotificationCleaner(t)
	target := time.Now().Add(12 * time.Hour).Format("15:04")

	finished := make(chan struct{})
	go func() {
		s.ScheduleNotificationCleanup(cleaner, target)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("ScheduleNotificationCleanup() blocked instead of returning after launching its background goroutine")
	}
}

func TestScheduleNotificationLimitEnforcement_ReturnsImmediately(t *testing.T) {
	s := NewScheduler(nil)
	cleaner, _, _ := newTestNotificationCleaner(t)
	target := time.Now().Add(12 * time.Hour).Format("15:04")

	finished := make(chan struct{})
	go func() {
		s.ScheduleNotificationLimitEnforcement(cleaner, target)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("ScheduleNotificationLimitEnforcement() blocked instead of returning after launching its background goroutine")
	}
}
