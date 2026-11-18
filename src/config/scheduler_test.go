package config

import (
	"testing"
	"time"
)

func TestSchedulerTaskDefsCoverRequiredTasks(t *testing.T) {
	required := []string{
		"ssl_renewal", "geoip_update", "blocklist_update", "cve_update", "update_check",
		"session_cleanup", "token_cleanup", "log_rotation", "backup_daily", "backup_hourly",
		"healthcheck_self", "tor_health", "i2p_health", "cluster_heartbeat",
	}
	for _, name := range required {
		if _, ok := SchedulerTaskDefFor(name); !ok {
			t.Errorf("required task %q has no definition", name)
		}
	}
	if _, ok := SchedulerTaskDefFor("no_such_task"); ok {
		t.Error("unknown task must not resolve")
	}
}

func TestSchedulerTaskDefsAreUniqueWithSchedules(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range SchedulerTaskDefs() {
		if seen[def.Name] {
			t.Errorf("duplicate task %q", def.Name)
		}
		seen[def.Name] = true
		if def.Defaults.Schedule == "" {
			t.Errorf("task %q has no default schedule", def.Name)
		}
		if def.Critical && !def.Defaults.Enabled {
			t.Errorf("critical task %q must default to enabled", def.Name)
		}
	}
}

func TestIsGlobalSchedulerTask(t *testing.T) {
	if !IsGlobalSchedulerTask("ssl_renewal") {
		t.Error("ssl_renewal is a global task")
	}
	if IsGlobalSchedulerTask("session_cleanup") {
		t.Error("session_cleanup is a local task")
	}
	if IsGlobalSchedulerTask("no_such_task") {
		t.Error("unknown task is not global")
	}
}

func TestDefaultSchedulerConfig(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	if cfg.Timezone != DefaultSchedulerTimezone || cfg.CatchUpWindow != DefaultSchedulerCatchUpWindow {
		t.Errorf("defaults = %q / %q", cfg.Timezone, cfg.CatchUpWindow)
	}
	if len(cfg.Tasks) != len(SchedulerTaskDefs()) {
		t.Errorf("tasks = %d, want %d", len(cfg.Tasks), len(SchedulerTaskDefs()))
	}
	if cfg.Tasks["backup_hourly"].Enabled {
		t.Error("backup_hourly must default to disabled")
	}
}

func TestSchedulerTaskSettings(t *testing.T) {
	cfg := SchedulerConfig{Tasks: map[string]SchedulerTask{
		"ssl_renewal":  {Enabled: false, Schedule: ""},
		"geoip_update": {Enabled: false, Schedule: "0 1 * * *"},
	}}

	critical := cfg.SchedulerTaskSettings("ssl_renewal")
	if !critical.Enabled {
		t.Error("a critical task is always enabled")
	}
	if critical.Schedule != "0 3 * * *" {
		t.Errorf("empty schedule must fall back to default, got %q", critical.Schedule)
	}

	skippable := cfg.SchedulerTaskSettings("geoip_update")
	if skippable.Enabled || skippable.Schedule != "0 1 * * *" {
		t.Errorf("configured values must win, got %+v", skippable)
	}

	missing := cfg.SchedulerTaskSettings("cve_update")
	if !missing.Enabled || missing.Schedule != "0 5 * * *" {
		t.Errorf("unconfigured task must use defaults, got %+v", missing)
	}
}

func TestSchedulerLocationAndCatchUp(t *testing.T) {
	cfg := SchedulerConfig{Timezone: "UTC", CatchUpWindow: "2h30m"}
	if cfg.SchedulerLocation().String() != "UTC" {
		t.Errorf("location = %s", cfg.SchedulerLocation())
	}
	if cfg.SchedulerCatchUp() != 2*time.Hour+30*time.Minute {
		t.Errorf("catch-up = %v", cfg.SchedulerCatchUp())
	}

	bad := SchedulerConfig{Timezone: "Not/AZone", CatchUpWindow: "banana"}
	if bad.SchedulerLocation().String() != DefaultSchedulerTimezone {
		t.Errorf("invalid timezone must fall back, got %s", bad.SchedulerLocation())
	}
	if bad.SchedulerCatchUp() != time.Hour {
		t.Errorf("invalid catch-up must fall back to 1h, got %v", bad.SchedulerCatchUp())
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"1h", time.Hour, true},
		{"30m", 30 * time.Minute, true},
		{"2h30m", 2*time.Hour + 30*time.Minute, true},
		{"30d", 30 * 24 * time.Hour, true},
		{"1w", 7 * 24 * time.Hour, true},
		{"1d12h", 36 * time.Hour, true},
		{" 5m ", 5 * time.Minute, true},
		{"", 0, false},
		{"h", 0, false},
		{"10", 0, false},
		{"10x", 0, false},
	}
	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseDuration(%q) err = %v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidateServerSchedulerRepairsValues(t *testing.T) {
	cfg := &AppConfig{}
	cfg.Server.Scheduler = SchedulerConfig{
		Timezone:      "Not/AZone",
		CatchUpWindow: "banana",
		Tasks: map[string]SchedulerTask{
			"ssl_renewal":      {Enabled: false},
			"log_rotation":     {Enabled: true, Schedule: "0 1 * * *", MaxAge: "bogus", MaxSize: ""},
			"blocklist_update": {Enabled: true, Schedule: "0 4 * * *", RetryDelay: "nonsense"},
		},
	}
	validateServerScheduler(cfg)
	sched := cfg.Server.Scheduler

	if sched.Timezone != DefaultSchedulerTimezone || sched.CatchUpWindow != DefaultSchedulerCatchUpWindow {
		t.Errorf("invalid values must be replaced, got %q / %q", sched.Timezone, sched.CatchUpWindow)
	}
	if !sched.Tasks["ssl_renewal"].Enabled || sched.Tasks["ssl_renewal"].Schedule == "" {
		t.Errorf("critical task must be enabled with a schedule, got %+v", sched.Tasks["ssl_renewal"])
	}
	if got := sched.Tasks["log_rotation"]; got.MaxAge != "30d" || got.MaxSize != "100MB" {
		t.Errorf("log_rotation not repaired: %+v", got)
	}
	if sched.Tasks["blocklist_update"].RetryDelay != "1h" {
		t.Errorf("retry_delay not repaired: %+v", sched.Tasks["blocklist_update"])
	}
	if len(sched.Tasks) != len(SchedulerTaskDefs()) {
		t.Errorf("every built-in task must be present, got %d", len(sched.Tasks))
	}
}

