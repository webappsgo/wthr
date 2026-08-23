package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/mode"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
)

// Note: Uses standard response helpers from response.go

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Implement proper origin checking per AI.md PART 17
		// Development mode: allow all origins
		if !mode.IsAppModeProd() {
			return true
		}

		// Production mode: check against configured CORS origins
		origin := r.Header.Get("Origin")
		if origin == "" {
			// No origin header - allow (same-origin)
			return true
		}

		// Load config to check allowed origins
		cfg, err := config.LoadConfig()
		if err != nil || cfg == nil {
			// Config load failed - reject for safety
			return false
		}

		// Check if CORS is set to allow all
		if cfg.Web.CORS == "*" {
			return true
		}

		// Check if origin is in allowed CORS list
		if cfg.Web.CORS != "" {
			allowedOrigins := strings.Split(cfg.Web.CORS, ",")
			for _, allowed := range allowedOrigins {
				if strings.TrimSpace(allowed) == origin {
					return true
				}
			}
		}

		// Origin not allowed
		return false
	},
}

// NotificationAPIHandlers handles all notification API endpoints
type NotificationAPIHandlers struct {
	NotificationService *service.NotificationService
	WSHub               *service.WebSocketHub
}

// NewNotificationAPIHandlers creates a new notification API handlers instance
func NewNotificationAPIHandlers(notificationService *service.NotificationService, wsHub *service.WebSocketHub) *NotificationAPIHandlers {
	return &NotificationAPIHandlers{
		NotificationService: notificationService,
		WSHub:               wsHub,
	}
}

// ========== USER NOTIFICATION ENDPOINTS ==========

// GetUserNotifications returns all notifications for the authenticated user
// GET /{api_version}/users/notifications
func (h *NotificationAPIHandlers) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

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

	if limit < 1 || limit > 100 {
		limit = 50
	}

	notifications, err := h.NotificationService.GetUserNotifications(userID, limit, offset)
	if err != nil {
		InternalError(w, r, "failed to retrieve notifications")
		return
	}

	RespondData(w, r, map[string]interface{}{
		"notifications": notifications,
		"limit":         limit,
		"offset":        offset,
		"count":         len(notifications),
	})
}

// GetUserUnreadNotifications returns unread notifications for the authenticated user
// GET /{api_version}/users/notifications/unread
func (h *NotificationAPIHandlers) GetUserUnreadNotifications(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notifications, err := h.NotificationService.GetUserUnreadNotifications(userID)
	if err != nil {
		InternalError(w, r, "failed to retrieve unread notifications")
		return
	}

	RespondData(w, r, map[string]interface{}{
		"notifications": notifications,
		"count":         len(notifications),
	})
}

// GetUserUnreadCount returns the count of unread notifications
// GET /{api_version}/users/notifications/count
func (h *NotificationAPIHandlers) GetUserUnreadCount(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	count, err := h.NotificationService.GetUserUnreadCount(userID)
	if err != nil {
		InternalError(w, r, "failed to get unread count")
		return
	}

	RespondData(w, r, map[string]interface{}{
		"count": count,
	})
}

// GetUserNotificationStats returns notification statistics
// GET /{api_version}/users/notifications/stats
func (h *NotificationAPIHandlers) GetUserNotificationStats(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	stats, err := h.NotificationService.GetUserStatistics(userID)
	if err != nil {
		InternalError(w, r, "failed to get statistics")
		return
	}

	RespondData(w, r, stats)
}

// MarkUserNotificationRead marks a notification as read
// PATCH /{api_version}/users/notifications/{id}/read
func (h *NotificationAPIHandlers) MarkUserNotificationRead(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		BadRequest(w, r, "notification ID required")
		return
	}

	err := h.NotificationService.MarkUserNotificationAsRead(notificationID, userID)
	if err != nil {
		NotFound(w, r, "notification not found or access denied")
		return
	}

	RespondSuccess(w, r, "notification marked as read", map[string]interface{}{
		"id": notificationID,
	})
}

