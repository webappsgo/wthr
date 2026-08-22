package handler

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestNewLoggingHandler verifies the constructor wires the logsDir field
// as passed.
func TestNewLoggingHandler(t *testing.T) {
	h := NewLoggingHandler("/tmp/example/logs")
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.logsDir != "/tmp/example/logs" {
		t.Errorf("logsDir = %q, want %q", h.logsDir, "/tmp/example/logs")
	}
}

// TestLoggingHandler_GetFormats verifies the static format list is returned
// with "standard" reported as the only active format.
func TestLoggingHandler_GetFormats(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newAPITestContext("/server/admin/config/logging/formats")

	h.GetFormats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Formats LogFormats `json:"formats"`
		Active  []string   `json:"active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !body.Formats.Standard {
		t.Error("expected Standard format to be true")
	}
	if body.Formats.JSON || body.Formats.Fail2ban || body.Formats.Syslog || body.Formats.CEF || body.Formats.Apache {
		t.Errorf("expected all non-standard formats false, got %+v", body.Formats)
	}
	if len(body.Active) != 1 || body.Active[0] != "standard" {
		t.Errorf("active = %v, want [standard]", body.Active)
	}
}

// TestLoggingHandler_UpdateFormats_Success verifies a valid JSON body is
// echoed back with a 200 confirming the update was accepted.
func TestLoggingHandler_UpdateFormats_Success(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logging/formats", LogFormats{JSON: true, Syslog: true})

	h.UpdateFormats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		Formats LogFormats `json:"formats"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !body.Formats.JSON || !body.Formats.Syslog {
		t.Errorf("expected echoed formats to include JSON+Syslog, got %+v", body.Formats)
	}
}

