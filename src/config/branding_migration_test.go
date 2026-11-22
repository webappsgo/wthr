package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMigrateBrandingAreaPreservesAreaFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server.yml")
	original := []byte("server:\n  port: 8100\n  branding:\n    title: Legacy title\n    description: Legacy description\n  seo:\n    canonical: https://legacy.example\n    title_separator: ' | '\nweb:\n  ui:\n    theme: light\n    accent: '#123456'\n")
	if err := os.WriteFile(serverPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	brandingDir := filepath.Join(dir, DropinDirName)
	if err := os.Mkdir(brandingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	existing := []byte("server:\n  branding:\n    title: Area title\n  seo:\n    canonical: https://area.example\n")
	if err := os.WriteFile(filepath.Join(brandingDir, "branding.yml"), existing, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigrateBrandingArea(serverPath); err != nil {
		t.Fatalf("MigrateBrandingArea() error = %v", err)
	}
	serverAfter, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(serverAfter, []byte("port: 8100")) || strings.Contains(string(serverAfter), "Legacy title") || strings.Contains(string(serverAfter), "theme: light") {
		t.Errorf("server.yml after migration = %s", serverAfter)
	}
	if !strings.Contains(string(serverAfter), "Settings of this node only") {
		t.Errorf("server.yml is missing the required header: %s", serverAfter)
	}

	areaData, err := os.ReadFile(filepath.Join(brandingDir, "branding.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var area map[string]interface{}
	if err := yaml.Unmarshal(areaData, &area); err != nil {
		t.Fatalf("parse migrated branding.yml: %v", err)
	}
	var merged AppConfig
	if err := yaml.Unmarshal(areaData, &merged); err != nil {
		t.Fatalf("decode migrated branding.yml: %v", err)
	}
	if merged.Server.Branding.Title != "Area title" || merged.Server.Branding.Description != "Legacy description" {
		t.Errorf("branding = %+v, want existing title and migrated description", merged.Server.Branding)
	}
	if merged.Server.SEO.Canonical != "https://area.example" || merged.Server.SEO.TitleSeparator != " | " {
		t.Errorf("SEO = %+v, want existing canonical and migrated separator", merged.Server.SEO)
	}
	if merged.Web.UI.Theme != "light" || merged.Web.UI.Accent != "#123456" {
		t.Errorf("UI = %+v, want migrated theme and accent", merged.Web.UI)
	}

	backup, err := os.ReadFile(serverPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, original) {
		t.Error("server.yml.bak does not preserve the original file")
	}
	for path, want := range map[string]os.FileMode{serverPath: 0o600, serverPath + ".bak": 0o600, filepath.Join(brandingDir, "branding.yml"): 0o600, brandingDir: 0o700} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode(%s) = %04o, want %04o", path, got, want)
		}
	}
}

func TestMigrateBrandingAreaIsIdempotentAndSkipsUnrelatedConfig(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(serverPath, []byte("server:\n  port: 8100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateBrandingArea(serverPath); err != nil {
		t.Fatalf("MigrateBrandingArea() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, DropinDirName)); !os.IsNotExist(err) {
		t.Fatalf("migration created an unrelated branding directory: %v", err)
	}

	if err := os.WriteFile(serverPath, []byte("server:\n  branding:\n    title: Legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, DropinDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	brandingPath := filepath.Join(dir, DropinDirName, "branding.yml")
	if err := os.WriteFile(brandingPath, []byte("server:\n  branding:\n    title: Area\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateBrandingArea(serverPath); err != nil {
		t.Fatalf("MigrateBrandingArea() second case error = %v", err)
	}
	first, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateBrandingArea(serverPath); err != nil {
		t.Fatalf("MigrateBrandingArea() repeated error = %v", err)
	}
	second, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("repeated migration changed server.yml")
	}
}
