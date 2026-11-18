package config

import (
	"strings"
	"time"
)

// This file holds the server.scheduler settings tree defined by AI.md PART 19.

// DefaultSchedulerTimezone is the AI.md PART 19 default timezone for scheduled tasks.
const DefaultSchedulerTimezone = "America/New_York"

// DefaultSchedulerCatchUpWindow is the AI.md PART 19 default catch_up_window.
const DefaultSchedulerCatchUpWindow = "1h"

// SchedulerConfig represents the server.scheduler settings per AI.md PART 19.
// The scheduler itself is always running, so there is deliberately no
// enabled switch for it; only individual tasks can be disabled.
type SchedulerConfig struct {
	// IANA timezone that cron schedules are evaluated in
	Timezone string `yaml:"timezone"`
	// Missed runs inside this window are executed at startup
	CatchUpWindow string                   `yaml:"catch_up_window"`
	Tasks         map[string]SchedulerTask `yaml:"tasks"`
}

// SchedulerTask represents one scheduled task per AI.md PART 19.
type SchedulerTask struct {
	Enabled bool `yaml:"enabled"`
	// Cron expression, @hourly/@daily/@weekly/@monthly, or @every {duration}
	Schedule    string `yaml:"schedule"`
	RetryOnFail bool   `yaml:"retry_on_fail"`
	// Delay before the first retry, for example 1h
	RetryDelay string `yaml:"retry_delay"`
	// Oldest log kept by log_rotation, for example 30d
	MaxAge string `yaml:"max_age"`
	// Largest log kept before rotation, for example 100MB
	MaxSize string `yaml:"max_size"`
	// Compress rotated logs (log_rotation)
	Compress bool `yaml:"compress"`
	// Verify a backup after creating it (backup_daily)
	Verify bool `yaml:"verify"`
	// Restart the service when its health check fails (tor_health, i2p_health)
	RestartOnFail bool `yaml:"restart_on_fail"`
}

// SchedulerTaskDef describes one built-in scheduled task: its stable id, the
// default settings it ships with and how it behaves in a cluster.
type SchedulerTaskDef struct {
	Name string
	// Critical tasks can never be disabled (AI.md PART 19 "Skippable: No")
	Critical bool
	// Global tasks run on one node in cluster mode; local tasks run on every node
	Global   bool
	Defaults SchedulerTask
}

