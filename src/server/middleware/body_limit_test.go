package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestBodySizeLimitMiddleware_RejectsOverLimitByContentLength verifies the
// early rejection path (Content-Length header exceeds max) returns 413
// without reading the body.
func TestBodySizeLimitMiddleware_RejectsOverLimitByContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxSize = 100

	router := gin.New()
	router.Use(BodySizeLimitMiddleware(maxSize))
	router.POST("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	body := bytes.Repeat([]byte("a"), 200)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d (body=%s)", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
}

// TestBodySizeLimitMiddleware_AcceptsUnderLimit verifies a body within the
// configured limit passes through and is fully readable by the handler.
func TestBodySizeLimitMiddleware_AcceptsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxSize = 100

	router := gin.New()
	router.Use(BodySizeLimitMiddleware(maxSize))
	router.POST("/", func(c *gin.Context) {
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "read error: %v", err)
			return
		}
		c.String(http.StatusOK, "read %d bytes", len(data))
	})

	body := strings.Repeat("a", 50)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

// TestBodySizeLimitMiddleware_RejectsAtReadTimeWithoutContentLength verifies
// that when Content-Length is absent/unreliable, the http.MaxBytesReader
// wrapper still enforces the limit once the handler actually reads past it
// (chunked-encoding style bodies can't be pre-checked via header alone).
func TestBodySizeLimitMiddleware_RejectsAtReadTimeWithoutContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxSize = 10

	router := gin.New()
	router.Use(BodySizeLimitMiddleware(maxSize))
	router.POST("/", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large: %v", err)
			return
		}
		c.String(http.StatusOK, "ok")
	})

	body := strings.Repeat("a", 500)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.ContentLength = -1 // simulate unknown length (chunked)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("handler reported success reading an oversized body with unknown Content-Length, want read-time rejection")
	}
}

// TestBodySizeLimitMiddleware_SkipsSafeMethods verifies GET/HEAD/OPTIONS are
// never subject to body-size enforcement, even with an oversized
// Content-Length header (these methods should not carry meaningful bodies).
func TestBodySizeLimitMiddleware_SkipsSafeMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxSize = 10

	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			router := gin.New()
			router.Use(BodySizeLimitMiddleware(maxSize))
			router.Handle(method, "/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			req := httptest.NewRequest(method, "/", nil)
			req.Header.Set("Content-Length", "999999")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200 (safe methods must skip body limit)", method, w.Code)
			}
		})
	}
}
