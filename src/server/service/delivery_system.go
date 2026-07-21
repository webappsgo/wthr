package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/webappsgo/wthr/src/database"
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
	db              *sql.DB
	channelManager  *ChannelManager
	templateEngine  *TemplateEngine
	maxRetries      int
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

// LoadSettings loads delivery system settings from database
func (ds *DeliverySystem) LoadSettings() error {
	settings := map[string]*int{
		"notifications.retry_max":          &ds.maxRetries,
		"notifications.queue_workers":      &ds.queueWorkers,
		"notifications.batch_size":         &ds.batchSize,
		"notifications.rate_limit_per_min": &ds.rateLimitPerMin,
	}

	for key, ptr := range settings {
		var value string
		err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = ?", key).Scan(&value)
		if err == nil {
			var intVal int
			fmt.Sscanf(value, "%d", &intVal)
			*ptr = intVal
		}
	}

	// Get backoff strategy
	var backoff string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = ?", "notifications.retry_backoff").Scan(&backoff)
	if err == nil {
		ds.retryBackoff = backoff
	}

	return nil
}

// Enqueue adds a notification to the queue
func (ds *DeliverySystem) Enqueue(userID *int, channelType, subject, body string, priority int, variables map[string]interface{}) (int, error) {
	variablesJSON, _ := json.Marshal(variables)

	result, err := database.GetServerDB().Exec(`
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
	rows, err := database.GetServerDB().Query(`
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
	defer rows.Close()

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
			continue
		}

		if variablesJSON.Valid {
			json.Unmarshal([]byte(variablesJSON.String), &nq.Variables)
		}

		// Process notification
		go ds.processNotification(&nq)
		processed++
	}

	return nil
}

// processNotification processes a single notification
func (ds *DeliverySystem) processNotification(nq *NotificationQueue) {
	// Update state to sending
	ds.updateState(nq.ID, StateSending)

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
	_, err := database.GetServerDB().Exec(`
		UPDATE notification_queue
		SET state = ?, delivered_at = ?, updated_at = ?
		WHERE id = ?
	`, StateDelivered, now, now, nq.ID)

	if err != nil {
		return
	}

	// Record in history
	ds.recordHistory(nq, "delivered", "")

	// Update channel stats
	ds.channelManager.RecordSuccess(nq.ChannelType)
}

// handleFailure handles delivery failure with retry logic
func (ds *DeliverySystem) handleFailure(nq *NotificationQueue, err error) {
	nq.RetryCount++
	errorMsg := err.Error()

	// Check if max retries exceeded
	if nq.RetryCount >= nq.MaxRetries {
		// Move to dead letter queue
		now := time.Now()
		_, dbErr := database.GetServerDB().Exec(`
			UPDATE notification_queue
			SET state = ?, failed_at = ?, error_message = ?, retry_count = ?, updated_at = ?
			WHERE id = ?
		`, StateDeadLetter, now, errorMsg, nq.RetryCount, now, nq.ID)

		if dbErr == nil {
			ds.recordHistory(nq, "dead_letter", errorMsg)
		}

		ds.channelManager.RecordFailure(nq.ChannelType, errorMsg)
		return
	}

	// Calculate next retry time
	nextRetry := ds.calculateNextRetry(nq.RetryCount)

	// Update for retry
	_, dbErr := database.GetServerDB().Exec(`
		UPDATE notification_queue
		SET state = ?, retry_count = ?, next_retry_at = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`, StateFailed, nq.RetryCount, nextRetry, errorMsg, time.Now(), nq.ID)

	if dbErr == nil {
		ds.recordHistory(nq, "failed", errorMsg)
	}

	ds.channelManager.RecordFailure(nq.ChannelType, errorMsg)
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
	_, err := database.GetServerDB().Exec(`
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
	metadataJSON, _ := json.Marshal(metadata)

	_, err := database.GetServerDB().Exec(`
		INSERT INTO server_notification_history
		(queue_id, user_id, channel_type, status, subject, body, sent_at, error_message, metadata)
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
		err := database.GetUsersDB().QueryRow(`
			SELECT config FROM user_notification_preferences
			WHERE user_id = ? AND channel_type = ? AND enabled = 1
			LIMIT 1
		`, nq.UserID.Int64, nq.ChannelType).Scan(&recipient)

		if err == nil && recipient != "" {
			var config map[string]interface{}
			json.Unmarshal([]byte(recipient), &config)
			if addr, ok := config["address"]; ok {
				return fmt.Sprintf("%v", addr)
			}
		}

		// Fallback to user's email
		if nq.ChannelType == "email" {
			var email string
			database.GetUsersDB().QueryRow("SELECT email FROM user_accounts WHERE id = ?", nq.UserID.Int64).Scan(&email)
			return email
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
	rows, err := database.GetServerDB().Query(`
		SELECT state, COUNT(*) as count
		FROM notification_queue
		GROUP BY state
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stateCounts := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		rows.Scan(&state, &count)
		stateCounts[state] = count
	}

	stats["by_state"] = stateCounts

	// Total pending (created + queued + failed with retry)
	var pending int
	database.GetServerDB().QueryRow(`
		SELECT COUNT(*)
		FROM notification_queue
		WHERE state IN (?, ?, ?)
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
	`, StateCreated, StateQueued, StateFailed, time.Now()).Scan(&pending)
	stats["pending"] = pending

	// Dead letters
	var deadLetters int
	database.GetServerDB().QueryRow(`
		SELECT COUNT(*)
		FROM notification_queue
		WHERE state = ?
	`, StateDeadLetter).Scan(&deadLetters)
	stats["dead_letters"] = deadLetters

	return stats, nil
}

// CleanupOld deletes old delivered notifications
func (ds *DeliverySystem) CleanupOld(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	_, err := database.GetServerDB().Exec(`
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
	`, string(placeholders[0]))

	args = append([]interface{}{StateQueued, time.Now()}, args...)
	args = append(args, StateDeadLetter)

	_, err := database.GetServerDB().Exec(query, args...)
	return err
}
