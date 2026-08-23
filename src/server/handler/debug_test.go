package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestNewDebugHandlers(t *testing.T) {
	db := newTestServerDB(t)
	router := chi.NewRouter()

	h := NewDebugHandlers(db, router)
	if h == nil {
		t.Fatal("NewDebugHandlers returned nil")
	}
	if h.db != db {
		t.Error("db not set correctly")
	}
	if h.router != router {
		t.Error("router not set correctly (expected chi.Router)")
	}
}

func TestDebugHandlers_ListRoutes(t *testing.T) {
	db := newTestServerDB(t)
	router := chi.NewRouter()
	router.Get("/example", func(w http.ResponseWriter, r *http.Request) {})

	h := NewDebugHandlers(db, router)

	r := httptest.NewRequest(http.MethodGet, "/debug/routes", nil)
	w := httptest.NewRecorder()

	h.ListRoutes(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "routes") {
		t.Error("expected response to contain 'routes'")
	}
}

func TestDebugHandlers_ShowMemory(t *testing.T) {
	db := newTestServerDB(t)
	router := chi.NewRouter()
	h := NewDebugHandlers(db, router)

	r := httptest.NewRequest(http.MethodGet, "/debug/memory", nil)
	w := httptest.NewRecorder()

	h.ShowMemory(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "alloc_mb") {
		t.Error("expected response to contain 'alloc_mb'")
	}
}

func TestDebugHandlers_TriggerGC(t *testing.T) {
	db := newTestServerDB(t)
	router := chi.NewRouter()
	h := NewDebugHandlers(db, router)

	r := httptest.NewRequest(http.MethodPost, "/debug/gc", nil)
	w := httptest.NewRecorder()

	h.TriggerGC(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Garbage collection triggered") {
		t.Error("expected response to contain 'Garbage collection triggered'")
	}
}

func TestDebugHandlers_ReloadConfig(t *testing.T) {
	db := newTestServerDB(t)
	router := chi.NewRouter()
	h := NewDebugHandlers(db, router)

	r := httptest.NewRequest(http.MethodPost, "/debug/reload", nil)
	w := httptest.NewRecorder()

	h.ReloadConfig(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Configuration reload triggered") {
		t.Error("expected response to contain 'Configuration reload triggered'")
	}
}
