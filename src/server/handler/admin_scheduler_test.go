package handler

import (
	"net/http"
	"strings"
	"testing"

	models "github.com/webappsgo/wthr/src/server/model"
)

// newSchedulerTestHandler wires an AdminHandler against a fresh in-memory
// server DB. SettingsModel.Get/Set (used by the scheduler handlers) read and
// write through database.GetServerDB() rather than the injected DB field
// (Delete/List/ListByPrefix use the injected field instead — an
// inconsistency in the model itself), so the global dual-DB must be wired.
func newSchedulerTestHandler(t *testing.T) *AdminHandler {
	t.Helper()
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	return &AdminHandler{DB: serverDB}
}

// TestAdminHandlerShowSchedulerConfig_Unauthenticated verifies the
// not-logged-in guard clause redirects before any HTML render is attempted,
// avoiding the HTMLRender-panic issue documented elsewhere in this package.
func TestAdminHandlerShowSchedulerConfig_Unauthenticated(t *testing.T) {
	h := newSchedulerTestHandler(t)
	c, w := newAPITestContext("/server/admin/config/scheduler")

	h.ShowSchedulerConfig(c)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", w.Code, w.Body.String())
	}
}

func TestAdminHandlerGetSchedulerConfigJSON(t *testing.T) {
	h := newSchedulerTestHandler(t)
	c, w := newAPITestContext("/server/admin/config/scheduler/json")

	h.GetSchedulerConfigJSON(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "\"timezone\": \"America/New_York\"") {
		t.Errorf("expected default timezone in JSON output, got: %s", w.Body.String())
	}
}

