package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStartupSettingsUsesMergedServerAndDropins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  port: 8080\n  address: 127.0.0.1\n  daemonize: false\n  token: ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dropins := filepath.Join(dir, DropinDirName)
	if err := os.Mkdir(dropins, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropins, "50-listener.yml"), []byte("server:\n  port: '8081,8444'\n  daemonize: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := LoadStartupSettings(dir)
	if got.Port != "8081,8444" || got.Address != "127.0.0.1" || !got.Daemonize {
		t.Fatalf("LoadStartupSettings() = %+v, want merged listener values", got)
	}
}

func TestLoadStartupSettingsSkipsUnparseableDropin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  port: 8082\n  address: '::1'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dropins := filepath.Join(dir, DropinDirName)
	if err := os.Mkdir(dropins, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropins, "50-bad.yml"), []byte("server: [broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := LoadStartupSettings(dir)
	if got.Port != "8082" || got.Address != "::1" || got.Daemonize {
		t.Fatalf("LoadStartupSettings() = %+v, want server.yml values after bad drop-in is skipped", got)
	}
}

func TestLoadStartupSettingsIgnoresInvalidAddressUntilFullValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  address: example.invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadStartupSettings(dir).Address; got != "[::]" {
		t.Fatalf("early invalid address = %q, want the normalized safe default", got)
	}
	cfg := DefaultAppConfig()
	cfg.Server.Address = "example.invalid"
	validateConfig(cfg)
	if cfg.Server.Address != "[::]" {
		t.Fatalf("full config address = %q, want [::]", cfg.Server.Address)
	}
}

func TestLoadStartupSettingsReadsPIDFileFromMergedView(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  pidfile: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadStartupSettings(dir)
	if enabled, ok := got.PIDFile.(bool); !ok || enabled {
		t.Fatalf("LoadStartupSettings().PIDFile = %#v, want false", got.PIDFile)
	}
}
