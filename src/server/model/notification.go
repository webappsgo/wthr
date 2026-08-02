package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/database"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeSuccess  NotificationType = "success"
	NotificationTypeInfo     NotificationType = "info"
	NotificationTypeWarning  NotificationType = "warning"
	NotificationTypeError    NotificationType = "error"
	NotificationTypeSecurity NotificationType = "security"
)

// NotificationDisplay represents how the notification should be displayed
type NotificationDisplay string

const (
	// Toast notification (top-right, auto-dismiss)
	NotificationDisplayToast NotificationDisplay = "toast"
	// Banner notification (top of page)
	NotificationDisplayBanner NotificationDisplay = "banner"
	// Notification center only
	NotificationDisplayCenter NotificationDisplay = "center"
)

// Notification represents a WebUI notification (user or admin)
type Notification struct {
	// ULID
	ID string `json:"id"`
	// NULL for admin notifications
	UserID *int `json:"user_id,omitempty"`
	// NULL for user notifications
	AdminID *int `json:"admin_id,omitempty"`
	// success, info, warning, error, security
	Type NotificationType `json:"type"`
	// toast, banner, center
	Display NotificationDisplay `json:"display"`
	// Notification title
	Title string `json:"title"`
	// Notification message
	Message string `json:"message"`
	// Optional action button
	Action *NotificationAction `json:"action,omitempty"`
	// JSON-encoded action (database field)
	ActionJSON *string `json:"-" db:"action_json"`
	// Whether read
	Read bool `json:"read"`
	// When read
	ReadAt *time.Time `json:"read_at,omitempty"`
	// Whether dismissed
	Dismissed bool `json:"dismissed"`
	// When created
	CreatedAt time.Time `json:"created_at"`
	// When last updated
	UpdatedAt time.Time `json:"updated_at"`
	// When to auto-delete (default: 30 days)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// NotificationAction represents an optional action button
type NotificationAction struct {
	// Button label (e.g., "View Details")
	Label string `json:"label"`
	// URL to navigate to
	URL string `json:"url"`
}

// NotificationPreferences represents user/admin notification preferences
type NotificationPreferences struct {
	UserID       *int `json:"user_id,omitempty"`
	AdminID      *int `json:"admin_id,omitempty"`
	EnableToast  bool `json:"enable_toast"`
	EnableBanner bool `json:"enable_banner"`
	EnableCenter bool `json:"enable_center"`
	EnableSound  bool `json:"enable_sound"`
	// seconds
	ToastDurationSuccess int `json:"toast_duration_success"`
	// seconds
	ToastDurationInfo int `json:"toast_duration_info"`
	// seconds
	ToastDurationWarning int       `json:"toast_duration_warning"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// NotificationStatistics represents notification statistics
type NotificationStatistics struct {
	Total     int                         `json:"total"`
	Unread    int                         `json:"unread"`
	Read      int                         `json:"read"`
	ByType    map[NotificationType]int    `json:"by_type"`
	ByDisplay map[NotificationDisplay]int `json:"by_display"`
}

// UserNotificationModel handles user notification database operations
type UserNotificationModel struct {
	DB *sql.DB
}

// Create creates a new user notification
func (m *UserNotificationModel) Create(userID int, notifType NotificationType, display NotificationDisplay, title, message string, action *NotificationAction) (*Notification, error) {
	// Generate ULID
	id := ulid.Make().String()

	// Calculate expiration (30 days from now)
	expiresAt := time.Now().AddDate(0, 0, 30)

	// Encode action if provided
	var actionJSON *string
	if action != nil {
		actionBytes, err := json.Marshal(action)
		if err != nil {
			return nil, fmt.Errorf("failed to encode action: %w", err)
		}
		actionStr := string(actionBytes)
		actionJSON = &actionStr
	}

	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		INSERT INTO user_notifications (id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, notifType, display, title, message, actionJSON, false, false, time.Now(), expiresAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create user notification: %w", err)
	}

	return m.GetByID(id)
}

// GetByID retrieves a user notification by ID
func (m *UserNotificationModel) GetByID(id string) (*Notification, error) {
	notif := &Notification{}
	var actionJSON sql.NullString
	var expiresAt sql.NullTime

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM user_notifications WHERE id = ?
	`, id).Scan(&notif.ID, &notif.UserID, &notif.Type, &notif.Display, &notif.Title,
		&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &notif.CreatedAt, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("notification not found")
	}
	if err != nil {
		return nil, err
	}

	// Decode action if present
	if actionJSON.Valid && actionJSON.String != "" {
		var action NotificationAction
		if err := json.Unmarshal([]byte(actionJSON.String), &action); err == nil {
			notif.Action = &action
		}
	}

	if expiresAt.Valid {
		notif.ExpiresAt = &expiresAt.Time
	}

	return notif, nil
}

