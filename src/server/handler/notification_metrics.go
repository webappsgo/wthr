package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/server/service"
)

// NotificationMetricsHandler handles notification metrics API endpoints
type NotificationMetricsHandler struct {
	metrics *service.NotificationMetrics
}

// NewNotificationMetricsHandler creates a new notification metrics handler
func NewNotificationMetricsHandler(metrics *service.NotificationMetrics) *NotificationMetricsHandler {
	return &NotificationMetricsHandler{
		metrics: metrics,
	}
}

// GetSummary returns overall notification system metrics
// GET /api/admin/metrics/notifications/summary
func (h *NotificationMetricsHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.metrics.GetSummary()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get metrics summary",
		})
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GetChannelMetrics returns metrics for a specific channel
// GET /api/admin/metrics/notifications/channels/{type}
func (h *NotificationMetricsHandler) GetChannelMetrics(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	metrics, err := h.metrics.GetChannelMetrics(channelType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get channel metrics",
		})
		return
	}

	writeJSON(w, http.StatusOK, metrics)
}

// GetRecentErrors returns recent notification errors
// GET /api/admin/metrics/notifications/errors
func (h *NotificationMetricsHandler) GetRecentErrors(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	errors, err := h.metrics.GetRecentErrors(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get recent errors",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"errors": errors,
		"count":  len(errors),
	})
}

// GetHealthStatus returns the health status of the notification system
// GET /api/admin/metrics/notifications/health
func (h *NotificationMetricsHandler) GetHealthStatus(w http.ResponseWriter, r *http.Request) {
	status := h.metrics.GetHealthStatus()
	writeJSON(w, http.StatusOK, status)
}
