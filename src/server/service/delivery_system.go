package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// DeliveryState represents the state of a notification in the delivery pipeline
type DeliveryState string

const (
	StateCreated    DeliveryState = "created"
	StateQueued     DeliveryState = "queued"
	StateSending    DeliveryState = "sending"
	StateDelivered  DeliveryState = "delivered"
	StateFailed     DeliveryState = "failed"
	StateDeadLetter DeliveryState = "dead_letter"
)

// NotificationQueue represents a queued notification
type NotificationQueue struct {
	ID           int
	UserID       sql.NullInt64
	ChannelType  string
	TemplateID   sql.NullInt64
	Priority     int
	State        string
	Subject      string
	Body         string
	Variables    map[string]interface{}
	RetryCount   int
	MaxRetries   int
	NextRetryAt  sql.NullTime
	DeliveredAt  sql.NullTime
	FailedAt     sql.NullTime
	ErrorMessage sql.NullString
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DeliverySystem handles notification delivery with retry logic
type DeliverySystem struct {
	db             *sql.DB
	channelManager *ChannelManager
	templateEngine *TemplateEngine
	maxRetries     int
	// "linear" or "exponential"
	retryBackoff    string
	queueWorkers    int
	batchSize       int
	rateLimitPerMin int
}

// NewDeliverySystem creates a new delivery system
func NewDeliverySystem(db *sql.DB, cm *ChannelManager, te *TemplateEngine) *DeliverySystem {
	return &DeliverySystem{
		db:              db,
		channelManager:  cm,
		templateEngine:  te,
		maxRetries:      3,
		retryBackoff:    "exponential",
		queueWorkers:    5,
		batchSize:       100,
		rateLimitPerMin: 60,
	}
}

// serverDB returns the server.db handle this delivery system was constructed
// with. Its notification_queue and notification_history tables are declared
// in database.ServerSchema; settings are read from their owning config files.
// Fallback: when the injected handle is nil (unit tests constructing the
// service with nil, or construction before the global dual DB is wired) the
// process-global server handle is used instead, so a nil handle degrades to
// the previous behavior rather than panicking.
func (ds *DeliverySystem) serverDB() *sql.DB {
	if ds.db != nil {
		return ds.db
	}
	return database.GetServerDB()
}

// LoadSettings loads delivery system settings from configuration files.
func (ds *DeliverySystem) LoadSettings() error {
	settings := map[string]*int{
		"notifications.retry_max":          &ds.maxRetries,
		"notifications.queue_workers":      &ds.queueWorkers,
		"notifications.batch_size":         &ds.batchSize,
		"notifications.rate_limit_per_min": &ds.rateLimitPerMin,
	}

	for key, ptr := range settings {
		value, exists, err := config.ReadSetting(key)
		if err == nil && exists {
			var intVal int
			if _, scanErr := fmt.Sscanf(fmt.Sprint(value), "%d", &intVal); scanErr == nil {
				*ptr = intVal
			}
		}
	}

	// Get backoff strategy
	backoff, exists, err := config.ReadSetting("notifications.retry_backoff")
	if err == nil && exists {
		ds.retryBackoff = fmt.Sprint(backoff)
	}

	return nil
}

// Enqueue adds a notification to the queue
func (ds *DeliverySystem) Enqueue(userID *int, channelType, subject, body string, priority int, variables map[string]interface{}) (int, error) {
	variablesJSON, _ := json.Marshal(variables)

	result, err := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, `
		INSERT INTO notification_queue
		(user_id, channel_type, priority, state, subject, body, variables,
		 retry_count, max_retries, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
	`, userID, channelType, priority, StateCreated, subject, body,
		string(variablesJSON), ds.maxRetries, time.Now(), time.Now())

	if err != nil {
		return 0, fmt.Errorf("failed to enqueue notification: %w", err)
	}

	id, _ := result.LastInsertId()
	return int(id), nil
}

