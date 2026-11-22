package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/server/lifecycle"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
)

type LogsHandler struct {
	logsDir string
	logFile string
}

func NewLogsHandler(logsDir string) *LogsHandler {
	return &LogsHandler{
		logsDir: logsDir,
		logFile: filepath.Join(logsDir, "wthr.log"),
	}
}

// ShowLogsPage renders log entries and filtering controls in HTML for no-JavaScript use.
func (h *LogsHandler) ShowLogsPage(w http.ResponseWriter, r *http.Request) {
	tail := 250
	tailValue := r.URL.Query().Get("tail")
	if tailValue == "" {
		tailValue = "250"
	}
	if raw := tailValue; raw != "" {
		if raw == "all" {
			tail = -1
		} else if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
			tail = parsed
		}
	}
	if tail == -1 {
		tailValue = "all"
	} else {
		tailValue = strconv.Itoa(tail)
	}
	entries, err := h.readLogs(tail)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_read_logs"))
		return
	}
	level := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("level")))
	source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	filtered := make([]LogEntry, 0, len(entries))
	errorCount, warningCount := 0, 0
	for _, entry := range entries {
		if level != "" && level != "ALL" && entry.Level != level {
			continue
		}
		if source != "" && source != "all" && !strings.EqualFold(entry.Source, source) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(query)) {
			continue
		}
		filtered = append(filtered, entry)
		if entry.Level == "ERROR" || entry.Level == "FATAL" {
			errorCount++
		}
		if entry.Level == "WARN" {
			warningCount++
		}
	}
	data := AdminTemplateData(r, map[string]interface{}{
		"title": "System Logs - Admin", "page": "logs", "breadcrumb": "System Logs",
		"logs": filtered, "total": len(entries), "filtered": len(filtered), "errors": errorCount, "warnings": warningCount,
		"level": level, "source": source, "query": query, "tail": tailValue,
		"cleared":     r.URL.Query().Get("cleared") == "1",
		"clear_error": r.URL.Query().Get("clear_error") == "1",
	})
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_logs.tmpl", data)
}

// ClearLogsForm clears the current application log through the session-authenticated form route.
func (h *LogsHandler) ClearLogsForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.Form.Get("confirm") != "yes" {
		http.Redirect(w, r, adminPagePath(r, "/logs")+"?clear_error=1", http.StatusSeeOther)
		return
	}
	if err := os.Truncate(h.logFile, 0); err != nil {
		http.Redirect(w, r, adminPagePath(r, "/logs")+"?clear_error=1", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, adminPagePath(r, "/logs")+"?cleared=1", http.StatusSeeOther)
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Message   string `json:"message"`
}

