package service

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
)

// setupNotificationMetricsTestDB opens a fresh in-memory SQLite database with
// the real production ServerSchema applied (notification_queue and
// notification_metrics tables live there), uniquely named per test so
// parallel/table-driven subtests never collide.
func setupNotificationMetricsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_metrics?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	return db
}

// insertQueueRow inserts a notification_queue row with the given state,
// channel, priority, error message and created_at/delivered_at offsets
// (in seconds from now) for use across the metrics tests below.
func insertQueueRow(t *testing.T, db *sql.DB, state, channel string, priority int, errMsg string, createdAgo, deliveredAgo *int) {
	t.Helper()

	createdAt := "datetime('now')"
	if createdAgo != nil {
		createdAt = "datetime('now', ?)"
	}

	var deliveredAtExpr string
	args := []interface{}{}
	query := `INSERT INTO notification_queue (user_id, channel_type, priority, state, subject, body, error_message, created_at, updated_at, delivered_at) VALUES (?, ?, ?, ?, ?, ?, ?, `

	args = append(args, 1, channel, priority, state, "subject", "body", nullableString(errMsg))

	if createdAgo != nil {
		query += createdAt + ", " + createdAt
		args = append(args, offsetToModifier(*createdAgo), offsetToModifier(*createdAgo))
	} else {
		query += "datetime('now'), datetime('now')"
	}

	if deliveredAgo != nil {
		query += ", datetime('now', ?))"
		args = append(args, offsetToModifier(*deliveredAgo))
	} else {
		query += ", NULL)"
	}

	_ = deliveredAtExpr
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("insert notification_queue row: %v", err)
	}
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// offsetToModifier converts a signed second offset into a SQLite datetime
// modifier string, e.g. -3600 -> "-3600 seconds" (past), 0 -> "0 seconds".
func offsetToModifier(secondsAgo int) string {
	if secondsAgo == 0 {
		return "0 seconds"
	}
	return "-" + itoaMetrics(secondsAgo) + " seconds"
}

