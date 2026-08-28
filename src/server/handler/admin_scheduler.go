package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// SchedulerConfig represents the complete scheduler configuration
type SchedulerConfig struct {
	Timezone      string         `json:"timezone"`
	CatchUpWindow string         `json:"catch_up_window"`
	Tasks         SchedulerTasks `json:"tasks"`
}

// SchedulerTasks contains configuration for all scheduler tasks
type SchedulerTasks struct {
	SSLRenewal      TaskConfigBasic     `json:"ssl_renewal"`
	GeoIPUpdate     TaskConfigBasic     `json:"geoip_update"`
	BlocklistUpdate TaskConfigRetry     `json:"blocklist_update"`
	CVEUpdate       TaskConfigRetry     `json:"cve_update"`
	SessionCleanup  TaskConfigBasic     `json:"session_cleanup"`
	TokenCleanup    TaskConfigBasic     `json:"token_cleanup"`
	LogRotation     TaskConfigLogRot    `json:"log_rotation"`
	HealthcheckSelf TaskConfigBasic     `json:"healthcheck_self"`
	TorHealth       TaskConfigTorHealth `json:"tor_health"`
}

// TaskConfigBasic is for tasks with schedule and enabled only
type TaskConfigBasic struct {
	Schedule string `json:"schedule"`
	Enabled  bool   `json:"enabled"`
}

// TaskConfigRetry is for tasks with retry capabilities
type TaskConfigRetry struct {
	Schedule    string `json:"schedule"`
	Enabled     bool   `json:"enabled"`
	RetryOnFail bool   `json:"retry_on_fail"`
	RetryDelay  string `json:"retry_delay"`
}

// TaskConfigLogRot is for log rotation task
type TaskConfigLogRot struct {
	Schedule string `json:"schedule"`
	Enabled  bool   `json:"enabled"`
	MaxAge   string `json:"max_age"`
	MaxSize  string `json:"max_size"`
	Compress bool   `json:"compress"`
}

// TaskConfigTorHealth is for Tor health check task
type TaskConfigTorHealth struct {
	Schedule      string `json:"schedule"`
	Enabled       bool   `json:"enabled"`
	RestartOnFail bool   `json:"restart_on_fail"`
}

// ShowSchedulerConfig displays the scheduler configuration page
func (h *AdminHandler) ShowSchedulerConfig(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}

	settingsModel := &model.SettingsModel{DB: database.GetServerDB()}

	// Load scheduler configuration from settings
	config := SchedulerConfig{
		Timezone:      settingsModel.GetString("scheduler.timezone", "America/New_York"),
		CatchUpWindow: settingsModel.GetString("scheduler.catch_up_window", "1h"),
		Tasks: SchedulerTasks{
			SSLRenewal: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.ssl_renewal.schedule", "0 3 * * *"),
				// Always enabled (critical)
				Enabled: true,
			},
			GeoIPUpdate: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.geoip_update.schedule", "0 3 * * 0"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.geoip_update.enabled", true),
			},
			BlocklistUpdate: TaskConfigRetry{
				Schedule:    settingsModel.GetString("scheduler.tasks.blocklist_update.schedule", "0 4 * * *"),
				Enabled:     settingsModel.GetBool("scheduler.tasks.blocklist_update.enabled", true),
				RetryOnFail: settingsModel.GetBool("scheduler.tasks.blocklist_update.retry_on_fail", true),
				RetryDelay:  settingsModel.GetString("scheduler.tasks.blocklist_update.retry_delay", "1h"),
			},
			CVEUpdate: TaskConfigRetry{
				Schedule:    settingsModel.GetString("scheduler.tasks.cve_update.schedule", "0 5 * * *"),
				Enabled:     settingsModel.GetBool("scheduler.tasks.cve_update.enabled", true),
				RetryOnFail: settingsModel.GetBool("scheduler.tasks.cve_update.retry_on_fail", true),
				RetryDelay:  settingsModel.GetString("scheduler.tasks.cve_update.retry_delay", "1h"),
			},
			SessionCleanup: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.session_cleanup.schedule", "@hourly"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.session_cleanup.enabled", true),
			},
			TokenCleanup: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.token_cleanup.schedule", "0 6 * * *"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.token_cleanup.enabled", true),
			},
			LogRotation: TaskConfigLogRot{
				Schedule: settingsModel.GetString("scheduler.tasks.log_rotation.schedule", "0 0 * * *"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.log_rotation.enabled", true),
				MaxAge:   settingsModel.GetString("scheduler.tasks.log_rotation.max_age", "30d"),
				MaxSize:  settingsModel.GetString("scheduler.tasks.log_rotation.max_size", "100MB"),
				Compress: settingsModel.GetBool("scheduler.tasks.log_rotation.compress", true),
			},
			HealthcheckSelf: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.healthcheck_self.schedule", "@every 5m"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.healthcheck_self.enabled", true),
			},
			TorHealth: TaskConfigTorHealth{
				Schedule:      settingsModel.GetString("scheduler.tasks.tor_health.schedule", "@every 10m"),
				Enabled:       settingsModel.GetBool("scheduler.tasks.tor_health.enabled", true),
				RestartOnFail: settingsModel.GetBool("scheduler.tasks.tor_health.restart_on_fail", true),
			},
		},
	}

	themeServerCtx, _ := middleware.GetServerContext(r.Context())

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_scheduler.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":  Translate(r, "admin.scheduler.scheduler_configuration"),
		"user":   user,
		"config": config,
		"server": themeServerCtx,
	}))
}

