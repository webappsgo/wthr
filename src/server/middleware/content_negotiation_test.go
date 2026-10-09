package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContentNegotiation(t *testing.T) {
	jsonHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	errHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ok":false}`))
	})
	htmlHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p>hi</p>`))
	})
	badJSON := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{nope`))
	})

	tests := []struct {
		name     string
		h        http.Handler
		path     string
		accept   string
		wantCT   string
		wantBody string
		wantCode int
	}{
		{"accept text/plain converts", jsonHandler, "/api/v1/x", "text/plain", "text/plain; charset=utf-8", "{\n  \"ok\": true\n}\n", 200},
		{"txt extension converts", jsonHandler, "/api/v1/x.txt", "", "text/plain; charset=utf-8", "{\n  \"ok\": true\n}\n", 200},
		{"json accept untouched", jsonHandler, "/api/v1/x", "application/json", "application/json; charset=utf-8", `{"ok":true}`, 200},
		{"non-api path untouched", jsonHandler, "/server/x", "text/plain", "application/json; charset=utf-8", `{"ok":true}`, 200},
		{"error status passes through", errHandler, "/api/v1/x", "text/plain", "application/json; charset=utf-8", `{"ok":false}`, 404},
		{"html passes through", htmlHandler, "/api/v1/x", "text/plain", "text/html; charset=utf-8", `<p>hi</p>`, 200},
		{"invalid json verbatim", badJSON, "/api/v1/x", "text/plain", "application/json", `{nope`, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			w := httptest.NewRecorder()
			ContentNegotiation(tt.h).ServeHTTP(w, req)
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if ct := w.Header().Get("Content-Type"); ct != tt.wantCT {
				t.Errorf("Content-Type = %q, want %q", ct, tt.wantCT)
			}
			if w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestContentNegotiationFlushCommits(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":1}`))
		w.(http.Flusher).Flush()
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Accept", "text/plain")
	w := httptest.NewRecorder()
	ContentNegotiation(h).ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"a"`) {
		t.Errorf("body lost after flush: %q", w.Body.String())
	}
}
