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

// insertQueueRowAt inserts a notification_queue row whose created_at/updated_at
// hold the exact text given, so a test can plant a specific on-disk timestamp
// encoding instead of whatever datetime('now') produces.
func insertQueueRowAt(t *testing.T, db *sql.DB, state, channel, createdAt string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO notification_queue (user_id, channel_type, priority, state, subject, body, created_at, updated_at)
		VALUES (1, ?, 1, ?, 'subject', 'body', ?, ?)
	`, channel, state, createdAt, createdAt); err != nil {
		t.Fatalf("insert notification_queue row: %v", err)
	}
}

// TestNotificationMetrics_GetTimePeriodMetrics_CountsByInstant is the
// regression test for the reporting window, which used to be expressed as
// "created_at >= datetime('now', '-N hours')". That predicate returns NULL for
// a row stored in the driver's local-zone time.Time.String() layout, so such
// rows silently vanished from every period report, while a row whose skewed
// text reads as recent but whose true instant is days old would be counted by
// any text comparison. Both fixtures below contradict text ordering, so this
// test fails against the old implementation on any host timezone.
func TestNotificationMetrics_GetTimePeriodMetrics_CountsByInstant(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)
	now := time.Now()

	insertQueueRowAt(t, db, "delivered", "email", now.Add(-time.Minute).In(serviceZoneWest).Format(serviceLocalLayout))
	insertQueueRowAt(t, db, "failed", "sms", now.Add(-72*time.Hour).In(serviceZoneEast).Format(serviceLocalLayout))
	insertQueueRowAt(t, db, "queued", "email", "not-a-timestamp")

	metrics, err := nm.GetTimePeriodMetrics(24 * time.Hour)
	if err != nil {
		t.Fatalf("GetTimePeriodMetrics() error = %v", err)
	}
	if metrics.Total != 1 {
		t.Errorf("Total = %d, want 1 (only the row whose true instant is inside the window)", metrics.Total)
	}
	if metrics.Delivered != 1 {
		t.Errorf("Delivered = %d, want 1", metrics.Delivered)
	}
	if metrics.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (the failed row is three days old however recent its text reads)", metrics.Failed)
	}
	if metrics.Pending != 0 {
		t.Errorf("Pending = %d, want 0 (an unparseable created_at is never counted)", metrics.Pending)
	}
}

// TestNotificationMetrics_GetHealthStatus_StuckDetectionByInstant is the
// regression test for the stuck-notification cutoff, previously
// "created_at < datetime('now', '-1 hour')". A row queued three hours ago but
// written in a zone whose text reads later than UTC was never reported stuck.
func TestNotificationMetrics_GetHealthStatus_StuckDetectionByInstant(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	insertQueueRowAt(t, db, "queued", "email", time.Now().Add(-3*time.Hour).In(serviceZoneEast).Format(serviceLocalLayout))

	status := nm.GetHealthStatus()
	stuck, _ := status["stuck_notifications"].(int64)
	if stuck != 1 {
		t.Errorf("stuck_notifications = %v, want 1", status["stuck_notifications"])
	}
	if healthy, _ := status["healthy"].(bool); healthy {
		t.Errorf("healthy = %v, want false when a notification is stuck", status["healthy"])
	}
}

// TestNotificationMetrics_GetHealthStatus_UnparseableCreatedAtIgnored asserts
// the fail-closed half of the same rewrite: a row whose created_at cannot be
// interpreted is neither reported stuck nor counted toward the recent error
// rate.
func TestNotificationMetrics_GetHealthStatus_UnparseableCreatedAtIgnored(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	insertQueueRowAt(t, db, "queued", "email", "not-a-timestamp")

	status := nm.GetHealthStatus()
	if stuck, _ := status["stuck_notifications"].(int64); stuck != 0 {
		t.Errorf("stuck_notifications = %v, want 0", status["stuck_notifications"])
	}
	if _, ok := status["recent_error_rate"]; ok {
		t.Errorf("recent_error_rate should be absent when no row has an interpretable created_at: %+v", status)
	}
}

// insertDeliveredQueueRowAt inserts a delivered notification_queue row whose
// created_at and delivered_at hold the exact text given, so a test controls the
// on-disk encoding of both ends of the delivery interval.
func insertDeliveredQueueRowAt(t *testing.T, db *sql.DB, channel, createdAt, deliveredAt string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO notification_queue (user_id, channel_type, priority, state, subject, body, created_at, updated_at, delivered_at)
		VALUES (1, ?, 1, 'delivered', 'subject', 'body', ?, ?, ?)
	`, channel, createdAt, createdAt, deliveredAt); err != nil {
		t.Fatalf("insert delivered notification_queue row: %v", err)
	}
}

