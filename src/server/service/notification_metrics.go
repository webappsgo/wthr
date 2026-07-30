package service

import (
	"database/sql"
	"fmt"
	"time"
)

// NotificationMetrics provides metrics and statistics for the notification system
type NotificationMetrics struct {
	db *sql.DB
}

// MetricsSummary contains aggregate notification metrics
type MetricsSummary struct {
	TotalNotifications int64              `json:"total_notifications"`
	ByState            map[string]int64   `json:"by_state"`
	ByChannel          map[string]int64   `json:"by_channel"`
	ByPriority         map[string]int64   `json:"by_priority"`
	QueueDepth         int64              `json:"queue_depth"`
	AvgDeliveryTime    float64            `json:"avg_delivery_time_seconds"`
	DeliveryRate       float64            `json:"delivery_rate_percent"`
	ErrorRate          float64            `json:"error_rate_percent"`
	Last24Hours        *TimePeriodMetrics `json:"last_24_hours"`
	Last7Days          *TimePeriodMetrics `json:"last_7_days"`
}

// TimePeriodMetrics contains metrics for a specific time period
type TimePeriodMetrics struct {
	Total     int64            `json:"total"`
	Delivered int64            `json:"delivered"`
	Failed    int64            `json:"failed"`
	Pending   int64            `json:"pending"`
	ByChannel map[string]int64 `json:"by_channel"`
}

// ChannelMetrics contains metrics for a specific channel
type ChannelMetrics struct {
	ChannelType     string  `json:"channel_type"`
	Total           int64   `json:"total"`
	Delivered       int64   `json:"delivered"`
	Failed          int64   `json:"failed"`
	Pending         int64   `json:"pending"`
	DeliveryRate    float64 `json:"delivery_rate_percent"`
	AvgDeliveryTime float64 `json:"avg_delivery_time_seconds"`
}

// NewNotificationMetrics creates a new notification metrics service
func NewNotificationMetrics(db *sql.DB) *NotificationMetrics {
	return &NotificationMetrics{
		db: db,
	}
}

// GetSummary returns overall notification system metrics
func (nm *NotificationMetrics) GetSummary() (*MetricsSummary, error) {
	summary := &MetricsSummary{
		ByState:    make(map[string]int64),
		ByChannel:  make(map[string]int64),
		ByPriority: make(map[string]int64),
	}

	// Get total notifications
	err := nm.db.QueryRow("SELECT COUNT(*) FROM notification_queue").Scan(&summary.TotalNotifications)
	if err != nil {
		return nil, err
	}

	// Get counts by state
	rows, err := nm.db.Query(`
		SELECT state, COUNT(*) as count
		FROM notification_queue
		GROUP BY state
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var state string
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			continue
		}
		summary.ByState[state] = count
	}

	// Get counts by channel
	rows, err = nm.db.Query(`
		SELECT channel_type, COUNT(*) as count
		FROM notification_queue
		GROUP BY channel_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var channel string
		var count int64
		if err := rows.Scan(&channel, &count); err != nil {
			continue
		}
		summary.ByChannel[channel] = count
	}

	// Get counts by priority
	rows, err = nm.db.Query(`
		SELECT
			CASE
				WHEN priority = 1 THEN 'low'
				WHEN priority = 2 THEN 'normal'
				WHEN priority = 3 THEN 'high'
				WHEN priority = 4 THEN 'critical'
				ELSE 'unknown'
			END as priority_label,
			COUNT(*) as count
		FROM notification_queue
		GROUP BY priority
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var priority string
		var count int64
		if err := rows.Scan(&priority, &count); err != nil {
			continue
		}
		summary.ByPriority[priority] = count
	}

	// Get queue depth (queued + sending)
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state IN ('queued', 'sending')
	`).Scan(&summary.QueueDepth)
	if err != nil {
		summary.QueueDepth = 0
	}

	// Get average delivery time
	err = nm.db.QueryRow(`
		SELECT AVG(
			CAST((julianday(delivered_at) - julianday(created_at)) * 86400 AS REAL)
		)
		FROM notification_queue
		WHERE state = 'delivered' AND delivered_at IS NOT NULL
	`).Scan(&summary.AvgDeliveryTime)
	if err != nil {
		summary.AvgDeliveryTime = 0
	}

	// Calculate delivery and error rates
	delivered := summary.ByState["delivered"]
	failed := summary.ByState["failed"]
	total := delivered + failed
	if total > 0 {
		summary.DeliveryRate = float64(delivered) / float64(total) * 100
		summary.ErrorRate = float64(failed) / float64(total) * 100
	}

	// Get 24 hour metrics
	summary.Last24Hours, _ = nm.GetTimePeriodMetrics(24 * time.Hour)

	// Get 7 day metrics
	summary.Last7Days, _ = nm.GetTimePeriodMetrics(7 * 24 * time.Hour)

	return summary, nil
}