// SchedulerTaskDefs lists every built-in task with its AI.md PART 19 defaults,
// followed by the project's own weather and notification tasks.
func SchedulerTaskDefs() []SchedulerTaskDef {
	return []SchedulerTaskDef{
		{Name: "ssl_renewal", Critical: true, Global: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 3 * * *"}},
		{Name: "geoip_update", Global: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 3 * * 0"}},
		{Name: "blocklist_update", Global: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 4 * * *", RetryOnFail: true, RetryDelay: "1h"}},
		{Name: "cve_update", Global: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 5 * * *", RetryOnFail: true, RetryDelay: "1h"}},
		{Name: "update_check", Global: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 6 * * *"}},
		{Name: "session_cleanup", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "@every 15m"}},
		{Name: "token_cleanup", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "@every 15m"}},
		{Name: "log_rotation", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 0 * * *", MaxAge: "30d", MaxSize: "100MB", Compress: true}},
		{Name: "backup_daily", Global: true, Defaults: SchedulerTask{Enabled: true, Schedule: "0 2 * * *", Verify: true}},
		{Name: "backup_hourly", Global: true, Defaults: SchedulerTask{Enabled: false, Schedule: "@hourly"}},
		{Name: "healthcheck_self", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "@every 5m"}},
		{Name: "tor_health", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "@every 10m", RestartOnFail: true}},
		{Name: "i2p_health", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "@every 10m", RestartOnFail: true}},
		{Name: "cluster_heartbeat", Critical: true, Defaults: SchedulerTask{Enabled: true, Schedule: "@every 30s"}},
		{Name: "rate_limit_cleanup", Defaults: SchedulerTask{Enabled: true, Schedule: "@hourly"}},
		{Name: "audit_log_cleanup", Defaults: SchedulerTask{Enabled: true, Schedule: "@daily"}},
		{Name: "security_report_cleanup", Defaults: SchedulerTask{Enabled: true, Schedule: "@daily"}},
		{Name: "weather_alert_check", Defaults: SchedulerTask{Enabled: true, Schedule: "@every 5m"}},
		{Name: "weather_alert_expire", Defaults: SchedulerTask{Enabled: true, Schedule: "@every 5m"}},
		{Name: "weather_daily_forecast", Defaults: SchedulerTask{Enabled: true, Schedule: "0 7 * * *"}},
		{Name: "weather_cache_refresh", Defaults: SchedulerTask{Enabled: true, Schedule: "@every 15m"}},
		{Name: "earthquake_poll", Defaults: SchedulerTask{Enabled: true, Schedule: "@every 1m"}},
		{Name: "hurricane_poll", Defaults: SchedulerTask{Enabled: true, Schedule: "@every 15m"}},
		{Name: "notification_queue", Defaults: SchedulerTask{Enabled: true, Schedule: "@every 2m"}},
		{Name: "notification_delivery_cleanup", Defaults: SchedulerTask{Enabled: true, Schedule: "@daily"}},
		{Name: "notification_cleanup", Defaults: SchedulerTask{Enabled: true, Schedule: "0 2 * * *"}},
		{Name: "notification_limit_enforcement", Defaults: SchedulerTask{Enabled: true, Schedule: "0 3 * * *"}},
	}
}

// SchedulerTaskDefFor returns the built-in definition for a task id.
func SchedulerTaskDefFor(name string) (SchedulerTaskDef, bool) {
	for _, def := range SchedulerTaskDefs() {
		if def.Name == name {
			return def, true
		}
	}
	return SchedulerTaskDef{}, false
}

// IsGlobalSchedulerTask reports whether a task runs on one node only in cluster mode.
func IsGlobalSchedulerTask(name string) bool {
	def, ok := SchedulerTaskDefFor(name)
	return ok && def.Global
}

// DefaultSchedulerConfig returns the AI.md PART 19 scheduler defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	tasks := make(map[string]SchedulerTask)
	for _, def := range SchedulerTaskDefs() {
		tasks[def.Name] = def.Defaults
	}
	return SchedulerConfig{
		Timezone:      DefaultSchedulerTimezone,
		CatchUpWindow: DefaultSchedulerCatchUpWindow,
		Tasks:         tasks,
	}
}

// SchedulerTaskSettings returns the effective settings of one task: the
// configured values when present, otherwise the built-in defaults.
func (c *SchedulerConfig) SchedulerTaskSettings(name string) SchedulerTask {
	def, known := SchedulerTaskDefFor(name)
	task, configured := c.Tasks[name]
	if !configured {
		return def.Defaults
	}
	if strings.TrimSpace(task.Schedule) == "" {
		task.Schedule = def.Defaults.Schedule
	}
	if known && def.Critical {
		task.Enabled = true
	}
	return task
}

// SchedulerLocation resolves the configured timezone, falling back to the default.
func (c *SchedulerConfig) SchedulerLocation() *time.Location {
	if loc, err := time.LoadLocation(strings.TrimSpace(c.Timezone)); err == nil {
		return loc
	}
	if loc, err := time.LoadLocation(DefaultSchedulerTimezone); err == nil {
		return loc
	}
	return time.UTC
}

// SchedulerCatchUp resolves the configured catch-up window, falling back to the default.
func (c *SchedulerConfig) SchedulerCatchUp() time.Duration {
	if d, err := ParseDuration(c.CatchUpWindow); err == nil && d >= 0 {
		return d
	}
	d, _ := ParseDuration(DefaultSchedulerCatchUpWindow)
	return d
}