// SaveSchedulerConfig saves the scheduler configuration
func (h *AdminHandler) SaveSchedulerConfig(w http.ResponseWriter, r *http.Request) {
	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_request_body")+": "+err.Error())
		return
	}

	settingsModel := &model.SettingsModel{DB: database.GetServerDB()}

	// Validate and save global settings
	if timezone, ok := config["timezone"].(string); ok {
		if !isValidTimezone(timezone) {
			BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_timezone"))
			return
		}
		if err := settingsModel.SetString("scheduler.timezone", timezone); err != nil {
			InternalError(w, r, Translate(r, "errors.admin.scheduler.failed_to_save_timezone"))
			return
		}
	}

	if catchUpWindow, ok := config["catch_up_window"].(string); ok {
		if !isValidDuration(catchUpWindow) {
			BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_catch_up_window_format_use_format_like"))
			return
		}
		if err := settingsModel.SetString("scheduler.catch_up_window", catchUpWindow); err != nil {
			InternalError(w, r, Translate(r, "errors.admin.scheduler.failed_to_save_catch_up_window"))
			return
		}
	}

	// Process task configurations
	tasks := []struct {
		name   string
		config map[string]interface{}
	}{
		{"ssl_renewal", getTaskConfig(config, "ssl_renewal")},
		{"geoip_update", getTaskConfig(config, "geoip_update")},
		{"blocklist_update", getTaskConfig(config, "blocklist_update")},
		{"cve_update", getTaskConfig(config, "cve_update")},
		{"session_cleanup", getTaskConfig(config, "session_cleanup")},
		{"token_cleanup", getTaskConfig(config, "token_cleanup")},
		{"log_rotation", getTaskConfig(config, "log_rotation")},
		{"healthcheck_self", getTaskConfig(config, "healthcheck_self")},
		{"tor_health", getTaskConfig(config, "tor_health")},
	}

	for _, task := range tasks {
		if task.config == nil {
			continue
		}

		prefix := fmt.Sprintf("scheduler.tasks.%s", task.name)

		// Validate and save schedule
		if schedule, ok := task.config["schedule"].(string); ok {
			if !isValidCronOrSpecial(schedule) {
				BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_schedule_for")+fmt.Sprintf(": %s: %s", task.name, schedule))
				return
			}
			if err := settingsModel.SetString(prefix+".schedule", schedule); err != nil {
				InternalError(w, r, Translate(r, "errors.admin.scheduler.failed_to_save_schedule_for")+": "+task.name)
				return
			}
		}

		// Save enabled state (ssl_renewal is always enabled)
		if task.name != "ssl_renewal" {
			if enabled, ok := task.config["enabled"].(bool); ok {
				if err := settingsModel.SetBool(prefix+".enabled", enabled); err != nil {
					InternalError(w, r, Translate(r, "errors.admin.scheduler.failed_to_save_enabled_state_for")+": "+task.name)
					return
				}
			}
		}

		// Task-specific fields
		switch task.name {
		case "blocklist_update", "cve_update":
			if retryOnFail, ok := task.config["retry_on_fail"].(bool); ok {
				settingsModel.SetBool(prefix+".retry_on_fail", retryOnFail)
			}
			if retryDelay, ok := task.config["retry_delay"].(string); ok {
				if !isValidDuration(retryDelay) {
					BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_retry_delay_for")+": "+task.name)
					return
				}
				settingsModel.SetString(prefix+".retry_delay", retryDelay)
			}

		case "log_rotation":
			if maxAge, ok := task.config["max_age"].(string); ok {
				if !isValidDuration(maxAge) {
					BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_max_age_format"))
					return
				}
				settingsModel.SetString(prefix+".max_age", maxAge)
			}
			if maxSize, ok := task.config["max_size"].(string); ok {
				if !isValidSize(maxSize) {
					BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_max_size_format"))
					return
				}
				settingsModel.SetString(prefix+".max_size", maxSize)
			}
			if compress, ok := task.config["compress"].(bool); ok {
				settingsModel.SetBool(prefix+".compress", compress)
			}

		case "tor_health":
			if restartOnFail, ok := task.config["restart_on_fail"].(bool); ok {
				settingsModel.SetBool(prefix+".restart_on_fail", restartOnFail)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.scheduler.scheduler_configuration_saved_successfully"),
	})
}

