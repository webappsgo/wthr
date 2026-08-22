package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/database"
)

// noNotificationLimit asks the scan helpers for every matching row. It mirrors
// SQLite's own rule that a negative LIMIT means "no limit", so a caller that
// passes a limit straight through from a request keeps the semantics the SQL
// LIMIT clause used to give it.
const noNotificationLimit = -1

// countUnexpiredRows counts rows of a single-column result set holding a stored
// expires_at value, keeping only those that parse to an instant later than now.
// An unparseable or NULL value fails closed and is not counted.
func countUnexpiredRows(rows *sql.Rows, now time.Time) (int, error) {
	count := 0

	for rows.Next() {
		var storedExpiresAt interface{}
		if err := rows.Scan(&storedExpiresAt); err != nil {
			return 0, err
		}

		expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
		if !ok || !expiresAt.After(now) {
			continue
		}

		count++
	}

	if err := rows.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// tallyNotificationStatistics accumulates the totals, per-type and per-display
// counters GetStatistics reports from a "type, display, read, dismissed,
// expires_at" result set, skipping every row whose expires_at does not parse to
// an instant later than now.
func tallyNotificationStatistics(rows *sql.Rows, now time.Time, stats *NotificationStatistics) error {
	for rows.Next() {
		var notifType NotificationType
		var display NotificationDisplay
		var read, dismissed bool
		var storedExpiresAt interface{}

		if err := rows.Scan(&notifType, &display, &read, &dismissed, &storedExpiresAt); err != nil {
			return err
		}

		expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
		if !ok || !expiresAt.After(now) {
			continue
		}

		stats.Total++
		if !read && !dismissed {
			stats.Unread++
		}
		if read {
			stats.Read++
		}

		stats.ByType[notifType]++
		stats.ByDisplay[display]++
	}

	return rows.Err()
}

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
	`, id, userID, notifType, display, title, message, actionJSON, false, false, sqlTimestamp(time.Now()), sqlTimestamp(expiresAt))

	if err != nil {
		return nil, fmt.Errorf("failed to create user notification: %w", err)
	}

	return m.GetByID(id)
}

// GetByID retrieves a user notification by ID
func (m *UserNotificationModel) GetByID(id string) (*Notification, error) {
	notif := &Notification{}
	var actionJSON sql.NullString
	var storedCreatedAt, storedExpiresAt interface{}

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM user_notifications WHERE id = ?
	`, id).Scan(&notif.ID, &notif.UserID, &notif.Type, &notif.Display, &notif.Title,
		&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &storedCreatedAt, &storedExpiresAt)

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

	// created_at and expires_at are scanned as raw driver values and parsed with
	// parseStoredTimestamp so a row written in the old local-zone layout and a
	// row written in the canonical UTC layout both come back as the same
	// absolute instant.
	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		notif.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedExpiresAt); ok {
		notif.ExpiresAt = &parsed
	}

	return notif, nil
}

// GetByUserID retrieves all notifications for a user with pagination
// The "not expired" test runs in Go rather than as an SQL "expires_at > ?"
// comparison: that comparison is lexicographic on TEXT, so it is only correct
// while every stored value uses the canonical UTC layout, and a row written by
// an older local-zone writer compares wrong in either direction. The LIMIT and
// OFFSET window moves into Go for the same reason - SQL cannot slice the rows
// before the expiry test it cannot be trusted to make. Scanning still stops as
// soon as the window is full, so the query reads no more rows than the
// LIMIT-ed statement did.
func (m *UserNotificationModel) GetByUserID(userID int, limit, offset int) ([]*Notification, error) {
	query := `
		SELECT id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM user_notifications
		WHERE user_id = ? AND expires_at IS NOT NULL
		ORDER BY created_at DESC
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanUnexpiredNotifications(rows, time.Now().UTC(), offset, limit)
}

// GetUnread retrieves unread notifications for a user
// The expiry test runs in Go for the reason described on GetByUserID.
func (m *UserNotificationModel) GetUnread(userID int) ([]*Notification, error) {
	query := `
		SELECT id, user_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM user_notifications
		WHERE user_id = ? AND read = 0 AND dismissed = 0 AND expires_at IS NOT NULL
		ORDER BY created_at DESC
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanUnexpiredNotifications(rows, time.Now().UTC(), 0, noNotificationLimit)
}

