package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"log"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// LogFormatHandler handles log format configuration
// TEMPLATE.md Part 25: Support 7 log formats
type LogFormatHandler struct {
	DB *sql.DB
}

// NewLogFormatHandler creates a new log format handler
func NewLogFormatHandler(db *sql.DB) *LogFormatHandler {
	return &LogFormatHandler{DB: db}
}

// GetLogFormat returns the current log format setting
func (h *LogFormatHandler) GetLogFormat(w http.ResponseWriter, r *http.Request) {
	// Get log format from server config
	var logFormat string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT value FROM server_config WHERE key = 'logging.format'
	`).Scan(&logFormat)

	if err != nil && err != sql.ErrNoRows {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get log format setting",
		})
		return
	}

	// Default to apache if not set
	if logFormat == "" {
		logFormat = "apache"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"format":  logFormat,
		"formats": []string{"apache", "nginx", "json", "fail2ban", "syslog", "cef", "text"},
	})
}

// SetLogFormat updates the log format setting
func (h *LogFormatHandler) SetLogFormat(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Format string `json:"format" binding:"required"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request body"})
		return
	}

	// Validate format
	validFormats := map[string]bool{
		"apache":   true,
		"nginx":    true,
		"json":     true,
		"fail2ban": true,
		"syslog":   true,
		"cef":      true,
		"text":     true,
	}

	if !validFormats[request.Format] {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":         "Invalid log format",
			"valid_formats": []string{"apache", "nginx", "json", "fail2ban", "syslog", "cef", "text"},
		})
		return
	}

	// Update or insert setting
	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_config (key, value, type, description, updated_at)
		VALUES ('logging.format', ?, 'string', 'Log format', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, request.Format, time.Now(), request.Format, time.Now())

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":   "Failed to update log format",
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "Log format updated successfully",
		"format":  request.Format,
		"note":    "Restart the server for changes to take effect",
	})
}

// PreviewLogFormat shows a preview of different log formats
func (h *LogFormatHandler) PreviewLogFormat(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "apache"
	}

	// Create sample log entry
	sampleEntry := &service.LogEntry{
		Timestamp:   time.Now(),
		RemoteAddr:  "192.168.1.100",
		Method:      "GET",
		Path:        "/api/v1/weather",
		Protocol:    "HTTP/1.1",
		StatusCode:  200,
		BytesSent:   1234,
		Referer:     "https://example.com/page",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		RequestTime: 0.045,
		RequestID:   "req-abc123def456",
		Username:    "john.doe",
	}

	// Format preview for all formats
	previews := make(map[string]string)
	formats := []service.LogFormat{
		service.LogFormatApache,
		service.LogFormatNginx,
		service.LogFormatJSON,
		service.LogFormatFail2ban,
		service.LogFormatSyslog,
		service.LogFormatCEF,
		service.LogFormatText,
	}

	for _, fmt := range formats {
		formatter := service.NewLogFormatter(fmt)
		previews[string(fmt)] = formatter.Format(sampleEntry)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":             true,
		"current_format": format,
		"previews":       previews,
		"sample_data": map[string]interface{}{
			"timestamp":    sampleEntry.Timestamp.Format(time.RFC3339),
			"remote_addr":  sampleEntry.RemoteAddr,
			"method":       sampleEntry.Method,
			"path":         sampleEntry.Path,
			"status_code":  sampleEntry.StatusCode,
			"bytes_sent":   sampleEntry.BytesSent,
			"request_time": sampleEntry.RequestTime,
			"username":     sampleEntry.Username,
		},
	})
}

// ShowLogFormatPage renders the log format configuration page
func (h *LogFormatHandler) ShowLogFormatPage(w http.ResponseWriter, r *http.Request) {
	// Get current format
	var logFormat string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT value FROM server_config WHERE key = 'logging.format'
	`).Scan(&logFormat)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("ERROR: ShowLogFormatPage: failed to read logging.format: %v", err)
	}

	if logFormat == "" {
		logFormat = "apache"
	}

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin-logs-format.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":          "Log Format Configuration",
		"page":           "logs-format",
		"current_format": logFormat,
		"formats": []map[string]string{
			{"id": "apache", "name": "Apache Combined", "description": "Standard Apache combined log format"},
			{"id": "nginx", "name": "Nginx", "description": "Nginx access log format with request time"},
			{"id": "json", "name": "JSON", "description": "Structured JSON logs for log aggregation"},
			{"id": "fail2ban", "name": "fail2ban", "description": "Compatible with fail2ban for IP blocking"},
			{"id": "syslog", "name": "Syslog (RFC 5424)", "description": "RFC 5424 compliant syslog format"},
			{"id": "cef", "name": "CEF (ArcSight)", "description": "Common Event Format for SIEM systems"},
			{"id": "text", "name": "Custom Text", "description": "Human-readable custom text format"},
		},
	}))
}
