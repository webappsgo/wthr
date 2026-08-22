package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	DB *sql.DB
}

// User Management APIs

func (h *AdminHandler) ListUsers(c *gin.Context) {
	userModel := &model.UserModel{DB: h.DB}
	users, err := userModel.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, users)
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
		Role     string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate username
	if err := util.ValidateUsername(req.Username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize username
	username := util.NormalizeUsername(req.Username)

	userModel := &model.UserModel{DB: h.DB}
	user, err := userModel.Create(username, req.Email, req.Password, req.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Prevent modifying your own account
	currentUser, ok := middleware.GetCurrentUser(c)
	if !ok || currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	if currentUser.ID == id {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify your own account credentials. Contact another administrator if you need to change your username, email, or role."})
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Role     string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate username
	if err := util.ValidateUsername(req.Username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize username
	username := util.NormalizeUsername(req.Username)

	userModel := &model.UserModel{DB: h.DB}
	if err := userModel.Update(id, username, req.Email, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Prevent deleting yourself
	currentUser, ok := middleware.GetCurrentUser(c)
	if !ok || currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	if currentUser.ID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete your own account"})
		return
	}

	userModel := &model.UserModel{DB: h.DB}
	if err := userModel.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

func (h *AdminHandler) UpdateUserPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req struct {
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userModel := &model.UserModel{DB: h.DB}
	if err := userModel.UpdatePassword(id, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}

// Settings Management APIs

func (h *AdminHandler) ListSettings(c *gin.Context) {
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT key, value, type, COALESCE(description, ''), updated_at FROM server_config ORDER BY key")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch settings"})
		return
	}
	defer rows.Close()

	settings := make([]map[string]interface{}, 0)
	for rows.Next() {
		var key, value, settingType, description string
		var updatedAt time.Time
		if err := rows.Scan(&key, &value, &settingType, &description, &updatedAt); err != nil {
			continue
		}
		settings = append(settings, map[string]interface{}{
			"key":         key,
			"value":       value,
			"type":        settingType,
			"description": description,
			"updated_at":  updatedAt,
		})
	}

	c.JSON(http.StatusOK, settings)
}

func (h *AdminHandler) GetSetting(c *gin.Context) {
	key := c.Param("key")
	var value, settingType, description string
	var updatedAt time.Time

	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value, type, COALESCE(description, ''), updated_at FROM server_config WHERE key = ?", key).
		Scan(&value, &settingType, &description, &updatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Setting not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch setting"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":         key,
		"value":       value,
		"type":        settingType,
		"description": description,
		"updated_at":  updatedAt,
	})
}

func (h *AdminHandler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Value string `json:"value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, "UPDATE server_config SET value = ?, updated_at = ? WHERE key = ?",
		req.Value, time.Now(), key)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update setting"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Setting updated successfully"})
}

// API Token Management APIs

func (h *AdminHandler) ListTokens(c *gin.Context) {
	userID := c.Query("user_id")

	var rows *sql.Rows
	var err error

	if userID != "" {
		uid, _ := strconv.Atoi(userID)
		tokenModel := &model.TokenModel{DB: h.DB}
		tokens, err := tokenModel.GetByUserID(uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tokens"})
			return
		}
		c.JSON(http.StatusOK, tokens)
		return
	}

	// Get all tokens for admin view - user_tokens lives in users.db (real UsersSchema)
	rows, err = database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutComplexSelect, `
		SELECT t.id, t.user_id, u.email, t.name, t.token_prefix, t.created_at, t.last_used_at, t.expires_at
		FROM user_tokens t
		LEFT JOIN user_accounts u ON t.user_id = u.id
		ORDER BY t.created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tokens"})
		return
	}
	defer rows.Close()

	// Timestamp columns are scanned as raw driver values and parsed with
	// dbtime rather than scanned straight into time.Time/sql.NullTime: rows
	// written as canonical UTC text and rows written by the driver's
	// local-zone time.Time.String() layout coexist on disk, and a direct
	// time.Time scan fails on whichever layout the driver does not recognise,
	// which previously dropped the whole token row from the listing.
	var tokens []map[string]interface{}
	for rows.Next() {
		var id, ownerID int
		var name, tokenPrefix string
		// email comes from a LEFT JOIN and is NULL when the owning account row
		// is missing, so it cannot be scanned into a plain string.
		var email sql.NullString
		var storedCreatedAt, storedLastUsedAt, storedExpiresAt interface{}

		if err := rows.Scan(&id, &ownerID, &email, &name, &tokenPrefix, &storedCreatedAt, &storedLastUsedAt, &storedExpiresAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tokens"})
			return
		}

		token := map[string]interface{}{
			"id":           id,
			"user_id":      ownerID,
			"user_email":   email.String,
			"name":         name,
			"token_prefix": tokenPrefix,
		}

		if createdAt, ok := dbtime.ParseStoredTimestamp(storedCreatedAt); ok {
			token["created_at"] = createdAt
		}
		if lastUsedAt, ok := dbtime.ParseStoredTimestamp(storedLastUsedAt); ok {
			token["last_used_at"] = lastUsedAt
		}
		if expiresAt, ok := dbtime.ParseStoredTimestamp(storedExpiresAt); ok {
			token["expires_at"] = expiresAt
		}

		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tokens"})
		return
	}

	c.JSON(http.StatusOK, tokens)
}