// GetLogs retrieves application logs with filtering
func (h *LogsHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	// Get tail parameter (number of lines to return)
	tailParam := r.URL.Query().Get("tail")
	if tailParam == "" {
		tailParam = "250"
	}
	var tailLines int
	if tailParam == "all" {
		// All lines
		tailLines = -1
	} else {
		var err error
		tailLines, err = strconv.Atoi(tailParam)
		if err != nil {
			tailLines = 250
		}
	}

	// Read log file
	logs, err := h.readLogs(tailLines)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_read_logs"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

// DownloadLogs allows downloading the complete log file
func (h *LogsHandler) DownloadLogs(w http.ResponseWriter, r *http.Request) {
	// Check if log file exists
	if _, err := os.Stat(h.logFile); os.IsNotExist(err) {
		NotFound(w, r, Translate(r, "errors.admin.logs.log_file_not_found"))
		return
	}

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=weather_%s.log", time.Now().Format("2006-01-02")))
	w.Header().Set("Content-Type", "text/plain")
	http.ServeFile(w, r, h.logFile)
}

// ClearLogs clears all log entries
func (h *LogsHandler) ClearLogs(w http.ResponseWriter, r *http.Request) {
	// Truncate log file
	if err := os.Truncate(h.logFile, 0); err != nil {
		InternalError(w, r, Translate(r, "errors.admin.core.failed_to_clear_logs"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.logs.logs_cleared_successfully"),
	})
}

// GetLogStats returns statistics about the logs
func (h *LogsHandler) GetLogStats(w http.ResponseWriter, r *http.Request) {
	// Read all logs
	logs, err := h.readLogs(-1)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_read_logs"))
		return
	}

	stats := map[string]interface{}{
		"total":   len(logs),
		"debug":   0,
		"info":    0,
		"warn":    0,
		"error":   0,
		"fatal":   0,
		"sources": make(map[string]int),
	}

	for _, log := range logs {
		switch log.Level {
		case "DEBUG":
			stats["debug"] = stats["debug"].(int) + 1
		case "INFO":
			stats["info"] = stats["info"].(int) + 1
		case "WARN":
			stats["warn"] = stats["warn"].(int) + 1
		case "ERROR":
			stats["error"] = stats["error"].(int) + 1
		case "FATAL":
			stats["fatal"] = stats["fatal"].(int) + 1
		}

		sources := stats["sources"].(map[string]int)
		sources[log.Source]++
	}

	writeJSON(w, http.StatusOK, stats)
}

// StreamLogs streams logs in real-time (Server-Sent Events)
func (h *LogsHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(w, r, Translate(r, "errors.admin.logs.streaming_unsupported"))
		return
	}

	// Get current file size
	fileInfo, err := os.Stat(h.logFile)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_stat_log_file"))
		return
	}

	lastSize := fileInfo.Size()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// AI.md PART 8: clients reconnect with a 3s base delay after a clean close
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-lifecycle.Done():
			if _, err := fmt.Fprint(w, "event: shutdown\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
			return
		case <-ticker.C:
			// Check for new content
			fileInfo, err := os.Stat(h.logFile)
			if err != nil {
				continue
			}

			if fileInfo.Size() > lastSize {
				// Read new content
				file, err := os.Open(h.logFile)
				if err != nil {
					continue
				}

				if _, err := file.Seek(lastSize, 0); err != nil {
					_ = file.Close()
					return
				}
				scanner := bufio.NewScanner(file)

				for scanner.Scan() {
					line := scanner.Text()
					if line != "" {
						entry := h.parseLine(line)
						if err := writeSSEEvent(w, "log", entry); err != nil {
							_ = file.Close()
							return
						}
					}
				}

				if err := file.Close(); err != nil {
					log.Printf("WARNING: failed to close streamed log file: %v", err)
					return
				}
				lastSize = fileInfo.Size()
				flusher.Flush()
			}
		}
	}
}

// Helper: write a Server-Sent Event using the standard SSE wire framing
// ("event: <name>\ndata: <json>\n\n").
func writeSSEEvent(w http.ResponseWriter, event string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}

// Helper: Read logs from file
func (h *LogsHandler) readLogs(tailLines int) (result []LogEntry, retErr error) {
	// Check if log file exists
	if _, err := os.Stat(h.logFile); os.IsNotExist(err) {
		// Return empty logs if file doesn't exist yet
		return []LogEntry{}, nil
	}

	file, err := os.Open(h.logFile)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()

	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Apply tail if requested
	if tailLines > 0 && len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}

	// Parse lines into log entries
	logs := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			logs = append(logs, h.parseLine(line))
		}
	}

	return logs, nil
}

// Helper: Parse a log line into a LogEntry
func (h *LogsHandler) parseLine(line string) LogEntry {
	// Expected format: [2025-12-13 10:30:45] [INFO] [server] Message here
	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     "INFO",
		Source:    "unknown",
		Message:   line,
	}

	// Try to parse structured log
	if strings.HasPrefix(line, "[") {
		parts := strings.SplitN(line, "]", 4)
		if len(parts) >= 4 {
			entry.Timestamp = strings.TrimPrefix(parts[0], "[")
			entry.Level = strings.TrimPrefix(strings.TrimSpace(parts[1]), "[")
			entry.Source = strings.TrimPrefix(strings.TrimSpace(parts[2]), "[")
			entry.Message = strings.TrimSpace(parts[3])
		}
	}

	return entry
}

// RotateLogs rotates the current log file
func (h *LogsHandler) RotateLogs(w http.ResponseWriter, r *http.Request) {
	// Create archive filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	archivePath := filepath.Join(h.logsDir, fmt.Sprintf("weather_%s.log", timestamp))

	// Copy current log to archive
	if err := copyFile(h.logFile, archivePath); err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_archive_logs"))
		return
	}

	// Truncate current log file
	if err := os.Truncate(h.logFile, 0); err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_truncate_log_file"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.logs.logs_rotated_successfully"),
		"archive": archivePath,
	})
}

