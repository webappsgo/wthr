package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/apimgr/weather/src/config"
)

// Logger handles application logging to both stdout and files
type Logger struct {
	accessLog   *log.Logger
	serverLog   *log.Logger
	errorLog    *log.Logger
	auditLog    *log.Logger
	securityLog *log.Logger
	debugLog    *log.Logger
	logDir      string
	isDebug     bool
}

// NewLogger creates a new logger instance
func NewLogger(logDir string) (*Logger, error) {
	// Ensure log directory exists
	// AI.md PART 7: Permissions - root: 0755, user: 0700
	dirPerm := os.FileMode(0700)
	if os.Geteuid() == 0 {
		dirPerm = 0755
	}
	if err := os.MkdirAll(logDir, dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Check if debug mode per AI.md PART 5: Boolean Handling
	isDebug := os.Getenv("MODE") == "development" || config.IsTruthy(os.Getenv("DEBUG"))

	l := &Logger{
		logDir:  logDir,
		isDebug: isDebug,
	}

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
		accessFile.Close()
		return nil, fmt.Errorf("failed to open server.log: %w", err)
	}

	errorFile, err := os.OpenFile(
		filepath.Join(logDir, "error.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		accessFile.Close()
		serverFile.Close()
		return nil, fmt.Errorf("failed to open error.log: %w", err)
	}

	auditFile, err := os.OpenFile(
		filepath.Join(logDir, "audit.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		accessFile.Close()
		serverFile.Close()
		errorFile.Close()
		return nil, fmt.Errorf("failed to open audit.log: %w", err)
	}

	securityFile, err := os.OpenFile(
		filepath.Join(logDir, "security.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		accessFile.Close()
		serverFile.Close()
		errorFile.Close()
		auditFile.Close()
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

	l.auditLog = log.New(
		// Audit only to file, not stdout
		auditFile,
		"",
		0,
	)

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
	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf(
		`{"timestamp":"%s","user_id":"%s","action":"%s","resource":"%s","old_value":"%s","new_value":"%s","ip":"%s","user_agent":"%s","success":%t,"error":"%s"}`,
		timestamp, userID, action, resource, oldValue, newValue, ip, userAgent, success, errorMsg,
	)
	l.auditLog.Println(entry)
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
	logFiles := []string{"access.log", "error.log", "audit.log", "security.log", "debug.log"}

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

	// Compress old logs (older than 1 day)
	// Delete logs older than 30 days
	if err := l.cleanOldLogs(); err != nil {
		return fmt.Errorf("failed to clean old logs: %w", err)
	}

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
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