// deliveryTolerance is the slack allowed when comparing an averaged latency, so
// the assertion never depends on sub-second timing of the test itself.
const deliveryTolerance = 0.001

// TestNotificationMetrics_GetSummary_AvgDeliveryByInstant is the regression
// test for the average-delivery-time rewrite. The old query computed
// "AVG((julianday(delivered_at) - julianday(created_at)) * 86400)" in SQL.
// julianday() evaluates to NULL for the driver's local-zone
// time.Time.String() layout, so every row written that way was dropped from the
// average entirely: the old code reports 90 here (the canonical-UTC row alone)
// instead of the true 60. The unparseable row must be skipped either way.
func TestNotificationMetrics_GetSummary_AvgDeliveryByInstant(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)
	now := time.Now()

	// 30 seconds of true latency, both ends written in a zone 11 hours behind
	// UTC, which no SQL date function in this project's dialects can read.
	westCreated := now.Add(-2 * time.Hour)
	insertDeliveredQueueRowAt(t, db, "email",
		westCreated.In(serviceZoneWest).Format(serviceLocalLayout),
		westCreated.Add(30*time.Second).In(serviceZoneWest).Format(serviceLocalLayout))

	// 90 seconds of true latency in the canonical UTC text CURRENT_TIMESTAMP
	// emits - the only row the old query could see.
	utcCreated := now.Add(-time.Hour).UTC()
	insertDeliveredQueueRowAt(t, db, "email",
		utcCreated.Format("2006-01-02 15:04:05"),
		utcCreated.Add(90*time.Second).Format("2006-01-02 15:04:05"))

	// Fail closed: an uninterpretable delivered_at contributes nothing rather
	// than a zero-second delivery that would drag the average down.
	insertDeliveredQueueRowAt(t, db, "email",
		utcCreated.Format("2006-01-02 15:04:05"), "not-a-timestamp")

	summary, err := nm.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}

	want := 60.0
	if diff := summary.AvgDeliveryTime - want; diff > deliveryTolerance || diff < -deliveryTolerance {
		t.Errorf("AvgDeliveryTime = %v, want %v (mean of the 30s and 90s rows, the unparseable row skipped)", summary.AvgDeliveryTime, want)
	}
}

// TestNotificationMetrics_GetChannelMetrics_AvgDeliveryAcrossZones covers the
// per-channel path of the same rewrite with the harshest fixture: created_at is
// stored 13 hours ahead of UTC and delivered_at 11 hours behind it, so the
// stored TEXT of the later instant sorts a full day EARLIER than the text of
// the earlier one. Any SQL-side arithmetic over those strings yields NULL or a
// large negative latency; parsed as instants the gap is ten seconds.
func TestNotificationMetrics_GetChannelMetrics_AvgDeliveryAcrossZones(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	created := time.Now().Add(-30 * time.Minute)
	insertDeliveredQueueRowAt(t, db, "sms",
		created.In(serviceZoneEast).Format(serviceLocalLayout),
		created.Add(10*time.Second).In(serviceZoneWest).Format(serviceLocalLayout))

	metrics, err := nm.GetChannelMetrics("sms")
	if err != nil {
		t.Fatalf("GetChannelMetrics() error = %v", err)
	}

	want := 10.0
	if diff := metrics.AvgDeliveryTime - want; diff > deliveryTolerance || diff < -deliveryTolerance {
		t.Errorf("AvgDeliveryTime = %v, want %v (true instant gap, not text arithmetic)", metrics.AvgDeliveryTime, want)
	}
}