func (h *AdminHandler) GenerateToken(c *gin.Context) {
	var req struct {
		UserID int    `json:"user_id" binding:"required"`
		Name   string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokenModel := &model.TokenModel{DB: h.DB}
	token, err := tokenModel.Create(req.UserID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Token generated successfully. Save it now - it won't be shown again!",
		"token":   token,
	})
}

func (h *AdminHandler) RevokeToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid token ID"})
		return
	}

	tokenModel := &model.TokenModel{DB: h.DB}
	if err := tokenModel.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token revoked successfully"})
}

// Audit Log APIs

func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// server_audit_log's real columns are actor_id/resource_type/resource_id/timestamp
	// per src/database/server_schema.go, not user_id/resource/created_at.
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT a.id, a.actor_type, a.actor_id, a.action, a.resource_type, a.resource_id,
		       a.details, a.ip_address, a.user_agent, a.timestamp
		FROM server_audit_log a
		ORDER BY a.timestamp DESC
		LIMIT ?
	`, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit logs"})
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int
		var actorType, actorID, action, resourceType, resourceID, details, ipAddress, userAgent sql.NullString
		var timestamp time.Time

		if err := rows.Scan(&id, &actorType, &actorID, &action, &resourceType, &resourceID, &details, &ipAddress, &userAgent, &timestamp); err != nil {
			continue
		}

		log := map[string]interface{}{
			"id":            id,
			"action":        action.String,
			"resource_type": resourceType.String,
			"resource_id":   resourceID.String,
			"details":       details.String,
			"ip_address":    ipAddress.String,
			"user_agent":    userAgent.String,
			"created_at":    timestamp,
		}

		if actorID.Valid {
			log["actor_id"] = actorID.String
			log["actor_type"] = actorType.String
		}

		logs = append(logs, log)
	}

	c.JSON(http.StatusOK, logs)
}

func (h *AdminHandler) ClearAuditLogs(c *gin.Context) {
	// Default retention
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutBulk, "DELETE FROM server_audit_log WHERE timestamp < ?", cutoff)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear logs"})
		return
	}

	affected, _ := result.RowsAffected()
	c.JSON(http.StatusOK, gin.H{
		"message": "Audit logs cleared successfully",
		"deleted": affected,
	})
}

// GetLogsStats returns audit log statistics
func (h *AdminHandler) GetLogsStats(c *gin.Context) {
	var totalLogs, errorLogs, successLogs, recentLogs int64

	// server_audit_log's real columns are status/timestamp (server_schema.go),
	// not success/created_at. Scan errors must be propagated, not swallowed.
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_audit_log").Scan(&totalLogs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("count total audit logs: %w", err).Error()})
		return
	}

	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_audit_log WHERE status != 'success'").Scan(&errorLogs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("count error audit logs: %w", err).Error()})
		return
	}
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_audit_log WHERE status = 'success'").Scan(&successLogs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("count success audit logs: %w", err).Error()})
		return
	}

	// Get recent activity (last 24 hours).
	// The 24-hour cutoff is applied in Go: SQLite's datetime('now', '-1 day')
	// returns NULL for the local-zone layout the driver writes for a bound
	// time.Time, so those rows never counted as recent, and the function does
	// not exist on PostgreSQL or MySQL.
	recentRows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT timestamp
		FROM server_audit_log
		WHERE timestamp IS NOT NULL
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("count recent audit logs: %w", err).Error()})
		return
	}
	defer recentRows.Close()

	recentCutoff := time.Now().UTC().Add(-24 * time.Hour)

	for recentRows.Next() {
		var storedTimestamp interface{}
		if err := recentRows.Scan(&storedTimestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("count recent audit logs: %w", err).Error()})
			return
		}

		entryTime, ok := dbtime.ParseStoredTimestamp(storedTimestamp)
		if !ok || entryTime.Before(recentCutoff) {
			continue
		}

		recentLogs++
	}
	if err := recentRows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("count recent audit logs: %w", err).Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":      totalLogs,
		"errors":     errorLogs,
		"success":    successLogs,
		"recent_24h": recentLogs,
	})
}

// GetTasksStats returns scheduled task statistics
func (h *AdminHandler) GetTasksStats(c *gin.Context) {
	var totalTasks, enabledTasks, disabledTasks, failedTasks int64

	// Get total count
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state").Scan(&totalTasks); err != nil {
		log.Printf("ERROR: GetTasksStats: failed to count total tasks: %v", err)
	}

	// Get counts by status
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state WHERE enabled = 1").Scan(&enabledTasks); err != nil {
		log.Printf("ERROR: GetTasksStats: failed to count enabled tasks: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state WHERE enabled = 0").Scan(&disabledTasks); err != nil {
		log.Printf("ERROR: GetTasksStats: failed to count disabled tasks: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state WHERE last_status = 'failed'").Scan(&failedTasks); err != nil {
		log.Printf("ERROR: GetTasksStats: failed to count failed tasks: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    totalTasks,
		"enabled":  enabledTasks,
		"disabled": disabledTasks,
		"failed":   failedTasks,
	})
}

// System Stats APIs

func (h *AdminHandler) GetSystemStats(c *gin.Context) {
	userModel := &model.UserModel{DB: h.DB}

	totalUsers, _ := userModel.Count()
	adminCount, _ := userModel.CountByRole("admin")

	var totalLocations, totalTokens, totalSessions, totalNotifications int
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_saved_locations").Scan(&totalLocations); err != nil {
		log.Printf("ERROR: GetSystemStats: failed to count saved locations: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_tokens").Scan(&totalTokens); err != nil {
		log.Printf("ERROR: GetSystemStats: failed to count tokens: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_sessions").Scan(&totalSessions); err != nil {
		log.Printf("ERROR: GetSystemStats: failed to count sessions: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), database.GetUsersDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_notifications").Scan(&totalNotifications); err != nil {
		log.Printf("ERROR: GetSystemStats: failed to count notifications: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"users": gin.H{
			"total": totalUsers,
			"admin": adminCount,
			"user":  totalUsers - adminCount,
		},
		"locations":     totalLocations,
		"tokens":        totalTokens,
		"sessions":      totalSessions,
		"notifications": totalNotifications,
	})
}

// GetScheduledTasks returns status of all scheduled tasks
func (h *AdminHandler) GetScheduledTasks(c *gin.Context) {
	// Check if tasks are already seeded (real table is server_scheduler_state
	// per src/database/server_schema.go - there is no "scheduled_tasks" table)
	var count int
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state").Scan(&count); err != nil {
		log.Printf("ERROR: GetScheduledTasks: failed to count existing tasks: %v", err)
	}

	// If no tasks exist, seed them from the known scheduler tasks
	if count == 0 {
		h.seedScheduledTasks()
	}

	// Get scheduled tasks from database
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT task_name, schedule, enabled, last_run, next_run, last_status
		FROM server_scheduler_state
		ORDER BY task_name
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch scheduled tasks", "details": err.Error()})
		return
	}
	defer rows.Close()

	var tasks []map[string]interface{}
	for rows.Next() {
		var taskName, schedule, lastStatus sql.NullString
		var enabled bool
		var lastRun, nextRun sql.NullTime

		if err := rows.Scan(&taskName, &schedule, &enabled, &lastRun, &nextRun, &lastStatus); err != nil {
			continue
		}

		status := "disabled"
		if enabled {
			status = "success"
			if lastStatus.Valid && lastStatus.String != "" {
				status = lastStatus.String
			}
		}

		task := map[string]interface{}{
			"task_name": taskName.String,
			"name":      taskName.String,
			"status":    status,
			// Running state from scheduler (defaults to false)
			"running": false,
			"enabled": enabled,
		}

		if lastRun.Valid {
			task["last_run"] = lastRun.Time
		}
		if nextRun.Valid {
			task["next_run"] = nextRun.Time
		}

		// Use schedule as interval if available
		if schedule.Valid && schedule.String != "" {
			task["schedule"] = schedule.String
		}

		tasks = append(tasks, task)
	}

	// If no tasks in database after seeding, return empty array
	if len(tasks) == 0 {
		tasks = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, tasks)
}

// seedScheduledTasks seeds the database with default scheduled tasks
func (h *AdminHandler) seedScheduledTasks() {
	// Define all scheduled tasks matching main.go scheduler
	defaultTasks := []struct {
		Name     string
		Schedule string
		TaskType string
	}{
		{"rotate-logs", "Daily at midnight", "maintenance"},
		{"cleanup-sessions", "Every 1 hour", "cleanup"},
		{"cleanup-rate-limits", "Every 1 hour", "cleanup"},
		{"cleanup-audit-logs", "Every 24 hours", "cleanup"},
		{"check-weather-alerts", "Every 15 minutes", "weather"},
		{"daily-forecast", "Every 24 hours", "weather"},
		{"process-notification-queue", "Every 2 minutes", "notifications"},
		{"cleanup-notifications", "Every 24 hours", "cleanup"},
		{"system-backup", "Every 6 hours", "backup"},
		{"refresh-weather-cache", "Every 30 minutes", "weather"},
		{"update-geoip-database", "Every 7 days", "maintenance"},
	}

	for _, task := range defaultTasks {
		// server_scheduler_state's real primary key is task_id (TEXT), not an
		// autoincrement id, and there is no task_type column - use the task
		// name itself as the stable task_id.
		// next_run is bound as canonical UTC text rather than produced by
		// SQLite's datetime('now') so the seed works identically on every
		// supported driver.
		_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
			INSERT OR IGNORE INTO server_scheduler_state (task_id, task_name, schedule, enabled, next_run)
			VALUES (?, ?, ?, 1, ?)
		`, task.Name, task.Name, task.Schedule, dbtime.FormatSQLTimestamp(time.Now()))

		if err != nil {
			log.Printf("ERROR: seedScheduledTasks: failed to seed task %q: %v", task.Name, err)
			continue
		}
	}
}

