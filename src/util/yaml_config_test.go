package util

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSplitPath covers dot-notation splitting, including empty segments and
// no-dot paths.
func TestSplitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"simple", "server.port", []string{"server", "port"}},
		{"single_key", "port", []string{"port"}},
		{"three_levels", "server.users.enabled", []string{"server", "users", "enabled"}},
		{"empty", "", nil},
		{"leading_dot", ".port", []string{"port"}},
		{"trailing_dot", "port.", []string{"port"}},
		{"consecutive_dots", "a..b", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPath(tt.path)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestSetNestedValue covers setting a top-level key, creating intermediate
// maps, overwriting an existing scalar, and the error path where an
// intermediate path segment is not a map.
func TestSetNestedValue(t *testing.T) {
	t.Run("top_level", func(t *testing.T) {
		m := map[string]interface{}{}
		if err := setNestedValue(m, "port", 8080); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["port"] != 8080 {
			t.Errorf("m[port] = %v, want 8080", m["port"])
		}
	})

	t.Run("nested_creates_intermediate_maps", func(t *testing.T) {
		m := map[string]interface{}{}
		if err := setNestedValue(m, "server.users.enabled", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		server, ok := m["server"].(map[string]interface{})
		if !ok {
			t.Fatalf("m[server] not a map: %#v", m["server"])
		}
		users, ok := server["users"].(map[string]interface{})
		if !ok {
			t.Fatalf("server[users] not a map: %#v", server["users"])
		}
		if users["enabled"] != true {
			t.Errorf("users[enabled] = %v, want true", users["enabled"])
		}
	})

	t.Run("overwrite_existing_scalar", func(t *testing.T) {
		m := map[string]interface{}{"port": 80}
		if err := setNestedValue(m, "port", 443); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["port"] != 443 {
			t.Errorf("m[port] = %v, want 443", m["port"])
		}
	})

	t.Run("empty_path_errors", func(t *testing.T) {
		m := map[string]interface{}{}
		if err := setNestedValue(m, "", "x"); err == nil {
			t.Error("setNestedValue with empty path: err = nil, want error")
		}
	})

	t.Run("intermediate_not_a_map_errors", func(t *testing.T) {
		m := map[string]interface{}{"server": "not-a-map"}
		if err := setNestedValue(m, "server.port", 80); err == nil {
			t.Error("setNestedValue with non-map intermediate: err = nil, want error")
		}
	})
}

// TestUpdateYAMLConfig covers a full read-modify-write roundtrip on a real
// temp file, plus the missing-file error path.
func TestUpdateYAMLConfig(t *testing.T) {
	t.Run("updates_existing_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.yml")
		initial := "server:\n  port: 80\n  mode: production\n"
		if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
			t.Fatalf("setup WriteFile: %v", err)
		}

		err := UpdateYAMLConfig(path, map[string]interface{}{
			"server.port":          443,
			"server.users.enabled": true,
		})
		if err != nil {
			t.Fatalf("UpdateYAMLConfig: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		if !contains(content, "port: 443") {
			t.Errorf("updated config missing 'port: 443', got:\n%s", content)
		}
		if !contains(content, "mode: production") {
			t.Errorf("updated config lost existing key 'mode: production', got:\n%s", content)
		}
		if !contains(content, "enabled: true") {
			t.Errorf("updated config missing new nested key 'enabled: true', got:\n%s", content)
		}
	})

	t.Run("missing_file_errors", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "does-not-exist.yml")
		if err := UpdateYAMLConfig(path, map[string]interface{}{"a": 1}); err == nil {
			t.Error("UpdateYAMLConfig on missing file: err = nil, want error")
		}
	})

	t.Run("invalid_yaml_errors", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yml")
		if err := os.WriteFile(path, []byte("not: valid: yaml: content:"), 0644); err != nil {
			t.Fatalf("setup WriteFile: %v", err)
		}
		if err := UpdateYAMLConfig(path, map[string]interface{}{"a": 1}); err == nil {
			t.Error("UpdateYAMLConfig on invalid YAML: err = nil, want error")
		}
	})
}
