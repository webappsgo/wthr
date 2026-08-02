package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// sample builds a representative LogEntry for happy-path formatting tests.
func sampleLogEntry() *LogEntry {
	return &LogEntry{
		Timestamp:   time.Date(2026, 7, 18, 12, 30, 45, 0, time.UTC),
		RemoteAddr:  "203.0.113.5",
		Method:      "GET",
		Path:        "/api/v1/weather/Chicago",
		Protocol:    "HTTP/1.1",
		StatusCode:  200,
		BytesSent:   1234,
		Referer:     "https://example.com/",
		UserAgent:   "curl/8.0",
		RequestTime: 0.125,
		RequestID:   "req-abc123",
		Username:    "alice",
	}
}

// TestLogFormatter_Format_Dispatch covers the Format() switch, including
// the default (unknown-format) branch which must fall back to Apache.
func TestLogFormatter_Format_Dispatch(t *testing.T) {
	entry := sampleLogEntry()

	tests := []struct {
		name   string
		format LogFormat
	}{
		{"apache", LogFormatApache},
		{"nginx", LogFormatNginx},
		{"json", LogFormatJSON},
		{"fail2ban", LogFormatFail2ban},
		{"syslog", LogFormatSyslog},
		{"cef", LogFormatCEF},
		{"text", LogFormatText},
		{"unknown falls back to apache", LogFormat("bogus-format")},
		{"empty format falls back to apache", LogFormat("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewLogFormatter(tt.format)
			got := f.Format(entry)
			if got == "" {
				t.Fatalf("Format() returned empty string for format %q", tt.format)
			}
		})
	}

	// The unknown/empty formats must produce output identical to explicit apache.
	apacheOut := NewLogFormatter(LogFormatApache).Format(entry)
	if got := NewLogFormatter(LogFormat("bogus-format")).Format(entry); got != apacheOut {
		t.Errorf("unknown format output = %q, want same as apache %q", got, apacheOut)
	}
}