// ShowSettingsPage renders the admin settings page
func (h *AdminHandler) ShowSettingsPage(c *gin.Context) {
	adminIDValue, exists := c.Get("admin_id")
	if !exists {
		c.Redirect(http.StatusFound, "/server/admin")
		return
	}

	adminID, ok := adminIDValue.(int)
	if !ok {
		c.Redirect(http.StatusFound, "/server/admin")
		return
	}

	adminModel := &model.AdminModel{DB: database.GetServerDB()}
	admin, err := adminModel.GetByID(int64(adminID))
	if err != nil {
		c.Redirect(http.StatusFound, "/server/admin")
		return
	}

	// Load all settings
	settingsModel := &model.SettingsModel{DB: h.DB}
	settings := make(map[string]interface{})

	// Server settings
	settings["server_title"] = settingsModel.GetString("server.title", "Weather")
	settings["server_tagline"] = settingsModel.GetString("server.tagline", "Your personal weather dashboard")
	settings["server_description"] = settingsModel.GetString("server.description", "A comprehensive platform for weather forecasts, moon phases, earthquakes, and hurricane tracking.")
	settings["server_port"] = settingsModel.GetInt("server.port", 64580)
	settings["server_mode"] = settingsModel.GetString("server.mode", "production")
	settings["server_fqdn"] = settingsModel.GetString("server.fqdn", "")
	settings["server_address"] = settingsModel.GetString("server.address", "[::]")
	settings["server_daemonize"] = settingsModel.GetBool("server.daemonize", false)
	settings["server_pidfile"] = settingsModel.GetBool("server.pidfile", true)
	settings["server_http_port"] = settingsModel.GetInt("server.http_port", 3000)
	settings["server_https_port"] = settingsModel.GetInt("server.https_port", 0)
	settings["server_timezone"] = settingsModel.GetString("server.timezone", "America/New_York")
	settings["server_date_format"] = settingsModel.GetString("server.date_format", "US")
	settings["server_time_format"] = settingsModel.GetString("server.time_format", "12-hour")

	// Security settings
	settings["security_session_timeout"] = settingsModel.GetInt("security.session_timeout", 2592000)
	settings["security_max_login_attempts"] = settingsModel.GetInt("security.max_login_attempts", 5)
	settings["security_lockout_duration"] = settingsModel.GetInt("security.lockout_duration", 30)
	settings["security_password_min_length"] = settingsModel.GetInt("security.password_min_length", 8)

	// Features
	settings["features_registration_enabled"] = settingsModel.GetBool("features.registration_enabled", true)
	settings["features_api_enabled"] = settingsModel.GetBool("features.api_enabled", true)
	settings["features_weather_alerts"] = settingsModel.GetBool("features.weather_alerts", true)

	// Backup settings
	settings["backup_enabled"] = settingsModel.GetBool("backup.enabled", true)
	settings["backup_interval"] = settingsModel.GetInt("backup.interval", 6)
	settings["backup_retention"] = settingsModel.GetInt("backup.retention", 30)
	settings["backup_location"] = settingsModel.GetString("backup.location", "/data/backups")

	// Logging settings
	settings["logging_level"] = settingsModel.GetString("logging.level", "info")
	settings["logging_format"] = settingsModel.GetString("logging.format", "apache")
	settings["logging_access_log"] = settingsModel.GetBool("logging.access_log", true)
	settings["logging_error_log"] = settingsModel.GetBool("logging.error_log", true)
	settings["logging_audit_log"] = settingsModel.GetBool("logging.audit_log", true)
	settings["logging_rotation_days"] = settingsModel.GetInt("logging.rotation_days", 30)

	// SMTP settings
	settings["smtp_enabled"] = settingsModel.GetBool("smtp.enabled", false)
	settings["smtp_host"] = settingsModel.GetString("smtp.host", "")
	settings["smtp_port"] = settingsModel.GetInt("smtp.port", 587)
	settings["smtp_username"] = settingsModel.GetString("smtp.username", "")
	settings["smtp_password"] = settingsModel.GetString("smtp.password", "")
	settings["smtp_from_address"] = settingsModel.GetString("smtp.from_address", "")
	settings["smtp_from_name"] = settingsModel.GetString("smtp.from_name", "Weather")
	settings["smtp_use_tls"] = settingsModel.GetBool("smtp.use_tls", true)

	// Rate limiting settings
	settings["rate_limit_enabled"] = settingsModel.GetBool("rate_limit.enabled", true)
	settings["rate_limit_global"] = settingsModel.GetInt("rate_limit.global", 100)
	settings["rate_limit_api"] = settingsModel.GetInt("rate_limit.api", 120)
	settings["rate_limit_admin"] = settingsModel.GetInt("rate_limit.admin", 300)
	settings["rate_limit_window"] = settingsModel.GetInt("rate_limit.window", 900)

	// SSL/TLS settings
	settings["ssl_enabled"] = settingsModel.GetBool("ssl.enabled", false)
	settings["ssl_cert_file"] = settingsModel.GetString("ssl.cert_file", "")
	settings["ssl_key_file"] = settingsModel.GetString("ssl.key_file", "")
	settings["ssl_acme_enabled"] = settingsModel.GetBool("ssl.acme_enabled", false)
	settings["ssl_acme_email"] = settingsModel.GetString("ssl.acme_email", "")
	settings["ssl_acme_provider"] = settingsModel.GetString("ssl.acme_provider", "letsencrypt")

	// Weather settings
	settings["weather_refresh_interval"] = settingsModel.GetInt("weather.refresh_interval", 1800)
	settings["weather_default_units"] = settingsModel.GetString("weather.default_units", "auto")
	settings["weather_cache_enabled"] = settingsModel.GetBool("weather.cache_enabled", true)
	settings["alerts_enabled"] = settingsModel.GetBool("alerts.enabled", true)
	settings["alerts_check_interval"] = settingsModel.GetInt("alerts.check_interval", 900)

	// Notifications settings
	settings["notifications_enabled"] = settingsModel.GetBool("notifications.enabled", true)
	settings["notifications_queue_workers"] = settingsModel.GetInt("notifications.queue_workers", 4)
	settings["notifications_retry_max"] = settingsModel.GetInt("notifications.retry_max", 3)
	settings["notifications_retry_backoff"] = settingsModel.GetString("notifications.retry_backoff", "exponential")

	// GeoIP settings
	settings["geoip_enabled"] = settingsModel.GetBool("geoip.enabled", true)
	settings["geoip_update_interval"] = settingsModel.GetInt("geoip.update_interval", 604800)

	// CORS settings
	settings["cors_enabled"] = settingsModel.GetBool("cors.enabled", true)
	settings["cors_allowed_origins"] = settingsModel.GetString("cors.allowed_origins", "*")
	settings["cors_allow_credentials"] = settingsModel.GetBool("cors.allow_credentials", true)
	settings["cors_max_age"] = settingsModel.GetInt("cors.max_age", 43200)

	// Scheduler settings
	settings["scheduler_enabled"] = settingsModel.GetBool("scheduler.enabled", true)
	settings["scheduler_cleanup_sessions_interval"] = settingsModel.GetInt("scheduler.cleanup_sessions_interval", 3600)
	settings["scheduler_cleanup_audit_logs_days"] = settingsModel.GetInt("scheduler.cleanup_audit_logs_days", 90)
	settings["scheduler_cleanup_notifications_days"] = settingsModel.GetInt("scheduler.cleanup_notifications_days", 30)

	// Tor hidden service settings (TEMPLATE.md PART 32)
	settings["tor_enabled"] = settingsModel.GetBool("tor.enabled", true)
	settings["tor_onion_address"] = settingsModel.GetString("tor.onion_address", "")
	settings["tor_socks_port"] = settingsModel.GetInt("tor.socks_port", 9050)
	settings["tor_control_port"] = settingsModel.GetInt("tor.control_port", 9051)
	settings["tor_data_dir"] = settingsModel.GetString("tor.data_dir", "")

	// Historical weather settings (using Settings struct format for template compatibility)
	type HistorySettings struct {
		HistoryEnabled      bool
		HistoryDefaultYears int
		HistoryMinYears     int
		HistoryMaxYears     int
	}

	historySettings := HistorySettings{
		HistoryEnabled:      settingsModel.GetBool("history.enabled", true),
		HistoryDefaultYears: settingsModel.GetInt("history.default_years", 10),
		HistoryMinYears:     settingsModel.GetInt("history.min_years", 5),
		HistoryMaxYears:     settingsModel.GetInt("history.max_years", 50),
	}

	c.HTML(http.StatusOK, "admin/admin_settings.tmpl", util.TemplateData(c, gin.H{
		"title":    "Server Settings",
		"user":     admin,
		"settings": settings,
		"Settings": historySettings,
	}))
}
