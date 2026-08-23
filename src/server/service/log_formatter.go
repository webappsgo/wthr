package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// LogFormat represents supported log format types
// TEMPLATE.md Part 25: Support 7 log formats
type LogFormat string

const (
	// Apache Combined Log Format
	LogFormatApache LogFormat = "apache"
	// Nginx access log format
	LogFormatNginx LogFormat = "nginx"
	// JSON structured logs
	LogFormatJSON LogFormat = "json"
	// fail2ban-compatible format
	LogFormatFail2ban LogFormat = "fail2ban"
	// RFC 5424 Syslog
	LogFormatSyslog LogFormat = "syslog"
	// Common Event Format (ArcSight)
	LogFormatCEF LogFormat = "cef"
	// Custom text format
	LogFormatText LogFormat = "text"
)

// LogEntry represents a single log entry with all fields
type LogEntry struct {
	Timestamp  time.Time
	RemoteAddr string
	Method     string
	Path       string
	Protocol   string
	StatusCode int
	BytesSent  int
	Referer    string
	UserAgent  string
	// in seconds
	RequestTime float64
	RequestID   string
	// For authenticated requests
	Username string
	// For error logs
	ErrorMessage string
	// For syslog
	Facility string
	// For syslog/CEF
	Severity string
	// For CEF
	DeviceVendor string
	// For CEF
	DeviceProduct string
	// For CEF
	DeviceVersion string
	// For CEF
	SignatureID string
	// For CEF
	Name string
}

// LogFormatter handles different log output formats
type LogFormatter struct {
	format        LogFormat
	deviceVendor  string
	deviceProduct string
	deviceVersion string
}

// NewLogFormatter creates a new log formatter
func NewLogFormatter(format LogFormat) *LogFormatter {
	return &LogFormatter{
		format:        format,
		deviceVendor:  "webappsgo",
		deviceProduct: "wthr",
		deviceVersion: "1.0",
	}
}

// Format formats a log entry according to the configured format
func (f *LogFormatter) Format(entry *LogEntry) string {
	switch f.format {
	case LogFormatApache:
		return f.formatApache(entry)
	case LogFormatNginx:
		return f.formatNginx(entry)
	case LogFormatJSON:
		return f.formatJSON(entry)
	case LogFormatFail2ban:
		return f.formatFail2ban(entry)
	case LogFormatSyslog:
		return f.formatSyslog(entry)
	case LogFormatCEF:
		return f.formatCEF(entry)
	case LogFormatText:
		return f.formatText(entry)
	default:
		// Default to Apache
		return f.formatApache(entry)
	}
}

// formatApache formats in Apache Combined Log Format
// TEMPLATE.md Part 25: Apache Combined format
// Format: %h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-agent}i"
func (f *LogFormatter) formatApache(entry *LogEntry) string {
	username := "-"
	if entry.Username != "" {
		username = entry.Username
	}

	referer := "-"
	if entry.Referer != "" {
		referer = entry.Referer
	}

	userAgent := "-"
	if entry.UserAgent != "" {
		userAgent = entry.UserAgent
	}

	timestamp := entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700")

	return fmt.Sprintf("%s - %s [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"",
		entry.RemoteAddr,
		username,
		timestamp,
		entry.Method,
		entry.Path,
		entry.Protocol,
		entry.StatusCode,
		entry.BytesSent,
		referer,
		userAgent,
	)
}

// formatNginx formats in Nginx access log format
// TEMPLATE.md Part 25: Nginx format
func (f *LogFormatter) formatNginx(entry *LogEntry) string {
	username := "-"
	if entry.Username != "" {
		username = entry.Username
	}

	referer := "-"
	if entry.Referer != "" {
		referer = entry.Referer
	}

	userAgent := "-"
	if entry.UserAgent != "" {
		userAgent = entry.UserAgent
	}

	timestamp := entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700")

	return fmt.Sprintf("%s - %s [%s] \"%s %s %s\" %d %d \"%s\" \"%s\" %.3f",
		entry.RemoteAddr,
		username,
		timestamp,
		entry.Method,
		entry.Path,
		entry.Protocol,
		entry.StatusCode,
		entry.BytesSent,
		referer,
		userAgent,
		entry.RequestTime,
	)
}

