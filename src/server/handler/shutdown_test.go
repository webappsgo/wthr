package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/lifecycle"
)

// TestHealthReportsShuttingDown verifies readiness turns not-ready (503
// shutting_down) as soon as shutdown begins (AI.md PART 8 shutdown flow, a).
func TestHealthReportsShuttingDown(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("LOG_DIR", t.TempDir())
	resetInitStatus(t)
	SetInitStatus(true, true, true)
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, nil)

	lifecycle.Reset()
	defer lifecycle.Reset()
	lifecycle.BeginShutdown()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	APIHealthCheck(&database.DB{DB: serverDB}, time.Now())(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "shutting_down" {
		t.Fatalf("status = %v, want shutting_down", body["status"])
	}
}

// TestStreamLogsSendsShutdownEvent verifies an SSE stream announces its retry
// delay and ends with `event: shutdown` when the server shuts down (b).
func TestStreamLogsSendsShutdownEvent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wthr.log"), []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lifecycle.Reset()
	defer lifecycle.Reset()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/logs/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		NewLogsHandler(dir).StreamLogs(w, r)
		close(done)
	}()
	lifecycle.BeginShutdown()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not end on shutdown")
	}

	body := w.Body.String()
	if !strings.Contains(body, "retry: 3000") || !strings.Contains(body, "event: shutdown") {
		t.Fatalf("SSE body = %q", body)
	}
}
