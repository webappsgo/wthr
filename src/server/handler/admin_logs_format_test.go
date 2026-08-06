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

// TestLogFormatHandler_GetLogFormat_DefaultsToApache verifies that with no
// logging.format row present in server_config, the handler defaults to
// "apache" and lists all supported formats.
func TestLogFormatHandler_GetLogFormat_DefaultsToApache(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)

	h := &LogFormatHandler{DB: serverDB}
	c, w := newAPITestContext("/server/admin/config/logs/format")
	h.GetLogFormat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"format":"apache"`) {
		t.Errorf("expected default format apache, got: %s", w.Body.String())
	}
}

// TestLogFormatHandler_GetLogFormat_ReflectsStoredValue verifies a value
// previously written by SetLogFormat is read back correctly.
func TestLogFormatHandler_GetLogFormat_ReflectsStoredValue(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)

	h := &LogFormatHandler{DB: serverDB}

	setC, setW := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logs/format", map[string]string{"format": "syslog"})
	h.SetLogFormat(setC)
	if setW.Code != http.StatusOK {
		t.Fatalf("SetLogFormat status = %d, want 200: %s", setW.Code, setW.Body.String())
	}

	c, w := newAPITestContext("/server/admin/config/logs/format")
	h.GetLogFormat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"format":"syslog"`) {
		t.Errorf("expected stored format syslog, got: %s", w.Body.String())
	}
}

// TestLogFormatHandler_ShowLogFormatPage_LoadsData verifies the pre-render
// guard logic (reading current format from server_config, defaulting to
// apache) runs without error before the template render call.
func TestLogFormatHandler_ShowLogFormatPage_LoadsData(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)

	h := &LogFormatHandler{DB: serverDB}
	c, w := newAPITestContext("/server/admin/config/logs/format/page")
	defer htmlRenderGuard(t)
	h.ShowLogFormatPage(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