// formatJSON formats as JSON structured log
// TEMPLATE.md Part 25: JSON format
func (f *LogFormatter) formatJSON(entry *LogEntry) string {
	logData := map[string]interface{}{
		"timestamp":    entry.Timestamp.Format(time.RFC3339Nano),
		"remote_addr":  entry.RemoteAddr,
		"method":       entry.Method,
		"path":         entry.Path,
		"protocol":     entry.Protocol,
		"status_code":  entry.StatusCode,
		"bytes_sent":   entry.BytesSent,
		"request_time": entry.RequestTime,
		"request_id":   entry.RequestID,
	}

	if entry.Username != "" {
		logData["username"] = entry.Username
	}

	if entry.Referer != "" {
		logData["referer"] = entry.Referer
	}

	if entry.UserAgent != "" {
		logData["user_agent"] = entry.UserAgent
	}

	if entry.ErrorMessage != "" {
		logData["error"] = entry.ErrorMessage
	}

	jsonBytes, _ := json.Marshal(logData)
	return string(jsonBytes)
}

// formatFail2ban formats for fail2ban compatibility
// TEMPLATE.md Part 25: fail2ban-compatible format
func (f *LogFormatter) formatFail2ban(entry *LogEntry) string {
	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")

	// fail2ban looks for patterns like:
	// "Failed login from <IP>" or "Authentication failure from <IP>"
	action := "access"
	if entry.StatusCode >= 400 {
		action = "failed"
	}
	if entry.StatusCode == 401 || entry.StatusCode == 403 {
		action = "auth_failed"
	}

	return fmt.Sprintf("[%s] %s: %s %s %s from %s - status=%d",
		timestamp,
		action,
		entry.Method,
		entry.Path,
		entry.Protocol,
		entry.RemoteAddr,
		entry.StatusCode,
	)
}

// formatSyslog formats according to RFC 5424 Syslog
// TEMPLATE.md Part 25: RFC 5424 Syslog format
func (f *LogFormatter) formatSyslog(entry *LogEntry) string {
	// RFC 5424 format: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG

	// Calculate priority: facility * 8 + severity
	// local0
	facility := 16
	// informational
	severity := 6
	if entry.StatusCode >= 500 {
		// error
		severity = 3
	} else if entry.StatusCode >= 400 {
		// warning
		severity = 4
	}
	priority := facility*8 + severity

	timestamp := entry.Timestamp.Format(time.RFC3339)
	hostname := "-"
	appName := "wthr"
	procID := "-"
	msgID := entry.RequestID
	if msgID == "" {
		msgID = "-"
	}

	// Structured data
	structuredData := fmt.Sprintf("[request@48577 method=\"%s\" path=\"%s\" status=\"%d\" ip=\"%s\"]",
		entry.Method,
		entry.Path,
		entry.StatusCode,
		entry.RemoteAddr,
	)

	msg := fmt.Sprintf("%s %s %s %d bytes %.3fs",
		entry.Method,
		entry.Path,
		entry.Protocol,
		entry.BytesSent,
		entry.RequestTime,
	)

	return fmt.Sprintf("<%d>1 %s %s %s %s %s %s %s",
		priority,
		timestamp,
		hostname,
		appName,
		procID,
		msgID,
		structuredData,
		msg,
	)
}