// GetByUserID retrieves all notifications for a user with pagination
func (m *UserNotificationModel) GetByUserID(userID int, limit, offset int) ([]*Notification, error) {
	query := `
		SELECT id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM user_notifications
		WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, userID, time.Now(), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanNotifications(rows)
}

// GetUnread retrieves unread notifications for a user
func (m *UserNotificationModel) GetUnread(userID int) ([]*Notification, error) {
	query := `
		SELECT id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM user_notifications
		WHERE user_id = ? AND read = 0 AND dismissed = 0 AND expires_at > ?
		ORDER BY created_at DESC
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, userID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanNotifications(rows)
}

// GetUnreadCount returns the count of unread notifications for a user
func (m *UserNotificationModel) GetUnreadCount(userID int) (int, error) {
	var count int
	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM user_notifications
		WHERE user_id = ? AND read = 0 AND dismissed = 0 AND expires_at > ?
	`, userID, time.Now()).Scan(&count)
	return count, err
}

// MarkAsRead marks a notification as read
func (m *UserNotificationModel) MarkAsRead(id string, userID int) error {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "UPDATE user_notifications SET read = 1 WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or not owned by user")
	}

	return nil
}

// MarkAllAsRead marks all notifications as read for a user
func (m *UserNotificationModel) MarkAllAsRead(userID int) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "UPDATE user_notifications SET read = 1 WHERE user_id = ? AND read = 0", userID)
	return err
}

// Dismiss dismisses a notification
func (m *UserNotificationModel) Dismiss(id string, userID int) error {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "UPDATE user_notifications SET dismissed = 1 WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or not owned by user")
	}

	return nil
}

// Delete deletes a notification
func (m *UserNotificationModel) Delete(id string, userID int) error {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "DELETE FROM user_notifications WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or not owned by user")
	}

	return nil
}

// CleanupExpired deletes expired notifications
func (m *UserNotificationModel) CleanupExpired() (int64, error) {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutBulk, "DELETE FROM user_notifications WHERE expires_at <= ?", time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// EnforceLimit enforces the 100 notification limit per user
func (m *UserNotificationModel) EnforceLimit(userID int, limit int) (int64, error) {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		DELETE FROM user_notifications
		WHERE user_id = ? AND id NOT IN (
			SELECT id FROM user_notifications
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		)
	`, userID, userID, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetStatistics returns notification statistics for a user
func (m *UserNotificationModel) GetStatistics(userID int) (*NotificationStatistics, error) {
	stats := &NotificationStatistics{
		ByType:    make(map[NotificationType]int),
		ByDisplay: make(map[NotificationDisplay]int),
	}

	// Get total and read counts
	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN read = 0 AND dismissed = 0 THEN 1 ELSE 0 END), 0) as unread,
			COALESCE(SUM(CASE WHEN read = 1 THEN 1 ELSE 0 END), 0) as read
		FROM user_notifications
		WHERE user_id = ? AND expires_at > ?
	`, userID, time.Now()).Scan(&stats.Total, &stats.Unread, &stats.Read)
	if err != nil {
		return nil, err
	}

	// Get counts by type
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT type, COUNT(*) FROM user_notifications
		WHERE user_id = ? AND expires_at > ?
		GROUP BY type
	`, userID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var notifType NotificationType
		var count int
		if err := rows.Scan(&notifType, &count); err != nil {
			return nil, err
		}
		stats.ByType[notifType] = count
	}

	// Get counts by display
	rows, err = database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT display, COUNT(*) FROM user_notifications
		WHERE user_id = ? AND expires_at > ?
		GROUP BY display
	`, userID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var display NotificationDisplay
		var count int
		if err := rows.Scan(&display, &count); err != nil {
			return nil, err
		}
		stats.ByDisplay[display] = count
	}

	return stats, nil
}

// scanNotifications scans multiple notifications from rows
func (m *UserNotificationModel) scanNotifications(rows *sql.Rows) ([]*Notification, error) {
	var notifications []*Notification

	for rows.Next() {
		notif := &Notification{}
		var actionJSON sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(&notif.ID, &notif.UserID, &notif.Type, &notif.Display, &notif.Title,
			&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &notif.CreatedAt, &expiresAt)
		if err != nil {
			return nil, err
		}

		// Decode action if present
		if actionJSON.Valid && actionJSON.String != "" {
			var action NotificationAction
			if err := json.Unmarshal([]byte(actionJSON.String), &action); err == nil {
				notif.Action = &action
			}
		}

		if expiresAt.Valid {
			notif.ExpiresAt = &expiresAt.Time
		}

		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// AdminNotificationModel handles admin notification database operations
type AdminNotificationModel struct {
	DB *sql.DB
}

// Create creates a new admin notification
func (m *AdminNotificationModel) Create(adminID int, notifType NotificationType, display NotificationDisplay, title, message string, action *NotificationAction) (*Notification, error) {
	// Generate ULID
	id := ulid.Make().String()

	// Calculate expiration (30 days from now)
	expiresAt := time.Now().AddDate(0, 0, 30)

	// Encode action if provided
	var actionJSON *string
	if action != nil {
		actionBytes, err := json.Marshal(action)
		if err != nil {
			return nil, fmt.Errorf("failed to encode action: %w", err)
		}
		actionStr := string(actionBytes)
		actionJSON = &actionStr
	}

	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		INSERT INTO server_admin_notifications (id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, adminID, notifType, display, title, message, actionJSON, false, false, time.Now(), expiresAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create admin notification: %w", err)
	}

	return m.GetByID(id)
}

// GetByID retrieves an admin notification by ID
func (m *AdminNotificationModel) GetByID(id string) (*Notification, error) {
	notif := &Notification{}
	var actionJSON sql.NullString
	var expiresAt sql.NullTime

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM server_admin_notifications WHERE id = ?
	`, id).Scan(&notif.ID, &notif.AdminID, &notif.Type, &notif.Display, &notif.Title,
		&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &notif.CreatedAt, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("notification not found")
	}
	if err != nil {
		return nil, err
	}

	// Decode action if present
	if actionJSON.Valid && actionJSON.String != "" {
		var action NotificationAction
		if err := json.Unmarshal([]byte(actionJSON.String), &action); err == nil {
			notif.Action = &action
		}
	}

	if expiresAt.Valid {
		notif.ExpiresAt = &expiresAt.Time
	}

	return notif, nil
}