// validateServerScheduler replaces an invalid scheduler value with its default
// per AI.md PART 12 and adds every built-in task that the file omits. A
// critical task is always enabled.
func validateServerScheduler(cfg *AppConfig) {
	sched := &cfg.Server.Scheduler

	if strings.TrimSpace(sched.Timezone) == "" {
		sched.Timezone = DefaultSchedulerTimezone
	} else if _, err := time.LoadLocation(strings.TrimSpace(sched.Timezone)); err != nil {
		warnConfig("invalid scheduler.timezone %q, using default %q", sched.Timezone, DefaultSchedulerTimezone)
		sched.Timezone = DefaultSchedulerTimezone
	}

	if strings.TrimSpace(sched.CatchUpWindow) == "" {
		sched.CatchUpWindow = DefaultSchedulerCatchUpWindow
	} else if d, err := ParseDuration(sched.CatchUpWindow); err != nil || d < 0 {
		warnConfig("invalid scheduler.catch_up_window %q, using default %q", sched.CatchUpWindow, DefaultSchedulerCatchUpWindow)
		sched.CatchUpWindow = DefaultSchedulerCatchUpWindow
	}

	if sched.Tasks == nil {
		sched.Tasks = make(map[string]SchedulerTask)
	}
	for _, def := range SchedulerTaskDefs() {
		task, present := sched.Tasks[def.Name]
		if !present {
			sched.Tasks[def.Name] = def.Defaults
			continue
		}
		if strings.TrimSpace(task.Schedule) == "" {
			task.Schedule = def.Defaults.Schedule
		}
		if def.Critical && !task.Enabled {
			warnConfig("scheduler.tasks.%s cannot be disabled, enabling it", def.Name)
			task.Enabled = true
		}
		for _, field := range []struct {
			name  string
			value *string
			def   string
		}{
			{"retry_delay", &task.RetryDelay, def.Defaults.RetryDelay},
			{"max_age", &task.MaxAge, def.Defaults.MaxAge},
		} {
			if strings.TrimSpace(*field.value) == "" {
				*field.value = field.def
			} else if _, err := ParseDuration(*field.value); err != nil {
				warnConfig("invalid scheduler.tasks.%s.%s %q, using default %q", def.Name, field.name, *field.value, field.def)
				*field.value = field.def
			}
		}
		if strings.TrimSpace(task.MaxSize) == "" {
			task.MaxSize = def.Defaults.MaxSize
		}
		sched.Tasks[def.Name] = task
	}
}

// ParseDuration parses a duration that may use the d (day) and w (week)
// suffixes in addition to the units time.ParseDuration accepts, so values
// such as 30d or 2h30m are valid in server.yml.
func ParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, &time.ParseError{Layout: "duration", Value: value, Message: ": empty duration"}
	}
	var total time.Duration
	rest := value
	for rest != "" {
		end := 0
		for end < len(rest) && (rest[end] >= '0' && rest[end] <= '9' || rest[end] == '.') {
			end++
		}
		unitEnd := end
		for unitEnd < len(rest) && !(rest[unitEnd] >= '0' && rest[unitEnd] <= '9' || rest[unitEnd] == '.') {
			unitEnd++
		}
		if end == 0 || unitEnd == end {
			return 0, &time.ParseError{Layout: "duration", Value: value, Message: ": invalid duration"}
		}
		number, unit := rest[:end], rest[end:unitEnd]
		switch unit {
		case "d":
			n, err := time.ParseDuration(number + "h")
			if err != nil {
				return 0, err
			}
			total += n * 24
		case "w":
			n, err := time.ParseDuration(number + "h")
			if err != nil {
				return 0, err
			}
			total += n * 24 * 7
		default:
			n, err := time.ParseDuration(number + unit)
			if err != nil {
				return 0, err
			}
			total += n
		}
		rest = rest[unitEnd:]
	}
	return total, nil
}