func TestValidateServerSchedulerEmptyUsesDefaults(t *testing.T) {
	cfg := &AppConfig{}
	validateServerScheduler(cfg)
	if cfg.Server.Scheduler.Timezone != DefaultSchedulerTimezone {
		t.Errorf("timezone = %q", cfg.Server.Scheduler.Timezone)
	}
	if cfg.Server.Scheduler.CatchUpWindow != DefaultSchedulerCatchUpWindow {
		t.Errorf("catch_up_window = %q", cfg.Server.Scheduler.CatchUpWindow)
	}
}

func TestValidByteSize(t *testing.T) {
	for _, v := range []string{"100MB", "1gb", "5KB", "2TB"} {
		if !validByteSize(v) {
			t.Errorf("validByteSize(%q) = false", v)
		}
	}
	for _, v := range []string{"", "MB", "0MB", "10", "1.5GB", "tenMB", "100B"} {
		if validByteSize(v) {
			t.Errorf("validByteSize(%q) = true", v)
		}
	}
}

func withIsolatedSchedulerConfig(t *testing.T) {
	t.Helper()
	t.Setenv("CONFIG_DIR", t.TempDir())
	previous := GetGlobalConfig()
	cfg := &AppConfig{}
	validateServerScheduler(cfg)
	SetGlobalConfig(cfg)
	t.Cleanup(func() { SetGlobalConfig(previous) })
}

func TestUpdateSchedulerSettings(t *testing.T) {
	withIsolatedSchedulerConfig(t)

	if errs := UpdateSchedulerSettings("Not/AZone", "banana"); len(errs) != 2 {
		t.Fatalf("expected two validation errors, got %v", errs)
	}
	if GetGlobalConfig().Server.Scheduler.Timezone != DefaultSchedulerTimezone {
		t.Error("an invalid update must not change the live config")
	}

	if errs := UpdateSchedulerSettings("UTC", "2h"); len(errs) != 0 {
		t.Fatalf("valid update rejected: %v", errs)
	}
	got := GetGlobalConfig().Server.Scheduler
	if got.Timezone != "UTC" || got.CatchUpWindow != "2h" {
		t.Errorf("update not applied: %+v", got)
	}
}

func TestUpdateSchedulerSettingsWithoutGlobalConfig(t *testing.T) {
	previous := GetGlobalConfig()
	SetGlobalConfig(nil)
	t.Cleanup(func() { SetGlobalConfig(previous) })
	if errs := UpdateSchedulerSettings("UTC", "1h"); len(errs) != 1 {
		t.Errorf("expected one error, got %v", errs)
	}
}

func TestUpdateSchedulerTask(t *testing.T) {
	withIsolatedSchedulerConfig(t)
	accept := func(s string) bool { return s != "bad" }

	if errs := UpdateSchedulerTask("no_such_task", SchedulerTask{}, accept); len(errs) != 1 || errs[0].Field != "task" {
		t.Errorf("unknown task must be rejected, got %v", errs)
	}
	if errs := UpdateSchedulerTask("geoip_update", SchedulerTask{Enabled: true, Schedule: "bad"}, accept); len(errs) != 1 || errs[0].Field != "schedule" {
		t.Errorf("bad schedule must be rejected, got %v", errs)
	}
	if errs := UpdateSchedulerTask("log_rotation", SchedulerTask{Schedule: "0 0 * * *", MaxAge: "zzz", MaxSize: "big"}, accept); len(errs) != 2 {
		t.Errorf("bad max_age and max_size must be rejected, got %v", errs)
	}

	if errs := UpdateSchedulerTask("geoip_update", SchedulerTask{Enabled: false, Schedule: "0 1 * * 1"}, accept); len(errs) != 0 {
		t.Fatalf("valid update rejected: %v", errs)
	}
	if got := GetGlobalConfig().Server.Scheduler.Tasks["geoip_update"]; got.Enabled || got.Schedule != "0 1 * * 1" {
		t.Errorf("update not applied: %+v", got)
	}

	if errs := UpdateSchedulerTask("ssl_renewal", SchedulerTask{Enabled: false, Schedule: "0 4 * * *"}, accept); len(errs) != 0 {
		t.Fatalf("critical task update rejected: %v", errs)
	}
	if !GetGlobalConfig().Server.Scheduler.Tasks["ssl_renewal"].Enabled {
		t.Error("a critical task can never be disabled")
	}
}
