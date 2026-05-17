package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/casapps/wthr/src/server/middleware"
	"github.com/casapps/wthr/src/server/model"
	"github.com/gin-gonic/gin"
)

// SchedulerConfig represents the complete scheduler configuration
type SchedulerConfig struct {
	Timezone       string                 `json:"timezone"`
	CatchUpWindow  string                 `json:"catch_up_window"`
	Tasks          SchedulerTasks         `json:"tasks"`
}

// SchedulerTasks contains configuration for all scheduler tasks
type SchedulerTasks struct {
	SSLRenewal      TaskConfigBasic    `json:"ssl_renewal"`
	GeoIPUpdate     TaskConfigBasic    `json:"geoip_update"`
	BlocklistUpdate TaskConfigRetry    `json:"blocklist_update"`
	CVEUpdate       TaskConfigRetry    `json:"cve_update"`
	SessionCleanup  TaskConfigBasic    `json:"session_cleanup"`
	TokenCleanup    TaskConfigBasic    `json:"token_cleanup"`
	LogRotation     TaskConfigLogRot   `json:"log_rotation"`
	BackupAuto      TaskConfigBackup   `json:"backup_auto"`
	HealthcheckSelf TaskConfigBasic    `json:"healthcheck_self"`
	TorHealth       TaskConfigTorHealth `json:"tor_health"`
}

// TaskConfigBasic is for tasks with schedule and enabled only
type TaskConfigBasic struct {
	Schedule string `json:"schedule"`
	Enabled  bool   `json:"enabled"`
}

// TaskConfigRetry is for tasks with retry capabilities
type TaskConfigRetry struct {
	Schedule     string `json:"schedule"`
	Enabled      bool   `json:"enabled"`
	RetryOnFail  bool   `json:"retry_on_fail"`
	RetryDelay   string `json:"retry_delay"`
}

// TaskConfigLogRot is for log rotation task
type TaskConfigLogRot struct {
	Schedule string `json:"schedule"`
	Enabled  bool   `json:"enabled"`
	MaxAge   string `json:"max_age"`
	MaxSize  string `json:"max_size"`
	Compress bool   `json:"compress"`
}

// TaskConfigBackup is for backup task
type TaskConfigBackup struct {
	Schedule  string `json:"schedule"`
	Enabled   bool   `json:"enabled"`
	KeepCount int    `json:"keep_count"`
}

// TaskConfigTorHealth is for Tor health check task
type TaskConfigTorHealth struct {
	Schedule      string `json:"schedule"`
	Enabled       bool   `json:"enabled"`
	RestartOnFail bool   `json:"restart_on_fail"`
}

// ShowSchedulerConfig displays the scheduler configuration page
func (h *AdminHandler) ShowSchedulerConfig(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/auth/login")
		return
	}

	settingsModel := &models.SettingsModel{DB: h.DB}

	// Load scheduler configuration from settings
	config := SchedulerConfig{
		Timezone:      settingsModel.GetString("scheduler.timezone", "America/New_York"),
		CatchUpWindow: settingsModel.GetString("scheduler.catch_up_window", "1h"),
		Tasks: SchedulerTasks{
			SSLRenewal: TaskConfigBasic{
				Schedule: settingsModel.GetString("scheduler.tasks.ssl_renewal.schedule", "0 3 * * *"),
				// Always enabled (critical)
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
			BackupAuto: TaskConfigBackup{
				Schedule:  settingsModel.GetString("scheduler.tasks.backup_auto.schedule", "0 1 * * *"),
				Enabled:   settingsModel.GetBool("scheduler.tasks.backup_auto.enabled", false),
				KeepCount: settingsModel.GetInt("scheduler.tasks.backup_auto.keep_count", 4),
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

	c.HTML(http.StatusOK, "admin/admin_scheduler.tmpl", gin.H{
		"title":  "Scheduler Configuration",
		"user":   user,
		"config": config,
	})
}

// SaveSchedulerConfig saves the scheduler configuration
func (h *AdminHandler) SaveSchedulerConfig(c *gin.Context) {
	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	settingsModel := &models.SettingsModel{DB: h.DB}

	// Validate and save global settings
	if timezone, ok := config["timezone"].(string); ok {
		if !isValidTimezone(timezone) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid timezone"})
			return
		}
		if err := settingsModel.SetString("scheduler.timezone", timezone); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save timezone"})
			return
		}
	}

	if catchUpWindow, ok := config["catch_up_window"].(string); ok {
		if !isValidDuration(catchUpWindow) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid catch-up window format. Use format like: 1h, 30m, 2h30m"})
			return
		}
		if err := settingsModel.SetString("scheduler.catch_up_window", catchUpWindow); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save catch-up window"})
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
		{"backup_auto", getTaskConfig(config, "backup_auto")},
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
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Invalid schedule for %s: %s", task.name, schedule),
				})
				return
			}
			if err := settingsModel.SetString(prefix+".schedule", schedule); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": fmt.Sprintf("Failed to save schedule for %s", task.name),
				})
				return
			}
		}

		// Save enabled state (ssl_renewal is always enabled)
		if task.name != "ssl_renewal" {
			if enabled, ok := task.config["enabled"].(bool); ok {
				if err := settingsModel.SetBool(prefix+".enabled", enabled); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": fmt.Sprintf("Failed to save enabled state for %s", task.name),
					})
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
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("Invalid retry_delay for %s", task.name),
					})
					return
				}
				settingsModel.SetString(prefix+".retry_delay", retryDelay)
			}

		case "log_rotation":
			if maxAge, ok := task.config["max_age"].(string); ok {
				if !isValidDuration(maxAge) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid max_age format"})
					return
				}
				settingsModel.SetString(prefix+".max_age", maxAge)
			}
			if maxSize, ok := task.config["max_size"].(string); ok {
				if !isValidSize(maxSize) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid max_size format"})
					return
				}
				settingsModel.SetString(prefix+".max_size", maxSize)
			}
			if compress, ok := task.config["compress"].(bool); ok {
				settingsModel.SetBool(prefix+".compress", compress)
			}

		case "backup_auto":
			if keepCount, ok := task.config["keep_count"].(float64); ok {
				settingsModel.SetInt(prefix+".keep_count", int(keepCount))
			} else if keepCountStr, ok := task.config["keep_count"].(string); ok {
				if count, err := strconv.Atoi(keepCountStr); err == nil {
					settingsModel.SetInt(prefix+".keep_count", count)
				}
			}

		case "tor_health":
			if restartOnFail, ok := task.config["restart_on_fail"].(bool); ok {
				settingsModel.SetBool(prefix+".restart_on_fail", restartOnFail)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Scheduler configuration saved successfully",
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
		"America/New_York":   true,
		"America/Chicago":    true,
		"America/Denver":     true,
		"America/Los_Angeles": true,
		"UTC":                true,
		"Europe/London":      true,
		"Europe/Paris":       true,
		"Asia/Tokyo":         true,
		"Asia/Shanghai":      true,
		"Australia/Sydney":   true,
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
func (h *AdminHandler) GetSchedulerConfigJSON(c *gin.Context) {
	settingsModel := &models.SettingsModel{DB: h.DB}

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
			BackupAuto: TaskConfigBackup{
				Schedule:  settingsModel.GetString("scheduler.tasks.backup_auto.schedule", "0 1 * * *"),
				Enabled:   settingsModel.GetBool("scheduler.tasks.backup_auto.enabled", false),
				KeepCount: settingsModel.GetInt("scheduler.tasks.backup_auto.keep_count", 4),
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize configuration"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, string(jsonData))
}
