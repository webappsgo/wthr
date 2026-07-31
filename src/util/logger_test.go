package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewLogger verifies the log directory and all standard log files are
// created (debug.log excluded unless MODE=development or DEBUG is truthy).
func TestNewLogger(t *testing.T) {
	t.Setenv("MODE", "production")
	t.Setenv("DEBUG", "")
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")

	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	for _, f := range []string{"access.log", "server.log", "error.log", "audit.log", "security.log"} {
		if _, err := os.Stat(filepath.Join(logDir, f)); err != nil {
			t.Errorf("expected log file %s to exist: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(logDir, "debug.log")); !os.IsNotExist(err) {
		t.Errorf("debug.log should not exist in production mode without DEBUG set: err = %v", err)
	}
	if logger.isDebug {
		t.Error("isDebug = true, want false in production mode")
	}
}

// TestNewLogger_DebugMode verifies debug.log is created when MODE=development.
func TestNewLogger_DebugMode(t *testing.T) {
	t.Setenv("MODE", "development")
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")

	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if !logger.isDebug {
		t.Error("isDebug = false, want true in development mode")
	}
	if _, err := os.Stat(filepath.Join(logDir, "debug.log")); err != nil {
		t.Errorf("expected debug.log to exist: %v", err)
	}
}

func readLogFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", name, err)
	}
	return string(data)
}

// TestLogger_WriteMethods exercises each formatted-write method and
// verifies the expected content lands in the correct log file.
func TestLogger_WriteMethods(t *testing.T) {
	t.Setenv("MODE", "production")
	logDir := filepath.Join(t.TempDir(), "logs")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Info("info message %d", 1)
	logger.Error("error message %d", 2)
	logger.Server("server message %d", 3)
	logger.Warn("warn message %d", 4)

	access := readLogFile(t, logDir, "access.log")
	if !strings.Contains(access, "info message 1") {
		t.Error("access.log missing Info() output")
	}

	server := readLogFile(t, logDir, "server.log")
	if !strings.Contains(server, "server message 3") {
		t.Error("server.log missing Server() output")
	}
	if !strings.Contains(server, "[WARN] warn message 4") {
		t.Error("server.log missing Warn() output with [WARN] prefix")
	}

	errLog := readLogFile(t, logDir, "error.log")
	if !strings.Contains(errLog, "error message 2") {
		t.Error("error.log missing Error() output")
	}
}

// TestLogger_Access verifies Apache Combined Log Format and the "-"
// defaults for empty user/referer/userAgent.
func TestLogger_Access(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("MODE", "production")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Access("1.2.3.4", "", "GET", "/path", "HTTP/1.1", 200, 512, "", "")
	access := readLogFile(t, logDir, "access.log")
	if !strings.Contains(access, "1.2.3.4") {
		t.Error("access.log missing IP")
	}
	if !strings.Contains(access, `"GET /path HTTP/1.1"`) {
		t.Error("access.log missing request line")
	}
	if !strings.Contains(access, "200") || !strings.Contains(access, "512") {
		t.Error("access.log missing status/size")
	}
	// Empty user/referer/user-agent should be rendered as "-".
	if strings.Count(access, "\"-\"") < 1 && !strings.Contains(access, " - ") {
		t.Error("access.log does not appear to default empty fields to \"-\"")
	}
}

// TestLogger_Security verifies the fail2ban-style security log entry.
func TestLogger_Security(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("MODE", "production")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Security("5.6.7.8", "failed_login", "bad password")
	sec := readLogFile(t, logDir, "security.log")
	if !strings.Contains(sec, "5.6.7.8") {
		t.Error("security.log missing IP")
	}
	if !strings.Contains(sec, "failed_login") {
		t.Error("security.log missing event")
	}
	if !strings.Contains(sec, "bad password") {
		t.Error("security.log missing details")
	}
}

// TestLogger_Audit verifies the JSON-format audit log entry, including
// success and failure cases.
func TestLogger_Audit(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("MODE", "production")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Audit("user1", "update", "settings", "old", "new", "9.9.9.9", "agent", true, "")
	logger.Audit("user2", "delete", "account", "", "", "9.9.9.9", "agent", false, "permission denied")

	audit := readLogFile(t, logDir, "audit.log")
	if !strings.Contains(audit, "user1") || !strings.Contains(audit, "update") {
		t.Error("audit.log missing success entry fields")
	}
	if !strings.Contains(audit, "permission denied") {
		t.Error("audit.log missing failure error message")
	}
}

// TestLogger_Debug verifies Debug() only writes when isDebug is true.
func TestLogger_Debug(t *testing.T) {
	t.Run("debug_enabled", func(t *testing.T) {
		t.Setenv("MODE", "development")
		logDir := filepath.Join(t.TempDir(), "logs")
		logger, err := NewLogger(logDir)
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}
		logger.Debug("debug message %d", 1)
		debug := readLogFile(t, logDir, "debug.log")
		if !strings.Contains(debug, "debug message 1") {
			t.Error("debug.log missing Debug() output when debug enabled")
		}
	})

	t.Run("debug_disabled_no_panic", func(t *testing.T) {
		t.Setenv("MODE", "production")
		logDir := filepath.Join(t.TempDir(), "logs")
		logger, err := NewLogger(logDir)
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}
		// Must not panic even though debugLog is nil.
		logger.Debug("should be a no-op")
	})
}

// TestLogger_PrintAndWrite exercises stdout printing and the raw Write
// passthrough to accessLog, ensuring no panics.
func TestLogger_PrintAndWrite(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("MODE", "production")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Print("print line")
	logger.Printf("printf %d", 1)
	logger.Write("raw access line")

	access := readLogFile(t, logDir, "access.log")
	if !strings.Contains(access, "raw access line") {
		t.Error("access.log missing raw Write() line")
	}
}

// TestLogger_RotateLogs verifies rotation archives and truncates each log
// file, leaving the original (now-empty) file in place.
func TestLogger_RotateLogs(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("MODE", "production")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Access("1.1.1.1", "", "GET", "/", "HTTP/1.1", 200, 1, "", "")

	if err := logger.RotateLogs(); err != nil {
		t.Fatalf("RotateLogs: %v", err)
	}

	// access.log should now be empty (truncated) after rotation.
	info, err := os.Stat(filepath.Join(logDir, "access.log"))
	if err != nil {
		t.Fatalf("Stat access.log: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("access.log size = %d after rotation, want 0", info.Size())
	}

	// An archived copy with a date suffix should exist somewhere in logDir.
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	foundArchive := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "access.log.") {
			foundArchive = true
		}
	}
	if !foundArchive {
		t.Error("no archived access.log.<date> file found after RotateLogs")
	}
}

// TestLogger_CleanOldLogs verifies files older than 30 days are removed
// while recent files are kept.
func TestLogger_CleanOldLogs(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("MODE", "production")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	oldFile := filepath.Join(logDir, "access-old.log")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	oldTime := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	newFile := filepath.Join(logDir, "access-new.log")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := logger.cleanOldLogs(); err != nil {
		t.Fatalf("cleanOldLogs: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("old file still exists after cleanOldLogs: err = %v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("new file was removed, want kept: %v", err)
	}
}

// TestCopyFile verifies the unexported copy helper duplicates content.
func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("hello world"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst): %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("copied content = %q, want %q", data, "hello world")
	}
}
