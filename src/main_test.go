package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetDefaultListenAddress covers the IPv6-detection / fallback logic in
// getDefaultListenAddress. It only ever binds to an ephemeral local port
// ("[::]:0") and closes it immediately - no external network access, no
// fixed port binding - so it is safe to exercise in a unit test.
func TestGetDefaultListenAddress(t *testing.T) {
	t.Run("returns a valid listen address", func(t *testing.T) {
		got := getDefaultListenAddress()

		switch got {
		case "::", "0.0.0.0":
			// both are valid depending on whether the test environment
			// supports IPv6 dual-stack listening
		default:
			t.Fatalf("getDefaultListenAddress() = %q, want %q or %q", got, "::", "0.0.0.0")
		}
	})

	t.Run("does not leak the probe listener", func(t *testing.T) {
		// Calling it twice in a row must not fail or hang - if the first
		// call leaked its ephemeral listener, a second dual-stack bind on
		// "[::]:0" would still succeed (different ephemeral port), so this
		// mainly guards against a panic/deadlock regression rather than a
		// specific leaked-socket assertion.
		first := getDefaultListenAddress()
		second := getDefaultListenAddress()

		if first != second {
			t.Fatalf("getDefaultListenAddress() not stable across calls: first=%q second=%q", first, second)
		}
	})
}

// TestWriteJSON covers the local writeJSON mirror of the response helper -
// status code, content type, and body encoding.
func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusTeapot, map[string]string{"hello": "world"})

	if rec.Code != http.StatusTeapot {
		t.Fatalf("writeJSON() status = %d, want %d", rec.Code, http.StatusTeapot)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("writeJSON() Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("writeJSON() body did not decode as JSON: %v", err)
	}

	if body["hello"] != "world" {
		t.Fatalf("writeJSON() body = %v, want hello=world", body)
	}
}

// TestWriteText covers the local writeText mirror of the response helper -
// status code, content type, and formatted body.
func TestWriteText(t *testing.T) {
	rec := httptest.NewRecorder()

	writeText(rec, http.StatusAccepted, "count=%d", 3)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("writeText() status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("writeText() Content-Type = %q, want %q", ct, "text/plain; charset=utf-8")
	}

	if got := rec.Body.String(); got != "count=3" {
		t.Fatalf("writeText() body = %q, want %q", got, "count=3")
	}
}