// GetUnreadCount returns the count of unread notifications for a user
// The count is accumulated in Go rather than by SQL COUNT(*) so the expiry test
// is the same instant comparison every other read in this file performs.
func (m *UserNotificationModel) GetUnreadCount(userID int) (int, error) {
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT expires_at FROM user_notifications
		WHERE user_id = ? AND read = 0 AND dismissed = 0 AND expires_at IS NOT NULL
	`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	return countUnexpiredRows(rows, time.Now().UTC())
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
// The expiry test runs in Go against a UTC cutoff rather than as an SQL
// comparison against a local time.Now(): stored timestamps can still be in the
// local-zone time.Time.String() layout written before normalization, and
// comparing those lexicographically deleted notifications that had not expired.
// Rows whose expires_at is NULL or unparseable are left alone.
func (m *UserNotificationModel) CleanupExpired() (int64, error) {
	return deleteRowsWithTimestampBefore(m.DB, "user_notifications", "id", "expires_at", time.Now().UTC(), true)
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
// The three GROUP BY / aggregate queries collapse into a single scan that is
// tallied in Go: SQL cannot be trusted to decide which rows are still live, so
// every counter has to be driven by the same parsed-instant expiry test.
func (m *UserNotificationModel) GetStatistics(userID int) (*NotificationStatistics, error) {
	stats := &NotificationStatistics{
		ByType:    make(map[NotificationType]int),
		ByDisplay: make(map[NotificationDisplay]int),
	}

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT type, display, read, dismissed, expires_at FROM user_notifications
		WHERE user_id = ? AND expires_at IS NOT NULL
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if err := tallyNotificationStatistics(rows, time.Now().UTC(), stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// scanUnexpiredNotifications scans user notification rows, keeping only those
// whose expires_at parses to an instant later than now, then applies the offset
// and limit window in Go. A row whose stored timestamp cannot be parsed fails
// closed and is skipped, exactly as the SQL text comparison would have dropped
// a value it could not order.
func (m *UserNotificationModel) scanUnexpiredNotifications(rows *sql.Rows, now time.Time, offset, limit int) ([]*Notification, error) {
	if limit == 0 {
		return nil, nil
	}
	if offset < 0 {
		offset = 0
	}

	var notifications []*Notification
	skipped := 0

	for rows.Next() {
		notif := &Notification{}
		var actionJSON sql.NullString
		var storedCreatedAt, storedExpiresAt interface{}

		err := rows.Scan(&notif.ID, &notif.UserID, &notif.Type, &notif.Display, &notif.Title,
			&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &storedCreatedAt, &storedExpiresAt)
		if err != nil {
			return nil, err
		}

		expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
		if !ok || !expiresAt.After(now) {
			continue
		}

		if skipped < offset {
			skipped++
			continue
		}

		// Decode action if present
		if actionJSON.Valid && actionJSON.String != "" {
			var action NotificationAction
			if err := json.Unmarshal([]byte(actionJSON.String), &action); err == nil {
				notif.Action = &action
			}
		}

		if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
			notif.CreatedAt = parsed
		}

		notif.ExpiresAt = &expiresAt
		notifications = append(notifications, notif)

		if limit > 0 && len(notifications) >= limit {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
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
	`, id, adminID, notifType, display, title, message, actionJSON, false, false, sqlTimestamp(time.Now()), sqlTimestamp(expiresAt))

	if err != nil {
		return nil, fmt.Errorf("failed to create admin notification: %w", err)
	}

	return m.GetByID(id)
}