// GetTimePeriodMetrics returns metrics for a specific time period
func (nm *NotificationMetrics) GetTimePeriodMetrics(duration time.Duration) (*TimePeriodMetrics, error) {
	metrics := &TimePeriodMetrics{
		ByChannel: make(map[string]int64),
	}

	since := time.Now().Add(-duration)

	// Get total in period
	err := nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE created_at >= datetime(?, 'unixepoch')
	`, since.Unix()).Scan(&metrics.Total)
	if err != nil {
		return nil, err
	}

	// Get delivered in period
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state = 'delivered' AND created_at >= datetime(?, 'unixepoch')
	`, since.Unix()).Scan(&metrics.Delivered)
	if err != nil {
		metrics.Delivered = 0
	}

	// Get failed in period
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state = 'failed' AND created_at >= datetime(?, 'unixepoch')
	`, since.Unix()).Scan(&metrics.Failed)
	if err != nil {
		metrics.Failed = 0
	}

	// Get pending in period
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state IN ('queued', 'sending') AND created_at >= datetime(?, 'unixepoch')
	`, since.Unix()).Scan(&metrics.Pending)
	if err != nil {
		metrics.Pending = 0
	}

	// Get by channel
	rows, err := nm.db.Query(`
		SELECT channel_type, COUNT(*) as count
		FROM notification_queue
		WHERE created_at >= datetime(?, 'unixepoch')
		GROUP BY channel_type
	`, since.Unix())
	if err != nil {
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var channel string
		var count int64
		if err := rows.Scan(&channel, &count); err != nil {
			continue
		}
		metrics.ByChannel[channel] = count
	}

	return metrics, nil
}

// GetChannelMetrics returns metrics for a specific channel
func (nm *NotificationMetrics) GetChannelMetrics(channelType string) (*ChannelMetrics, error) {
	metrics := &ChannelMetrics{
		ChannelType: channelType,
	}

	// Get total
	err := nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue WHERE channel_type = ?
	`, channelType).Scan(&metrics.Total)
	if err != nil {
		return nil, err
	}

	// Get delivered
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE channel_type = ? AND state = 'delivered'
	`, channelType).Scan(&metrics.Delivered)
	if err != nil {
		metrics.Delivered = 0
	}

	// Get failed
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE channel_type = ? AND state = 'failed'
	`, channelType).Scan(&metrics.Failed)
	if err != nil {
		metrics.Failed = 0
	}

	// Get pending
	err = nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE channel_type = ? AND state IN ('queued', 'sending')
	`, channelType).Scan(&metrics.Pending)
	if err != nil {
		metrics.Pending = 0
	}

	// Calculate delivery rate
	total := metrics.Delivered + metrics.Failed
	if total > 0 {
		metrics.DeliveryRate = float64(metrics.Delivered) / float64(total) * 100
	}

	// Get average delivery time
	err = nm.db.QueryRow(`
		SELECT AVG(
			CAST((julianday(delivered_at) - julianday(created_at)) * 86400 AS REAL)
		)
		FROM notification_queue
		WHERE channel_type = ? AND state = 'delivered' AND delivered_at IS NOT NULL
	`, channelType).Scan(&metrics.AvgDeliveryTime)
	if err != nil {
		metrics.AvgDeliveryTime = 0
	}

	return metrics, nil
}

// GetRecentErrors returns recent notification errors
func (nm *NotificationMetrics) GetRecentErrors(limit int) ([]map[string]interface{}, error) {
	rows, err := nm.db.Query(`
		SELECT id, channel_type, subject, error_message, retry_count, created_at
		FROM notification_queue
		WHERE state IN ('failed', 'dead_letter') AND error_message IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var errors []map[string]interface{}
	for rows.Next() {
		var id int
		var channelType, subject, lastError, createdAt string
		var retryCount int

		if err := rows.Scan(&id, &channelType, &subject, &lastError, &retryCount, &createdAt); err != nil {
			continue
		}

		errors = append(errors, map[string]interface{}{
			"id":           id,
			"channel_type": channelType,
			"subject":      subject,
			"error":        lastError,
			"retry_count":  retryCount,
			"created_at":   createdAt,
		})
	}

	return errors, nil
}

// RecordMetric records a custom metric event
func (nm *NotificationMetrics) RecordMetric(metricType, channel string, value float64) error {
	_, err := nm.db.Exec(`
		INSERT INTO notification_metrics (metric_type, channel_type, value, recorded_at)
		VALUES (?, ?, ?, datetime('now'))
	`, metricType, channel, value)
	return err
}

// GetHealthStatus returns the health status of the notification system
func (nm *NotificationMetrics) GetHealthStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["healthy"] = true
	status["timestamp"] = time.Now().Format(time.RFC3339)

	// Check queue depth
	var queueDepth int64
	nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state IN ('queued', 'sending')
	`).Scan(&queueDepth)
	status["queue_depth"] = queueDepth

	// Warn if queue is getting large
	if queueDepth > 1000 {
		status["healthy"] = false
		status["warning"] = "Queue depth exceeds 1000"
	}

	// Check for stuck notifications (queued for > 1 hour)
	var stuckCount int64
	nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state = 'queued'
		AND created_at < datetime('now', '-1 hour')
	`).Scan(&stuckCount)
	status["stuck_notifications"] = stuckCount

	if stuckCount > 0 {
		status["healthy"] = false
		status["warning"] = fmt.Sprintf("%d notifications stuck in queue", stuckCount)
	}

	// Check error rate in last hour
	var recentTotal, recentFailed int64
	nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE created_at >= datetime('now', '-1 hour')
	`).Scan(&recentTotal)

	nm.db.QueryRow(`
		SELECT COUNT(*) FROM notification_queue
		WHERE state = 'failed' AND created_at >= datetime('now', '-1 hour')
	`).Scan(&recentFailed)

	if recentTotal > 0 {
		errorRate := float64(recentFailed) / float64(recentTotal) * 100
		status["recent_error_rate"] = fmt.Sprintf("%.2f%%", errorRate)

		if errorRate > 50 {
			status["healthy"] = false
			status["warning"] = fmt.Sprintf("High error rate: %.2f%%", errorRate)
		}
	}

	return status
}