func TestAdminHandlerSaveSchedulerConfig(t *testing.T) {
	t.Run("malformed body", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", "not json")
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid timezone", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"timezone": "Mars/Olympus",
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid catch_up_window", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"catch_up_window": "not-a-duration",
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid task schedule", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"geoip_update": map[string]interface{}{"schedule": "not a cron", "enabled": true},
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid retry_delay", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"blocklist_update": map[string]interface{}{"retry_delay": "bogus"},
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid log_rotation max_age", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"log_rotation": map[string]interface{}{"max_age": "bogus"},
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid log_rotation max_size", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"log_rotation": map[string]interface{}{"max_size": "bogus"},
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid full config persists all task fields", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"timezone":        "UTC",
			"catch_up_window": "30m",
			"ssl_renewal":     map[string]interface{}{"schedule": "0 3 * * *", "enabled": false},
			"geoip_update":    map[string]interface{}{"schedule": "@weekly", "enabled": false},
			"blocklist_update": map[string]interface{}{
				"schedule": "0 4 * * *", "enabled": true,
				"retry_on_fail": true, "retry_delay": "1h",
			},
			"log_rotation": map[string]interface{}{
				"schedule": "@daily", "enabled": true,
				"max_age": "30d", "max_size": "100MB", "compress": true,
			},
			"backup_auto": map[string]interface{}{
				"schedule": "@every 1h", "enabled": true, "keep_count": "7",
			},
			"tor_health": map[string]interface{}{
				"schedule": "@every 10m", "enabled": true, "restart_on_fail": false,
			},
		})

		h.SaveSchedulerConfig(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}

		settingsModel := &models.SettingsModel{DB: h.DB}
		if got := settingsModel.GetString("scheduler.timezone", ""); got != "UTC" {
			t.Errorf("timezone = %q, want UTC", got)
		}
		if got := settingsModel.GetString("scheduler.catch_up_window", ""); got != "30m" {
			t.Errorf("catch_up_window = %q, want 30m", got)
		}
		// ssl_renewal.enabled is never written (always-enabled task), even
		// though the request included enabled:false.
		if got := settingsModel.GetString("scheduler.tasks.ssl_renewal.schedule", ""); got != "0 3 * * *" {
			t.Errorf("ssl_renewal.schedule = %q, want 0 3 * * *", got)
		}
		if got := settingsModel.GetBool("scheduler.tasks.geoip_update.enabled", true); got != false {
			t.Errorf("geoip_update.enabled = %v, want false", got)
		}
		if got := settingsModel.GetString("scheduler.tasks.blocklist_update.retry_delay", ""); got != "1h" {
			t.Errorf("blocklist_update.retry_delay = %q, want 1h", got)
		}
		if got := settingsModel.GetInt("scheduler.tasks.backup_auto.keep_count", 0); got != 7 {
			t.Errorf("backup_auto.keep_count = %d, want 7 (string branch)", got)
		}
		if got := settingsModel.GetBool("scheduler.tasks.tor_health.restart_on_fail", true); got != false {
			t.Errorf("tor_health.restart_on_fail = %v, want false", got)
		}
		if got := settingsModel.GetBool("scheduler.tasks.log_rotation.compress", false); got != true {
			t.Errorf("log_rotation.compress = %v, want true", got)
		}
	})

	t.Run("unknown task keys are ignored", func(t *testing.T) {
		h := newSchedulerTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/scheduler", map[string]interface{}{
			"nonexistent_field": "value",
		})
		h.SaveSchedulerConfig(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestGetTaskConfig(t *testing.T) {
	config := map[string]interface{}{
		"geoip_update": map[string]interface{}{"schedule": "@weekly"},
		"not_a_map":    "value",
	}

	if got := getTaskConfig(config, "geoip_update"); got == nil {
		t.Errorf("expected non-nil map for existing key")
	}
	if got := getTaskConfig(config, "not_a_map"); got != nil {
		t.Errorf("expected nil for non-map value, got %v", got)
	}
	if got := getTaskConfig(config, "missing_key"); got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

func TestIsValidTimezone(t *testing.T) {
	tests := []struct {
		tz   string
		want bool
	}{
		{"UTC", true},
		{"America/New_York", true},
		{"Asia/Tokyo", true},
		{"Mars/Olympus", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isValidTimezone(tt.tz); got != tt.want {
			t.Errorf("isValidTimezone(%q) = %v, want %v", tt.tz, got, tt.want)
		}
	}
}

func TestIsValidDuration(t *testing.T) {
	tests := []struct {
		d    string
		want bool
	}{
		{"1h", true},
		{"30m", true},
		{"2h30m", true},
		{"1d", true},
		{"", false},
		{"1 hour", false},
		{"abc", false},
	}
	for _, tt := range tests {
		if got := isValidDuration(tt.d); got != tt.want {
			t.Errorf("isValidDuration(%q) = %v, want %v", tt.d, got, tt.want)
		}
	}
}

func TestIsValidSize(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"100MB", true},
		{"1GB", true},
		{"500KB", true},
		{"2tb", true},
		{"100", false},
		{"MB", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isValidSize(tt.s); got != tt.want {
			t.Errorf("isValidSize(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestIsValidCronOrSpecial(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"@hourly", true},
		{"@daily", true},
		{"@weekly", true},
		{"@monthly", true},
		{"@yearly", true},
		{"@every 5m", true},
		{"@every bogus", false},
		{"@bogus", false},
		{"0 3 * * *", true},
		{"*/5 * * * *", true},
		{"1-5 * * * *", true},
		{"1,2,3 * * * *", true},
		{"* * * *", false},
		{"a b c d e", false},
	}
	for _, tt := range tests {
		if got := isValidCronOrSpecial(tt.s); got != tt.want {
			t.Errorf("isValidCronOrSpecial(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestIsValidCronField(t *testing.T) {
	tests := []struct {
		f    string
		want bool
	}{
		{"*", true},
		{"*/5", true},
		{"*/bogus", false},
		{"1-5", true},
		{"1-", false},
		{"1,2,3", true},
		{"1,x,3", false},
		{"7", true},
		{"x", false},
	}
	for _, tt := range tests {
		if got := isValidCronField(tt.f); got != tt.want {
			t.Errorf("isValidCronField(%q) = %v, want %v", tt.f, got, tt.want)
		}
	}
}
