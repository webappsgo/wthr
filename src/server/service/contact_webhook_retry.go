package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
)

var contactWebhookRetryDelays = [...]time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

const contactWebhookRetryBatchSize = 100

var webhookFailureLogLimiter = struct {
	sync.Mutex
	last map[string]time.Time
}{last: make(map[string]time.Time)}

type queuedContactWebhookRetry struct {
	id           string
	role         string
	transport    string
	webhookID    string
	messageJSON  string
	attemptCount int
}

// DispatchContactWebhooksWithRetry delivers configured transports and stores
// failed deliveries for the built-in scheduler. One UUID v7 is shared by each
// transport attempt and its retries so receivers can deduplicate deliveries.
func DispatchContactWebhooksWithRetry(ctx context.Context, contact config.ContactConfig, role string, message ContactWebhookMessage) (int, error) {
	if ctx == nil {
		return 0, errors.New("webhook context is required")
	}
	deps := defaultContactWebhookDependencies()
	if message.Timestamp.IsZero() {
		message.Timestamp = deps.now().UTC()
	} else {
		message.Timestamp = message.Timestamp.UTC()
	}
	if message.WebhookID == "" {
		id, err := deps.newID()
		if err != nil {
			return 0, errors.New("contact webhook ID generation failed")
		}
		message.WebhookID = id.String()
	}
	if id, err := uuid.Parse(message.WebhookID); err != nil || id.Version() != 7 {
		return 0, errors.New("contact webhook ID must be a UUID v7")
	}

	sent := 0
	var failures []error
	for _, transport := range contactWebhookTransports(contact, role) {
		if err := dispatchContactWebhook(ctx, contact, role, transport, message, deps); err != nil {
			if queueErr := enqueueContactWebhookRetry(ctx, database.GetServerDB(), role, transport, message, time.Now().UTC()); queueErr != nil {
				failures = append(failures, fmt.Errorf("%s webhook failed and retry could not be queued: %w", transport, queueErr))
			} else {
				failures = append(failures, fmt.Errorf("%s webhook delivery failed; retry queued", transport))
			}
			continue
		}
		sent++
	}
	return sent, errors.Join(failures...)
}

func enqueueContactWebhookRetry(ctx context.Context, db *sql.DB, role, transport string, message ContactWebhookMessage, now time.Time) error {
	if db == nil {
		return errors.New("server database is unavailable")
	}
	if ctx == nil {
		return errors.New("retry context is required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	transport = strings.ToLower(strings.TrimSpace(transport))
	if len(contactRoleFallback(role)) == 0 || transport == "" || strings.TrimSpace(message.Event) == "" {
		return errors.New("retry event is invalid")
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = now.UTC()
	}
	id, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate retry record ID: %w", err)
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode retry event: %w", err)
	}
	_, err = database.ExecContext(ctx, db, database.TimeoutWrite, `
		INSERT INTO contact_webhook_retries
		(id, role, transport, webhook_id, message_json, attempt_count, next_retry_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)
	`, id.String(), role, transport, message.WebhookID, string(payload), now.Add(contactWebhookRetryDelays[0]).Unix(), now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("persist contact webhook retry: %w", err)
	}
	return nil
}

// ProcessContactWebhookRetries delivers due webhook retries. It is registered
// as a built-in scheduler task so retry timing survives restarts.
func ProcessContactWebhookRetries(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("server database is unavailable")
	}
	if ctx == nil {
		return errors.New("retry context is required")
	}
	now := time.Now().UTC()
	rows, err := database.QueryContext(ctx, db, database.TimeoutSimpleSelect, `
		SELECT id, role, transport, webhook_id, message_json, attempt_count
		FROM contact_webhook_retries
		WHERE next_retry_at <= ?
		ORDER BY next_retry_at ASC, created_at ASC
		LIMIT ?
	`, now.Unix(), contactWebhookRetryBatchSize)
	if err != nil {
		return fmt.Errorf("query contact webhook retries: %w", err)
	}
	var pending []queuedContactWebhookRetry
	for rows.Next() {
		var retry queuedContactWebhookRetry
		if err := rows.Scan(&retry.id, &retry.role, &retry.transport, &retry.webhookID, &retry.messageJSON, &retry.attemptCount); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan contact webhook retry: %w", err)
		}
		pending = append(pending, retry)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read contact webhook retries: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close contact webhook retries: %w", err)
	}

	for _, retry := range pending {
		var message ContactWebhookMessage
		if err := json.Unmarshal([]byte(retry.messageJSON), &message); err != nil {
			if _, deleteErr := database.ExecContext(ctx, db, database.TimeoutWrite, "DELETE FROM contact_webhook_retries WHERE id = ?", retry.id); deleteErr != nil {
				return fmt.Errorf("discard invalid contact webhook retry: %w", deleteErr)
			}
			continue
		}
		message.WebhookID = retry.webhookID
		cfg := config.GetGlobalConfig()
		if cfg == nil {
			return errors.New("configuration is unavailable for contact webhook retry")
		}
		err := dispatchContactWebhook(ctx, cfg.Server.Contact, retry.role, retry.transport, message, defaultContactWebhookDependencies())
		if err == nil {
			if _, deleteErr := database.ExecContext(ctx, db, database.TimeoutWrite, "DELETE FROM contact_webhook_retries WHERE id = ?", retry.id); deleteErr != nil {
				return fmt.Errorf("remove delivered contact webhook retry: %w", deleteErr)
			}
			continue
		}

		nextAttempt := retry.attemptCount + 1
		if nextAttempt >= len(contactWebhookRetryDelays) {
			if _, deleteErr := database.ExecContext(ctx, db, database.TimeoutWrite, "DELETE FROM contact_webhook_retries WHERE id = ?", retry.id); deleteErr != nil {
				return fmt.Errorf("drop exhausted contact webhook retry: %w", deleteErr)
			}
			logPermanentContactWebhookFailure(retry.role, retry.transport, message.FailureRateKey, now)
			continue
		}
		nextRun := now.Add(contactWebhookRetryDelays[nextAttempt])
		if _, updateErr := database.ExecContext(ctx, db, database.TimeoutWrite, `
			UPDATE contact_webhook_retries
			SET attempt_count = ?, next_retry_at = ?, updated_at = ?
			WHERE id = ?
		`, nextAttempt, nextRun.Unix(), now.Unix(), retry.id); updateErr != nil {
			return fmt.Errorf("reschedule contact webhook retry: %w", updateErr)
		}
	}
	return nil
}

func logPermanentContactWebhookFailure(role, transport, rateKey string, now time.Time) {
	if rateKey == "" {
		rateKey = "unknown"
	}
	key := rateKey + ":" + role + ":" + transport
	webhookFailureLogLimiter.Lock()
	defer webhookFailureLogLimiter.Unlock()
	if last := webhookFailureLogLimiter.last[key]; now.Sub(last) < time.Minute {
		return
	}
	webhookFailureLogLimiter.last[key] = now
	log.Printf("WARN: notify.webhook_failed role=%s transport=%s", role, transport)
}