// ProcessQueue processes notifications in the queue
func (ds *DeliverySystem) ProcessQueue() error {
	// Get pending notifications (created or ready for retry)
	now := time.Now()
	rows, err := database.QueryContext(context.Background(), ds.serverDB(), database.TimeoutSimpleSelect, `
		SELECT id, user_id, channel_type, template_id, priority, state,
		       subject, body, variables, retry_count, max_retries,
		       next_retry_at, created_at, updated_at
		FROM notification_queue
		WHERE state IN (?, ?, ?)
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY priority DESC, created_at ASC
		LIMIT ?
	`, StateCreated, StateQueued, StateFailed, now, ds.batchSize)

	if err != nil {
		return fmt.Errorf("failed to query queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	processed := 0
	for rows.Next() {
		var nq NotificationQueue
		var variablesJSON sql.NullString

		err := rows.Scan(
			&nq.ID, &nq.UserID, &nq.ChannelType, &nq.TemplateID, &nq.Priority,
			&nq.State, &nq.Subject, &nq.Body, &variablesJSON, &nq.RetryCount,
			&nq.MaxRetries, &nq.NextRetryAt, &nq.CreatedAt, &nq.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("scan notification queue row: %w", err)
		}

		if variablesJSON.Valid {
			if err := json.Unmarshal([]byte(variablesJSON.String), &nq.Variables); err != nil {
				return fmt.Errorf("decode variables for notification %d: %w", nq.ID, err)
			}
		}

		// Process notification
		go ds.processNotification(&nq)
		processed++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate notification queue: %w", err)
	}

	return nil
}

// processNotification processes a single notification
func (ds *DeliverySystem) processNotification(nq *NotificationQueue) {
	// Update state to sending
	if err := ds.updateState(nq.ID, StateSending); err != nil {
		util.LogError("Unable to mark notification %d as sending: %v", nq.ID, err)
		return
	}

	// Get channel
	channel, err := ds.channelManager.GetChannel(nq.ChannelType)
	if err != nil {
		ds.handleFailure(nq, fmt.Errorf("channel not found: %w", err))
		return
	}

	// Get recipient
	recipient := ds.getRecipient(nq)
	if recipient == "" {
		ds.handleFailure(nq, fmt.Errorf("recipient not found"))
		return
	}

	// Send notification
	err = channel.Send(recipient, nq.Subject, nq.Body, nq.Variables)
	if err != nil {
		ds.handleFailure(nq, err)
		return
	}

	// Success
	ds.handleSuccess(nq)
}

// handleSuccess handles successful delivery
func (ds *DeliverySystem) handleSuccess(nq *NotificationQueue) {
	now := time.Now()

	// Update queue entry
	_, err := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, `
		UPDATE notification_queue
		SET state = ?, delivered_at = ?, updated_at = ?
		WHERE id = ?
	`, StateDelivered, now, now, nq.ID)

	if err != nil {
		util.LogError("Unable to mark notification %d as delivered: %v", nq.ID, err)
		return
	}

	// Record in history
	if err := ds.recordHistory(nq, "delivered", ""); err != nil {
		util.LogError("Unable to record delivery history for notification %d: %v", nq.ID, err)
	}

	// Update channel stats
	if err := ds.channelManager.RecordSuccess(nq.ChannelType); err != nil {
		util.LogError("Unable to record channel success for %s: %v", nq.ChannelType, err)
	}
}

// handleFailure handles delivery failure with retry logic
func (ds *DeliverySystem) handleFailure(nq *NotificationQueue, err error) {
	nq.RetryCount++
	errorMsg := err.Error()

	// Check if max retries exceeded
	if nq.RetryCount >= nq.MaxRetries {
		// Move to dead letter queue
		now := time.Now()
		_, dbErr := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, `
			UPDATE notification_queue
			SET state = ?, failed_at = ?, error_message = ?, retry_count = ?, updated_at = ?
			WHERE id = ?
		`, StateDeadLetter, now, errorMsg, nq.RetryCount, now, nq.ID)

		if dbErr == nil {
			if err := ds.recordHistory(nq, "dead_letter", errorMsg); err != nil {
				util.LogError("Unable to record dead-letter history for notification %d: %v", nq.ID, err)
			}
		} else {
			util.LogError("Unable to dead-letter notification %d: %v", nq.ID, dbErr)
		}

		if err := ds.channelManager.RecordFailure(nq.ChannelType, errorMsg); err != nil {
			util.LogError("Unable to record channel failure for %s: %v", nq.ChannelType, err)
		}
		return
	}

	// Calculate next retry time
	nextRetry := ds.calculateNextRetry(nq.RetryCount)

	// Update for retry
	_, dbErr := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, `
		UPDATE notification_queue
		SET state = ?, retry_count = ?, next_retry_at = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`, StateFailed, nq.RetryCount, nextRetry, errorMsg, time.Now(), nq.ID)

	if dbErr == nil {
		if err := ds.recordHistory(nq, "failed", errorMsg); err != nil {
			util.LogError("Unable to record failed-delivery history for notification %d: %v", nq.ID, err)
		}
	} else {
		util.LogError("Unable to schedule retry for notification %d: %v", nq.ID, dbErr)
	}

	if err := ds.channelManager.RecordFailure(nq.ChannelType, errorMsg); err != nil {
		util.LogError("Unable to record channel failure for %s: %v", nq.ChannelType, err)
	}
}

// calculateNextRetry calculates the next retry time based on retry count
func (ds *DeliverySystem) calculateNextRetry(retryCount int) time.Time {
	var delayMinutes float64

	if ds.retryBackoff == "exponential" {
		// Exponential backoff: 1, 2, 4, 8, 16, 32, 64 minutes
		delayMinutes = math.Pow(2, float64(retryCount))
	} else {
		// Linear backoff: 5, 10, 15, 20 minutes
		delayMinutes = float64(retryCount * 5)
	}

	// Cap at 60 minutes
	if delayMinutes > 60 {
		delayMinutes = 60
	}

	return time.Now().Add(time.Duration(delayMinutes) * time.Minute)
}

