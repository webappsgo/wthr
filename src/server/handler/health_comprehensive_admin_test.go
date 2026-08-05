package handler

import (
	"net/http"
	"strings"
	"testing"
)

// TestAdminServerStatus_Healthy verifies the admin status endpoint returns
// 200 with a healthy status when the wired DB responds normally.
func TestAdminServerStatus_Healthy(t *testing.T) {
	db := newTestDatabaseDB(t)

	handlerFunc := AdminServerStatus(db, "8090", 8443, nil)
	c, w := newAPITestContext("/server/admin/status")

	handlerFunc(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"healthy"`) {
		t.Errorf("expected healthy status in body, got: %s", body)
	}
}

// TestBuildAdminServerStatusResponse_Healthy verifies the response builder
// returns 200 and a gin.H payload with the expected top-level keys when the
// wired DB responds normally.
func TestBuildAdminServerStatusResponse_Healthy(t *testing.T) {
	db := newTestDatabaseDB(t)
	c, _ := newAPITestContext("/server/admin/status")

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
