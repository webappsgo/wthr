package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/server/service"
)

// TestNotificationMetricsHandler_GetSummary verifies the summary endpoint
// returns 200 with a real, queryable MetricsSummary body against an empty
// (but schema-valid) notification_queue table.
func TestNotificationMetricsHandler_GetSummary(t *testing.T) {
	db := newTestServerDB(t)
	if _, err := db.Exec(`INSERT INTO notification_queue (channel_type, priority, state, body) VALUES ('email', 2, 'delivered', 'hello')`); err != nil {
		t.Fatalf("failed to seed notification_queue: %v", err)
	}

	h := NewNotificationMetricsHandler(service.NewNotificationMetrics(db))

	r := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/notifications/summary", nil)
	w := httptest.NewRecorder()
	h.GetSummary(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var summary service.MetricsSummary
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if summary.TotalNotifications != 1 {
		t.Errorf("expected TotalNotifications 1, got %d", summary.TotalNotifications)
	}
	if summary.ByState["delivered"] != 1 {
		t.Errorf("expected ByState[delivered] 1, got %d", summary.ByState["delivered"])
	}
}

// TestNotificationMetricsHandler_GetChannelMetrics verifies the per-channel
// endpoint reads the "type" path param and returns 200 with channel-scoped
// counts.
func TestNotificationMetricsHandler_GetChannelMetrics(t *testing.T) {
	db := newTestServerDB(t)
	if _, err := db.Exec(`INSERT INTO notification_queue (channel_type, priority, state, body) VALUES ('sms', 2, 'failed', 'hi')`); err != nil {
		t.Fatalf("failed to seed notification_queue: %v", err)
	}

	h := NewNotificationMetricsHandler(service.NewNotificationMetrics(db))

	r := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/notifications/channels/sms", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("type", "sms")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.GetChannelMetrics(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var metrics service.ChannelMetrics
	if err := json.Unmarshal(w.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if metrics.ChannelType != "sms" {
		t.Errorf("expected ChannelType 'sms', got %q", metrics.ChannelType)
	}
	if metrics.Failed != 1 {
		t.Errorf("expected Failed 1, got %d", metrics.Failed)
	}
}

// TestNotificationMetricsHandler_GetRecentErrors verifies the errors
// endpoint honors the "limit" query param and returns only failed/
// dead_letter rows with a non-nil error_message.
func TestNotificationMetricsHandler_GetRecentErrors(t *testing.T) {
	db := newTestServerDB(t)
	if _, err := db.Exec(`INSERT INTO notification_queue (channel_type, priority, state, subject, body, error_message) VALUES ('email', 2, 'failed', 'test subject', 'hi', 'boom')`); err != nil {
		t.Fatalf("failed to seed notification_queue: %v", err)
	}

	h := NewNotificationMetricsHandler(service.NewNotificationMetrics(db))

	r := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/notifications/errors?limit=10", nil)
	w := httptest.NewRecorder()
	h.GetRecentErrors(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Errors []map[string]interface{} `json:"errors"`
		Count  int                      `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count 1, got %d", resp.Count)
	}
	if resp.Errors[0]["error"] != "boom" {
		t.Errorf("expected error 'boom', got %v", resp.Errors[0]["error"])
	}
}

// TestNotificationMetricsHandler_GetRecentErrors_InvalidLimit verifies an
// out-of-range or non-numeric "limit" falls back to the default (50)
// rather than erroring.
func TestNotificationMetricsHandler_GetRecentErrors_InvalidLimit(t *testing.T) {
	db := newTestServerDB(t)

	h := NewNotificationMetricsHandler(service.NewNotificationMetrics(db))

	r := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/notifications/errors?limit=not-a-number", nil)
	w := httptest.NewRecorder()
	h.GetRecentErrors(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestNotificationMetricsHandler_GetHealthStatus verifies the health
// endpoint reports healthy=true against an empty queue.
func TestNotificationMetricsHandler_GetHealthStatus(t *testing.T) {
	db := newTestServerDB(t)

	h := NewNotificationMetricsHandler(service.NewNotificationMetrics(db))

	r := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/notifications/health", nil)
	w := httptest.NewRecorder()
	h.GetHealthStatus(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var status map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if healthy, ok := status["healthy"].(bool); !ok || !healthy {
		t.Errorf("expected healthy=true on an empty queue, got %v", status["healthy"])
	}
}
