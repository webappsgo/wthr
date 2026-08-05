package handler

import (
	"net/http"
	"strings"
	"testing"
)

// TestNewLogFormatHandler verifies the constructor wires the DB field as
// passed.
func TestNewLogFormatHandler(t *testing.T) {
	db := newTestServerDB(t)
	h := NewLogFormatHandler(db)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.DB != db {
		t.Error("expected DB field to be the passed *sql.DB")
	}
}

// TestLogFormatHandler_SetLogFormat_InvalidFormat verifies an unrecognized
// format value is rejected with 400 before any DB write is attempted (DB is
// nil here, so a write attempt would panic — proving the guard runs first).
func TestLogFormatHandler_SetLogFormat_InvalidFormat(t *testing.T) {
	h := &LogFormatHandler{}
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logs/format", map[string]string{"format": "not-a-real-format"})

	h.SetLogFormat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestLogFormatHandler_SetLogFormat_MissingFormat verifies a request with no
// "format" field is rejected with 400 by the binding validator.
func TestLogFormatHandler_SetLogFormat_MissingFormat(t *testing.T) {
	h := &LogFormatHandler{}
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logs/format", map[string]string{})

	h.SetLogFormat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestLogFormatHandler_PreviewLogFormat_DefaultsToApache verifies the
// preview endpoint defaults to "apache" when no format query param is
// given, and renders all 7 supported formats without touching the DB.
func TestLogFormatHandler_PreviewLogFormat_DefaultsToApache(t *testing.T) {
	h := &LogFormatHandler{}
	c, w := newAPITestContext("/server/admin/config/logs/format/preview")

	h.PreviewLogFormat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"current_format":"apache"`) {
		t.Errorf("expected default current_format apache, got: %s", body)
	}
	for _, format := range []string{"apache", "nginx", "json", "fail2ban", "syslog", "cef", "text"} {
		if !strings.Contains(body, format) {
			t.Errorf("expected preview body to mention format %q, got: %s", format, body)
		}
	}
}

// TestLogFormatHandler_PreviewLogFormat_ExplicitFormat verifies an explicit
// ?format= query value is echoed back as current_format.
func TestLogFormatHandler_PreviewLogFormat_ExplicitFormat(t *testing.T) {
	h := &LogFormatHandler{}
	c, w := newAPITestContext("/server/admin/config/logs/format/preview?format=cef")

	h.PreviewLogFormat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"current_format":"cef"`) {
		t.Errorf("expected current_format cef, got: %s", w.Body.String())
	}
}
