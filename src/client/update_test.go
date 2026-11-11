package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestVersionLessThan(t *testing.T) {
	tests := []struct {
		current  string
		required string
		want     bool
	}{
		{"1.2.0", "1.2.1", true},
		{"v1.2.1", "1.2.0", false},
		{"1.2.0-beta.1", "1.2.0", true},
		{"dev", "1.2.0", false},
		{"not-semver", "1.2.0", false},
	}
	for _, test := range tests {
		if got := versionLessThan(test.current, test.required); got != test.want {
			t.Errorf("versionLessThan(%q, %q) = %v, want %v", test.current, test.required, got, test.want)
		}
	}
}

func TestValidateCLIUpdateMode(t *testing.T) {
	tests := []struct {
		name        string
		auto        bool
		interactive bool
		wantError   bool
	}{
		{name: "interactive update can be explicit", interactive: true},
		{name: "non-interactive auto update", auto: true},
		{name: "non-interactive update requires auto", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCLIUpdateMode(test.auto, test.interactive)
			if (err != nil) != test.wantError {
				t.Fatalf("validateCLIUpdateMode(%t, %t) error = %v, want error %t", test.auto, test.interactive, err, test.wantError)
			}
		})
	}
}

func TestNormalizeClientUpdateArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "bare update defaults to yes", args: []string{"--update"}, want: []string{"--update=yes"}},
		{name: "bare update before another flag", args: []string{"--update", "--server", "https://example.test"}, want: []string{"--update=yes", "--server", "https://example.test"}},
		{name: "explicit update mode", args: []string{"--update", "check"}, want: []string{"--update", "check"}},
		{name: "explicit update yes", args: []string{"--update", "yes"}, want: []string{"--update", "yes"}},
		{name: "command after bare update", args: []string{"--update", "current"}, want: []string{"--update=yes", "current"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeClientUpdateArgs(test.args)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("normalizeClientUpdateArgs(%q) = %q, want %q", test.args, got, test.want)
			}
		})
	}
}

func TestCLIUpdateLanguageUsesConfiguredLanguageAndFallsBack(t *testing.T) {
	previous := cliUpdateLanguage
	t.Cleanup(func() { cliUpdateLanguage = previous })
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	setCLIUpdateLanguage("es")
	if got := cliUpdateText("cli.update.client_current", map[string]string{"version": "1.2.3"}); got != "CLI está actualizado (1.2.3)." {
		t.Errorf("Spanish update text = %q", got)
	}
	setCLIUpdateLanguage("unsupported")
	if got := cliUpdateText("cli.update.client_current", map[string]string{"version": "1.2.3"}); got != "CLI is up to date (1.2.3)." {
		t.Errorf("fallback update text = %q", got)
	}
}

func TestDiscoverCLIUpdateCachesServerMetadataAndEnforcesMinimum(t *testing.T) {
	withTempHome(t)
	oldVersion := Version
	Version = "1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/autodiscover" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(autodiscoverResponse{
			Primary:       "https://ignored.example",
			Cluster:       []string{"https://node.example"},
			APIVersion:    "v2",
			CLIMinVersion: "1.0.1",
			CLIVersions: map[string]cliBinaryVersion{
				runtime.GOOS + "-" + runtime.GOARCH: {Version: "1.1.0", SHA256: "aabb"},
			},
		})
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Server.Primary = server.URL
	info, err := discoverCLIUpdate(config, "test-profile")
	if err != nil {
		t.Fatalf("discoverCLIUpdate() error = %v", err)
	}
	if info.Available != "1.1.0" || !info.MinimumRequired || info.Minimum != "1.0.1" {
		t.Fatalf("update info = %+v", info)
	}
	if config.Server.APIVersion != "v2" || len(config.Server.Cluster) != 1 || config.Server.Cluster[0] != "https://node.example" {
		t.Fatalf("discovered server config not applied: %+v", config.Server)
	}
	profilePath := filepath.Join(CLIConfigDir(), "test-profile.yml")
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("discovered profile was not saved: %v", err)
	}
}

func TestNormalizeConfigProfileRejectsPaths(t *testing.T) {
	for _, profile := range []string{"../outside", `dir\\profile`, "bad\x00name"} {
		if _, err := normalizeConfigProfile(profile); err == nil {
			t.Errorf("normalizeConfigProfile(%q) error = nil", profile)
		}
	}
}

func TestConfigProfilesRejectSymbolicLinks(t *testing.T) {
	withTempHome(t)
	configDir := CLIConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("auth:\n  token: secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(configDir, "linked.yml")
	if err := os.Symlink(outside, profile); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if _, err := LoadConfigFromProfile("linked"); err == nil {
		t.Fatal("LoadConfigFromProfile accepted a symbolic link")
	}
	if err := SaveConfigToProfile(DefaultConfig(), "linked"); err == nil {
		t.Fatal("SaveConfigToProfile accepted a symbolic link")
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "auth:\n  token: secret\n" {
		t.Fatalf("outside config was changed: %q", data)
	}
}
