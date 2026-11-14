package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLayerFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func layeredFixture(t *testing.T, serverYML string, dropins map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	writeLayerFile(t, path, serverYML, 0o600)
	for name, content := range dropins {
		writeLayerFile(t, filepath.Join(dir, DropinDirName, name), content, 0o600)
	}
	return path
}

func TestLoadLayersMergeOrderAndPrecedence(t *testing.T) {
	path := layeredFixture(t, "server:\n  fqdn: base.example\n  port: 8080\n", map[string]string{
		"10-tls.yml":  "server:\n  fqdn: tls.example\n",
		"20-mail.yml": "server:\n  fqdn: mail.example\n  address: 127.0.0.1\n",
	})

	merged, report, err := loadLayers(path, true)
	if err != nil {
		t.Fatalf("loadLayers: %v", err)
	}
	var cfg AppConfig
	if err := applyLayers(&cfg, merged); err != nil {
		t.Fatalf("applyLayers: %v", err)
	}
	if cfg.Server.FQDN != "mail.example" {
		t.Errorf("fqdn = %q, want the last drop-in (byte-wise order) to win", cfg.Server.FQDN)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want server.yml value preserved", cfg.Server.Port)
	}
	if cfg.Server.Address != "127.0.0.1" {
		t.Errorf("address = %q, want drop-in value", cfg.Server.Address)
	}
	if got := strings.Join(report.Files, ","); got != "yaml.d/10-tls.yml,yaml.d/20-mail.yml" {
		t.Errorf("loaded files = %q, want both drop-ins in order", got)
	}
	if report.Sources["server.fqdn"] != "yaml.d/20-mail.yml" {
		t.Errorf("source of server.fqdn = %q, want yaml.d/20-mail.yml", report.Sources["server.fqdn"])
	}
	overrides := 0
	for _, o := range report.Overrides {
		if o.Key == "server.fqdn" {
			overrides++
		}
	}
	if overrides != 2 {
		t.Errorf("server.fqdn override count = %d, want 2 (server.yml then 10-tls, 10-tls then 20-mail)", overrides)
	}
}

func TestLoadLayersListsReplaceWholeAndMapsMerge(t *testing.T) {
	path := layeredFixture(t, "server:\n  seo:\n    keywords: [a, b, c]\n  branding:\n    title: Base\n    tagline: Tag\n", map[string]string{
		"10-x.yml": "server:\n  seo:\n    keywords: [z]\n  branding:\n    title: Over\n",
	})
	merged, _, err := loadLayers(path, true)
	if err != nil {
		t.Fatalf("loadLayers: %v", err)
	}
	var cfg AppConfig
	if err := applyLayers(&cfg, merged); err != nil {
		t.Fatalf("applyLayers: %v", err)
	}
	if len(cfg.Server.SEO.Keywords) != 1 || cfg.Server.SEO.Keywords[0] != "z" {
		t.Errorf("keywords = %v, want the list replaced whole by [z]", cfg.Server.SEO.Keywords)
	}
	if cfg.Server.Branding.Title != "Over" || cfg.Server.Branding.Tagline != "Tag" {
		t.Errorf("branding = %+v, want title overridden and tagline kept", cfg.Server.Branding)
	}
}

func TestLoadLayersIgnoresNonDropinEntries(t *testing.T) {
	path := layeredFixture(t, "server:\n  port: 8080\n", map[string]string{
		"10-ok.yml":       "server:\n  fqdn: ok.example\n",
		".hidden.yml":     "server:\n  fqdn: hidden.example\n",
		"20-note.yml.bak": "server:\n  fqdn: bak.example\n",
		"30-old.yml~":     "server:\n  fqdn: tilde.example\n",
		"40-old.yml.orig": "server:\n  fqdn: orig.example\n",
		"50-notes.txt":    "server:\n  fqdn: txt.example\n",
		"sub/60-deep.yml": "server:\n  fqdn: deep.example\n",
	})
	_, report, err := loadLayers(path, true)
	if err != nil {
		t.Fatalf("loadLayers: %v", err)
	}
	if len(report.Files) != 1 || report.Files[0] != "yaml.d/10-ok.yml" {
		t.Errorf("loaded = %v, want only yaml.d/10-ok.yml", report.Files)
	}
}