// GetByID retrieves an admin notification by ID
func (m *AdminNotificationModel) GetByID(id string) (*Notification, error) {
	notif := &Notification{}
	var actionJSON sql.NullString
	var storedCreatedAt, storedExpiresAt interface{}

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM server_admin_notifications WHERE id = ?
	`, id).Scan(&notif.ID, &notif.AdminID, &notif.Type, &notif.Display, &notif.Title,
		&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &storedCreatedAt, &storedExpiresAt)

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

	// Both timestamps are parsed with parseStoredTimestamp so a row written in
	// the old local-zone layout resolves to the same absolute instant as one
	// written in the canonical UTC layout.
	if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
		notif.CreatedAt = parsed
	}

	if parsed, ok := parseStoredTimestamp(storedExpiresAt); ok {
		notif.ExpiresAt = &parsed
	}

	return notif, nil
}

// GetByAdminID retrieves all notifications for an admin with pagination
// The expiry test and the LIMIT/OFFSET window both move into Go for the reason
// described on UserNotificationModel.GetByUserID - an SQL "expires_at > ?" is a
// lexicographic TEXT comparison that misreads any row written in the old
// local-zone layout. The scan still stops as soon as the window is full.
func (m *AdminNotificationModel) GetByAdminID(adminID int, limit, offset int) ([]*Notification, error) {
	query := `
		SELECT id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM server_admin_notifications
		WHERE admin_id = ? AND expires_at IS NOT NULL
		ORDER BY created_at DESC
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanUnexpiredNotifications(rows, time.Now().UTC(), offset, limit)
}

// GetUnread retrieves unread notifications for an admin
// The expiry test runs in Go for the reason described on GetByAdminID.
func (m *AdminNotificationModel) GetUnread(adminID int) ([]*Notification, error) {
	query := `
		SELECT id, admin_id, type, display, title, message, action_json, read, dismissed, created_at, expires_at
		FROM server_admin_notifications
		WHERE admin_id = ? AND read = 0 AND dismissed = 0 AND expires_at IS NOT NULL
		ORDER BY created_at DESC
	`

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, query, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanUnexpiredNotifications(rows, time.Now().UTC(), 0, noNotificationLimit)
}

