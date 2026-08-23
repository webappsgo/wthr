package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/server/service"
)

// newLogsTestRequest builds a bare request/recorder pair for handlers that
// take (http.ResponseWriter, *http.Request) directly.
func newLogsTestRequest(method, target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	return r, w
}

func writeLogFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestLogsHandler_GetLogs(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		tail       string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "no log file returns empty",
			content:    "",
			tail:       "",
			wantCount:  0,
			wantStatus: 200,
		},
		{
			name:       "default tail returns all short lines",
			content:    "[2025-12-13 10:30:45] [INFO] [server] started\n[2025-12-13 10:30:46] [ERROR] [db] failed\n",
			tail:       "",
			wantCount:  2,
			wantStatus: 200,
		},
		{
			name:       "tail=all returns everything",
			content:    "[2025-12-13 10:30:45] [INFO] [server] one\n[2025-12-13 10:30:46] [INFO] [server] two\n",
			tail:       "all",
			wantCount:  2,
			wantStatus: 200,
		},
		{
			name:       "tail=1 limits to last line",
			content:    "[2025-12-13 10:30:45] [INFO] [server] one\n[2025-12-13 10:30:46] [INFO] [server] two\n",
			tail:       "1",
			wantCount:  1,
			wantStatus: 200,
		},
		{
			name:       "invalid tail falls back to default 250",
			content:    "[2025-12-13 10:30:45] [INFO] [server] one\n",
			tail:       "notanumber",
			wantCount:  1,
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" || tt.name == "default tail returns all short lines" {
				writeLogFile(t, dir, "wthr.log", tt.content)
			} else if tt.name != "no log file returns empty" {
				writeLogFile(t, dir, "wthr.log", tt.content)
			}
			h := NewLogsHandler(dir)

			target := "/logs"
			if tt.tail != "" {
				target += "?tail=" + tt.tail
			}
			r, w := newLogsTestRequest("GET", target)
			h.GetLogs(w, r)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			var resp struct {
				Logs  []LogEntry `json:"logs"`
				Count int        `json:"count"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.Count != tt.wantCount {
				t.Fatalf("count = %d, want %d", resp.Count, tt.wantCount)
			}
		})
	}
}

func TestLogsHandler_ParseLine(t *testing.T) {
	h := NewLogsHandler(t.TempDir())

	t.Run("structured line", func(t *testing.T) {
		entry := h.parseLine("[2025-12-13 10:30:45] [INFO] [server] Message here")
		if entry.Timestamp != "2025-12-13 10:30:45" {
			t.Errorf("timestamp = %q", entry.Timestamp)
		}
		if entry.Level != "INFO" {
			t.Errorf("level = %q", entry.Level)
		}
		if entry.Source != "server" {
			t.Errorf("source = %q", entry.Source)
		}
		if entry.Message != "Message here" {
			t.Errorf("message = %q", entry.Message)
		}
	})

	t.Run("unstructured line falls back to defaults", func(t *testing.T) {
		entry := h.parseLine("plain log line without brackets")
		if entry.Level != "INFO" {
			t.Errorf("level = %q, want INFO", entry.Level)
		}
		if entry.Source != "unknown" {
			t.Errorf("source = %q, want unknown", entry.Source)
		}
		if entry.Message != "plain log line without brackets" {
			t.Errorf("message = %q", entry.Message)
		}
	})

	t.Run("malformed bracketed line falls back to defaults", func(t *testing.T) {
		entry := h.parseLine("[incomplete")
		if entry.Message != "[incomplete" {
			t.Errorf("message = %q", entry.Message)
		}
	})
}

func TestLogsHandler_DownloadLogs(t *testing.T) {
	t.Run("missing file returns 404", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newLogsTestRequest("GET", "/logs/download")
		h.DownloadLogs(w, r)
		if w.Code != 404 {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("existing file downloads", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "wthr.log", "hello world\n")
		h := NewLogsHandler(dir)
		r, w := newLogsTestRequest("GET", "/logs/download")
		h.DownloadLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if w.Body.String() != "hello world\n" {
			t.Fatalf("body = %q", w.Body.String())
		}
		if w.Header().Get("Content-Type") != "text/plain" {
			t.Errorf("content-type = %q", w.Header().Get("Content-Type"))
		}
	})
}

func TestLogsHandler_ClearLogs(t *testing.T) {
	t.Run("truncates existing file", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "wthr.log", "some content\n")
		h := NewLogsHandler(dir)
		r, w := newLogsTestRequest("POST", "/logs/clear")
		h.ClearLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		data, err := os.ReadFile(h.logFile)
		if err != nil {
			t.Fatalf("read after clear: %v", err)
		}
		if len(data) != 0 {
			t.Fatalf("expected empty file, got %q", data)
		}
	})

	t.Run("missing file returns 500", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newLogsTestRequest("POST", "/logs/clear")
		h.ClearLogs(w, r)
		if w.Code != 500 {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestLogsHandler_GetLogStats(t *testing.T) {
	dir := t.TempDir()
	content := "" +
		"[2025-12-13 10:00:00] [DEBUG] [db] d\n" +
		"[2025-12-13 10:00:01] [INFO] [server] i\n" +
		"[2025-12-13 10:00:02] [WARN] [server] w\n" +
		"[2025-12-13 10:00:03] [ERROR] [db] e\n" +
		"[2025-12-13 10:00:04] [FATAL] [server] f\n" +
		"[2025-12-13 10:00:05] [INFO] [server] i2\n"
	writeLogFile(t, dir, "wthr.log", content)
	h := NewLogsHandler(dir)

	r, w := newLogsTestRequest("GET", "/logs/stats")
	h.GetLogStats(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stats["total"].(float64) != 6 {
		t.Errorf("total = %v, want 6", stats["total"])
	}
	if stats["info"].(float64) != 2 {
		t.Errorf("info = %v, want 2", stats["info"])
	}
	if stats["debug"].(float64) != 1 {
		t.Errorf("debug = %v, want 1", stats["debug"])
	}
	if stats["error"].(float64) != 1 {
		t.Errorf("error = %v, want 1", stats["error"])
	}
	if stats["fatal"].(float64) != 1 {
		t.Errorf("fatal = %v, want 1", stats["fatal"])
	}
}

func TestLogsHandler_RotateLogs(t *testing.T) {
	t.Run("archives and truncates", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "wthr.log", "log content\n")
		h := NewLogsHandler(dir)

		r, w := newLogsTestRequest("POST", "/logs/rotate")
		h.RotateLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		data, err := os.ReadFile(h.logFile)
		if err != nil {
			t.Fatalf("read log file: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected truncated log file, got %q", data)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		found := false
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".log" && e.Name() != "wthr.log" {
				found = true
			}
		}
		if !found {
			t.Error("expected an archived log file to exist")
		}
	})

	t.Run("missing source file fails", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newLogsTestRequest("POST", "/logs/rotate")
		h.RotateLogs(w, r)
		if w.Code != 500 {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestLogsHandler_ListArchivedLogs(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "wthr.log", "current\n")
	writeLogFile(t, dir, "weather_2025-01-01_00-00-00.log", "archived\n")
	writeLogFile(t, dir, "audit.log", "not an archive\n")
	h := NewLogsHandler(dir)

	r, w := newLogsTestRequest("GET", "/logs/archives")
	h.ListArchivedLogs(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Archives []map[string]interface{} `json:"archives"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Archives) != 1 {
		t.Fatalf("archives count = %d, want 1", len(resp.Archives))
	}
	if resp.Archives[0]["name"] != "weather_2025-01-01_00-00-00.log" {
		t.Errorf("archive name = %v", resp.Archives[0]["name"])
	}
}

