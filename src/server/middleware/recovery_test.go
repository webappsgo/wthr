package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/mode"
)

// recordingLogger captures Error() calls so tests can assert a panic was
// logged without depending on util.Logger's concrete file-writing behavior.
type recordingLogger struct {
	calls []string
}

func (l *recordingLogger) Error(format string, v ...interface{}) {
	l.calls = append(l.calls, format)
}

func panicHandler(v interface{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(v)
	})
}

func TestRecovery_JSONResponse(t *testing.T) {
	logger := &recordingLogger{}
	h := Recovery(logger)(panicHandler("boom"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Accept", "application/json")

	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (%s)", err, w.Body.String())
	}
	if body["ok"] != false {
		t.Errorf("body[ok] = %v, want false", body["ok"])
	}
	if body["error"] != "INTERNAL_ERROR" {
		t.Errorf("body[error] = %v, want INTERNAL_ERROR", body["error"])
	}
	if _, hasStack := body["stack"]; hasStack {
		t.Error("response leaked a stack trace field")
	}
	if len(logger.calls) != 1 {
		t.Fatalf("logger.Error called %d times, want 1", len(logger.calls))
	}
}

func TestRecovery_TextResponse(t *testing.T) {
	logger := &recordingLogger{}
	h := Recovery(logger)(panicHandler("boom"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Accept", "text/plain")

	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Errorf("text body = %q, want it to mention INTERNAL_ERROR", w.Body.String())
	}
}

func TestRecovery_DebugModeAddsPanicDetail(t *testing.T) {
	origDebug := mode.IsDebugEnabled()
	defer mode.SetDebugEnabled(origDebug)
	mode.SetDebugEnabled(true)

	logger := &recordingLogger{}
	h := Recovery(logger)(panicHandler("boom detail"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Accept", "application/json")

	h.ServeHTTP(w, r)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	debugField, ok := body["_debug"].(map[string]interface{})
	if !ok {
		t.Fatalf("body[_debug] missing or wrong type: %v", body["_debug"])
	}
	if debugField["panic"] != "boom detail" {
		t.Errorf("body[_debug][panic] = %v, want %q", debugField["panic"], "boom detail")
	}
}

func TestRecovery_ProductionModeOmitsDebugField(t *testing.T) {
	origDebug := mode.IsDebugEnabled()
	defer mode.SetDebugEnabled(origDebug)
	mode.SetDebugEnabled(false)

	logger := &recordingLogger{}
	h := Recovery(logger)(panicHandler("boom"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Accept", "application/json")

	h.ServeHTTP(w, r)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, hasDebug := body["_debug"]; hasDebug {
		t.Error("production response leaked _debug field")
	}
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	logger := &recordingLogger{}
	h := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)

	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", w.Body.String(), "ok")
	}
	if len(logger.calls) != 0 {
		t.Errorf("logger.Error called %d times on non-panic path, want 0", len(logger.calls))
	}
}

func TestRecovery_NilLoggerDoesNotPanic(t *testing.T) {
	h := Recovery(nil)(panicHandler("boom"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)

	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
