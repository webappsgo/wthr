package service

import (
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestEvaluateRestartRequiredTracksRestartSettings(t *testing.T) {
	tests := []struct {
		name   string
		change func(*config.AppConfig)
		want   string
	}{
		{"address", func(cfg *config.AppConfig) { cfg.Server.Address = "127.0.0.1" }, "server.address"},
		{"port", func(cfg *config.AppConfig) { cfg.Server.Port = 8080 }, "server.port"},
		{"daemonize", func(cfg *config.AppConfig) { cfg.Server.Daemonize = true }, "server.daemonize"},
		{"pidfile", func(cfg *config.AppConfig) { cfg.Server.PIDFile = false }, "server.pidfile"},
		{"ssl", func(cfg *config.AppConfig) { cfg.Server.SSL.MinVersion = "TLS1.3" }, "server.ssl.*"},
		{"database", func(cfg *config.AppConfig) { cfg.Server.Database.URL = "postgres://db.example" }, "server.database.*"},
		{"tor", func(cfg *config.AppConfig) { cfg.Server.Tor.Enabled = true }, "server.tor.*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boot := config.DefaultAppConfig()
			current := config.DefaultAppConfig()
			boot.Server.Port = 8081
			current.Server.Port = 8081
			RecordBootSettings(boot)
			tt.change(current)
			EvaluateRestartRequired(current)

			pending, reasons := GetRestartRequired()
			if !pending {
				t.Fatal("GetRestartRequired() pending = false, want true")
			}
			if len(reasons) != 1 || reasons[0] != tt.want {
				t.Fatalf("GetRestartRequired() reasons = %v, want [%s]", reasons, tt.want)
			}
		})
	}
}

func TestEvaluateRestartRequiredClearsWhenSettingsReturnToBootValues(t *testing.T) {
	boot := config.DefaultAppConfig()
	changed := config.DefaultAppConfig()
	RecordBootSettings(boot)
	changed.Server.Tor.Enabled = !changed.Server.Tor.Enabled
	EvaluateRestartRequired(changed)

	EvaluateRestartRequired(boot)
	if pending, reasons := GetRestartRequired(); pending || len(reasons) != 0 {
		t.Fatalf("GetRestartRequired() = (%t, %v), want (false, [])", pending, reasons)
	}
}
