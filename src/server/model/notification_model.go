package model

import (
	"context"
	"database/sql"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// NotificationModel handles notification-related database operations
type NotificationModel struct {
	DB *sql.DB
}

// GetUnreadCount returns the count of unread, non-dismissed, non-expired notifications for a user.
func (m *NotificationModel) GetUnreadCount(userID int64) (int, error) {
	var count int
	// user_notifications.expires_at is populated from a raw time.Time bind
	// parameter (see UserNotificationModel.Create), which serializes to
	// RFC3339Nano with a numeric UTC offset -- SQLite's datetime() cannot
	// parse that and would silently make this comparison always false.
	// Compare against a like-formatted bound time.Now() instead, matching
	// every other expiry check in notification.go.
	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM user_notifications
		WHERE user_id = ? AND read = 0 AND dismissed = 0
		  AND (expires_at IS NULL OR expires_at > ?)
	`, userID, time.Now()).Scan(&count)
	return count, err
}
