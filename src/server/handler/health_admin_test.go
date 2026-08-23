package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
)

// TestAdminServerStatus_Healthy verifies the admin status endpoint returns
// 200 with a healthy status when the wired DB responds normally.
func TestAdminServerStatus_Healthy(t *testing.T) {
	db := newTestDatabaseDB(t)

	handlerFunc := AdminServerStatus(db, "8090", 8443, nil)
	c, w := newAPITestContext("/server/admin/config/status")

	handlerFunc(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"healthy"`) {
		t.Errorf("expected healthy status in body, got: %s", body)
	}
}

// TestBuildAdminServerStatusResponse_Healthy verifies the response builder
// returns 200 and a JSON payload with the expected top-level keys when the
// wired DB responds normally.
func TestBuildAdminServerStatusResponse_Healthy(t *testing.T) {
	db := newTestDatabaseDB(t)
	c, _ := newAPITestContext("/server/admin/config/status")

	httpStatus, response := buildAdminServerStatusResponse(db, c, "8090", 8443, nil)

	if httpStatus != http.StatusOK {
		t.Fatalf("expected status 200, got %d", httpStatus)
	}
	for _, key := range []string{"status", "timestamp", "version", "uptime_seconds", "checks"} {
		if _, ok := response[key]; !ok {
			t.Errorf("expected response to contain key %q, got: %+v", key, response)
		}
	}
	if response["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", response["status"])
	}
}

// seedAuditEntry inserts one server_audit_log row with an explicit timestamp
// text, so a test can plant the exact on-disk encoding it wants to exercise
// (ulid and action are NOT NULL in ServerSchema).
func seedAuditEntry(t *testing.T, db *sql.DB, ulid, status, timestamp string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO server_audit_log (ulid, action, status, timestamp)
		VALUES (?, 'test.action', ?, ?)
	`, ulid, status, timestamp); err != nil {
		t.Fatalf("seed audit entry %s: %v", ulid, err)
	}
}

// TestGetRequestStats_CountsZoneSkewedRowsByInstant is the regression test for
// the three audit-log cutoffs in getRequestStats. They were expressed as
// "timestamp >= date('now', 'start of day')" and
// "timestamp >= datetime('now', '-1 minute')", both of which return NULL for a
// row stored in the driver's local-zone time.Time.String() layout, so such rows
// silently vanished from every count. The first subtest plants exactly one such
// row and fails against that implementation, on any host timezone.
func TestGetRequestStats_CountsZoneSkewedRowsByInstant(t *testing.T) {
	now := time.Now()

	t.Run("zone-skewed row whose text reads as past is still counted", func(t *testing.T) {
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
		seedAuditEntry(t, serverDB, "audit-west", "success", now.In(handlerZoneWest).Format(handlerLocalLayout))

		stats := getRequestStats()
		if stats["total_today"] != 1 {
			t.Errorf("total_today = %v, want 1", stats["total_today"])
		}
		if stats["rate_per_minute"] != 1 {
			t.Errorf("rate_per_minute = %v, want 1", stats["rate_per_minute"])
		}
	})

	t.Run("old row whose zone-skewed text reads as recent is not counted", func(t *testing.T) {
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
		seedAuditEntry(t, serverDB, "audit-east", "success", now.Add(-25*time.Hour).In(handlerZoneEast).Format(handlerLocalLayout))

		stats := getRequestStats()
		if stats["total_today"] != 0 {
			t.Errorf("total_today = %v, want 0", stats["total_today"])
		}
		if stats["rate_per_minute"] != 0 {
			t.Errorf("rate_per_minute = %v, want 0", stats["rate_per_minute"])
		}
	})

	t.Run("unparseable timestamp is ignored rather than counted", func(t *testing.T) {
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
		seedAuditEntry(t, serverDB, "audit-bad", "error", "not-a-timestamp")

		stats := getRequestStats()
		if stats["total_today"] != 0 {
			t.Errorf("total_today = %v, want 0", stats["total_today"])
		}
		if stats["errors_today"] != 0 {
			t.Errorf("errors_today = %v, want 0", stats["errors_today"])
		}
	})

	t.Run("error rate is computed from today's canonical rows", func(t *testing.T) {
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
		seedAuditEntry(t, serverDB, "audit-ok", "success", dbtime.FormatSQLTimestamp(now))
		seedAuditEntry(t, serverDB, "audit-err", "error", dbtime.FormatSQLTimestamp(now))

		stats := getRequestStats()
		if stats["total_today"] != 2 {
			t.Errorf("total_today = %v, want 2", stats["total_today"])
		}
		if stats["errors_today"] != 1 {
			t.Errorf("errors_today = %v, want 1", stats["errors_today"])
		}
		if stats["error_rate"] != 50.0 {
			t.Errorf("error_rate = %v, want 50", stats["error_rate"])
		}
	})
}
