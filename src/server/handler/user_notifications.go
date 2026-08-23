package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
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
func (h *UserNotificationHandlers) GetNotifications(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user ID from context
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "50"
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = "0"
	}
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	// Validate limit
	if limit < 1 || limit > 100 {
		limit = 50
	}

	// Get notifications
	notifications, err := h.NotificationService.GetUserNotifications(userID.(int), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to retrieve notifications"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
		"limit":         limit,
		"offset":        offset,
		"count":         len(notifications),
	})
}

// GetUnreadNotifications returns unread notifications for the authenticated user
// GET /{api_version}/users/notifications/unread
func (h *UserNotificationHandlers) GetUnreadNotifications(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	notifications, err := h.NotificationService.GetUserUnreadNotifications(userID.(int))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to retrieve unread notifications"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
		"count":         len(notifications),
	})
}

// GetUnreadCount returns the count of unread notifications
// GET /{api_version}/users/notifications/count
func (h *UserNotificationHandlers) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	count, err := h.NotificationService.GetUserUnreadCount(userID.(int))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to get unread count"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": count,
	})
}

// GetStatistics returns notification statistics for the authenticated user
// GET /{api_version}/users/notifications/stats
func (h *UserNotificationHandlers) GetStatistics(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	stats, err := h.NotificationService.GetUserStatistics(userID.(int))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to get statistics"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// MarkAsRead marks a notification as read
// PATCH /{api_version}/users/notifications/{id}/read
func (h *UserNotificationHandlers) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "notification ID required"})
		return
	}

	err := h.NotificationService.MarkUserNotificationAsRead(notificationID, userID.(int))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "notification not found or access denied"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "notification marked as read",
		"id":      notificationID,
	})
}

// MarkAllAsRead marks all notifications as read
// PATCH /{api_version}/users/notifications/read
func (h *UserNotificationHandlers) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	err := h.NotificationService.MarkAllUserNotificationsAsRead(userID.(int))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to mark notifications as read"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "all notifications marked as read",
	})
}

// Dismiss dismisses a notification
// PATCH /{api_version}/users/notifications/{id}/dismiss
func (h *UserNotificationHandlers) Dismiss(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "notification ID required"})
		return
	}

	err := h.NotificationService.DismissUserNotification(notificationID, userID.(int))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "notification not found or access denied"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "notification dismissed",
		"id":      notificationID,
	})
}

// Delete deletes a notification
// DELETE /{api_version}/users/notifications/{id}
func (h *UserNotificationHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "notification ID required"})
		return
	}

	err := h.NotificationService.DeleteUserNotification(notificationID, userID.(int))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "notification not found or access denied"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "notification deleted",
		"id":      notificationID,
	})
}

// GetPreferences returns notification preferences for the authenticated user
// GET /{api_version}/users/notifications/preferences
func (h *UserNotificationHandlers) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	prefs, err := h.NotificationService.GetUserPreferences(userID.(int))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to get preferences"})
		return
	}

	writeJSON(w, http.StatusOK, prefs)
}

// UpdatePreferences updates notification preferences for the authenticated user
// PATCH /{api_version}/users/notifications/preferences
func (h *UserNotificationHandlers) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
		return
	}

	var prefs model.NotificationPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid request body"})
		return
	}

	// Validate toast durations (1-60 seconds)
	if prefs.ToastDurationSuccess < 1 || prefs.ToastDurationSuccess > 60 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "toast_duration_success must be between 1 and 60 seconds"})
		return
	}
	if prefs.ToastDurationInfo < 1 || prefs.ToastDurationInfo > 60 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "toast_duration_info must be between 1 and 60 seconds"})
		return
	}
	if prefs.ToastDurationWarning < 1 || prefs.ToastDurationWarning > 60 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "toast_duration_warning must be between 1 and 60 seconds"})
		return
	}

	err := h.NotificationService.UpdateUserPreferences(userID.(int), &prefs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to update preferences"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "preferences updated successfully",
	})
}

// RegisterUserNotificationRoutes registers all user notification routes
func RegisterUserNotificationRoutes(router chi.Router, handlers *UserNotificationHandlers) {
	router.Route("/notifications", func(notifications chi.Router) {
		// List and retrieve
		notifications.Get("/", handlers.GetNotifications)
		notifications.Get("/unread", handlers.GetUnreadNotifications)
		notifications.Get("/count", handlers.GetUnreadCount)
		notifications.Get("/stats", handlers.GetStatistics)

		// Mark as read
		notifications.Patch("/{id}/read", handlers.MarkAsRead)
		notifications.Patch("/read", handlers.MarkAllAsRead)

		// Dismiss and delete
		notifications.Patch("/{id}/dismiss", handlers.Dismiss)
		notifications.Delete("/{id}", handlers.Delete)

		// Preferences
		notifications.Get("/preferences", handlers.GetPreferences)
		notifications.Patch("/preferences", handlers.UpdatePreferences)
	})
}
