package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSaveGeoIPAreaPreservesOtherSecuritySettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	yamlDir := filepath.Join(dir, "yaml.d")
	if err := os.MkdirAll(yamlDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(yamlDir, "security.yml")
	if err := os.WriteFile(path, []byte("server:\n  security:\n    allowlist:\n      - cidr: 10.0.0.0/8\n        description: office\n"), 0600); err != nil {
		t.Fatal(err)
	}
	want := GeoIPConfig{Enabled: true, DenyCountries: []string{"CN"}, Presets: map[string][]string{"custom": {"CN"}}}
	if err := SaveGeoIPArea(want); err != nil {
		t.Fatalf("SaveGeoIPArea: %v", err)
	}
	allowlist := []IPAllowlistEntry{{CIDR: "203.0.113.0/24", Description: "office"}}
	if err := SaveIPAllowlistArea(allowlist); err != nil {
		t.Fatalf("SaveIPAllowlistArea: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]interface{}
	if err := yaml.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	server := saved["server"].(map[string]interface{})
	if _, ok := server["security"]; !ok {
		t.Fatal("existing server.security configuration was removed")
	}
	var got struct {
		Server struct {
			GeoIP    GeoIPConfig    `yaml:"geoip"`
			Security SecurityConfig `yaml:"security"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Server.GeoIP.DenyCountries[0] != "CN" || got.Server.GeoIP.Presets["custom"][0] != "CN" {
		t.Fatalf("saved GeoIP settings = %#v", got.Server.GeoIP)
	}
	if len(got.Server.Security.Allowlist) != 1 || got.Server.Security.Allowlist[0] != allowlist[0] {
		t.Fatalf("saved allowlist = %#v, want %#v", got.Server.Security.Allowlist, allowlist)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("security.yml mode = %o, want 600", info.Mode().Perm())
	}
}