// updateState updates the state of a notification
func (ds *DeliverySystem) updateState(id int, state DeliveryState) error {
	_, err := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, `
		UPDATE notification_queue
		SET state = ?, updated_at = ?
		WHERE id = ?
	`, state, time.Now(), id)
	return err
}

// recordHistory records notification delivery in history
func (ds *DeliverySystem) recordHistory(nq *NotificationQueue, status, errorMsg string) error {
	var userID interface{}
	if nq.UserID.Valid {
		userID = nq.UserID.Int64
	}

	metadata := map[string]interface{}{
		"retry_count": nq.RetryCount,
		"priority":    nq.Priority,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode notification history metadata: %w", err)
	}

	_, err = database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, `
		INSERT INTO notification_history
		(queue_id, user_id, channel_type, status, subject, body, delivered_at, error_message, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nq.ID, userID, nq.ChannelType, status, nq.Subject, nq.Body,
		time.Now(), errorMsg, string(metadataJSON))

	return err
}

// getRecipient gets the recipient for a notification
func (ds *DeliverySystem) getRecipient(nq *NotificationQueue) string {
	// If user_id is set, get user's preferred contact for this channel
	if nq.UserID.Valid {
		var recipient string
		// Reads users.db: user_notification_channel_preferences is declared in
		// database.UsersSchema, not ServerSchema, so the injected handle (the
		// server database) cannot serve this lookup — the global users handle
		// is the correct source here.
		err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, `
			SELECT config FROM user_notification_channel_preferences
			WHERE user_id = ? AND channel_type = ? AND enabled = 1
			LIMIT 1
		`, nq.UserID.Int64, nq.ChannelType).Scan(&recipient)

		if err == nil && recipient != "" {
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(recipient), &config); err == nil {
				if addr, ok := config["address"]; ok {
					return fmt.Sprintf("%v", addr)
				}
			}
		}

		// Fallback to user's email
		if nq.ChannelType == "email" {
			var email string
			// Reads users.db: users is declared in
			// database.UsersSchema, not ServerSchema, so the injected server
			// handle is not the right database for this fallback lookup.
			if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT email FROM users WHERE id = ?", nq.UserID.Int64).Scan(&email); err == nil {
				return email
			} else {
				util.LogWarn("Unable to find email recipient for notification %d: %v", nq.ID, err)
			}
		}
	}

	// Check if recipient is in variables
	if nq.Variables != nil {
		if recipient, ok := nq.Variables["recipient"]; ok {
			return fmt.Sprintf("%v", recipient)
		}
	}

	return ""
}

// GetQueueStats returns statistics about the queue
func (ds *DeliverySystem) GetQueueStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count by state
	rows, err := database.QueryContext(context.Background(), ds.serverDB(), database.TimeoutReport, `
		SELECT state, COUNT(*) as count
		FROM notification_queue
		GROUP BY state
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	stateCounts := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, fmt.Errorf("scan notification state count: %w", err)
		}
		stateCounts[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification state counts: %w", err)
	}

	stats["by_state"] = stateCounts

	// Total pending (created + queued + failed with retry)
	var pending int
	if err := database.QueryRowContext(context.Background(), ds.serverDB(), database.TimeoutSimpleSelect, `
		SELECT COUNT(*)
		FROM notification_queue
		WHERE state IN (?, ?, ?)
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
	`, StateCreated, StateQueued, StateFailed, time.Now()).Scan(&pending); err != nil {
		return nil, fmt.Errorf("count pending notifications: %w", err)
	}
	stats["pending"] = pending

	// Dead letters
	var deadLetters int
	if err := database.QueryRowContext(context.Background(), ds.serverDB(), database.TimeoutSimpleSelect, `
		SELECT COUNT(*)
		FROM notification_queue
		WHERE state = ?
	`, StateDeadLetter).Scan(&deadLetters); err != nil {
		return nil, fmt.Errorf("count dead-letter notifications: %w", err)
	}
	stats["dead_letters"] = deadLetters

	return stats, nil
}

// CleanupOld deletes old delivered notifications
func (ds *DeliverySystem) CleanupOld(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	_, err := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutBulk, `
		DELETE FROM notification_queue
		WHERE state = ? AND delivered_at < ?
	`, StateDelivered, cutoff)

	return err
}

// RequeueDeadLetters moves dead letter notifications back to queue
func (ds *DeliverySystem) RequeueDeadLetters(ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	// Build placeholders
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		UPDATE notification_queue
		SET state = ?, retry_count = 0, next_retry_at = NULL,
		    error_message = NULL, updated_at = ?
		WHERE id IN (%s) AND state = ?
	`, strings.Join(placeholders, ","))

	args = append([]interface{}{StateQueued, time.Now()}, args...)
	args = append(args, StateDeadLetter)

	_, err := database.ExecContext(context.Background(), ds.serverDB(), database.TimeoutWrite, query, args...)
	return err
}
