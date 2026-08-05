package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestNewDebugHandlers verifies the constructor wires both dependencies
// into the returned handler as passed.
func TestNewDebugHandlers(t *testing.T) {
	db := newTestServerDB(t)
	router := gin.New()
	h := NewDebugHandlers(db, router)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.db != db {
		t.Error("expected db field to be the passed *sql.DB")
	}
	if h.router != router {
		t.Error("expected router field to be the passed *gin.Engine")
	}
}

// TestDebugHandlers_ListRoutes verifies routes registered on the wired
// router are reflected in the JSON response.
func TestDebugHandlers_ListRoutes(t *testing.T) {
	router := gin.New()
	router.GET("/example", func(c *gin.Context) {})
	h := NewDebugHandlers(nil, router)

	c, w := newAPITestContext("/debug/routes")
	h.ListRoutes(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/example") {
		t.Errorf("expected route list to contain /example, got: %s", w.Body.String())
	}
}

// TestDebugHandlers_ShowMemory verifies the memory stats endpoint responds
// 200 with real runtime.MemStats fields populated, with no DB dependency.
func TestDebugHandlers_ShowMemory(t *testing.T) {
	h := &DebugHandlers{}
	c, w := newAPITestContext("/debug/memory")

	h.ShowMemory(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alloc_mb") {
		t.Errorf("expected body to contain alloc_mb, got: %s", w.Body.String())
	}
}

// TestDebugHandlers_TriggerGC verifies triggering GC responds 200 with
// before/after/freed memory fields, with no DB dependency.
func TestDebugHandlers_TriggerGC(t *testing.T) {
	h := &DebugHandlers{}
	c, w := newAPITestContext("/debug/gc")

	h.TriggerGC(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "freed_mb") {
		t.Errorf("expected body to contain freed_mb, got: %s", w.Body.String())
	}
}

// TestDebugHandlers_ReloadConfig verifies the static reload-notice response,
// with no DB dependency.
func TestDebugHandlers_ReloadConfig(t *testing.T) {
	h := &DebugHandlers{}
	c, w := newAPITestContext("/debug/reload")

	h.ReloadConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Configuration reload triggered") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}