// insertFailedQueueRowAt inserts a failed notification_queue row carrying an
// error message, with created_at/updated_at holding the exact text given, so a
// test can plant a specific on-disk timestamp encoding under GetRecentErrors.
func insertFailedQueueRowAt(t *testing.T, db *sql.DB, subject, errMsg, createdAt string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO notification_queue (user_id, channel_type, priority, state, subject, body, error_message, created_at, updated_at)
		VALUES (1, 'email', 1, 'failed', ?, 'body', ?, ?, ?)
	`, subject, errMsg, createdAt, createdAt); err != nil {
		t.Fatalf("insert failed notification_queue row: %v", err)
	}
}

// TestNotificationMetrics_GetRecentErrors_CanonicalTimestamps is the regression
// test for created_at leaving GetRecentErrors as whatever raw text happened to
// be on disk. The column can hold the canonical UTC text CURRENT_TIMESTAMP
// writes or the driver's local-zone time.Time.String() form, and the old code
// scanned it into a plain string and handed it straight to API consumers, so the
// same endpoint emitted several different, zone-ambiguous timestamp formats.
// Every returned timestamp must now be RFC 3339 UTC, and an uninterpretable
// value must come back as an empty string rather than leaked raw text.
func TestNotificationMetrics_GetRecentErrors_CanonicalTimestamps(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	canonicalInstant := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	legacyInstant := time.Date(2026, 3, 5, 6, 30, 0, 0, time.UTC)

	// The canonical UTC text CURRENT_TIMESTAMP emits.
	insertFailedQueueRowAt(t, db, "canonical", "smtp timeout", canonicalInstant.Format("2006-01-02 15:04:05"))
	// The legacy local-zone time.Time.String() form the SQLite driver writes for
	// a bound time.Time, planted at a fixed numeric offset so the stored text
	// disagrees with the canonical row's text ordering.
	insertFailedQueueRowAt(t, db, "legacy", "carrier rejected", legacyInstant.In(serviceZoneEast).Format(serviceLocalLayout))
	// Fail-safe case: nothing in the column can be interpreted.
	insertFailedQueueRowAt(t, db, "unparseable", "unknown failure", "not-a-timestamp")

	errs, err := nm.GetRecentErrors(10)
	if err != nil {
		t.Fatalf("GetRecentErrors() error = %v", err)
	}
	if len(errs) != 3 {
		t.Fatalf("GetRecentErrors() returned %d rows, want 3: %+v", len(errs), errs)
	}

	got := make(map[string]string, len(errs))
	for _, entry := range errs {
		subject, _ := entry["subject"].(string)
		createdAt, ok := entry["created_at"].(string)
		if !ok {
			t.Fatalf("created_at for %q is %T, want string", subject, entry["created_at"])
		}
		got[subject] = createdAt
	}

	want := map[string]string{
		"canonical":   canonicalInstant.Format(time.RFC3339),
		"legacy":      legacyInstant.Format(time.RFC3339),
		"unparseable": "",
	}

	for subject, wantCreatedAt := range want {
		if got[subject] != wantCreatedAt {
			t.Errorf("created_at for %q = %q, want %q", subject, got[subject], wantCreatedAt)
		}
	}

	// Both interpretable rows must be readable back as RFC 3339, which the raw
	// stored text of the legacy row is not.
	for _, subject := range []string{"canonical", "legacy"} {
		if _, parseErr := time.Parse(time.RFC3339, got[subject]); parseErr != nil {
			t.Errorf("created_at for %q (%q) is not RFC 3339: %v", subject, got[subject], parseErr)
		}
	}
}

// TestNotificationMetrics_AvgDeliveryTime_NoUsableRows asserts the empty case
// still reports 0 rather than a NaN from dividing by a zero row count.
func TestNotificationMetrics_AvgDeliveryTime_NoUsableRows(t *testing.T) {
	db := setupNotificationMetricsTestDB(t)
	nm := NewNotificationMetrics(db)

	insertDeliveredQueueRowAt(t, db, "email", "not-a-timestamp", "not-a-timestamp")

	summary, err := nm.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if summary.AvgDeliveryTime != 0 {
		t.Errorf("AvgDeliveryTime = %v, want 0 when no row has interpretable timestamps", summary.AvgDeliveryTime)
	}
}
