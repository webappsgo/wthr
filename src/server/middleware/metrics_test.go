package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestNormalizeMetricPath covers cardinality-control path normalization -
// AI.md PART 21 requires dynamic segments collapsed so metric label sets
// stay bounded regardless of how many distinct IDs/UUIDs are requested.
func TestNormalizeMetricPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty path becomes root", "", "/"},
		{"numeric id collapsed", "/api/v1/users/42", "/api/v1/users/:id/"},
		{"numeric id at end no trailing slash still collapsed", "/api/v1/users/42", "/api/v1/users/:id/"},
		{"uuid collapsed", "/api/v1/sessions/550e8400-e29b-41d4-a716-446655440000", "/api/v1/sessions/:id"},
		{"ulid collapsed", "/api/v1/tokens/01ARZ3NDEKTSV4RRFFQ69G5FAV", "/api/v1/tokens/:id"},
		{"no dynamic segment unchanged", "/api/v1/weather", "/api/v1/weather"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMetricPath(tt.path); got != tt.want {
				t.Errorf("normalizeMetricPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestMetricsMiddleware_RecordsRequestWithoutError verifies the middleware
// runs end-to-end (registers/increments/observes on the promauto
// package-level collectors) without panicking, and does not interfere with
// the response. Prometheus collectors are registered once at package init
// (promauto vars), so invoking MetricsMiddleware() repeatedly across
// subtests/requests must never panic on duplicate registration.
func TestMetricsMiddleware_RecordsRequestWithoutError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MetricsMiddleware())
	router.GET("/api/v1/weather/:city", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Multiple requests, including one with content-length, exercise the
	// request-size observation branch too.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/weather/london", nil)
		req.ContentLength = 12
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("iteration %d: status = %d, want 200", i, w.Code)
		}
	}
}

// TestMetricsMiddleware_TracksErrorStatus verifies the middleware completes
// normally (does not abort/alter the response) for a non-2xx response, since
// metrics must be recorded for failed requests too.
func TestMetricsMiddleware_TracksErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MetricsMiddleware())
	router.GET("/api/v1/broken", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/broken", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
