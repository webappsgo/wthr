package handler

import (
	"net/http"
	"strconv"

	"github.com/casapps/wthr/src/server/service"

	"github.com/gin-gonic/gin"
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
func (h *NotificationMetricsHandler) GetSummary(c *gin.Context) {
	summary, err := h.metrics.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get metrics summary",
		})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetChannelMetrics returns metrics for a specific channel
// GET /api/admin/metrics/notifications/channels/:type
func (h *NotificationMetricsHandler) GetChannelMetrics(c *gin.Context) {
	channelType := c.Param("type")

	metrics, err := h.metrics.GetChannelMetrics(channelType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get channel metrics",
		})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetRecentErrors returns recent notification errors
// GET /api/admin/metrics/notifications/errors
func (h *NotificationMetricsHandler) GetRecentErrors(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	errors, err := h.metrics.GetRecentErrors(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get recent errors",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"errors": errors,
		"count":  len(errors),
	})
}

// GetHealthStatus returns the health status of the notification system
// GET /api/admin/metrics/notifications/health
func (h *NotificationMetricsHandler) GetHealthStatus(c *gin.Context) {
	status := h.metrics.GetHealthStatus()
	c.JSON(http.StatusOK, status)
}