func TestLoadLayersRejectsExplicitNull(t *testing.T) {
	path := layeredFixture(t, "server:\n  port: 8080\n", map[string]string{
		"10-null.yml": "server:\n  fqdn: ~\n",
	})
	_, _, err := loadLayers(path, true)
	if err == nil || !strings.Contains(err.Error(), "yaml.d/10-null.yml") || !strings.Contains(err.Error(), "server.fqdn") {
		t.Fatalf("error = %v, want one naming the file and key", err)
	}
}

func TestLoadLayersDuplicateKeyIsError(t *testing.T) {
	path := layeredFixture(t, "server:\n  port: 8080\n", map[string]string{
		"10-dup.yml": "server:\n  fqdn: a.example\n  fqdn: b.example\n",
	})
	_, _, err := loadLayers(path, true)
	if err == nil || !strings.Contains(err.Error(), "yaml.d/10-dup.yml") {
		t.Fatalf("error = %v, want a duplicate key error naming the file", err)
	}
}

func TestLoadLayersInvalidDropinSkippedWhenNotStrict(t *testing.T) {
	path := layeredFixture(t, "server:\n  fqdn: base.example\n", map[string]string{
		"10-bad.yml":  "server: [unterminated\n",
		"20-good.yml": "server:\n  address: 127.0.0.1\n",
	})
	merged, report, err := loadLayers(path, false)
	if err != nil {
		t.Fatalf("loadLayers: %v", err)
	}
	var cfg AppConfig
	if err := applyLayers(&cfg, merged); err != nil {
		t.Fatalf("applyLayers: %v", err)
	}
	if cfg.Server.Address != "127.0.0.1" || cfg.Server.FQDN != "base.example" {
		t.Errorf("cfg = %+v, want good drop-in applied and base kept", cfg.Server)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "10-bad.yml") {
		t.Errorf("warnings = %v, want one naming 10-bad.yml", report.Warnings)
	}

	if _, _, err := loadLayers(path, true); err == nil {
		t.Error("strict load must fail on an invalid drop-in")
	}
}

func TestListDropinFilesRefusesWritableAndEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	dropins := filepath.Join(dir, DropinDirName)
	writeLayerFile(t, filepath.Join(dropins, "10-ok.yml"), "server:\n  port: 1\n", 0o600)
	writeLayerFile(t, filepath.Join(dropins, "20-open.yml"), "server:\n  port: 2\n", 0o600)
	if err := os.Chmod(filepath.Join(dropins, "20-open.yml"), 0o666); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	outside := filepath.Join(dir, "outside.yml")
	writeLayerFile(t, outside, "server:\n  port: 3\n", 0o600)
	if err := os.Symlink(outside, filepath.Join(dropins, "30-escape.yml")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	writeLayerFile(t, filepath.Join(dropins, "real.yml"), "server:\n  port: 4\n", 0o600)
	if err := os.Symlink(filepath.Join(dropins, "real.yml"), filepath.Join(dropins, "40-inside.yml")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	files, warnings := listDropinFiles(dropins)
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	if got := strings.Join(names, ","); got != "10-ok.yml,40-inside.yml,real.yml" {
		t.Errorf("files = %q, want the safe ones only", got)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want 2 (writable file, escaping symlink)", warnings)
	}
}

func TestSaveConfigLeavesDropinOwnedKeysOutOfServerYML(t *testing.T) {
	path := layeredFixture(t, "server:\n  port: 8080\n", map[string]string{
		"10-x.yml": "server:\n  fqdn: owned.example\n",
	})
	t.Setenv("CONFIG_DIR", filepath.Dir(path))

	cfg, err := LoadConfigStrict()
	if err != nil {
		t.Fatalf("LoadConfigStrict: %v", err)
	}
	if cfg.Server.FQDN != "owned.example" {
		t.Fatalf("fqdn = %q, want the drop-in value", cfg.Server.FQDN)
	}
	cfg.Server.Port = 9091
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(saved)
	if strings.Contains(text, "owned.example") {
		t.Errorf("server.yml copied a drop-in-owned value:\n%s", text)
	}
	if !strings.Contains(text, "9091") {
		t.Errorf("server.yml lost the panel-edited port:\n%s", text)
	}
	if DropinSource("server.fqdn") != "yaml.d/10-x.yml" {
		t.Errorf("DropinSource(server.fqdn) = %q", DropinSource("server.fqdn"))
	}
	if DropinSource("server.port") != "" {
		t.Errorf("DropinSource(server.port) = %q, want empty", DropinSource("server.port"))
	}
}
