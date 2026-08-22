package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
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
	err := database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue").Scan(&summary.TotalNotifications)
	if err != nil {
		return nil, err
	}

	// Get counts by state
	rows, err := database.QueryContext(context.Background(), nm.db, database.TimeoutReport, `
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
	rows, err = database.QueryContext(context.Background(), nm.db, database.TimeoutReport, `
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
	rows, err = database.QueryContext(context.Background(), nm.db, database.TimeoutReport, `
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
	err = database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM notification_queue
		WHERE state IN ('queued', 'sending')
	`).Scan(&summary.QueueDepth)
	if err != nil {
		summary.QueueDepth = 0
	}

	// Get average delivery time. The gap between created_at and delivered_at is
	// measured in Go rather than with SQLite's julianday() arithmetic:
	// julianday() evaluates to NULL for the driver's local-zone
	// time.Time.String() layout, so every row written that way silently dropped
	// out of the average, and julianday() does not exist on PostgreSQL, MySQL or
	// SQL Server, where the query errored and the average was reported as 0.
	summary.AvgDeliveryTime = 0
	deliveryRows, deliveryErr := database.QueryContext(context.Background(), nm.db, database.TimeoutReport, `
		SELECT created_at, delivered_at
		FROM notification_queue
		WHERE state = 'delivered' AND delivered_at IS NOT NULL AND created_at IS NOT NULL
	`)
	if deliveryErr == nil {
		if average, averageErr := averageDeliverySeconds(deliveryRows); averageErr == nil {
			summary.AvgDeliveryTime = average
		}
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

// GetTimePeriodMetrics returns metrics for a specific time period.
// The period cutoff is applied in Go rather than with SQLite's
// datetime(?, 'unixepoch'): created_at can hold either the canonical UTC text
// CURRENT_TIMESTAMP emits or the driver's local-zone time.Time.String() layout,
// and datetime() yields NULL for the latter, so those rows dropped out of every
// count regardless of when they were really created. One pass over the rows
// also replaces the five separate aggregate queries this used to run, and works
// unchanged on PostgreSQL and MySQL.
func (nm *NotificationMetrics) GetTimePeriodMetrics(duration time.Duration) (*TimePeriodMetrics, error) {
	metrics := &TimePeriodMetrics{
		ByChannel: make(map[string]int64),
	}

	since := time.Now().Add(-duration)

	rows, err := database.QueryContext(context.Background(), nm.db, database.TimeoutReport, `
		SELECT state, channel_type, created_at
		FROM notification_queue
		WHERE created_at IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var state, channelType string
		var storedCreatedAt interface{}
		if scanErr := rows.Scan(&state, &channelType, &storedCreatedAt); scanErr != nil {
			return nil, scanErr
		}

		// A row created exactly at the cutoff counted under the old ">=" test,
		// so accept equality as well as "strictly after".
		createdAt, ok := dbtime.ParseStoredTimestamp(storedCreatedAt)
		if !ok || createdAt.Before(since.UTC()) {
			continue
		}

		metrics.Total++
		metrics.ByChannel[channelType]++

		switch state {
		case "delivered":
			metrics.Delivered++
		case "failed":
			metrics.Failed++
		case "queued", "sending":
			metrics.Pending++
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	return metrics, nil
}

// GetChannelMetrics returns metrics for a specific channel
func (nm *NotificationMetrics) GetChannelMetrics(channelType string) (*ChannelMetrics, error) {
	metrics := &ChannelMetrics{
		ChannelType: channelType,
	}

	// Get total
	err := database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM notification_queue WHERE channel_type = ?
	`, channelType).Scan(&metrics.Total)
	if err != nil {
		return nil, err
	}

	// Get delivered
	err = database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM notification_queue
		WHERE channel_type = ? AND state = 'delivered'
	`, channelType).Scan(&metrics.Delivered)
	if err != nil {
		metrics.Delivered = 0
	}

	// Get failed
	err = database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM notification_queue
		WHERE channel_type = ? AND state = 'failed'
	`, channelType).Scan(&metrics.Failed)
	if err != nil {
		metrics.Failed = 0
	}

	// Get pending
	err = database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
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

	// Get average delivery time, measured in Go for the reasons given in
	// GetSummary.
	metrics.AvgDeliveryTime = 0
	deliveryRows, deliveryErr := database.QueryContext(context.Background(), nm.db, database.TimeoutReport, `
		SELECT created_at, delivered_at
		FROM notification_queue
		WHERE channel_type = ? AND state = 'delivered' AND delivered_at IS NOT NULL AND created_at IS NOT NULL
	`, channelType)
	if deliveryErr == nil {
		if average, averageErr := averageDeliverySeconds(deliveryRows); averageErr == nil {
			metrics.AvgDeliveryTime = average
		}
	}

	return metrics, nil
}

// averageDeliverySeconds reduces (created_at, delivered_at) pairs into the mean
// delivery latency in seconds, parsing each stored value with
// dbtime.ParseStoredTimestamp so that canonical UTC text and the driver's
// local-zone time.Time.String() layout both resolve to the same instant. A row
// whose timestamps do not parse, or whose delivered_at precedes its created_at,
// is skipped rather than counted as a zero or negative latency. It returns 0
// when no row is usable, matching the 0 the old AVG() query reported when it
// scanned a NULL result. It takes ownership of rows and closes them.
func averageDeliverySeconds(rows *sql.Rows) (float64, error) {
	defer rows.Close()

	var totalSeconds float64
	var counted int64

	for rows.Next() {
		var storedCreatedAt, storedDeliveredAt interface{}
		if scanErr := rows.Scan(&storedCreatedAt, &storedDeliveredAt); scanErr != nil {
			return 0, scanErr
		}

		createdAt, createdOK := dbtime.ParseStoredTimestamp(storedCreatedAt)
		deliveredAt, deliveredOK := dbtime.ParseStoredTimestamp(storedDeliveredAt)
		if !createdOK || !deliveredOK || deliveredAt.Before(createdAt) {
			continue
		}

		totalSeconds += deliveredAt.Sub(createdAt).Seconds()
		counted++
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return 0, rowsErr
	}

	if counted == 0 {
		return 0, nil
	}

	return totalSeconds / float64(counted), nil
}

// GetRecentErrors returns recent notification errors
func (nm *NotificationMetrics) GetRecentErrors(limit int) ([]map[string]interface{}, error) {
	rows, err := database.QueryContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
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
		var channelType, subject, lastError string
		var storedCreatedAt interface{}
		var retryCount int

		if err := rows.Scan(&id, &channelType, &subject, &lastError, &retryCount, &storedCreatedAt); err != nil {
			continue
		}

		// created_at crosses an API boundary here, so it is emitted in exactly one
		// form: RFC 3339 (ISO 8601) UTC, per the project rule to use an existing
		// standard rather than pass through whatever layout happens to be on disk.
		// The column can legitimately hold the canonical UTC text
		// CURRENT_TIMESTAMP writes or the driver's local-zone
		// time.Time.String() form, and handing either straight to a consumer made
		// the API emit inconsistent, zone-ambiguous timestamps.
		// Fail-safe: a NULL or otherwise uninterpretable value yields an empty
		// string rather than leaked raw text or a panic, so a consumer sees
		// "created_at": "" and can treat the timestamp as unknown. The row itself
		// is still returned - its error message is the point of the endpoint.
		createdAt := ""
		if parsed, ok := dbtime.ParseStoredTimestamp(storedCreatedAt); ok {
			createdAt = parsed.UTC().Format(time.RFC3339)
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

// RecordMetric records a custom metric event.
// recorded_at is bound as canonical UTC text rather than produced by SQLite's
// datetime('now') so the write works identically on every supported driver.
func (nm *NotificationMetrics) RecordMetric(metricType, channel string, value float64) error {
	_, err := database.ExecContext(context.Background(), nm.db, database.TimeoutWrite, `
		INSERT INTO notification_metrics (metric_type, channel_type, value, recorded_at)
		VALUES (?, ?, ?, ?)
	`, metricType, channel, value, dbtime.FormatSQLTimestamp(time.Now()))
	return err
}

// GetHealthStatus returns the health status of the notification system
func (nm *NotificationMetrics) GetHealthStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["healthy"] = true
	status["timestamp"] = time.Now().Format(time.RFC3339)

	// Check queue depth
	var queueDepth int64
	database.QueryRowContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
		SELECT COUNT(*) FROM notification_queue
		WHERE state IN ('queued', 'sending')
	`).Scan(&queueDepth)
	status["queue_depth"] = queueDepth

	// Warn if queue is getting large
	if queueDepth > 1000 {
		status["healthy"] = false
		status["warning"] = "Queue depth exceeds 1000"
	}

	// Count stuck notifications (queued for > 1 hour) and the last hour's
	// throughput in one pass, deciding "older than an hour" in Go. SQLite's
	// datetime('now', '-1 hour') returns NULL for the local-zone layout the
	// driver writes for a bound time.Time, so those rows never counted as stuck
	// and never counted toward the recent error rate; the Go comparison is also
	// portable to PostgreSQL and MySQL.
	var stuckCount, recentTotal, recentFailed int64
	stuckCutoff := time.Now().Add(-time.Hour)

	rows, err := database.QueryContext(context.Background(), nm.db, database.TimeoutSimpleSelect, `
		SELECT state, created_at
		FROM notification_queue
		WHERE created_at IS NOT NULL
	`)
	if err == nil {
		defer rows.Close()

		for rows.Next() {
			var state string
			var storedCreatedAt interface{}
			if scanErr := rows.Scan(&state, &storedCreatedAt); scanErr != nil {
				break
			}

			createdAt, ok := dbtime.ParseStoredTimestamp(storedCreatedAt)
			if !ok {
				continue
			}

			if createdAt.Before(stuckCutoff.UTC()) {
				if state == "queued" {
					stuckCount++
				}
				continue
			}

			recentTotal++
			if state == "failed" {
				recentFailed++
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			stuckCount, recentTotal, recentFailed = 0, 0, 0
		}
	}

	status["stuck_notifications"] = stuckCount

	if stuckCount > 0 {
		status["healthy"] = false
		status["warning"] = fmt.Sprintf("%d notifications stuck in queue", stuckCount)
	}

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