// GetByAdminID retrieves all notifications for an admin with pagination
func (m *AdminNotificationModel) GetByAdminID(adminID int, limit, offset int) ([]*Notification, error) {
	query := `
		SELECT id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM server_admin_notifications
		WHERE admin_id = ? AND expires_at > ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, adminID, time.Now(), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanNotifications(rows)
}

// GetUnread retrieves unread notifications for an admin
func (m *AdminNotificationModel) GetUnread(adminID int) ([]*Notification, error) {
	query := `
		SELECT id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM server_admin_notifications
		WHERE admin_id = ? AND read = 0 AND dismissed = 0 AND expires_at > ?
		ORDER BY created_at DESC
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, adminID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanNotifications(rows)
}

// GetUnreadCount returns the count of unread notifications for an admin
func (m *AdminNotificationModel) GetUnreadCount(adminID int) (int, error) {
	var count int
	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM server_admin_notifications
		WHERE admin_id = ? AND read = 0 AND dismissed = 0 AND expires_at > ?
	`, adminID, time.Now()).Scan(&count)
	return count, err
}

// MarkAsRead marks a notification as read
func (m *AdminNotificationModel) MarkAsRead(id string, adminID int) error {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "UPDATE server_admin_notifications SET read = 1 WHERE id = ? AND admin_id = ?", id, adminID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or not owned by admin")
	}

	return nil
}

// MarkAllAsRead marks all notifications as read for an admin
func (m *AdminNotificationModel) MarkAllAsRead(adminID int) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "UPDATE server_admin_notifications SET read = 1 WHERE admin_id = ? AND read = 0", adminID)
	return err
}

// Dismiss dismisses a notification
func (m *AdminNotificationModel) Dismiss(id string, adminID int) error {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "UPDATE server_admin_notifications SET dismissed = 1 WHERE id = ? AND admin_id = ?", id, adminID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or not owned by admin")
	}

	return nil
}

// Delete deletes a notification
func (m *AdminNotificationModel) Delete(id string, adminID int) error {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "DELETE FROM server_admin_notifications WHERE id = ? AND admin_id = ?", id, adminID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or not owned by admin")
	}

	return nil
}

// CleanupExpired deletes expired notifications
func (m *AdminNotificationModel) CleanupExpired() (int64, error) {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutBulk, "DELETE FROM server_admin_notifications WHERE expires_at <= ?", time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// EnforceLimit enforces the 100 notification limit per admin
func (m *AdminNotificationModel) EnforceLimit(adminID int, limit int) (int64, error) {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		DELETE FROM server_admin_notifications
		WHERE admin_id = ? AND id NOT IN (
			SELECT id FROM server_admin_notifications
			WHERE admin_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		)
	`, adminID, adminID, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetStatistics returns notification statistics for an admin
func (m *AdminNotificationModel) GetStatistics(adminID int) (*NotificationStatistics, error) {
	stats := &NotificationStatistics{
		ByType:    make(map[NotificationType]int),
		ByDisplay: make(map[NotificationDisplay]int),
	}

	// Get total and read counts
	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN read = 0 AND dismissed = 0 THEN 1 ELSE 0 END), 0) as unread,
			COALESCE(SUM(CASE WHEN read = 1 THEN 1 ELSE 0 END), 0) as read
		FROM server_admin_notifications
		WHERE admin_id = ? AND expires_at > ?
	`, adminID, time.Now()).Scan(&stats.Total, &stats.Unread, &stats.Read)
	if err != nil {
		return nil, err
	}

	// Get counts by type
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT type, COUNT(*) FROM server_admin_notifications
		WHERE admin_id = ? AND expires_at > ?
		GROUP BY type
	`, adminID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var notifType NotificationType
		var count int
		if err := rows.Scan(&notifType, &count); err != nil {
			return nil, err
		}
		stats.ByType[notifType] = count
	}

	// Get counts by display
	rows, err = database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT display, COUNT(*) FROM server_admin_notifications
		WHERE admin_id = ? AND expires_at > ?
		GROUP BY display
	`, adminID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var display NotificationDisplay
		var count int
		if err := rows.Scan(&display, &count); err != nil {
			return nil, err
		}
		stats.ByDisplay[display] = count
	}

	return stats, nil
}

// scanNotifications scans multiple notifications from rows
func (m *AdminNotificationModel) scanNotifications(rows *sql.Rows) ([]*Notification, error) {
	var notifications []*Notification

	for rows.Next() {
		notif := &Notification{}
		var actionJSON sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(&notif.ID, &notif.AdminID, &notif.Type, &notif.Display, &notif.Title,
			&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &notif.CreatedAt, &expiresAt)
		if err != nil {
			return nil, err
		}

		// Decode action if present
		if actionJSON.Valid && actionJSON.String != "" {
			var action NotificationAction
			if err := json.Unmarshal([]byte(actionJSON.String), &action); err == nil {
				notif.Action = &action
			}
		}

		if expiresAt.Valid {
			notif.ExpiresAt = &expiresAt.Time
		}

		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// NotificationPreferencesModel handles notification preferences
type NotificationPreferencesModel struct {
	UserDB   *sql.DB
	ServerDB *sql.DB
}

// GetUserPreferences retrieves notification preferences for a user
func (m *NotificationPreferencesModel) GetUserPreferences(userID int) (*NotificationPreferences, error) {
	prefs := &NotificationPreferences{
		UserID: &userID,
		// Defaults per TEMPLATE.md Part 25
		EnableToast:          true,
		EnableBanner:         true,
		EnableCenter:         true,
		EnableSound:          false,
		ToastDurationSuccess: 5,
		ToastDurationInfo:    5,
		ToastDurationWarning: 10,
	}

	err := database.QueryRowContext(context.Background(), m.UserDB, database.TimeoutSimpleSelect, `
		SELECT enable_toast, enable_banner, enable_center, enable_sound,
		       toast_duration_success, toast_duration_info, toast_duration_warning, updated_at
		FROM user_notification_preferences
		WHERE user_id = ?
	`, userID).Scan(&prefs.EnableToast, &prefs.EnableBanner, &prefs.EnableCenter, &prefs.EnableSound,
		&prefs.ToastDurationSuccess, &prefs.ToastDurationInfo, &prefs.ToastDurationWarning, &prefs.UpdatedAt)

	if err == sql.ErrNoRows {
		// Return defaults if no preferences set
		prefs.UpdatedAt = time.Now()
		return prefs, nil
	}
	if err != nil {
		return nil, err
	}

	return prefs, nil
}

// UpdateUserPreferences updates notification preferences for a user
func (m *NotificationPreferencesModel) UpdateUserPreferences(userID int, prefs *NotificationPreferences) error {
	_, err := database.ExecContext(context.Background(), m.UserDB, database.TimeoutWrite, `
		INSERT INTO user_notification_preferences
		(user_id, enable_toast, enable_banner, enable_center, enable_sound,
		 toast_duration_success, toast_duration_info, toast_duration_warning, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			enable_toast = excluded.enable_toast,
			enable_banner = excluded.enable_banner,
			enable_center = excluded.enable_center,
			enable_sound = excluded.enable_sound,
			toast_duration_success = excluded.toast_duration_success,
			toast_duration_info = excluded.toast_duration_info,
			toast_duration_warning = excluded.toast_duration_warning,
			updated_at = excluded.updated_at
	`, userID, prefs.EnableToast, prefs.EnableBanner, prefs.EnableCenter, prefs.EnableSound,
		prefs.ToastDurationSuccess, prefs.ToastDurationInfo, prefs.ToastDurationWarning, time.Now())

	return err
}

// GetAdminPreferences retrieves notification preferences for an admin
func (m *NotificationPreferencesModel) GetAdminPreferences(adminID int) (*NotificationPreferences, error) {
	prefs := &NotificationPreferences{
		AdminID: &adminID,
		// Defaults per TEMPLATE.md Part 25
		EnableToast:          true,
		EnableBanner:         true,
		EnableCenter:         true,
		EnableSound:          false,
		ToastDurationSuccess: 5,
		ToastDurationInfo:    5,
		ToastDurationWarning: 10,
	}

	err := database.QueryRowContext(context.Background(), m.ServerDB, database.TimeoutSimpleSelect, `
		SELECT enable_toast, enable_banner, enable_center, enable_sound,
		       toast_duration_success, toast_duration_info, toast_duration_warning, updated_at
		FROM server_admin_notification_preferences
		WHERE admin_id = ?
	`, adminID).Scan(&prefs.EnableToast, &prefs.EnableBanner, &prefs.EnableCenter, &prefs.EnableSound,
		&prefs.ToastDurationSuccess, &prefs.ToastDurationInfo, &prefs.ToastDurationWarning, &prefs.UpdatedAt)

	if err == sql.ErrNoRows {
		// Return defaults if no preferences set
		prefs.UpdatedAt = time.Now()
		return prefs, nil
	}
	if err != nil {
		return nil, err
	}

	return prefs, nil
}

// UpdateAdminPreferences updates notification preferences for an admin
func (m *NotificationPreferencesModel) UpdateAdminPreferences(adminID int, prefs *NotificationPreferences) error {
	_, err := database.ExecContext(context.Background(), m.ServerDB, database.TimeoutWrite, `
		INSERT INTO server_admin_notification_preferences
		(admin_id, enable_toast, enable_banner, enable_center, enable_sound,
		 toast_duration_success, toast_duration_info, toast_duration_warning, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(admin_id) DO UPDATE SET
			enable_toast = excluded.enable_toast,
			enable_banner = excluded.enable_banner,
			enable_center = excluded.enable_center,
			enable_sound = excluded.enable_sound,
			toast_duration_success = excluded.toast_duration_success,
			toast_duration_info = excluded.toast_duration_info,
			toast_duration_warning = excluded.toast_duration_warning,
			updated_at = excluded.updated_at
	`, adminID, prefs.EnableToast, prefs.EnableBanner, prefs.EnableCenter, prefs.EnableSound,
		prefs.ToastDurationSuccess, prefs.ToastDurationInfo, prefs.ToastDurationWarning, time.Now())

	return err
}
