package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/webappsgo/wthr/src/database"
	_ "modernc.org/sqlite"
)

func TestContactWebhookRetrySchedule(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}
	if len(contactWebhookRetryDelays) != len(want) {
		t.Fatalf("retry count = %d, want %d", len(contactWebhookRetryDelays), len(want))
	}
	for i := range want {
		if contactWebhookRetryDelays[i] != want[i] {
			t.Errorf("retry delay %d = %s, want %s", i, contactWebhookRetryDelays[i], want[i])
		}
	}
}

func TestEnqueueContactWebhookRetryPersistsStableIDAndPayload(t *testing.T) {
	db, err := sql.Open("sqlite", "file:contact-webhook-retry?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply server schema: %v", err)
	}

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	generatedID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate webhook ID: %v", err)
	}
	webhookID := generatedID.String()
	message := ContactWebhookMessage{
		Event: "security.report_received", Subject: "New report", Body: "Summary only", Severity: "high",
		Timestamp: now, TrackingID: "tracking-123", WebhookID: webhookID,
	}
	if err := enqueueContactWebhookRetry(context.Background(), db, "security", "generic", message, now); err != nil {
		t.Fatalf("enqueue contact webhook retry: %v", err)
	}

	var gotRole, gotTransport, gotID, payload string
	var attemptCount int
	var nextRun int64
	if err := db.QueryRow(`SELECT role, transport, webhook_id, message_json, attempt_count, next_retry_at FROM contact_webhook_retries`).Scan(
		&gotRole, &gotTransport, &gotID, &payload, &attemptCount, &nextRun,
	); err != nil {
		t.Fatalf("read queued retry: %v", err)
	}
	if gotRole != "security" || gotTransport != "generic" || gotID != webhookID {
		t.Fatalf("retry identity = (%q, %q, %q)", gotRole, gotTransport, gotID)
	}
	if attemptCount != 0 || nextRun != now.Add(time.Minute).Unix() {
		t.Fatalf("retry state = (%d, %d), want attempt 0 due after one minute", attemptCount, nextRun)
	}
	var gotMessage ContactWebhookMessage
	if err := json.Unmarshal([]byte(payload), &gotMessage); err != nil {
		t.Fatalf("decode retry payload: %v", err)
	}
	if gotMessage.Event != message.Event || gotMessage.TrackingID != message.TrackingID || !gotMessage.Timestamp.Equal(now) {
		t.Fatalf("retry payload = %+v, want original event", gotMessage)
	}
}
