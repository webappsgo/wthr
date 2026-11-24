package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultAuditLogSettings(t *testing.T) {
	audit := DefaultAppConfig().Server.Logs.Audit
	if !audit.Enabled || audit.Filename != "audit.log" || audit.Format != "json" || audit.Rotate != "daily" || audit.Keep != "none" || audit.Compress {
		t.Fatalf("default audit settings = %+v", audit)
	}
}

func TestAuditLogConfigLoadsAndValidatesRetention(t *testing.T) {
	cfg := DefaultAppConfig()
	if err := yaml.Unmarshal([]byte("server:\n  logs:\n    audit:\n      enabled: true\n      filename: security-audit.jsonl\n      format: json\n      rotate: daily\n      keep: 90d\n      compress: true\n"), cfg); err != nil {
		t.Fatal(err)
	}
	validateConfig(cfg)
	audit := cfg.Server.Logs.Audit
	if audit.Filename != "security-audit.jsonl" || audit.Keep != "90d" || !audit.Compress {
		t.Fatalf("loaded audit settings = %+v", audit)
	}
	for _, keep := range []string{"none", "0", "7", "30d", "12m", "forever"} {
		if !validAuditRetention(keep) {
			t.Errorf("validAuditRetention(%q) = false", keep)
		}
	}
	for _, keep := range []string{"", "-1d", "1y", "daily", "1.5d"} {
		if validAuditRetention(keep) {
			t.Errorf("validAuditRetention(%q) = true", keep)
		}
	}
}

func TestInvalidAuditLogSettingsUseDefaults(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Server.Logs.Audit.Filename = "../outside.log"
	cfg.Server.Logs.Audit.Format = "text"
	cfg.Server.Logs.Audit.Keep = "forever-ish"
	validateConfig(cfg)
	audit := cfg.Server.Logs.Audit
	if audit.Filename != "audit.log" || audit.Format != "json" || audit.Keep != "none" {
		t.Fatalf("invalid audit settings were not reset: %+v", audit)
	}
}