func writeAuditEvent(t *testing.T, path string, ev service.AuditEvent) {
	t.Helper()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatalf("write audit event: %v", err)
	}
}

func sampleAuditEvents(now time.Time) []service.AuditEvent {
	return []service.AuditEvent{
		{
			ID:     "1",
			Time:   now.Add(-2 * time.Hour),
			Event:  "admin.login",
			Actor:  service.Actor{Type: "admin", ID: "alice", IP: "1.2.3.4"},
			Result: "success",
		},
		{
			ID:     "2",
			Time:   now.Add(-1 * time.Hour),
			Event:  "admin.login",
			Actor:  service.Actor{Type: "admin", ID: "bob", IP: "5.6.7.8"},
			Result: "failure",
		},
		{
			ID:     "3",
			Time:   now,
			Event:  "user.delete",
			Actor:  service.Actor{Type: "admin", ID: "alice", IP: "1.2.3.4"},
			Result: "success",
		},
	}
}

func TestLogsHandler_GetAuditLogs(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("no audit file returns empty", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newLogsTestRequest("GET", "/audit")
		h.GetAuditLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d", w.Code)
		}
		var resp struct {
			Data struct {
				Events []service.AuditEvent `json:"events"`
				Total  int                  `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data.Total != 0 || len(resp.Data.Events) != 0 {
			t.Fatalf("expected empty events, got %+v", resp.Data)
		}
	})

	t.Run("filters by event type and username", func(t *testing.T) {
		dir := t.TempDir()
		auditPath := filepath.Join(dir, "audit.log")
		for _, ev := range sampleAuditEvents(now) {
			writeAuditEvent(t, auditPath, ev)
		}
		h := NewLogsHandler(dir)

		r, w := newLogsTestRequest("GET", "/audit?event_type=admin.login&username=alice")
		h.GetAuditLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d", w.Code)
		}
		var resp struct {
			Data struct {
				Events []service.AuditEvent `json:"events"`
				Total  int                  `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data.Total != 1 {
			t.Fatalf("total = %d, want 1", resp.Data.Total)
		}
		if len(resp.Data.Events) != 1 || resp.Data.Events[0].ID != "1" {
			t.Fatalf("unexpected events: %+v", resp.Data.Events)
		}
	})

	t.Run("pagination with limit and offset", func(t *testing.T) {
		dir := t.TempDir()
		auditPath := filepath.Join(dir, "audit.log")
		for _, ev := range sampleAuditEvents(now) {
			writeAuditEvent(t, auditPath, ev)
		}
		h := NewLogsHandler(dir)

		r, w := newLogsTestRequest("GET", "/audit?limit=1&offset=1")
		h.GetAuditLogs(w, r)
		var resp struct {
			Data struct {
				Events []service.AuditEvent `json:"events"`
				Total  int                  `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data.Total != 3 {
			t.Fatalf("total = %d, want 3", resp.Data.Total)
		}
		if len(resp.Data.Events) != 1 {
			t.Fatalf("events len = %d, want 1", len(resp.Data.Events))
		}
	})

	t.Run("invalid json lines are skipped", func(t *testing.T) {
		dir := t.TempDir()
		auditPath := filepath.Join(dir, "audit.log")
		writeLogFile(t, dir, "audit.log", "not json\n")
		writeAuditEvent(t, auditPath, sampleAuditEvents(now)[0])
		h := NewLogsHandler(dir)

		r, w := newLogsTestRequest("GET", "/audit")
		h.GetAuditLogs(w, r)
		var resp struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data.Total != 1 {
			t.Fatalf("total = %d, want 1", resp.Data.Total)
		}
	})
}

func TestLogsHandler_DownloadAuditLogs(t *testing.T) {
	t.Run("missing file returns 404", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newLogsTestRequest("GET", "/audit/download")
		h.DownloadAuditLogs(w, r)
		if w.Code != 404 {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("existing file downloads", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "audit.log", `{"id":"1"}`+"\n")
		h := NewLogsHandler(dir)
		r, w := newLogsTestRequest("GET", "/audit/download")
		h.DownloadAuditLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestLogsHandler_SearchAuditLogs(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	newSearchRequest := func(t *testing.T, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
		return newTestContextJSON(t, "POST", "/audit/search", body)
	}

	t.Run("malformed json returns 400", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newSearchRequest(t, "{not json")
		h.SearchAuditLogs(w, r)
		if w.Code != 400 {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("filters by success flag", func(t *testing.T) {
		dir := t.TempDir()
		auditPath := filepath.Join(dir, "audit.log")
		for _, ev := range sampleAuditEvents(now) {
			writeAuditEvent(t, auditPath, ev)
		}
		h := NewLogsHandler(dir)

		successFalse := false
		r, w := newSearchRequest(t, map[string]interface{}{"success": &successFalse})
		h.SearchAuditLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp struct {
			Data struct {
				Events []service.AuditEvent `json:"events"`
				Total  int                  `json:"total"`
				Limit  int                  `json:"limit"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data.Total != 1 || resp.Data.Events[0].ID != "2" {
			t.Fatalf("unexpected result: %+v", resp.Data)
		}
		if resp.Data.Limit != 100 {
			t.Fatalf("default limit = %d, want 100", resp.Data.Limit)
		}
	})

	t.Run("filters by ip", func(t *testing.T) {
		dir := t.TempDir()
		auditPath := filepath.Join(dir, "audit.log")
		for _, ev := range sampleAuditEvents(now) {
			writeAuditEvent(t, auditPath, ev)
		}
		h := NewLogsHandler(dir)

		r, w := newSearchRequest(t, map[string]interface{}{"ip": "5.6.7.8"})
		h.SearchAuditLogs(w, r)
		var resp struct {
			Data struct {
				Events []service.AuditEvent `json:"events"`
				Total  int                  `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data.Total != 1 || resp.Data.Events[0].ID != "2" {
			t.Fatalf("unexpected result: %+v", resp.Data)
		}
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newSearchRequest(t, map[string]interface{}{"event_type": "nonexistent"})
		h.SearchAuditLogs(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestLogsHandler_GetAuditStats(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("computes aggregate stats", func(t *testing.T) {
		dir := t.TempDir()
		auditPath := filepath.Join(dir, "audit.log")
		for _, ev := range sampleAuditEvents(now) {
			writeAuditEvent(t, auditPath, ev)
		}
		h := NewLogsHandler(dir)

		r, w := newLogsTestRequest("GET", "/audit/stats")
		h.GetAuditStats(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Data["total"].(float64) != 3 {
			t.Errorf("total = %v, want 3", resp.Data["total"])
		}
		if resp.Data["successful"].(float64) != 2 {
			t.Errorf("successful = %v, want 2", resp.Data["successful"])
		}
		if resp.Data["failed"].(float64) != 1 {
			t.Errorf("failed = %v, want 1", resp.Data["failed"])
		}
	})

	t.Run("no audit file returns zeroed stats", func(t *testing.T) {
		h := NewLogsHandler(t.TempDir())
		r, w := newLogsTestRequest("GET", "/audit/stats")
		h.GetAuditStats(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}