// ListArchivedLogs lists all archived log files
func (h *LogsHandler) ListArchivedLogs(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir(h.logsDir)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_read_logs_directory"))
		return
	}

	archives := make([]map[string]interface{}, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasPrefix(file.Name(), "weather_") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		archives = append(archives, map[string]interface{}{
			"name": file.Name(),
			"size": info.Size(),
			"date": info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"archives": archives})
}

// Helper: Copy file
func copyFile(src, dst string) (retErr error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, sourceFile.Close()) }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, destFile.Close()) }()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

// GetAuditLogs retrieves audit logs with filtering and pagination
func (h *LogsHandler) GetAuditLogs(w http.ResponseWriter, r *http.Request) {
	auditFile := filepath.Join(h.logsDir, "audit.log")

	// Get query parameters
	query := r.URL.Query()
	limitParam := query.Get("limit")
	if limitParam == "" {
		limitParam = "100"
	}
	offsetParam := query.Get("offset")
	if offsetParam == "" {
		offsetParam = "0"
	}
	eventType := query.Get("event_type")
	username := query.Get("username")
	startDate := query.Get("start_date")
	endDate := query.Get("end_date")

	limit, _ := strconv.Atoi(limitParam)
	offset, _ := strconv.Atoi(offsetParam)

	// Read audit log
	events, total, err := h.readAuditLogs(auditFile, limit, offset, eventType, username, startDate, endDate)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "READ_FAILED", Translate(r, "errors.admin.logs.failed_to_read_audit_logs"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"events": events,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// DownloadAuditLogs allows downloading the complete audit log file
func (h *LogsHandler) DownloadAuditLogs(w http.ResponseWriter, r *http.Request) {
	auditFile := filepath.Join(h.logsDir, "audit.log")

	// Check if file exists
	if _, err := os.Stat(auditFile); os.IsNotExist(err) {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", Translate(r, "errors.admin.logs.audit_log_file_not_found"))
		return
	}

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=audit_%s.log", time.Now().Format("2006-01-02")))
	w.Header().Set("Content-Type", "application/json")
	http.ServeFile(w, r, auditFile)
}

// SearchAuditLogs searches audit logs with advanced filtering
func (h *LogsHandler) SearchAuditLogs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventType string `json:"event_type"`
		Username  string `json:"username"`
		IP        string `json:"ip"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Success   *bool  `json:"success"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.admin.logs.invalid_search_parameters"))
		return
	}

	// Default limits
	if req.Limit == 0 {
		req.Limit = 100
	}

	auditFile := filepath.Join(h.logsDir, "audit.log")
	events, total, err := h.searchAuditLogs(auditFile, req)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "SEARCH_FAILED", Translate(r, "errors.admin.logs.failed_to_search_audit_logs"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"events": events,
			"total":  total,
			"limit":  req.Limit,
			"offset": req.Offset,
		},
	})
}

// GetAuditStats returns statistics about audit events
func (h *LogsHandler) GetAuditStats(w http.ResponseWriter, r *http.Request) {
	auditFile := filepath.Join(h.logsDir, "audit.log")

	// Read all audit logs
	events, _, err := h.readAuditLogs(auditFile, -1, 0, "", "", "", "")
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "READ_FAILED", Translate(r, "errors.admin.logs.failed_to_read_audit_logs"))
		return
	}

	stats := map[string]interface{}{
		"total":           len(events),
		"successful":      0,
		"failed":          0,
		"event_types":     make(map[string]int),
		"users":           make(map[string]int),
		"recent_events":   []service.AuditEvent{},
		"recent_failures": []service.AuditEvent{},
		"top_event_types": []map[string]interface{}{},
		"hourly_activity": make(map[string]int),
	}

	for _, event := range events {
		if event.Result == "success" {
			stats["successful"] = stats["successful"].(int) + 1
		} else {
			stats["failed"] = stats["failed"].(int) + 1
		}

		// Count event types
		eventTypes := stats["event_types"].(map[string]int)
		eventTypes[event.Event]++

		// Count users
		if event.Actor.ID != "" {
			users := stats["users"].(map[string]int)
			users[event.Actor.ID]++
		}

		// Hourly activity (last 24 hours)
		hour := event.Time.Format("2006-01-02 15")
		hourlyActivity := stats["hourly_activity"].(map[string]int)
		hourlyActivity[hour]++
	}

	// Get recent events (last 10)
	if len(events) > 10 {
		stats["recent_events"] = events[:10]
	} else {
		stats["recent_events"] = events
	}

	// Get recent failures
	failures := []service.AuditEvent{}
	for _, event := range events {
		if event.Result == "failure" {
			failures = append(failures, event)
			if len(failures) >= 10 {
				break
			}
		}
	}
	stats["recent_failures"] = failures

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"data": stats,
	})
}