// TestLoggingHandler_UpdateFormats_InvalidJSON verifies malformed bodies are
// rejected with 400 rather than silently accepted.
func TestLoggingHandler_UpdateFormats_InvalidJSON(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logging/formats", "{not valid json")

	h.UpdateFormats(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestLoggingHandler_GetFail2banConfig verifies the plaintext filter config
// is returned as a downloadable attachment.
func TestLoggingHandler_GetFail2banConfig(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newAPITestContext("/server/admin/config/logging/fail2ban")

	h.GetFail2banConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "attachment; filename=weather.conf" {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if len(w.Body.String()) == 0 {
		t.Error("expected non-empty fail2ban config body")
	}
}

// TestLoggingHandler_GetSyslogConfig verifies the syslog config JSON has the
// expected fields.
func TestLoggingHandler_GetSyslogConfig(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newAPITestContext("/server/admin/config/logging/syslog")

	h.GetSyslogConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["protocol"] != "UDP" || body["format"] != "RFC5424" {
		t.Errorf("unexpected syslog config: %+v", body)
	}
	if _, ok := body["example"].(string); !ok {
		t.Error("expected example field to be a string")
	}
}

// TestLoggingHandler_GetCEFConfig verifies the CEF config JSON has the
// expected fields.
func TestLoggingHandler_GetCEFConfig(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newAPITestContext("/server/admin/config/logging/cef")

	h.GetCEFConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["vendor"] != "Weather" {
		t.Errorf("unexpected CEF config: %+v", body)
	}
}

// TestLoggingHandler_ExportLogs covers every dispatch branch of the format
// query-param switch, including the invalid-format rejection.
func TestLoggingHandler_ExportLogs(t *testing.T) {
	tests := []struct {
		format         string
		wantStatus     int
		wantDisposName string
	}{
		{"fail2ban", http.StatusOK, "weather_fail2ban.log"},
		{"syslog", http.StatusOK, "weather_syslog.log"},
		{"cef", http.StatusOK, "weather_cef.log"},
		{"json", http.StatusOK, ""},
		{"bogus", http.StatusBadRequest, ""},
		{"", http.StatusBadRequest, ""},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			h := NewLoggingHandler("/tmp/logs")
			c, w := newAPITestContext("/server/admin/config/logging/export?format=" + tt.format)

			h.ExportLogs(c)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantDisposName != "" {
				want := "attachment; filename=" + tt.wantDisposName
				if got := w.Header().Get("Content-Disposition"); got != want {
					t.Errorf("Content-Disposition = %q, want %q", got, want)
				}
			}
			if w.Code == http.StatusOK && len(w.Body.Bytes()) == 0 {
				t.Error("expected non-empty export body")
			}
		})
	}
}

// TestLoggingHandler_ExportJSON_Shape verifies exportJSON (reached via
// ExportLogs?format=json) emits the documented logs array shape.
func TestLoggingHandler_ExportJSON_Shape(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newAPITestContext("/server/admin/config/logging/export?format=json")

	h.ExportLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Logs []map[string]interface{} `json:"logs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(body.Logs) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(body.Logs))
	}
	if body.Logs[0]["level"] != "ERROR" || body.Logs[1]["level"] != "INFO" {
		t.Errorf("unexpected log levels: %+v", body.Logs)
	}
}

// TestLoggingHandler_ConfigureFail2ban_Success verifies a valid config body
// is echoed back with a confirmation message.
func TestLoggingHandler_ConfigureFail2ban_Success(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	payload := map[string]interface{}{
		"enabled":    true,
		"filterPath": "/etc/fail2ban/filter.d/weather.conf",
		"jailPath":   "/etc/fail2ban/jail.d/weather.conf",
		"banTime":    3600,
		"maxRetry":   5,
	}
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logging/fail2ban/configure", payload)

	h.ConfigureFail2ban(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !jsonFieldEquals(t, w.Body.Bytes(), "message", "Fail2ban configuration saved") {
		t.Errorf("unexpected response body: %s", w.Body.String())
	}
}

// TestLoggingHandler_ConfigureFail2ban_InvalidJSON verifies malformed bodies
// are rejected with 400.
func TestLoggingHandler_ConfigureFail2ban_InvalidJSON(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logging/fail2ban/configure", "{not valid")

	h.ConfigureFail2ban(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestLoggingHandler_ConfigureSyslog_Success verifies a valid config body is
// echoed back with a confirmation message.
func TestLoggingHandler_ConfigureSyslog_Success(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	payload := map[string]interface{}{
		"enabled":  true,
		"protocol": "UDP",
		"host":     "127.0.0.1",
		"port":     514,
		"facility": "local0",
	}
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logging/syslog/configure", payload)

	h.ConfigureSyslog(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !jsonFieldEquals(t, w.Body.Bytes(), "message", "Syslog configuration saved") {
		t.Errorf("unexpected response body: %s", w.Body.String())
	}
}

// TestLoggingHandler_ConfigureSyslog_InvalidJSON verifies malformed bodies
// are rejected with 400.
func TestLoggingHandler_ConfigureSyslog_InvalidJSON(t *testing.T) {
	h := NewLoggingHandler("/tmp/logs")
	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/logging/syslog/configure", "{not valid")

	h.ConfigureSyslog(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestLoggingHandler_TestFormat covers every known sample format plus the
// unknown-format rejection branch.
func TestLoggingHandler_TestFormat(t *testing.T) {
	known := []string{"standard", "json", "fail2ban", "syslog", "cef"}
	for _, format := range known {
		t.Run(format, func(t *testing.T) {
			h := NewLoggingHandler("/tmp/logs")
			c, w := newAPITestContext("/server/admin/config/logging/test?format=" + format)

			h.TestFormat(c)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
			}
			var body struct {
				Format string `json:"format"`
				Sample string `json:"sample"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body.Format != format {
				t.Errorf("format = %q, want %q", body.Format, format)
			}
			if body.Sample == "" {
				t.Error("expected non-empty sample")
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		h := NewLoggingHandler("/tmp/logs")
		c, w := newAPITestContext("/server/admin/config/logging/test?format=bogus")

		h.TestFormat(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// jsonFieldEquals is a small helper asserting a top-level string field in a
// JSON response body equals the expected value.
func jsonFieldEquals(t *testing.T, raw []byte, field, want string) bool {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got, _ := body[field].(string)
	return got == want
}
