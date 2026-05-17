package handler

import (
	"net/http"
	"strconv"

	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/service"
	"github.com/gin-gonic/gin"
)

// UserNotificationHandlers handles user notification endpoints
type UserNotificationHandlers struct {
	NotificationService *service.NotificationService
}

// NewUserNotificationHandlers creates a new user notification handlers instance
func NewUserNotificationHandlers(notificationService *service.NotificationService) *UserNotificationHandlers {
	return &UserNotificationHandlers{
		NotificationService: notificationService,
	}
}

// GetNotifications returns all notifications for the authenticated user
// GET /{api_version}/users/notifications
func (h *UserNotificationHandlers) GetNotifications(c *gin.Context) {
	// Get authenticated user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	// Validate limit
	if limit < 1 || limit > 100 {
		limit = 50
	}

	// Get notifications
	notifications, err := h.NotificationService.GetUserNotifications(userID.(int), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"limit":         limit,
		"offset":        offset,
		"count":         len(notifications),
	})
}

// GetUnreadNotifications returns unread notifications for the authenticated user
// GET /{api_version}/users/notifications/unread
func (h *UserNotificationHandlers) GetUnreadNotifications(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	notifications, err := h.NotificationService.GetUserUnreadNotifications(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve unread notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"count":         len(notifications),
	})
}

// GetUnreadCount returns the count of unread notifications
// GET /{api_version}/users/notifications/count
func (h *UserNotificationHandlers) GetUnreadCount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	count, err := h.NotificationService.GetUserUnreadCount(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get unread count"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count": count,
	})
}

// GetStatistics returns notification statistics for the authenticated user
// GET /{api_version}/users/notifications/stats
func (h *UserNotificationHandlers) GetStatistics(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	stats, err := h.NotificationService.GetUserStatistics(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// MarkAsRead marks a notification as read
// PATCH /{api_version}/users/notifications/:id/read
func (h *UserNotificationHandlers) MarkAsRead(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	notificationID := c.Param("id")
	if notificationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notification ID required"})
		return
	}

	err := h.NotificationService.MarkUserNotificationAsRead(notificationID, userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found or access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "notification marked as read",
		"id":      notificationID,
	})
}

// MarkAllAsRead marks all notifications as read
// PATCH /{api_version}/users/notifications/read
func (h *UserNotificationHandlers) MarkAllAsRead(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err := h.NotificationService.MarkAllUserNotificationsAsRead(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark notifications as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "all notifications marked as read",
	})
}

// Dismiss dismisses a notification
// PATCH /{api_version}/users/notifications/:id/dismiss
func (h *UserNotificationHandlers) Dismiss(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	notificationID := c.Param("id")
	if notificationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notification ID required"})
		return
	}

	err := h.NotificationService.DismissUserNotification(notificationID, userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found or access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "notification dismissed",
		"id":      notificationID,
	})
}

// Delete deletes a notification
// DELETE /{api_version}/users/notifications/:id
func (h *UserNotificationHandlers) Delete(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	notificationID := c.Param("id")
	if notificationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notification ID required"})
		return
	}

	err := h.NotificationService.DeleteUserNotification(notificationID, userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found or access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "notification deleted",
		"id":      notificationID,
	})
}

// GetPreferences returns notification preferences for the authenticated user
// GET /{api_version}/users/notifications/preferences
func (h *UserNotificationHandlers) GetPreferences(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	prefs, err := h.NotificationService.GetUserPreferences(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get preferences"})
		return
	}

	c.JSON(http.StatusOK, prefs)
}

// UpdatePreferences updates notification preferences for the authenticated user
// PATCH /{api_version}/users/notifications/preferences
func (h *UserNotificationHandlers) UpdatePreferences(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var prefs models.NotificationPreferences
	if err := c.ShouldBindJSON(&prefs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate toast durations (1-60 seconds)
	if prefs.ToastDurationSuccess < 1 || prefs.ToastDurationSuccess > 60 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "toast_duration_success must be between 1 and 60 seconds"})
		return
	}
	if prefs.ToastDurationInfo < 1 || prefs.ToastDurationInfo > 60 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "toast_duration_info must be between 1 and 60 seconds"})
		return
	}
	if prefs.ToastDurationWarning < 1 || prefs.ToastDurationWarning > 60 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "toast_duration_warning must be between 1 and 60 seconds"})
		return
	}

	err := h.NotificationService.UpdateUserPreferences(userID.(int), &prefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update preferences"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "preferences updated successfully",
	})
}

// RegisterUserNotificationRoutes registers all user notification routes
func RegisterUserNotificationRoutes(router *gin.RouterGroup, handlers *UserNotificationHandlers) {
	notifications := router.Group("/notifications")
	{
		// List and retrieve
		notifications.GET("", handlers.GetNotifications)
		notifications.GET("/unread", handlers.GetUnreadNotifications)
		notifications.GET("/count", handlers.GetUnreadCount)
		notifications.GET("/stats", handlers.GetStatistics)

		// Mark as read
		notifications.PATCH("/:id/read", handlers.MarkAsRead)
		notifications.PATCH("/read", handlers.MarkAllAsRead)

		// Dismiss and delete
		notifications.PATCH("/:id/dismiss", handlers.Dismiss)
		notifications.DELETE("/:id", handlers.Delete)

		// Preferences
		notifications.GET("/preferences", handlers.GetPreferences)
		notifications.PATCH("/preferences", handlers.UpdatePreferences)
	}
}