// Helper: Read audit logs from file
func (h *LogsHandler) readAuditLogs(filePath string, limit, offset int, eventType, username, startDate, endDate string) (events []service.AuditEvent, count int, retErr error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Return empty events if file doesn't exist yet
		return []service.AuditEvent{}, 0, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()

	var allEvents []service.AuditEvent
	scanner := bufio.NewScanner(file)

	// Parse start and end dates if provided
	var start, end time.Time
	if startDate != "" {
		start, _ = time.Parse("2006-01-02", startDate)
	}
	if endDate != "" {
		end, _ = time.Parse("2006-01-02", endDate)
		// Include full end date
		end = end.Add(24 * time.Hour)
	}

	for scanner.Scan() {
		var event service.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			// Skip invalid lines
			continue
		}

		// Apply filters
		if eventType != "" && event.Event != eventType {
			continue
		}
		if username != "" && event.Actor.ID != username {
			continue
		}
		if !start.IsZero() && event.Time.Before(start) {
			continue
		}
		if !end.IsZero() && event.Time.After(end) {
			continue
		}

		allEvents = append(allEvents, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	// Reverse to show newest first
	for i := len(allEvents)/2 - 1; i >= 0; i-- {
		opp := len(allEvents) - 1 - i
		allEvents[i], allEvents[opp] = allEvents[opp], allEvents[i]
	}

	total := len(allEvents)

	// Apply pagination
	if offset > 0 && offset < len(allEvents) {
		allEvents = allEvents[offset:]
	}
	if limit > 0 && limit < len(allEvents) {
		allEvents = allEvents[:limit]
	}

	return allEvents, total, nil
}

// Helper: Search audit logs with advanced filtering
func (h *LogsHandler) searchAuditLogs(filePath string, searchReq struct {
	EventType string `json:"event_type"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Success   *bool  `json:"success"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}) (events []service.AuditEvent, count int, retErr error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []service.AuditEvent{}, 0, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()

	var allEvents []service.AuditEvent
	scanner := bufio.NewScanner(file)

	// Parse dates
	var start, end time.Time
	if searchReq.StartDate != "" {
		start, _ = time.Parse("2006-01-02", searchReq.StartDate)
	}
	if searchReq.EndDate != "" {
		end, _ = time.Parse("2006-01-02", searchReq.EndDate)
		end = end.Add(24 * time.Hour)
	}

	for scanner.Scan() {
		var event service.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Apply all filters
		if searchReq.EventType != "" && event.Event != searchReq.EventType {
			continue
		}
		if searchReq.Username != "" && event.Actor.ID != searchReq.Username {
			continue
		}
		if searchReq.IP != "" && event.Actor.IP != searchReq.IP {
			continue
		}
		if searchReq.Success != nil {
			isSuccess := event.Result == "success"
			if isSuccess != *searchReq.Success {
				continue
			}
		}
		if !start.IsZero() && event.Time.Before(start) {
			continue
		}
		if !end.IsZero() && event.Time.After(end) {
			continue
		}

		allEvents = append(allEvents, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	// Reverse to show newest first
	for i := len(allEvents)/2 - 1; i >= 0; i-- {
		opp := len(allEvents) - 1 - i
		allEvents[i], allEvents[opp] = allEvents[opp], allEvents[i]
	}

	total := len(allEvents)

	// Apply pagination
	if searchReq.Offset > 0 && searchReq.Offset < len(allEvents) {
		allEvents = allEvents[searchReq.Offset:]
	}
	if searchReq.Limit > 0 && searchReq.Limit < len(allEvents) {
		allEvents = allEvents[:searchReq.Limit]
	}

	return allEvents, total, nil
}