// UpdateSchedulerSettings validates and persists the scheduler timezone and
// catch-up window (AI.md PART 19). An invalid value is rejected with a
// ValidationError and nothing is written.
func UpdateSchedulerSettings(timezone, catchUpWindow string) []ValidationError {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return []ValidationError{{Field: "scheduler", Message: "errors.scheduler.config_unavailable"}}
	}

	timezone = strings.TrimSpace(timezone)
	catchUpWindow = strings.TrimSpace(catchUpWindow)
	var errs []ValidationError
	if _, err := time.LoadLocation(timezone); err != nil || timezone == "" {
		errs = append(errs, ValidationError{Field: "timezone", Message: "errors.scheduler.invalid_timezone"})
	}
	if d, err := ParseDuration(catchUpWindow); err != nil || d < 0 {
		errs = append(errs, ValidationError{Field: "catch_up_window", Message: "errors.scheduler.invalid_catch_up_window"})
	}
	if len(errs) > 0 {
		return errs
	}

	updated := *cfg
	updated.Server.Scheduler = cloneSchedulerConfig(cfg.Server.Scheduler)
	updated.Server.Scheduler.Timezone = timezone
	updated.Server.Scheduler.CatchUpWindow = catchUpWindow
	if err := SaveConfig(&updated); err != nil {
		return []ValidationError{{Field: "scheduler", Message: err.Error()}}
	}
	SetGlobalConfig(&updated)
	return nil
}

// UpdateSchedulerTask validates and persists one task's settings (AI.md PART 19).
// validSchedule reports whether a schedule expression can run; a critical task
// can never be disabled. Nothing is written when any field is invalid.
func UpdateSchedulerTask(name string, next SchedulerTask, validSchedule func(string) bool) []ValidationError {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return []ValidationError{{Field: "scheduler", Message: "errors.scheduler.config_unavailable"}}
	}
	def, known := SchedulerTaskDefFor(name)
	if !known {
		return []ValidationError{{Field: "task", Message: "errors.scheduler.unknown_task"}}
	}

	next.Schedule = strings.TrimSpace(next.Schedule)
	next.RetryDelay = strings.TrimSpace(next.RetryDelay)
	next.MaxAge = strings.TrimSpace(next.MaxAge)
	next.MaxSize = strings.TrimSpace(next.MaxSize)

	var errs []ValidationError
	if next.Schedule == "" || (validSchedule != nil && !validSchedule(next.Schedule)) {
		errs = append(errs, ValidationError{Field: "schedule", Message: "errors.scheduler.invalid_schedule"})
	}
	if def.Critical {
		next.Enabled = true
	}
	for _, field := range []struct {
		name  string
		value string
		used  bool
	}{
		{"retry_delay", next.RetryDelay, def.Defaults.RetryDelay != ""},
		{"max_age", next.MaxAge, def.Defaults.MaxAge != ""},
	} {
		if !field.used {
			continue
		}
		if d, err := ParseDuration(field.value); err != nil || d <= 0 {
			errs = append(errs, ValidationError{Field: field.name, Message: "errors.scheduler.invalid_duration"})
		}
	}
	if def.Defaults.MaxSize != "" && !validByteSize(next.MaxSize) {
		errs = append(errs, ValidationError{Field: "max_size", Message: "errors.scheduler.invalid_size"})
	}
	if len(errs) > 0 {
		return errs
	}

	updated := *cfg
	updated.Server.Scheduler = cloneSchedulerConfig(cfg.Server.Scheduler)
	updated.Server.Scheduler.Tasks[name] = next
	if err := SaveConfig(&updated); err != nil {
		return []ValidationError{{Field: "scheduler", Message: err.Error()}}
	}
	SetGlobalConfig(&updated)
	return nil
}

// cloneSchedulerConfig copies the task map so an update never mutates the live config.
func cloneSchedulerConfig(in SchedulerConfig) SchedulerConfig {
	out := in
	out.Tasks = make(map[string]SchedulerTask, len(in.Tasks))
	for name, task := range in.Tasks {
		out.Tasks[name] = task
	}
	return out
}

// validByteSize reports whether value is a positive size with a KB, MB, GB or TB suffix.
func validByteSize(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		if strings.HasSuffix(value, suffix) {
			digits := strings.TrimSuffix(value, suffix)
			if digits == "" {
				return false
			}
			for _, r := range digits {
				if r < '0' || r > '9' {
					return false
				}
			}
			return digits != "0"
		}
	}
	return false
}