func TestLogFormatter_formatApache(t *testing.T) {
	tests := []struct {
		name  string
		entry *LogEntry
		want  string
	}{
		{
			name:  "happy path with all fields",
			entry: sampleLogEntry(),
			want:  `203.0.113.5 - alice [18/Jul/2026:12:30:45 +0000] "GET /api/v1/weather/Chicago HTTP/1.1" 200 1234 "https://example.com/" "curl/8.0"`,
		},
		{
			name: "empty username referer user-agent become dash",
			entry: &LogEntry{
				Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				RemoteAddr: "127.0.0.1",
				Method:     "POST",
				Path:       "/",
				Protocol:   "HTTP/1.1",
				StatusCode: 404,
				BytesSent:  0,
			},
			want: `127.0.0.1 - - [01/Jan/2026:00:00:00 +0000] "POST / HTTP/1.1" 404 0 "-" "-"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewLogFormatter(LogFormatApache)
			if got := f.formatApache(tt.entry); got != tt.want {
				t.Errorf("formatApache() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogFormatter_formatNginx(t *testing.T) {
	f := NewLogFormatter(LogFormatNginx)
	entry := sampleLogEntry()

	got := f.formatNginx(entry)
	want := `203.0.113.5 - alice [18/Jul/2026:12:30:45 +0000] "GET /api/v1/weather/Chicago HTTP/1.1" 200 1234 "https://example.com/" "curl/8.0" 0.125`
	if got != want {
		t.Errorf("formatNginx() = %q, want %q", got, want)
	}

	// Zero-value RequestTime must still print with three decimal places.
	zero := &LogEntry{Timestamp: time.Unix(0, 0).UTC(), RemoteAddr: "1.2.3.4", Method: "GET", Path: "/", Protocol: "HTTP/1.1"}
	if got := f.formatNginx(zero); !strings.HasSuffix(got, "0.000") {
		t.Errorf("formatNginx() with zero RequestTime = %q, want suffix 0.000", got)
	}
}

func TestLogFormatter_formatJSON(t *testing.T) {
	tests := []struct {
		name       string
		entry      *LogEntry
		wantKeys   []string
		absentKeys []string
	}{
		{
			name:       "optional fields present when set",
			entry:      sampleLogEntry(),
			wantKeys:   []string{"timestamp", "remote_addr", "method", "path", "protocol", "status_code", "bytes_sent", "request_time", "request_id", "username", "referer", "user_agent"},
			absentKeys: []string{"error"},
		},
		{
			name: "optional fields omitted when empty",
			entry: &LogEntry{
				Timestamp:  time.Now(),
				RemoteAddr: "1.2.3.4",
				Method:     "GET",
				Path:       "/",
				Protocol:   "HTTP/1.1",
				StatusCode: 200,
			},
			wantKeys:   []string{"timestamp", "remote_addr", "method", "path", "protocol", "status_code", "bytes_sent", "request_time", "request_id"},
			absentKeys: []string{"username", "referer", "user_agent", "error"},
		},
		{
			name: "error message included when set",
			entry: &LogEntry{
				Timestamp:    time.Now(),
				RemoteAddr:   "1.2.3.4",
				Method:       "GET",
				Path:         "/",
				Protocol:     "HTTP/1.1",
				StatusCode:   500,
				ErrorMessage: "boom",
			},
			wantKeys: []string{"error"},
		},
	}

	f := NewLogFormatter(LogFormatJSON)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := f.formatJSON(tt.entry)

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(out), &data); err != nil {
				t.Fatalf("formatJSON() produced invalid JSON: %v\noutput: %s", err, out)
			}

			for _, k := range tt.wantKeys {
				if _, ok := data[k]; !ok {
					t.Errorf("formatJSON() missing key %q in output %s", k, out)
				}
			}
			for _, k := range tt.absentKeys {
				if _, ok := data[k]; ok {
					t.Errorf("formatJSON() unexpectedly contains key %q in output %s", k, out)
				}
			}
		})
	}
}

func TestLogFormatter_formatFail2ban(t *testing.T) {
	f := NewLogFormatter(LogFormatFail2ban)

	tests := []struct {
		name       string
		statusCode int
		wantAction string
	}{
		{"200 is access", 200, "access"},
		{"399 is access", 399, "access"},
		{"400 is failed", 400, "failed"},
		{"401 is auth_failed", 401, "auth_failed"},
		{"403 is auth_failed", 403, "auth_failed"},
		{"404 is failed", 404, "failed"},
		{"500 is failed", 500, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Timestamp:  time.Now(),
				RemoteAddr: "9.9.9.9",
				Method:     "GET",
				Path:       "/login",
				Protocol:   "HTTP/1.1",
				StatusCode: tt.statusCode,
			}
			got := f.formatFail2ban(entry)
			wantSubstr := "] " + tt.wantAction + ":"
			if !strings.Contains(got, wantSubstr) {
				t.Errorf("formatFail2ban(status=%d) = %q, want to contain %q", tt.statusCode, got, wantSubstr)
			}
			if !strings.Contains(got, "from 9.9.9.9") {
				t.Errorf("formatFail2ban() = %q, want to contain remote addr", got)
			}
		})
	}
}

func TestLogFormatter_formatSyslog(t *testing.T) {
	f := NewLogFormatter(LogFormatSyslog)

	tests := []struct {
		name          string
		statusCode    int
		requestID     string
		wantPriority  string
		wantMsgIDDash bool
	}{
		{"2xx is informational severity", 200, "req-1", "<134>", false},
		{"4xx is warning severity", 404, "req-2", "<132>", false},
		{"5xx is error severity", 500, "", "<131>", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Timestamp:  time.Now(),
				RemoteAddr: "10.0.0.1",
				Method:     "GET",
				Path:       "/x",
				Protocol:   "HTTP/1.1",
				StatusCode: tt.statusCode,
				RequestID:  tt.requestID,
			}
			got := f.formatSyslog(entry)
			if !strings.HasPrefix(got, tt.wantPriority) {
				t.Errorf("formatSyslog() = %q, want prefix %q", got, tt.wantPriority)
			}
			if tt.wantMsgIDDash && !strings.Contains(got, " - [request@") {
				t.Errorf("formatSyslog() with empty RequestID = %q, want dash msgid before structured data", got)
			}
		})
	}
}

func TestLogFormatter_formatCEF(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		wantSeverity string
	}{
		{"1xx has no dedicated branch, defaults to medium", 100, "3"},
		{"2xx is very low", 200, "1"},
		{"3xx is low", 301, "2"},
		{"4xx is medium-high", 404, "5"},
		{"5xx is high", 500, "8"},
		{"0 status defaults to medium", 0, "3"},
	}

	f := NewLogFormatter(LogFormatCEF)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Timestamp:  time.Now(),
				RemoteAddr: "1.1.1.1",
				Method:     "GET",
				Path:       "/x",
				Protocol:   "HTTP/1.1",
				StatusCode: tt.statusCode,
			}
			got := f.formatCEF(entry)
			prefix := "CEF:0|casapps|wthr|1.0|HTTP_" + itoaHelper(tt.statusCode) + "|HTTP GET|" + tt.wantSeverity + "|"
			if !strings.HasPrefix(got, prefix) {
				t.Errorf("formatCEF(status=%d) = %q, want prefix %q", tt.statusCode, got, prefix)
			}
		})
	}

	t.Run("special characters in user agent are escaped", func(t *testing.T) {
		entry := &LogEntry{
			Timestamp:  time.Now(),
			RemoteAddr: "1.1.1.1",
			Method:     "GET",
			Path:       "/x",
			Protocol:   "HTTP/1.1",
			StatusCode: 200,
			UserAgent:  "weird|agent=1\\2\nline",
			Username:   "bob",
		}
		got := f.formatCEF(entry)
		if !strings.Contains(got, `requestClientApplication=weird\|agent\=1\\2\nline`) {
			t.Errorf("formatCEF() did not escape special CEF chars, got %q", got)
		}
		if !strings.Contains(got, "suser=bob") {
			t.Errorf("formatCEF() missing suser field, got %q", got)
		}
	})

	t.Run("empty user agent and username omit optional extensions", func(t *testing.T) {
		entry := &LogEntry{Timestamp: time.Now(), RemoteAddr: "1.1.1.1", Method: "GET", Path: "/x", Protocol: "HTTP/1.1", StatusCode: 200}
		got := f.formatCEF(entry)
		if strings.Contains(got, "requestClientApplication=") || strings.Contains(got, "suser=") {
			t.Errorf("formatCEF() with no user agent/username should omit those fields, got %q", got)
		}
	})
}

func itoaHelper(n int) string {
	// Local helper to avoid importing strconv solely for a handful of assertions.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}

func TestLogFormatter_formatText(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantStatus string
	}{
		{"2xx is OK", 200, "OK 200"},
		{"3xx is REDIR", 302, "REDIR 302"},
		{"4xx is WARN", 404, "WARN 404"},
		{"5xx is ERROR", 500, "ERROR 500"},
	}

	f := NewLogFormatter(LogFormatText)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Timestamp:  time.Now(),
				RemoteAddr: "1.2.3.4",
				Method:     "GET",
				Path:       "/x",
				Protocol:   "HTTP/1.1",
				StatusCode: tt.statusCode,
			}
			got := f.formatText(entry)
			if !strings.Contains(got, "["+tt.wantStatus+"]") {
				t.Errorf("formatText(status=%d) = %q, want to contain [%s]", tt.statusCode, got, tt.wantStatus)
			}
		})
	}

	t.Run("request id and username appended only when set", func(t *testing.T) {
		withBoth := &LogEntry{Timestamp: time.Now(), RemoteAddr: "1.2.3.4", Method: "GET", Path: "/x", Protocol: "HTTP/1.1", StatusCode: 200, RequestID: "rid-1", Username: "carol"}
		got := f.formatText(withBoth)
		if !strings.Contains(got, "id=rid-1") || !strings.Contains(got, "user=carol") {
			t.Errorf("formatText() = %q, want id and user suffix", got)
		}

		withNeither := &LogEntry{Timestamp: time.Now(), RemoteAddr: "1.2.3.4", Method: "GET", Path: "/x", Protocol: "HTTP/1.1", StatusCode: 200}
		got = f.formatText(withNeither)
		if strings.Contains(got, "id=") || strings.Contains(got, "user=") {
			t.Errorf("formatText() = %q, should not contain id= or user= when unset", got)
		}
	})
}

// TestLogFormatter_escapeCEF is a direct table-driven test of the escaping
// helper covering every special character and the empty-string boundary.
func TestLogFormatter_escapeCEF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"no special characters", "plain-text", "plain-text"},
		{"backslash", `a\b`, `a\\b`},
		{"pipe", "a|b", `a\|b`},
		{"equals", "a=b", `a\=b`},
		{"newline", "a\nb", `a\nb`},
		{"carriage return", "a\rb", `a\rb`},
		{"all special characters combined", "\\|=\n\r", `\\\|\=\n\r`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeCEF(tt.input); got != tt.want {
				t.Errorf("escapeCEF(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestLogFormatter_ExtractLogEntry drives ExtractLogEntry through a real
// gin.Context built from an httptest request/recorder, covering the
// request-id/username-present and absent branches.
func TestLogFormatter_ExtractLogEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("request id and username present in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/weather/Chicago?x=1", nil)
		req.Header.Set("Referer", "https://ref.example/")
		req.Header.Set("User-Agent", "test-agent/1.0")
		c.Request = req
		c.Set("request_id", "req-xyz")
		c.Set("username", "dave")
		c.Writer.WriteHeader(http.StatusOK)

		start := time.Now().Add(-50 * time.Millisecond)
		entry := ExtractLogEntry(c, start, 42)

		if entry.RequestID != "req-xyz" {
			t.Errorf("RequestID = %q, want req-xyz", entry.RequestID)
		}
		if entry.Username != "dave" {
			t.Errorf("Username = %q, want dave", entry.Username)
		}
		if entry.Method != http.MethodGet {
			t.Errorf("Method = %q, want GET", entry.Method)
		}
		if entry.Path != "/api/v1/weather/Chicago" {
			t.Errorf("Path = %q, want /api/v1/weather/Chicago", entry.Path)
		}
		if entry.BytesSent != 42 {
			t.Errorf("BytesSent = %d, want 42", entry.BytesSent)
		}
		if entry.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want 200", entry.StatusCode)
		}
		if entry.RequestTime <= 0 {
			t.Errorf("RequestTime = %v, want > 0", entry.RequestTime)
		}
		if entry.Referer != "https://ref.example/" {
			t.Errorf("Referer = %q, want https://ref.example/", entry.Referer)
		}
		if entry.UserAgent != "test-agent/1.0" {
			t.Errorf("UserAgent = %q, want test-agent/1.0", entry.UserAgent)
		}
	})

	t.Run("request id and username absent leave zero values", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/unauth", nil)

		entry := ExtractLogEntry(c, time.Now(), 0)

		if entry.RequestID != "" {
			t.Errorf("RequestID = %q, want empty when not set in context", entry.RequestID)
		}
		if entry.Username != "" {
			t.Errorf("Username = %q, want empty when not set in context", entry.Username)
		}
		if entry.BytesSent != 0 {
			t.Errorf("BytesSent = %d, want 0", entry.BytesSent)
		}
	})
}