func itoaMetrics(n int) string {
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// TestNotificationMetrics_GetSummary_Empty covers the boundary condition of
// an empty notification_queue: every count must be zero and no division by
// zero should occur in the rate calculations.
func TestNotificationMetrics_GetSummary_Empty(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	summary, err := nm.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if summary.TotalNotifications != 0 {
		t.Errorf("TotalNotifications = %d, want 0", summary.TotalNotifications)
	}
	if summary.DeliveryRate != 0 {
		t.Errorf("DeliveryRate = %v, want 0", summary.DeliveryRate)
	}
	if summary.ErrorRate != 0 {
		t.Errorf("ErrorRate = %v, want 0", summary.ErrorRate)
	}
	if summary.QueueDepth != 0 {
		t.Errorf("QueueDepth = %d, want 0", summary.QueueDepth)
	}
	if summary.Last24Hours == nil || summary.Last7Days == nil {
		t.Fatalf("Last24Hours/Last7Days must not be nil even when empty")
	}
}

// TestNotificationMetrics_GetSummary_Populated exercises the happy path with
// a mix of states, channels and priorities, verifying breakdown maps and
// rate math.
func TestNotificationMetrics_GetSummary_Populated(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	insertQueueRow(t, db, "delivered", "email", 1, "", nil, nil)
	insertQueueRow(t, db, "delivered", "email", 2, "", nil, nil)
	insertQueueRow(t, db, "failed", "sms", 3, "boom", nil, nil)
	insertQueueRow(t, db, "queued", "email", 4, "", nil, nil)
	insertQueueRow(t, db, "sending", "sms", 1, "", nil, nil)

	summary, err := nm.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if summary.TotalNotifications != 5 {
		t.Errorf("TotalNotifications = %d, want 5", summary.TotalNotifications)
	}
	if summary.ByState["delivered"] != 2 {
		t.Errorf("ByState[delivered] = %d, want 2", summary.ByState["delivered"])
	}
	if summary.ByState["failed"] != 1 {
		t.Errorf("ByState[failed] = %d, want 1", summary.ByState["failed"])
	}
	if summary.ByChannel["email"] != 3 {
		t.Errorf("ByChannel[email] = %d, want 3", summary.ByChannel["email"])
	}
	if summary.ByChannel["sms"] != 2 {
		t.Errorf("ByChannel[sms] = %d, want 2", summary.ByChannel["sms"])
	}
	if summary.ByPriority["low"] != 2 {
		t.Errorf("ByPriority[low] = %d, want 2", summary.ByPriority["low"])
	}
	if summary.QueueDepth != 2 {
		t.Errorf("QueueDepth = %d, want 2 (queued+sending)", summary.QueueDepth)
	}
	// delivered=2, failed=1 -> delivered rate 2/3*100, error rate 1/3*100
	if summary.DeliveryRate < 66 || summary.DeliveryRate > 67 {
		t.Errorf("DeliveryRate = %v, want ~66.67", summary.DeliveryRate)
	}
	if summary.ErrorRate < 33 || summary.ErrorRate > 34 {
		t.Errorf("ErrorRate = %v, want ~33.33", summary.ErrorRate)
	}
}

// TestNotificationMetrics_GetTimePeriodMetrics_WindowBoundary verifies rows
// outside the requested duration are excluded and rows just inside are
// included, exercising the created_at >= cutoff boundary.
func TestNotificationMetrics_GetTimePeriodMetrics_WindowBoundary(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	recent := 60     // 1 minute ago: inside a 24h window
	old := 2 * 86400 // 2 days ago: outside a 24h window

	insertQueueRow(t, db, "delivered", "email", 1, "", &recent, nil)
	insertQueueRow(t, db, "delivered", "email", 1, "", &old, nil)
	insertQueueRow(t, db, "failed", "sms", 1, "err", &recent, nil)

	metrics, err := nm.GetTimePeriodMetrics(24 * time.Hour)
	if err != nil {
		t.Fatalf("GetTimePeriodMetrics() error = %v", err)
	}
	if metrics.Total != 2 {
		t.Errorf("Total = %d, want 2 (only rows within 24h)", metrics.Total)
	}
	if metrics.Delivered != 1 {
		t.Errorf("Delivered = %d, want 1", metrics.Delivered)
	}
	if metrics.Failed != 1 {
		t.Errorf("Failed = %d, want 1", metrics.Failed)
	}
	if metrics.ByChannel["email"] != 1 || metrics.ByChannel["sms"] != 1 {
		t.Errorf("ByChannel = %+v, want email=1 sms=1", metrics.ByChannel)
	}
}

// TestNotificationMetrics_GetTimePeriodMetrics_Empty is the boundary case of
// no rows in the window at all.
func TestNotificationMetrics_GetTimePeriodMetrics_Empty(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	metrics, err := nm.GetTimePeriodMetrics(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("GetTimePeriodMetrics() error = %v", err)
	}
	if metrics.Total != 0 || metrics.Delivered != 0 || metrics.Failed != 0 || metrics.Pending != 0 {
		t.Errorf("expected all-zero metrics on empty table, got %+v", metrics)
	}
}

// TestNotificationMetrics_GetChannelMetrics covers a known channel with data
// and an unknown channel that must yield all-zero metrics rather than error.
func TestNotificationMetrics_GetChannelMetrics(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	insertQueueRow(t, db, "delivered", "email", 1, "", nil, nil)
	insertQueueRow(t, db, "delivered", "email", 1, "", nil, nil)
	insertQueueRow(t, db, "failed", "email", 1, "err", nil, nil)

	t.Run("known channel with data", func(t *testing.T) {
		cm, err := nm.GetChannelMetrics("email")
		if err != nil {
			t.Fatalf("GetChannelMetrics() error = %v", err)
		}
		if cm.Total != 3 {
			t.Errorf("Total = %d, want 3", cm.Total)
		}
		if cm.Delivered != 2 {
			t.Errorf("Delivered = %d, want 2", cm.Delivered)
		}
		if cm.Failed != 1 {
			t.Errorf("Failed = %d, want 1", cm.Failed)
		}
		if cm.DeliveryRate < 66 || cm.DeliveryRate > 67 {
			t.Errorf("DeliveryRate = %v, want ~66.67", cm.DeliveryRate)
		}
	})

	t.Run("unknown channel yields all zero, no error", func(t *testing.T) {
		cm, err := nm.GetChannelMetrics("carrier-pigeon")
		if err != nil {
			t.Fatalf("GetChannelMetrics() error = %v", err)
		}
		if cm.Total != 0 || cm.Delivered != 0 || cm.Failed != 0 {
			t.Errorf("expected all-zero metrics for unknown channel, got %+v", cm)
		}
	})
}

// TestNotificationMetrics_GetRecentErrors is a regression test for the
// production bug in GetRecentErrors: the query referenced a non-existent
// last_error column on notification_queue (the real column is
// error_message). This test fails against the pre-fix query with a SQLite
// "no such column" error and passes against the corrected query, also
// covering the state filter (failed/dead_letter only), the "IS NOT NULL"
// filter, and the limit boundary.
func TestNotificationMetrics_GetRecentErrors(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	insertQueueRow(t, db, "failed", "email", 1, "smtp timeout", nil, nil)
	insertQueueRow(t, db, "dead_letter", "sms", 1, "carrier rejected", nil, nil)
	// Delivered row must never appear, even though it has no error.
	insertQueueRow(t, db, "delivered", "email", 1, "", nil, nil)
	// Failed row with NULL error_message must be excluded by "IS NOT NULL".
	insertQueueRow(t, db, "failed", "email", 1, "", nil, nil)

	t.Run("returns only failed/dead_letter rows with non-null error", func(t *testing.T) {
		errs, err := nm.GetRecentErrors(10)
		if err != nil {
			t.Fatalf("GetRecentErrors() error = %v", err)
		}
		if len(errs) != 2 {
			t.Fatalf("GetRecentErrors() returned %d rows, want 2: %+v", len(errs), errs)
		}
		for _, e := range errs {
			state, _ := e["error"].(string)
			if state == "" {
				t.Errorf("row missing error message: %+v", e)
			}
		}
	})

	t.Run("limit boundary of zero returns empty slice", func(t *testing.T) {
		errs, err := nm.GetRecentErrors(0)
		if err != nil {
			t.Fatalf("GetRecentErrors(0) error = %v", err)
		}
		if len(errs) != 0 {
			t.Errorf("GetRecentErrors(0) returned %d rows, want 0", len(errs))
		}
	})

	t.Run("limit of one returns exactly one row", func(t *testing.T) {
		errs, err := nm.GetRecentErrors(1)
		if err != nil {
			t.Fatalf("GetRecentErrors(1) error = %v", err)
		}
		if len(errs) != 1 {
			t.Errorf("GetRecentErrors(1) returned %d rows, want 1", len(errs))
		}
	})
}

// TestNotificationMetrics_RecordMetric_Idempotent verifies that calling
// RecordMetric twice with identical arguments inserts two independent rows
// (it is an append-only metrics log, not an upsert) and that both calls
// succeed without error - i.e. repeating the operation is safe.
func TestNotificationMetrics_RecordMetric_Idempotent(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	for i := 0; i < 2; i++ {
		if err := nm.RecordMetric("latency", "email", 1.5); err != nil {
			t.Fatalf("RecordMetric() call %d error = %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM notification_metrics WHERE metric_type = 'latency' AND channel_type = 'email'").Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (RecordMetric appends, does not upsert)", count)
	}
}

// TestNotificationMetrics_RecordMetric_ZeroValue is a boundary test ensuring
// a zero value is recorded without error (not treated as "unset").
func TestNotificationMetrics_RecordMetric_ZeroValue(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	if err := nm.RecordMetric("queue_depth", "sms", 0); err != nil {
		t.Fatalf("RecordMetric(0) error = %v", err)
	}

	var value float64
	if err := db.QueryRow("SELECT value FROM notification_metrics WHERE metric_type = 'queue_depth'").Scan(&value); err != nil {
		t.Fatalf("query recorded value: %v", err)
	}
	if value != 0 {
		t.Errorf("recorded value = %v, want 0", value)
	}
}

// TestNotificationMetrics_GetHealthStatus_Healthy covers the happy path on
// an empty queue: healthy stays true and no warning key is set.
func TestNotificationMetrics_GetHealthStatus_Healthy(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	status := nm.GetHealthStatus()
	healthy, _ := status["healthy"].(bool)
	if !healthy {
		t.Errorf("healthy = %v, want true on empty queue", status["healthy"])
	}
	if _, ok := status["warning"]; ok {
		t.Errorf("warning present on healthy status: %+v", status)
	}
	if _, ok := status["timestamp"]; !ok {
		t.Errorf("timestamp missing from health status: %+v", status)
	}
}

// TestNotificationMetrics_GetHealthStatus_StuckNotifications covers the
// stuck-in-queue warning branch: a row queued more than an hour ago must
// flip healthy to false and report the stuck count.
func TestNotificationMetrics_GetHealthStatus_StuckNotifications(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	twoHoursAgo := 2 * 3600
	insertQueueRow(t, db, "queued", "email", 1, "", &twoHoursAgo, nil)

	status := nm.GetHealthStatus()
	healthy, _ := status["healthy"].(bool)
	if healthy {
		t.Errorf("healthy = %v, want false when a notification is stuck", status["healthy"])
	}
	warning, _ := status["warning"].(string)
	if warning == "" {
		t.Errorf("warning missing when a notification is stuck: %+v", status)
	}
}

// TestNotificationMetrics_GetHealthStatus_HighErrorRate covers the recent
// error-rate warning branch (>50% failures within the last hour).
func TestNotificationMetrics_GetHealthStatus_HighErrorRate(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	recent := 60
	insertQueueRow(t, db, "failed", "email", 1, "err", &recent, nil)
	insertQueueRow(t, db, "failed", "email", 1, "err", &recent, nil)
	insertQueueRow(t, db, "delivered", "email", 1, "", &recent, nil)

	status := nm.GetHealthStatus()
	healthy, _ := status["healthy"].(bool)
	if healthy {
		t.Errorf("healthy = %v, want false with 2/3 recent failures", status["healthy"])
	}
	if _, ok := status["recent_error_rate"]; !ok {
		t.Errorf("recent_error_rate missing: %+v", status)
	}
	warning, _ := status["warning"].(string)
	if warning == "" {
		t.Errorf("warning missing with high error rate: %+v", status)
	}
}