// MarkAllUserNotificationsRead marks all notifications as read
// PATCH /{api_version}/users/notifications/read
func (h *NotificationAPIHandlers) MarkAllUserNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	err := h.NotificationService.MarkAllUserNotificationsAsRead(userID)
	if err != nil {
		InternalError(w, r, "failed to mark notifications as read")
		return
	}

	RespondSuccess(w, r, "all notifications marked as read")
}

// DismissUserNotification dismisses a notification
// PATCH /{api_version}/users/notifications/{id}/dismiss
func (h *NotificationAPIHandlers) DismissUserNotification(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		BadRequest(w, r, "notification ID required")
		return
	}

	err := h.NotificationService.DismissUserNotification(notificationID, userID)
	if err != nil {
		NotFound(w, r, "notification not found or access denied")
		return
	}

	RespondSuccess(w, r, "notification dismissed", map[string]interface{}{
		"id": notificationID,
	})
}

// DeleteUserNotification deletes a notification
// DELETE /{api_version}/users/notifications/{id}
func (h *NotificationAPIHandlers) DeleteUserNotification(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		BadRequest(w, r, "notification ID required")
		return
	}

	err := h.NotificationService.DeleteUserNotification(notificationID, userID)
	if err != nil {
		NotFound(w, r, "notification not found or access denied")
		return
	}

	RespondSuccess(w, r, "notification deleted", map[string]interface{}{
		"id": notificationID,
	})
}

// GetUserNotificationPreferences returns notification preferences
// GET /{api_version}/users/notifications/preferences
func (h *NotificationAPIHandlers) GetUserNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	prefs, err := h.NotificationService.GetUserPreferences(userID)
	if err != nil {
		InternalError(w, r, "failed to get preferences")
		return
	}

	RespondData(w, r, prefs)
}

// UpdateUserNotificationPreferences updates notification preferences
// PATCH /{api_version}/users/notifications/preferences
func (h *NotificationAPIHandlers) UpdateUserNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	userIDVal, exists := reqctx.Get(r.Context(), "user_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		Unauthorized(w, r, "unauthorized")
		return
	}

	var prefs model.NotificationPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		BadRequest(w, r, "invalid request body")
		return
	}

	// Validate toast durations (1-60 seconds)
	if prefs.ToastDurationSuccess < 1 || prefs.ToastDurationSuccess > 60 ||
		prefs.ToastDurationInfo < 1 || prefs.ToastDurationInfo > 60 ||
		prefs.ToastDurationWarning < 1 || prefs.ToastDurationWarning > 60 {
		BadRequest(w, r, "toast durations must be between 1 and 60 seconds")
		return
	}

	err := h.NotificationService.UpdateUserPreferences(userID, &prefs)
	if err != nil {
		InternalError(w, r, "failed to update preferences")
		return
	}

	RespondSuccess(w, r, "preferences updated successfully")
}

// ========== ADMIN NOTIFICATION ENDPOINTS ==========

// GetAdminNotifications returns all notifications for the authenticated admin
// GET /{api_version}/admin/notifications
func (h *NotificationAPIHandlers) GetAdminNotifications(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

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

	if limit < 1 || limit > 100 {
		limit = 50
	}

	notifications, err := h.NotificationService.GetAdminNotifications(adminID.(int), limit, offset)
	if err != nil {
		InternalError(w, r, "failed to retrieve notifications")
		return
	}

	RespondData(w, r, map[string]interface{}{
		"notifications": notifications,
		"limit":         limit,
		"offset":        offset,
		"count":         len(notifications),
	})
}

// GetAdminUnreadNotifications returns unread notifications for the authenticated admin
// GET /{api_version}/admin/notifications/unread
func (h *NotificationAPIHandlers) GetAdminUnreadNotifications(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notifications, err := h.NotificationService.GetAdminUnreadNotifications(adminID.(int))
	if err != nil {
		InternalError(w, r, "failed to retrieve unread notifications")
		return
	}

	RespondData(w, r, map[string]interface{}{
		"notifications": notifications,
		"count":         len(notifications),
	})
}

