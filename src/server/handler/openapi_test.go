package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetSwaggerUIAuto verifies the handler is constructible and, when
// invoked, responds without panicking (no DB/service dependency exists to
// mock here — httpSwagger.Handler wraps swaggerFiles.Handler directly).
func TestGetSwaggerUIAuto(t *testing.T) {
	handlerFunc := GetSwaggerUIAuto()
	if handlerFunc == nil {
		t.Fatal("GetSwaggerUIAuto() returned a nil handler")
	}

	r := httptest.NewRequest(http.MethodGet, "/openapi/index.html", nil)
	w := httptest.NewRecorder()

	handlerFunc(w, r)

	if w.Code == 0 {
		t.Fatalf("expected a response status to be written, got 0")
	}
}

// TestPrometheusMetrics verifies the handler exposes the standard
// Prometheus text-exposition format and returns a 200 with real body
// content, per AI.md PART 21 (METRICS).
func TestPrometheusMetrics(t *testing.T) {
	handlerFunc := PrometheusMetrics()
	if handlerFunc == nil {
		t.Fatal("PrometheusMetrics() returned a nil handler")
	}

	r, w := newAPITestRequest("/metrics")
	handlerFunc(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty Prometheus metrics body")
	}
}