// Helper functions

func getTaskConfig(config map[string]interface{}, taskName string) map[string]interface{} {
	if taskConfig, ok := config[taskName].(map[string]interface{}); ok {
		return taskConfig
	}
	return nil
}

func isValidTimezone(tz string) bool {
	validTimezones := map[string]bool{
		"America/New_York":    true,
		"America/Chicago":     true,
		"America/Denver":      true,
		"America/Los_Angeles": true,
		"UTC":                 true,
		"Europe/London":       true,
		"Europe/Paris":        true,
		"Asia/Tokyo":          true,
		"Asia/Shanghai":       true,
		"Australia/Sydney":    true,
	}
	return validTimezones[tz]
}

func isValidDuration(duration string) bool {
	// Valid formats: 1h, 30m, 2h30m, 1d, 30d, etc.
	pattern := `^(\d+[smhd])+$`
	matched, _ := regexp.MatchString(pattern, duration)
	return matched
}

func isValidSize(size string) bool {
	// Valid formats: 100MB, 1GB, 500KB, etc.
	pattern := `^\d+(KB|MB|GB|TB)$`
	matched, _ := regexp.MatchString(pattern, strings.ToUpper(size))
	return matched
}

func isValidCronOrSpecial(schedule string) bool {
	// Special schedules
	if strings.HasPrefix(schedule, "@") {
		special := []string{"@hourly", "@daily", "@weekly", "@monthly", "@yearly"}
		for _, s := range special {
			if schedule == s {
				return true
			}
		}
		// @every format
		if strings.HasPrefix(schedule, "@every ") {
			duration := strings.TrimPrefix(schedule, "@every ")
			return isValidDuration(duration)
		}
		return false
	}

	// Cron format: minute hour day month weekday
	// Simple validation (basic 5-field format)
	parts := strings.Fields(schedule)
	if len(parts) != 5 {
		return false
	}

	// Each field can be: number, *, */number, range, list
	for _, part := range parts {
		if !isValidCronField(part) {
			return false
		}
	}

	return true
}

