package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// decodeErrorBody decodes a canonical error response and fails the test if the
// body is not valid JSON.
func decodeErrorBody(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (%s)", err, raw)
	}
	return body
}

// assertCanonicalError checks the AI.md PART 14 error envelope: ok=false, a
// machine-readable error code, a human message, and none of the forbidden
// legacy fields (a duplicated status, or an ad-hoc top-level code field).
func assertCanonicalError(t *testing.T, body map[string]interface{}, wantCode string) {
	t.Helper()
	if ok, _ := body["ok"].(bool); ok {
		t.Errorf("body[ok] = %v, want false", body["ok"])
	}
	if got, _ := body["error"].(string); got != wantCode {
		t.Errorf("body[error] = %v, want %q", body["error"], wantCode)
	}
	if msg, _ := body["message"].(string); msg == "" {
		t.Errorf("body[message] is empty, want a human-readable message")
	}
	if _, exists := body["status"]; exists {
		t.Errorf("body must not duplicate the HTTP status: %v", body)
	}
	if _, exists := body["code"]; exists {
		t.Errorf("body must not carry a separate code field: %v", body)
	}
}

func TestWriteAPIError_CanonicalShape(t *testing.T) {
	w := httptest.NewRecorder()
	writeAPIError(w, http.StatusForbidden, "FORBIDDEN", "Permission denied")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	assertCanonicalError(t, decodeErrorBody(t, w.Body.Bytes()), "FORBIDDEN")
}

func TestBodySizeLimitMiddleware_CanonicalErrorBody(t *testing.T) {
	const maxSize = 100

	handler := BodySizeLimitMiddleware(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	payload := bytes.Repeat([]byte("a"), 200)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Length", strconv.Itoa(len(payload)))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assertCanonicalError(t, decodeErrorBody(t, w.Body.Bytes()), "PAYLOAD_TOO_LARGE")
}
