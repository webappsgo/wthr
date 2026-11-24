package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func loggerAuditTestDir(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(os.TempDir(), "webappsgo")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(parent, "wthr-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestLoggerConfiguresAuditFileRetentionAndDisable(t *testing.T) {
	previous := config.GetGlobalConfig()
	cfg := config.DefaultAppConfig()
	cfg.Server.Logs.Audit.Enabled = false
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	t.Setenv("MODE", "production")
	logDir := filepath.Join(loggerAuditTestDir(t), "logs")
	logger, err := NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "audit.log")); !os.IsNotExist(err) {
		t.Fatalf("disabled audit log was created: %v", err)
	}
	logger.Audit("admin", "admin.updated", "account", "", "", "127.0.0.1", "test", true, "")

	settings := cfg.Server.Logs.Audit
	settings.Enabled = true
	settings.Filename = "security-audit.jsonl"
	settings.Keep = "forever"
	settings.Compress = true
	if err := logger.ConfigureAudit(settings); err != nil {
		t.Fatal(err)
	}
	logger.Audit("admin", "admin.updated", "account", "", "", "127.0.0.1", "test", true, "")
	path := filepath.Join(logDir, settings.Filename)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("configured audit log was not created: %v", err)
	}
	if err := logger.RotateLogs(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(path + ".20*.gz")
	if err != nil || len(archives) != 1 {
		t.Fatalf("configured compressed archive=%v err=%v", archives, err)
	}
	if err := logger.ConfigureAudit(config.DefaultLogsConfig().Audit); err != nil {
		t.Fatal(err)
	}
	logger.Audit("admin", "admin.updated", "account", "", "", "127.0.0.1", "test", true, "")
	if _, err := os.Stat(filepath.Join(logDir, "audit.log")); err != nil {
		t.Fatalf("re-enabled default audit log was not created: %v", err)
	}
}
