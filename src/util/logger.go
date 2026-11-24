package util

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/auditlog"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/privilege"
)

// Logger handles application logging to both stdout and files
type Logger struct {
	auditMu      sync.RWMutex
	accessLog    *log.Logger
	serverLog    *log.Logger
	errorLog     *log.Logger
	auditLog     *auditlog.Logger
	auditPath    string
	auditEnabled bool
	securityLog  *log.Logger
	debugLog     *log.Logger
	logDir       string
	isDebug      bool
}

// NewLogger creates a new logger instance
func NewLogger(logDir string) (*Logger, error) {
	// Ensure log directory exists
	// AI.md PART 7: Permissions - root: 0755, user: 0700
	dirPerm := os.FileMode(0700)
	if privilege.StartedAsRoot() {
		dirPerm = 0755
	}
	if err := os.MkdirAll(logDir, dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Check if debug mode per AI.md PART 5: Boolean Handling
	isDebug := os.Getenv("MODE") == "development" || config.IsTruthy(os.Getenv("DEBUG"))

	l := &Logger{logDir: logDir, isDebug: isDebug}

	// Open log files
	accessFile, err := os.OpenFile(
		filepath.Join(logDir, "access.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open access.log: %w", err)
	}

	serverFile, err := os.OpenFile(
		filepath.Join(logDir, "server.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		_ = accessFile.Close()
		return nil, fmt.Errorf("failed to open server.log: %w", err)
	}

	errorFile, err := os.OpenFile(
		filepath.Join(logDir, "error.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		_ = accessFile.Close()
		_ = serverFile.Close()
		return nil, fmt.Errorf("failed to open error.log: %w", err)
	}

	auditSettings := config.DefaultLogsConfig().Audit
	if cfg := config.GetGlobalConfig(); cfg != nil {
		auditSettings = cfg.Server.Logs.Audit
	}
	if auditSettings.Filename == "" {
		auditSettings.Filename = "audit.log"
	}
	if filepath.Base(auditSettings.Filename) != auditSettings.Filename || auditSettings.Filename == "." || auditSettings.Filename == ".." {
		_ = accessFile.Close()
		_ = serverFile.Close()
		_ = errorFile.Close()
		return nil, fmt.Errorf("invalid audit log filename %q", auditSettings.Filename)
	}
	auditPath := filepath.Join(logDir, auditSettings.Filename)
	var auditFile *auditlog.Logger
	if auditSettings.Enabled {
		auditFile, err = auditlog.Open(auditPath, auditlog.Options{Keep: auditSettings.Keep, Compress: auditSettings.Compress})
		if err != nil {
			_ = accessFile.Close()
			_ = serverFile.Close()
			_ = errorFile.Close()
			return nil, fmt.Errorf("failed to open audit log: %w", err)
		}
	}

	securityFile, err := os.OpenFile(
		filepath.Join(logDir, "security.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		_ = accessFile.Close()
		_ = serverFile.Close()
		_ = errorFile.Close()
		if auditFile != nil {
			_ = auditFile.Close()
		}
		return nil, fmt.Errorf("failed to open security.log: %w", err)
	}

	// Create loggers that write to both file and stdout
	l.accessLog = log.New(
		io.MultiWriter(accessFile, os.Stdout),
		"",
		0,
	)

	l.serverLog = log.New(
		io.MultiWriter(serverFile, os.Stdout),
		"",
		0,
	)

	l.errorLog = log.New(
		io.MultiWriter(errorFile, os.Stderr),
		"",
		0,
	)

	l.auditLog = auditFile
	l.auditPath = auditPath
	l.auditEnabled = auditSettings.Enabled

	l.securityLog = log.New(
		// Security only to file, not stdout
		securityFile,
		"",
		0,
	)

	// Debug log only in debug mode
	if isDebug {
		debugFile, err := os.OpenFile(
			filepath.Join(logDir, "debug.log"),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY,
			0644,
		)
		if err == nil {
			l.debugLog = log.New(
				io.MultiWriter(debugFile, os.Stdout),
				"",
				0,
			)
		}
	}

	return l, nil
}

// Info logs an informational message
func (l *Logger) Info(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.accessLog.Printf("[%s] [INFO] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.errorLog.Printf("[%s] [ERROR] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.errorLog.Printf("[%s] [FATAL] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	os.Exit(1)
}

// Access logs an access entry (Apache Combined Log Format)
func (l *Logger) Access(ip, user, method, path, protocol string, status int, size int64, referer, userAgent string) {
	timestamp := time.Now().Format("02/Jan/2006:15:04:05 -0700")
	if user == "" {
		user = "-"
	}
	if referer == "" {
		referer = "-"
	}
	if userAgent == "" {
		userAgent = "-"
	}

	// Apache Combined Log Format
	entry := fmt.Sprintf(
		`%s - %s [%s] "%s %s %s" %d %d "%s" "%s"`,
		ip, user, timestamp, method, path, protocol, status, size, referer, userAgent,
	)
	l.accessLog.Println(entry)
}

// Server logs a server event
func (l *Logger) Server(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.serverLog.Printf("[%s] [SERVER] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
}

// Security logs a security event (fail2ban format)
func (l *Logger) Security(ip, event, details string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	// fail2ban compatible format
	entry := fmt.Sprintf("[%s] %s from %s: %s", timestamp, event, ip, details)
	l.securityLog.Println(entry)
}

// Debug logs a debug message (only in development mode)
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.isDebug && l.debugLog != nil {
		msg := fmt.Sprintf(format, v...)
		l.debugLog.Printf("[%s] [DEBUG] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	}
}

// Audit logs an audit entry (JSON format)
func (l *Logger) Audit(userID, action, resource, oldValue, newValue, ip, userAgent string, success bool, errorMsg string) {
	if l == nil {
		return
	}
	l.auditMu.RLock()
	defer l.auditMu.RUnlock()
	if !l.auditEnabled || l.auditLog == nil {
		return
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	result, severity := "failure", "warn"
	if success {
		result, severity = "success", "info"
	}
	details := map[string]any{"old_value": oldValue, "new_value": newValue}
	if errorMsg != "" {
		details["error"] = errorMsg
	}
	entry := map[string]any{
		"id":       ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(),
		"time":     now.Format("2006-01-02T15:04:05.000Z"),
		"event":    action,
		"category": "administration",
		"severity": severity,
		"actor":    map[string]string{"type": "user", "id": userID, "ip": ip, "user_agent": userAgent},
		"target":   map[string]string{"type": "resource", "id": resource},
		"details":  details,
		"result":   result,
		"source":   "web",
	}
	if _, err := l.auditLog.AppendJSON(entry); err != nil {
		l.Error("Failed to append audit event: %v", err)
	}
}

// Print is a helper for general output (goes to access log)
func (l *Logger) Print(v ...interface{}) {
	msg := fmt.Sprint(v...)
	// Also to stdout for compatibility
	fmt.Println(msg)
}

// Printf is a helper for formatted general output
func (l *Logger) Printf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	// Also to stdout for compatibility
	fmt.Println(msg)
}

// Write writes a pre-formatted log line to the access log (for LogFormatter)
func (l *Logger) Write(line string) {
	if l.accessLog != nil {
		l.accessLog.Print(line)
	}
}

// Warn logs a warning message (alias for Server with prefix)
func (l *Logger) Warn(format string, v ...interface{}) {
	l.Server("[WARN] "+format, v...)
}

// RotateLogs rotates log files (called by scheduler)
func (l *Logger) RotateLogs() error {
	timestamp := time.Now().Format("2006-01-02")

	// AI.md PART 11: Include all log files in rotation
	logFiles := []string{"access.log", "error.log", "security.log", "debug.log"}

	for _, logFile := range logFiles {
		currentPath := filepath.Join(l.logDir, logFile)
		archivePath := filepath.Join(l.logDir, fmt.Sprintf("%s.%s", logFile, timestamp))

		// Check if file exists and has content
		info, err := os.Stat(currentPath)
		if err != nil || info.Size() == 0 {
			continue
		}

		// Copy current log to archive
		if err := copyFile(currentPath, archivePath); err != nil {
			return fmt.Errorf("failed to archive %s: %w", logFile, err)
		}

		// Truncate current log
		if err := os.Truncate(currentPath, 0); err != nil {
			return fmt.Errorf("failed to truncate %s: %w", logFile, err)
		}
	}
	l.auditMu.RLock()
	if l.auditLog != nil {
		if err := l.auditLog.Rotate(); err != nil {
			l.auditMu.RUnlock()
			return fmt.Errorf("failed to rotate audit log: %w", err)
		}
	}
	l.auditMu.RUnlock()

	// Compress old logs (older than 1 day)
	// Delete logs older than 30 days
	if err := l.cleanOldLogs(); err != nil {
		return fmt.Errorf("failed to clean old logs: %w", err)
	}

	return nil
}

// ConfigureAudit applies the audit settings loaded at startup or by a config reload.
func (l *Logger) ConfigureAudit(settings config.AuditLogConfig) error {
	if l == nil {
		return nil
	}
	l.auditMu.Lock()
	defer l.auditMu.Unlock()
	if settings.Filename == "" {
		settings.Filename = "audit.log"
	}
	if filepath.Base(settings.Filename) != settings.Filename || settings.Filename == "." || settings.Filename == ".." {
		return fmt.Errorf("invalid audit log filename %q", settings.Filename)
	}
	nextPath := filepath.Join(l.logDir, settings.Filename)
	if !settings.Enabled {
		if l.auditLog != nil {
			if err := l.auditLog.Close(); err != nil {
				return err
			}
			l.auditLog = nil
		}
		l.auditPath = nextPath
		l.auditEnabled = false
		return nil
	}
	if l.auditLog != nil && l.auditPath == nextPath {
		if err := l.auditLog.Configure(auditlog.Options{Keep: settings.Keep, Compress: settings.Compress}); err != nil {
			return err
		}
	} else {
		if l.auditLog != nil {
			if err := l.auditLog.Close(); err != nil {
				return err
			}
		}
		opened, err := auditlog.Open(nextPath, auditlog.Options{Keep: settings.Keep, Compress: settings.Compress})
		if err != nil {
			return err
		}
		l.auditLog = opened
	}
	l.auditPath = nextPath
	l.auditEnabled = true
	return nil
}

// cleanOldLogs removes logs older than retention period
func (l *Logger) cleanOldLogs() error {
	// 30 days ago
	cutoff := time.Now().AddDate(0, 0, -30)

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == filepath.Base(l.auditPath)+".chain" || strings.HasPrefix(entry.Name(), filepath.Base(l.auditPath)+".20") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			logPath := filepath.Join(l.logDir, entry.Name())
			if err := os.Remove(logPath); err != nil {
				l.Error("Failed to remove old log %s: %v", entry.Name(), err)
			}
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
