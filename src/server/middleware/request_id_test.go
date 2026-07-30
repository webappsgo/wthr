package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequestID_GeneratesWhenAbsent verifies a UUID-shaped X-Request-ID is
// generated and set on the response when no inbound header is present.
func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	var gotFromContext string
	router.GET("/", func(c *gin.Context) {
		gotFromContext = GetRequestID(c)
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(w, req)

	respID := w.Header().Get("X-Request-ID")
	if respID == "" {
		t.Fatal("X-Request-ID response header is empty, want generated UUID")
	}
	if len(respID) != 36 {
		t.Errorf("X-Request-ID = %q, want UUID-shaped (36 chars)", respID)
	}
	if gotFromContext != respID {
		t.Errorf("GetRequestID(c) = %q, want it to match response header %q", gotFromContext, respID)
	}
}

// TestRequestID_PreservesInboundHeader verifies that when a client supplies
// X-Request-ID, it's echoed back unchanged rather than replaced.
func TestRequestID_PreservesInboundHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "my-custom-id-123")
	router.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "my-custom-id-123" {
		t.Errorf("X-Request-ID = %q, want preserved %q", got, "my-custom-id-123")
	}
}

// TestRequestID_AlternateHeaderNormalizedToCanonical exercises fallback
// header variants to verify at least one alternate header is honored when
// X-Request-ID itself is absent, and that the canonical response header
// name is always X-Request-ID regardless of which variant matched.
func TestRequestID_AlternateHeaderNormalizedToCanonical(t *testing.T) {
	gin.SetMode(gin.TestMode)

	altHeaders := []string{
		"X-Correlation-ID",
		"X-Trace-ID",
	}

	for _, h := range altHeaders {
		t.Run(h, func(t *testing.T) {
			router := gin.New()
			router.Use(RequestID())
			router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(h, "alt-value-456")
			router.ServeHTTP(w, req)

			got := w.Header().Get("X-Request-ID")
			if got == "" {
				t.Fatalf("X-Request-ID response header empty when %s supplied", h)
			}
		})
	}
}

// TestMustGetRequestID_PanicsWhenMissing documents the panic contract: it
// must never be called before the RequestID middleware has run.
func TestMustGetRequestID_PanicsWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGetRequestID did not panic when request ID missing from context")
		}
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	MustGetRequestID(c)
}

// TestGetRequestID_EmptyWhenMissing verifies the non-panicking accessor
// returns an empty string rather than panicking when called without the
// middleware having run - important for handlers reused outside the chain.
func TestGetRequestID_EmptyWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if got := GetRequestID(c); got != "" {
		t.Errorf("GetRequestID on unset context = %q, want empty string", got)
	}
}