// formatCEF formats as Common Event Format (ArcSight)
// TEMPLATE.md Part 25: CEF format
func (f *LogFormatter) formatCEF(entry *LogEntry) string {
	// CEF format:
	// CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension

	version := "0"
	signatureID := fmt.Sprintf("HTTP_%d", entry.StatusCode)
	name := fmt.Sprintf("HTTP %s", entry.Method)

	// Medium
	severity := "3"
	if entry.StatusCode >= 500 {
		// High
		severity = "8"
	} else if entry.StatusCode >= 400 {
		// Medium-High
		severity = "5"
	} else if entry.StatusCode >= 300 {
		// Low
		severity = "2"
	} else if entry.StatusCode >= 200 {
		// Very Low
		severity = "1"
	}

	// Extension fields (key=value pairs)
	extensions := []string{
		// milliseconds
		fmt.Sprintf("rt=%d", entry.Timestamp.Unix()*1000),
		fmt.Sprintf("src=%s", entry.RemoteAddr),
		fmt.Sprintf("request=%s %s", entry.Method, entry.Path),
		fmt.Sprintf("requestMethod=%s", entry.Method),
		fmt.Sprintf("requestUrl=%s", entry.Path),
		"cs1Label=Protocol",
		fmt.Sprintf("cs1=%s", entry.Protocol),
		"cs2Label=StatusCode",
		fmt.Sprintf("cs2=%d", entry.StatusCode),
		"cn1Label=BytesSent",
		fmt.Sprintf("cn1=%d", entry.BytesSent),
		"cn2Label=RequestTime",
		fmt.Sprintf("cn2=%.3f", entry.RequestTime),
	}

	if entry.UserAgent != "" {
		extensions = append(extensions, fmt.Sprintf("requestClientApplication=%s", escapeCEF(entry.UserAgent)))
	}

	if entry.Username != "" {
		extensions = append(extensions, fmt.Sprintf("suser=%s", entry.Username))
	}

	extensionStr := strings.Join(extensions, " ")

	return fmt.Sprintf("CEF:%s|%s|%s|%s|%s|%s|%s|%s",
		version,
		f.deviceVendor,
		f.deviceProduct,
		f.deviceVersion,
		signatureID,
		name,
		severity,
		extensionStr,
	)
}

// formatText formats as custom text format
// TEMPLATE.md Part 25: Custom text format
func (f *LogFormatter) formatText(entry *LogEntry) string {
	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05.000")

	var status string
	if entry.StatusCode >= 500 {
		status = fmt.Sprintf("ERROR %d", entry.StatusCode)
	} else if entry.StatusCode >= 400 {
		status = fmt.Sprintf("WARN %d", entry.StatusCode)
	} else if entry.StatusCode >= 300 {
		status = fmt.Sprintf("REDIR %d", entry.StatusCode)
	} else {
		status = fmt.Sprintf("OK %d", entry.StatusCode)
	}

	parts := []string{
		fmt.Sprintf("[%s]", timestamp),
		fmt.Sprintf("[%s]", entry.RemoteAddr),
		fmt.Sprintf("[%s]", status),
		fmt.Sprintf("%s %s", entry.Method, entry.Path),
		fmt.Sprintf("%.0fms", entry.RequestTime*1000),
		fmt.Sprintf("%dB", entry.BytesSent),
	}

	if entry.RequestID != "" {
		parts = append(parts, fmt.Sprintf("id=%s", entry.RequestID))
	}

	if entry.Username != "" {
		parts = append(parts, fmt.Sprintf("user=%s", entry.Username))
	}

	return strings.Join(parts, " ")
}

// escapeCSF escapes special characters for CEF format
func escapeCEF(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "=", "\\=")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// ExtractLogEntry extracts log entry data from an http.Request
func ExtractLogEntry(r *http.Request, startTime time.Time, statusCode int, bytesWritten int) *LogEntry {
	entry := &LogEntry{
		Timestamp:   startTime,
		RemoteAddr:  util.TrustedGetClientIP(r),
		Method:      r.Method,
		Path:        r.URL.Path,
		Protocol:    r.Proto,
		StatusCode:  statusCode,
		BytesSent:   bytesWritten,
		Referer:     r.Referer(),
		UserAgent:   r.UserAgent(),
		RequestTime: time.Since(startTime).Seconds(),
	}

	// Extract request ID if available
	if requestID, exists := reqctx.Get(r.Context(), "request_id"); exists {
		if rid, ok := requestID.(string); ok {
			entry.RequestID = rid
		}
	}

	// Extract username if authenticated
	if username, exists := reqctx.Get(r.Context(), "username"); exists {
		if uname, ok := username.(string); ok {
			entry.Username = uname
		}
	}

	return entry
}