func isValidCronField(field string) bool {
	// Allow: *, numbers, ranges (1-5), steps (*/5), lists (1,2,3)
	if field == "*" {
		return true
	}

	// Step values: */5
	if strings.HasPrefix(field, "*/") {
		num := strings.TrimPrefix(field, "*/")
		_, err := strconv.Atoi(num)
		return err == nil
	}

	// Ranges: 1-5
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return false
		}
		_, err1 := strconv.Atoi(parts[0])
		_, err2 := strconv.Atoi(parts[1])
		return err1 == nil && err2 == nil
	}

	// Lists: 1,2,3
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				return false
			}
		}
		return true
	}

	// Single number
	_, err := strconv.Atoi(field)
	return err == nil
}

// GetSchedulerConfigJSON returns scheduler configuration as JSON for API access
func (h *AdminHandler) GetSchedulerConfigJSON(w http.ResponseWriter, r *http.Request) {
	settingsModel := &model.SettingsModel{DB: database.GetServerDB()}

	config := SchedulerConfig{
		Timezone:      settingsModel.GetString("scheduler.timezone", "America/New_York"),
		CatchUpWindow: settingsModel.GetString("scheduler.catch_up_window", "1h"),
		Tasks: SchedulerTasks{
			SSLRenewal: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.ssl_renewal.schedule", "0 3 * * *"),
				Enabled:  true,
			},
			GeoIPUpdate: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.geoip_update.schedule", "0 3 * * 0"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.geoip_update.enabled", true),
			},
			BlocklistUpdate: TaskConfigRetry{
				Schedule:    settingsModel.GetString("scheduler.tasks.blocklist_update.schedule", "0 4 * * *"),
				Enabled:     settingsModel.GetBool("scheduler.tasks.blocklist_update.enabled", true),
				RetryOnFail: settingsModel.GetBool("scheduler.tasks.blocklist_update.retry_on_fail", true),
				RetryDelay:  settingsModel.GetString("scheduler.tasks.blocklist_update.retry_delay", "1h"),
			},
			CVEUpdate: TaskConfigRetry{
				Schedule:    settingsModel.GetString("scheduler.tasks.cve_update.schedule", "0 5 * * *"),
				Enabled:     settingsModel.GetBool("scheduler.tasks.cve_update.enabled", true),
				RetryOnFail: settingsModel.GetBool("scheduler.tasks.cve_update.retry_on_fail", true),
				RetryDelay:  settingsModel.GetString("scheduler.tasks.cve_update.retry_delay", "1h"),
			},
			SessionCleanup: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.session_cleanup.schedule", "@hourly"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.session_cleanup.enabled", true),
			},
			TokenCleanup: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.token_cleanup.schedule", "0 6 * * *"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.token_cleanup.enabled", true),
			},
			LogRotation: TaskConfigLogRot{
				Schedule: settingsModel.GetString("scheduler.tasks.log_rotation.schedule", "0 0 * * *"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.log_rotation.enabled", true),
				MaxAge:   settingsModel.GetString("scheduler.tasks.log_rotation.max_age", "30d"),
				MaxSize:  settingsModel.GetString("scheduler.tasks.log_rotation.max_size", "100MB"),
				Compress: settingsModel.GetBool("scheduler.tasks.log_rotation.compress", true),
			},
			HealthcheckSelf: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.healthcheck_self.schedule", "@every 5m"),
				Enabled:  settingsModel.GetBool("scheduler.tasks.healthcheck_self.enabled", true),
			},
			TorHealth: TaskConfigTorHealth{
				Schedule:      settingsModel.GetString("scheduler.tasks.tor_health.schedule", "@every 10m"),
				Enabled:       settingsModel.GetBool("scheduler.tasks.tor_health.enabled", true),
				RestartOnFail: settingsModel.GetBool("scheduler.tasks.tor_health.restart_on_fail", true),
			},
		},
	}

	// Convert to JSON for pretty output
	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.scheduler.failed_to_serialize_configuration"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jsonData)
}