// GetAdminUnreadCount returns the count of unread notifications
// GET /{api_version}/admin/notifications/count
func (h *NotificationAPIHandlers) GetAdminUnreadCount(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	count, err := h.NotificationService.GetAdminUnreadCount(adminID.(int))
	if err != nil {
		InternalError(w, r, "failed to get unread count")
		return
	}

	RespondData(w, r, map[string]interface{}{
		"count": count,
	})
}

// GetAdminNotificationStats returns notification statistics
// GET /{api_version}/admin/notifications/stats
func (h *NotificationAPIHandlers) GetAdminNotificationStats(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	stats, err := h.NotificationService.GetAdminStatistics(adminID.(int))
	if err != nil {
		InternalError(w, r, "failed to get statistics")
		return
	}

	RespondData(w, r, stats)
}

// MarkAdminNotificationRead marks a notification as read
// PATCH /{api_version}/admin/notifications/{id}/read
func (h *NotificationAPIHandlers) MarkAdminNotificationRead(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		BadRequest(w, r, "notification ID required")
		return
	}

	err := h.NotificationService.MarkAdminNotificationAsRead(notificationID, adminID.(int))
	if err != nil {
		NotFound(w, r, "notification not found or access denied")
		return
	}

	RespondSuccess(w, r, "notification marked as read", map[string]interface{}{
		"id": notificationID,
	})
}

// MarkAllAdminNotificationsRead marks all notifications as read
// PATCH /{api_version}/admin/notifications/read
func (h *NotificationAPIHandlers) MarkAllAdminNotificationsRead(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	err := h.NotificationService.MarkAllAdminNotificationsAsRead(adminID.(int))
	if err != nil {
		InternalError(w, r, "failed to mark notifications as read")
		return
	}

	RespondSuccess(w, r, "all notifications marked as read")
}

// DismissAdminNotification dismisses a notification
// PATCH /{api_version}/admin/notifications/{id}/dismiss
func (h *NotificationAPIHandlers) DismissAdminNotification(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		BadRequest(w, r, "notification ID required")
		return
	}

	err := h.NotificationService.DismissAdminNotification(notificationID, adminID.(int))
	if err != nil {
		NotFound(w, r, "notification not found or access denied")
		return
	}

	RespondSuccess(w, r, "notification dismissed", map[string]interface{}{
		"id": notificationID,
	})
}

// DeleteAdminNotification deletes a notification
// DELETE /{api_version}/admin/notifications/{id}
func (h *NotificationAPIHandlers) DeleteAdminNotification(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	notificationID := chi.URLParam(r, "id")
	if notificationID == "" {
		BadRequest(w, r, "notification ID required")
		return
	}

	err := h.NotificationService.DeleteAdminNotification(notificationID, adminID.(int))
	if err != nil {
		NotFound(w, r, "notification not found or access denied")
		return
	}

	RespondSuccess(w, r, "notification deleted", map[string]interface{}{
		"id": notificationID,
	})
}

// GetAdminNotificationPreferences returns notification preferences
// GET /{api_version}/admin/notifications/preferences
func (h *NotificationAPIHandlers) GetAdminNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	prefs, err := h.NotificationService.GetAdminPreferences(adminID.(int))
	if err != nil {
		InternalError(w, r, "failed to get preferences")
		return
	}

	RespondData(w, r, prefs)
}

// UpdateAdminNotificationPreferences updates notification preferences
// PATCH /{api_version}/admin/notifications/preferences
func (h *NotificationAPIHandlers) UpdateAdminNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	var prefs model.NotificationPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		BadRequest(w, r, "invalid request body")
		return
	}

	// Validate toast durations (1-60 seconds)
	if prefs.ToastDurationSuccess < 1 || prefs.ToastDurationSuccess > 60 ||
		prefs.ToastDurationInfo < 1 || prefs.ToastDurationInfo > 60 ||
		prefs.ToastDurationWarning < 1 || prefs.ToastDurationWarning > 60 {
		BadRequest(w, r, "toast durations must be between 1 and 60 seconds")
		return
	}

	err := h.NotificationService.UpdateAdminPreferences(adminID.(int), &prefs)
	if err != nil {
		InternalError(w, r, "failed to update preferences")
		return
	}

	RespondSuccess(w, r, "preferences updated successfully")
}

