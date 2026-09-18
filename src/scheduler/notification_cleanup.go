package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
)

// NotificationCleaner holds references to the notification service for cleanup tasks
type NotificationCleaner struct {
	notificationService *service.NotificationService
}

// NewNotificationCleaner creates a new notification cleaner
func NewNotificationCleaner(notificationService *service.NotificationService) *NotificationCleaner {
	return &NotificationCleaner{
		notificationService: notificationService,
	}
}

// maxNotificationsPerRecipient is AI.md PART 18's per-user/per-admin cap on
// retained WebUI notifications.
const maxNotificationsPerRecipient = 100

// notificationCleanupTaskName and notificationLimitTaskName are the scheduler
// task ids these two jobs register under.
const (
	notificationCleanupTaskName = "cleanup-notifications"
	notificationLimitTaskName   = "enforce-notification-limits"
)

// CleanupExpiredNotifications removes expired notifications (>30 days old)
// AI.md PART 18: notifications are retained for 30 days
func (nc *NotificationCleaner) CleanupExpiredNotifications() error {
	log.Println("INFO: Starting expired notification cleanup...")
	startTime := time.Now()

	// Cleanup expired user notifications
	userDeleted, err := nc.notificationService.UserNotif.CleanupExpired()
	if err != nil {
		log.Printf("WARNING: Failed to cleanup user notifications: %v", err)
		return err
	}

	// Cleanup expired admin notifications
	adminDeleted, err := nc.notificationService.AdminNotif.CleanupExpired()
	if err != nil {
		log.Printf("WARNING: Failed to cleanup admin notifications: %v", err)
		return err
	}

	elapsed := time.Since(startTime)
	totalDeleted := userDeleted + adminDeleted

	if totalDeleted > 0 {
		log.Printf("OK: Deleted %d expired notifications (%d user, %d admin) in %.2f seconds",
			totalDeleted, userDeleted, adminDeleted, elapsed.Seconds())
	} else {
		log.Printf("OK: No expired notifications to cleanup (%.2f seconds)", elapsed.Seconds())
	}

	return nil
}

// EnforceLimits trims every user's and admin's notification list down to the
// 100 most recent entries. AI.md PART 18 caps retained notifications per
// recipient; per-recipient enforcement on write can be missed by direct
// inserts, so this task re-applies the cap across the whole table.
func (nc *NotificationCleaner) EnforceLimits() error {
	log.Println("INFO: Starting notification limit enforcement...")
	startTime := time.Now()

	userTrimmed, err := enforceRecipientLimits(
		nc.notificationService.UserNotif.DB,
		"SELECT DISTINCT user_id FROM user_notifications",
		nc.notificationService.UserNotif.EnforceLimit,
	)
	if err != nil {
		log.Printf("WARNING: Failed to enforce user notification limits: %v", err)
		return err
	}

	adminTrimmed, err := enforceRecipientLimits(
		nc.notificationService.AdminNotif.DB,
		"SELECT DISTINCT admin_id FROM server_admin_notifications",
		nc.notificationService.AdminNotif.EnforceLimit,
	)
	if err != nil {
		log.Printf("WARNING: Failed to enforce admin notification limits: %v", err)
		return err
	}

	elapsed := time.Since(startTime)
	log.Printf("OK: Notification limit enforcement removed %d notification(s) (%d user, %d admin) in %.2f seconds",
		userTrimmed+adminTrimmed, userTrimmed, adminTrimmed, elapsed.Seconds())

	return nil
}

// enforceRecipientLimits applies enforce to every recipient id returned by
// query and totals the rows removed. Recipient ids are collected before any
// deletion so the result set is not invalidated mid-iteration.
func enforceRecipientLimits(db *sql.DB, query string, enforce func(id int, limit int) (int64, error)) (int64, error) {
	if db == nil {
		return 0, nil
	}

	rows, err := database.QueryContext(context.Background(), db, database.TimeoutSimpleSelect, query)
	if err != nil {
		return 0, err
	}

	ids := make([]int, 0)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	var total int64
	for _, id := range ids {
		deleted, err := enforce(id, maxNotificationsPerRecipient)
		if err != nil {
			return total, err
		}
		total += deleted
	}

	return total, nil
}

// ScheduleNotificationCleanup registers the daily expired-notification cleanup
// with the built-in scheduler. AI.md PART 19 forbids any other scheduling
// mechanism, so this task gets the same persistence, cluster locking, catch-up,
// retry, and admin visibility as every other task.
func (s *Scheduler) ScheduleNotificationCleanup(cleaner *NotificationCleaner, targetTime string) {
	s.addDailyTask(notificationCleanupTaskName, targetTime, cleaner.CleanupExpiredNotifications)
}

// ScheduleNotificationLimitEnforcement registers the daily per-recipient
// notification cap enforcement with the built-in scheduler.
func (s *Scheduler) ScheduleNotificationLimitEnforcement(cleaner *NotificationCleaner, targetTime string) {
	s.addDailyTask(notificationLimitTaskName, targetTime, cleaner.EnforceLimits)
}

// addDailyTask registers fn to run once a day at targetTime ("HH:MM").
func (s *Scheduler) addDailyTask(name, targetTime string, fn func() error) {
	schedule, err := dailyCronFromClockTime(targetTime)
	if err != nil {
		log.Printf("WARNING: Task '%s' not scheduled: %v", name, err)
		return
	}

	if err := s.AddTask(name, schedule, fn); err != nil {
		log.Printf("WARNING: Failed to schedule task '%s': %v", name, err)
		return
	}

	log.Printf("INFO: Task '%s' scheduled daily at %s", name, targetTime)
}

// dailyCronFromClockTime converts an "HH:MM" clock time into the daily cron
// expression the built-in scheduler parses.
func dailyCronFromClockTime(targetTime string) (string, error) {
	hour, minute, ok := strings.Cut(targetTime, ":")
	if !ok {
		return "", fmt.Errorf("invalid time %q: want HH:MM", targetTime)
	}

	h, err := strconv.Atoi(hour)
	if err != nil || h < 0 || h > 23 {
		return "", fmt.Errorf("invalid hour in %q", targetTime)
	}

	m, err := strconv.Atoi(minute)
	if err != nil || m < 0 || m > 59 {
		return "", fmt.Errorf("invalid minute in %q", targetTime)
	}

	return fmt.Sprintf("%d %d * * *", m, h), nil
}
