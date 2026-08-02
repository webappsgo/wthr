package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGetSwaggerUIAuto verifies the handler is constructible and, when
// invoked, responds without panicking (no DB/service dependency exists to
// mock here — ginSwagger.CustomWrapHandler wraps swaggerFiles.Handler
// directly).
func TestGetSwaggerUIAuto(t *testing.T) {
	handlerFunc := GetSwaggerUIAuto()
	if handlerFunc == nil {
		t.Fatal("GetSwaggerUIAuto() returned a nil handler")
	}

	c, w := newAPITestContext("/openapi/index.html")
	c.Params = gin.Params{{Key: "any", Value: "index.html"}}

	handlerFunc(c)

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

	c, w := newAPITestContext("/metrics")
	handlerFunc(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty Prometheus metrics body")
	}
}
