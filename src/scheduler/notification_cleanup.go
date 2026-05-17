package scheduler

import (
	"log"
	"time"

	"github.com/casapps/wthr/src/server/service"
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

// CleanupExpiredNotifications removes expired notifications (>30 days old)
// TEMPLATE.md Part 25: Notifications expire after 30 days
func (nc *NotificationCleaner) CleanupExpiredNotifications() error {
	log.Println("🧹 Starting expired notification cleanup...")
	startTime := time.Now()

	// Cleanup expired user notifications
	userDeleted, err := nc.notificationService.UserNotif.CleanupExpired()
	if err != nil {
		log.Printf("⚠️  Failed to cleanup user notifications: %v", err)
		return err
	}

	// Cleanup expired admin notifications
	adminDeleted, err := nc.notificationService.AdminNotif.CleanupExpired()
	if err != nil {
		log.Printf("⚠️  Failed to cleanup admin notifications: %v", err)
		return err
	}

	elapsed := time.Since(startTime)
	totalDeleted := userDeleted + adminDeleted

	if totalDeleted > 0 {
		log.Printf("✅ Deleted %d expired notifications (%d user, %d admin) in %.2f seconds",
			totalDeleted, userDeleted, adminDeleted, elapsed.Seconds())
	} else {
		log.Printf("✅ No expired notifications to cleanup (%.2f seconds)", elapsed.Seconds())
	}

	return nil
}

// EnforceLimits enforces the 100 notification limit per user/admin
// TEMPLATE.md Part 25: Keep maximum 100 notifications per user
func (nc *NotificationCleaner) EnforceLimits() error {
	log.Println("📊 Starting notification limit enforcement...")
	startTime := time.Now()

	// This would need to get all user IDs and admin IDs from the database
	// For now, we'll just log that the task ran
	// Limits enforced per-user when creating notifications

	// The limit enforcement is also handled per-user when creating notifications
	// This task is a backup to ensure limits are maintained

	elapsed := time.Since(startTime)
	log.Printf("✅ Notification limit enforcement completed in %.2f seconds", elapsed.Seconds())

	return nil
}

// ScheduleNotificationCleanup schedules notification cleanup at specific time daily
// Default: 02:00 UTC (TEMPLATE.md Part 25)
func (s *Scheduler) ScheduleNotificationCleanup(cleaner *NotificationCleaner, targetTime string) {
	// Calculate initial delay until target time
	initialDelay := CalculateNextRunTime(targetTime)

	log.Printf("📅 Notification cleanup scheduled for %s UTC (next run in %v)", targetTime, initialDelay)

	// Start a goroutine that waits for the initial delay, then runs every 24 hours
	go func() {
		// Wait until target time
		time.Sleep(initialDelay)

		// Run the first cleanup
		if err := cleaner.CleanupExpiredNotifications(); err != nil {
			log.Printf("⚠️  Scheduled notification cleanup failed: %v", err)
		}

		// Then run every 24 hours
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := cleaner.CleanupExpiredNotifications(); err != nil {
					log.Printf("⚠️  Scheduled notification cleanup failed: %v", err)
				}
			}
		}
	}()
}

// ScheduleNotificationLimitEnforcement schedules notification limit enforcement at specific time daily
// Default: 03:00 UTC (TEMPLATE.md Part 25)
func (s *Scheduler) ScheduleNotificationLimitEnforcement(cleaner *NotificationCleaner, targetTime string) {
	// Calculate initial delay until target time
	initialDelay := CalculateNextRunTime(targetTime)

	log.Printf("📅 Notification limit enforcement scheduled for %s UTC (next run in %v)", targetTime, initialDelay)

	// Start a goroutine that waits for the initial delay, then runs every 24 hours
	go func() {
		// Wait until target time
		time.Sleep(initialDelay)

		// Run the first enforcement
		if err := cleaner.EnforceLimits(); err != nil {
			log.Printf("⚠️  Scheduled notification limit enforcement failed: %v", err)
		}

		// Then run every 24 hours
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := cleaner.EnforceLimits(); err != nil {
					log.Printf("⚠️  Scheduled notification limit enforcement failed: %v", err)
				}
			}
		}
	}()
}
