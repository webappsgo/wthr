package models

import "database/sql"

// NotificationModel handles notification-related database operations
type NotificationModel struct {
	DB *sql.DB
}

// GetUnreadCount returns the count of unread, non-dismissed, non-expired notifications for a user.
func (m *NotificationModel) GetUnreadCount(userID int64) (int, error) {
	var count int
	err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM user_notifications
		WHERE user_id = ? AND read = 0 AND dismissed = 0
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
	`, userID).Scan(&count)
	return count, err
}
