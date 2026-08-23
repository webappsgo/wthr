package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestBodySizeLimitMiddleware_RejectsOverLimitByContentLength verifies the
// early rejection path (Content-Length header exceeds max) returns 413
// without reading the body.
func TestBodySizeLimitMiddleware_RejectsOverLimitByContentLength(t *testing.T) {
	const maxSize = 100

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := BodySizeLimitMiddleware(maxSize)(next)

	body := bytes.Repeat([]byte("a"), 200)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d (body=%s)", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
}

// TestBodySizeLimitMiddleware_AcceptsUnderLimit verifies a body within the
// configured limit passes through and is fully readable by the handler.
func TestBodySizeLimitMiddleware_AcceptsUnderLimit(t *testing.T) {
	const maxSize = 100

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("read error: " + err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("read " + strconv.Itoa(len(data)) + " bytes"))
	})
	handler := BodySizeLimitMiddleware(maxSize)(next)

	body := strings.Repeat("a", 50)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

// TestBodySizeLimitMiddleware_RejectsAtReadTimeWithoutContentLength verifies
// that when Content-Length is absent/unreliable, the http.MaxBytesReader
// wrapper still enforces the limit once the handler actually reads past it
// (chunked-encoding style bodies can't be pre-checked via header alone).
func TestBodySizeLimitMiddleware_RejectsAtReadTimeWithoutContentLength(t *testing.T) {
	const maxSize = 10

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte("too large: " + err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := BodySizeLimitMiddleware(maxSize)(next)

	body := strings.Repeat("a", 500)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.ContentLength = -1 // simulate unknown length (chunked)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("handler reported success reading an oversized body with unknown Content-Length, want read-time rejection")
	}
}

// TestBodySizeLimitMiddleware_SkipsSafeMethods verifies GET/HEAD/OPTIONS are
// never subject to body-size enforcement, even with an oversized
// Content-Length header (these methods should not carry meaningful bodies).
func TestBodySizeLimitMiddleware_SkipsSafeMethods(t *testing.T) {
	const maxSize = 10

	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			handler := BodySizeLimitMiddleware(maxSize)(next)

			req := httptest.NewRequest(method, "/", nil)
			req.Header.Set("Content-Length", "999999")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200 (safe methods must skip body limit)", method, w.Code)
			}
		})
	}
}