// GetUnreadCount returns the count of unread notifications for an admin
// The count is accumulated in Go rather than by SQL COUNT(*) so the expiry test
// is the same instant comparison every other read in this file performs.
func (m *AdminNotificationModel) GetUnreadCount(adminID int) (int, error) {
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT expires_at FROM server_admin_notifications
		WHERE admin_id = ? AND read = 0 AND dismissed = 0 AND expires_at IS NOT NULL
	`, adminID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	return countUnexpiredRows(rows, time.Now().UTC())
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
// The expiry test runs in Go against a UTC cutoff for the same reason as
// UserNotificationModel.CleanupExpired: stored timestamps may still carry the
// writer's local zone, so a lexicographic SQL comparison deleted rows that had
// not expired. Rows whose expires_at is NULL or unparseable are left alone.
func (m *AdminNotificationModel) CleanupExpired() (int64, error) {
	return deleteRowsWithTimestampBefore(m.DB, "server_admin_notifications", "id", "expires_at", time.Now().UTC(), true)
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
// The three GROUP BY / aggregate queries collapse into a single scan tallied in
// Go for the same reason as UserNotificationModel.GetStatistics: every counter
// has to be driven by the same parsed-instant expiry test.
func (m *AdminNotificationModel) GetStatistics(adminID int) (*NotificationStatistics, error) {
	stats := &NotificationStatistics{
		ByType:    make(map[NotificationType]int),
		ByDisplay: make(map[NotificationDisplay]int),
	}

	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT type, display, read, dismissed, expires_at FROM server_admin_notifications
		WHERE admin_id = ? AND expires_at IS NOT NULL
	`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if err := tallyNotificationStatistics(rows, time.Now().UTC(), stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// scanUnexpiredNotifications scans admin notification rows, keeping only those
// whose expires_at parses to an instant later than now, then applies the offset
// and limit window in Go. A row whose stored timestamp cannot be parsed fails
// closed and is skipped.
func (m *AdminNotificationModel) scanUnexpiredNotifications(rows *sql.Rows, now time.Time, offset, limit int) ([]*Notification, error) {
	if limit == 0 {
		return nil, nil
	}
	if offset < 0 {
		offset = 0
	}

	var notifications []*Notification
	skipped := 0

	for rows.Next() {
		notif := &Notification{}
		var actionJSON sql.NullString
		var storedCreatedAt, storedExpiresAt interface{}

		err := rows.Scan(&notif.ID, &notif.AdminID, &notif.Type, &notif.Display, &notif.Title,
			&notif.Message, &actionJSON, &notif.Read, &notif.Dismissed, &storedCreatedAt, &storedExpiresAt)
		if err != nil {
			return nil, err
		}

		expiresAt, ok := parseStoredTimestamp(storedExpiresAt)
		if !ok || !expiresAt.After(now) {
			continue
		}

		if skipped < offset {
			skipped++
			continue
		}

		// Decode action if present
		if actionJSON.Valid && actionJSON.String != "" {
			var action NotificationAction
			if err := json.Unmarshal([]byte(actionJSON.String), &action); err == nil {
				notif.Action = &action
			}
		}

		if parsed, ok := parseStoredTimestamp(storedCreatedAt); ok {
			notif.CreatedAt = parsed
		}

		notif.ExpiresAt = &expiresAt
		notifications = append(notifications, notif)

		if limit > 0 && len(notifications) >= limit {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
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

	var storedUpdatedAt interface{}

	err := database.QueryRowContext(context.Background(), m.UserDB, database.TimeoutSimpleSelect, `
		SELECT enable_toast, enable_banner, enable_center, enable_sound,
		       toast_duration_success, toast_duration_info, toast_duration_warning, updated_at
		FROM user_notification_preferences
		WHERE user_id = ?
	`, userID).Scan(&prefs.EnableToast, &prefs.EnableBanner, &prefs.EnableCenter, &prefs.EnableSound,
		&prefs.ToastDurationSuccess, &prefs.ToastDurationInfo, &prefs.ToastDurationWarning, &storedUpdatedAt)

	if err == sql.ErrNoRows {
		// Return defaults if no preferences set
		prefs.UpdatedAt = time.Now().UTC()
		return prefs, nil
	}
	if err != nil {
		return nil, err
	}

	// updated_at is scanned untyped and parsed here so rows written in the old
	// local-zone layout and rows written in the canonical UTC layout both come
	// back as the same absolute instant.
	if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
		prefs.UpdatedAt = parsed
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
		prefs.ToastDurationSuccess, prefs.ToastDurationInfo, prefs.ToastDurationWarning, sqlTimestamp(time.Now()))

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

	var storedUpdatedAt interface{}

	err := database.QueryRowContext(context.Background(), m.ServerDB, database.TimeoutSimpleSelect, `
		SELECT enable_toast, enable_banner, enable_center, enable_sound,
		       toast_duration_success, toast_duration_info, toast_duration_warning, updated_at
		FROM server_admin_notification_preferences
		WHERE admin_id = ?
	`, adminID).Scan(&prefs.EnableToast, &prefs.EnableBanner, &prefs.EnableCenter, &prefs.EnableSound,
		&prefs.ToastDurationSuccess, &prefs.ToastDurationInfo, &prefs.ToastDurationWarning, &storedUpdatedAt)

	if err == sql.ErrNoRows {
		// Return defaults if no preferences set
		prefs.UpdatedAt = time.Now().UTC()
		return prefs, nil
	}
	if err != nil {
		return nil, err
	}

	// updated_at is scanned untyped and parsed here for the same mixed-layout
	// reason as GetUserPreferences.
	if parsed, ok := parseStoredTimestamp(storedUpdatedAt); ok {
		prefs.UpdatedAt = parsed
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
		prefs.ToastDurationSuccess, prefs.ToastDurationInfo, prefs.ToastDurationWarning, sqlTimestamp(time.Now()))

	return err
}