// SendTestNotification sends a test notification to the authenticated admin
// POST /{api_version}/admin/notifications/send
func (h *NotificationAPIHandlers) SendTestNotification(w http.ResponseWriter, r *http.Request) {
	adminID, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	var req struct {
		Type    model.NotificationType    `json:"type"`
		Display model.NotificationDisplay `json:"display"`
		Title   string                    `json:"title"`
		Message string                    `json:"message"`
		Action  *model.NotificationAction `json:"action,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, "invalid request body")
		return
	}

	if req.Type == "" || req.Display == "" || req.Title == "" || req.Message == "" {
		BadRequest(w, r, "invalid request body")
		return
	}

	// Validate notification type
	validTypes := map[model.NotificationType]bool{
		model.NotificationTypeSuccess:  true,
		model.NotificationTypeInfo:     true,
		model.NotificationTypeWarning:  true,
		model.NotificationTypeError:    true,
		model.NotificationTypeSecurity: true,
	}
	if !validTypes[req.Type] {
		BadRequest(w, r, "invalid notification type")
		return
	}

	// Validate display type
	validDisplays := map[model.NotificationDisplay]bool{
		model.NotificationDisplayToast:  true,
		model.NotificationDisplayBanner: true,
		model.NotificationDisplayCenter: true,
	}
	if !validDisplays[req.Display] {
		BadRequest(w, r, "invalid display type")
		return
	}

	// Send test notification
	notification, err := h.NotificationService.SendAdminNotification(
		adminID.(int),
		req.Type,
		req.Display,
		req.Title,
		req.Message,
		req.Action,
	)
	if err != nil {
		InternalError(w, r, "failed to send test notification")
		return
	}

	RespondCreated(w, r, "test notification sent successfully", "", map[string]interface{}{
		"notification": notification,
	})
}

// ========== WEBSOCKET ENDPOINT ==========

// HandleWebSocketConnection handles WebSocket connections for real-time notifications
// GET /ws/notifications
func (h *NotificationAPIHandlers) HandleWebSocketConnection(w http.ResponseWriter, r *http.Request) {
	// Check if user or admin is authenticated
	userIDRaw, userExists := reqctx.Get(r.Context(), "user_id")
	adminIDRaw, adminExists := reqctx.Get(r.Context(), "admin_id")

	if !userExists && !adminExists {
		Unauthorized(w, r, "unauthorized")
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		InternalError(w, r, "failed to upgrade connection")
		return
	}

	// Create WebSocket client
	var client *service.WebSocketClient
	if userExists {
		userIDInt, ok := userIDRaw.(int)
		if !ok {
			InternalError(w, r, "invalid user context")
			return
		}
		client = &service.WebSocketClient{
			ID:       fmt.Sprintf("user-%d", userIDInt),
			Conn:     conn,
			Hub:      h.WSHub,
			Send:     make(chan []byte, 256),
			UserID:   &userIDInt,
			LastPing: time.Now(),
		}
	} else {
		adminIDInt, ok := adminIDRaw.(int)
		if !ok {
			InternalError(w, r, "invalid admin context")
			return
		}
		client = &service.WebSocketClient{
			ID:       fmt.Sprintf("admin-%d", adminIDInt),
			Conn:     conn,
			Hub:      h.WSHub,
			Send:     make(chan []byte, 256),
			AdminID:  &adminIDInt,
			LastPing: time.Now(),
		}
	}

	// Register client with hub
	h.WSHub.RegisterClient(client)

	// Start read and write pumps
	go client.WritePump()
	go client.ReadPump()
}

// ========== METHOD ALIASES FOR BACKWARDS COMPATIBILITY ==========

// GetUserStats is an alias for GetUserNotificationStats
func (h *NotificationAPIHandlers) GetUserStats(w http.ResponseWriter, r *http.Request) {
	h.GetUserNotificationStats(w, r)
}

// GetUserPreferences is an alias for GetUserNotificationPreferences
func (h *NotificationAPIHandlers) GetUserPreferences(w http.ResponseWriter, r *http.Request) {
	h.GetUserNotificationPreferences(w, r)
}

// UpdateUserPreferences is an alias for UpdateUserNotificationPreferences
func (h *NotificationAPIHandlers) UpdateUserPreferences(w http.ResponseWriter, r *http.Request) {
	h.UpdateUserNotificationPreferences(w, r)
}

// GetAdminStats is an alias for GetAdminNotificationStats
func (h *NotificationAPIHandlers) GetAdminStats(w http.ResponseWriter, r *http.Request) {
	h.GetAdminNotificationStats(w, r)
}

// GetAdminPreferences is an alias for GetAdminNotificationPreferences
func (h *NotificationAPIHandlers) GetAdminPreferences(w http.ResponseWriter, r *http.Request) {
	h.GetAdminNotificationPreferences(w, r)
}

// UpdateAdminPreferences is an alias for UpdateAdminNotificationPreferences
func (h *NotificationAPIHandlers) UpdateAdminPreferences(w http.ResponseWriter, r *http.Request) {
	h.UpdateAdminNotificationPreferences(w, r)
}

// RegisterNotificationAPIRoutes registers all notification API routes
func RegisterNotificationAPIRoutes(router chi.Router, handlers *NotificationAPIHandlers, userAuth, adminAuth func(http.Handler) http.Handler) {
	// WebSocket endpoint (must be registered before /api/v1 to avoid conflicts)
	router.Get("/ws/notifications", handlers.HandleWebSocketConnection)

	// User notification endpoints
	router.Route("/api/v1/user", func(userAPI chi.Router) {
		if userAuth != nil {
			userAPI.Use(userAuth)
		}
		userAPI.Route("/notifications", func(notifications chi.Router) {
			notifications.Get("/", handlers.GetUserNotifications)
			notifications.Get("/unread", handlers.GetUserUnreadNotifications)
			notifications.Get("/count", handlers.GetUserUnreadCount)
			notifications.Get("/stats", handlers.GetUserNotificationStats)

			notifications.Patch("/{id}/read", handlers.MarkUserNotificationRead)
			notifications.Patch("/read", handlers.MarkAllUserNotificationsRead)

			notifications.Patch("/{id}/dismiss", handlers.DismissUserNotification)
			notifications.Delete("/{id}", handlers.DeleteUserNotification)

			notifications.Get("/preferences", handlers.GetUserNotificationPreferences)
			notifications.Patch("/preferences", handlers.UpdateUserNotificationPreferences)
		})
	})

	// Admin notification endpoints
	router.Route("/api/v1/server/admin", func(adminAPI chi.Router) {
		if adminAuth != nil {
			adminAPI.Use(adminAuth)
		}
		adminAPI.Route("/notifications", func(notifications chi.Router) {
			notifications.Get("/", handlers.GetAdminNotifications)
			notifications.Get("/unread", handlers.GetAdminUnreadNotifications)
			notifications.Get("/count", handlers.GetAdminUnreadCount)
			notifications.Get("/stats", handlers.GetAdminNotificationStats)

			notifications.Patch("/{id}/read", handlers.MarkAdminNotificationRead)
			notifications.Patch("/read", handlers.MarkAllAdminNotificationsRead)

			notifications.Patch("/{id}/dismiss", handlers.DismissAdminNotification)
			notifications.Delete("/{id}", handlers.DeleteAdminNotification)

			notifications.Get("/preferences", handlers.GetAdminNotificationPreferences)
			notifications.Patch("/preferences", handlers.UpdateAdminNotificationPreferences)

			notifications.Post("/send", handlers.SendTestNotification)
		})
	})
}
