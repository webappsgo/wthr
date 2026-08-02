package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	DB *sql.DB
}

// Notification represents a user notification
type Notification struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Link      string    `json:"link,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// authedUserID returns the authenticated user's id from context. Unlike
// c.GetInt, which silently returns 0 when the key is absent, this reports
// whether a valid caller was actually authenticated so handlers can reject
// unauthenticated requests instead of silently scoping them to user_id=0.
func authedUserID(c *gin.Context) (int, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := val.(int)
	if !ok {
		return 0, false
	}
	return id, true
}

// ListNotifications returns all notifications for the current user
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	userID, ok := authedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get pagination params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	// Get unread filter
	unreadOnly := c.DefaultQuery("unread", "false") == "true"

	// Build query
	query := `
		SELECT id, user_id, type, title, message, link, read, created_at
		FROM notifications
		WHERE user_id = ?
	`
	args := []interface{}{userID}

	if unreadOnly {
		query += " AND read = 0"
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications"})
		return
	}
	defer rows.Close()

	notifications := []Notification{}
	for rows.Next() {
		var n Notification
		var link sql.NullString
		var readInt int

		err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Message, &link, &readInt, &n.CreatedAt)
		if err != nil {
			continue
		}

		n.Read = readInt == 1
		if link.Valid {
			n.Link = link.String
		}

		notifications = append(notifications, n)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM notifications WHERE user_id = ?"
	countArgs := []interface{}{userID}
	if unreadOnly {
		countQuery += " AND read = 0"
	}
	_ = database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, countQuery, countArgs...).Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"total":         total,
		"page":          page,
		"limit":         limit,
	})
}

// GetUnreadCount returns the count of unread notifications
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	userID, ok := authedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var count int
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read = 0", userID).Scan(&count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get unread count"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"unread_count": count,
	})
}

// MarkAsRead marks a notification as read
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userID, ok := authedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	notificationID := c.Param("id")

	// Verify ownership
	var ownerID int
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT user_id FROM notifications WHERE id = ?", notificationID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	if ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Mark as read
	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, "UPDATE notifications SET read = 1 WHERE id = ?", notificationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notification as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkAllAsRead marks all notifications as read for the current user
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	userID, ok := authedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, "UPDATE notifications SET read = 1 WHERE user_id = ? AND read = 0", userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notifications as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteNotification deletes a notification
func (h *NotificationHandler) DeleteNotification(c *gin.Context) {
	userID, ok := authedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	notificationID := c.Param("id")

	// Verify ownership
	var ownerID int
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT user_id FROM notifications WHERE id = ?", notificationID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	if ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Delete
	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, "DELETE FROM notifications WHERE id = ?", notificationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CreateNotification creates a new notification (internal use)
func (h *NotificationHandler) CreateNotification(userID int, notifType, title, message, link string) error {
	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO notifications (user_id, type, title, message, link, read)
		VALUES (?, ?, ?, ?, ?, 0)
	`, userID, notifType, title, message, sql.NullString{String: link, Valid: link != ""})

	return err
}

// ShowNotificationsPage renders the notifications page
func (h *NotificationHandler) ShowNotificationsPage(c *gin.Context) {
	userRole := c.GetString("user_role")

	NegotiateResponse(c, "page/notifications.tmpl", util.TemplateData(c, gin.H{
		"IsAdmin": userRole == "admin",
		"title":   "Notifications",
		"page":    "notifications",
	}))
}
