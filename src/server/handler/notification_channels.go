package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
)

// NotificationChannelHandler handles notification channel management
type NotificationChannelHandler struct {
	DB             *sql.DB
	ChannelManager *service.ChannelManager
	SMTP           *service.SMTPService
}

// NewNotificationChannelHandler creates a new notification channel handler
func NewNotificationChannelHandler(db *sql.DB) *NotificationChannelHandler {
	cm := service.NewChannelManager(db)
	smtp := service.SharedSMTPService(db)

	return &NotificationChannelHandler{
		DB:             db,
		ChannelManager: cm,
		SMTP:           smtp,
	}
}

// AdminChannelRow is one configured notification channel as the session-auth
// admin channels page renders it. It is deliberately narrower than the JSON
// API channel: the page only shows type, name, state, and the enabled flag.
type AdminChannelRow struct {
	ChannelType string
	ChannelName string
	Enabled     bool
	State       string
}

// ListChannelRows returns the configured channels for the admin channels page.
// The handler owns the query so the page does not reach around the handler to
// the database handle it already holds.
func (h *NotificationChannelHandler) ListChannelRows() ([]AdminChannelRow, error) {
	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT channel_type, channel_name, enabled, state
		FROM server_notification_channels
		ORDER BY channel_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []AdminChannelRow
	for rows.Next() {
		var row AdminChannelRow
		if err := rows.Scan(&row.ChannelType, &row.ChannelName, &row.Enabled, &row.State); err != nil {
			continue
		}

		channels = append(channels, row)
	}

	return channels, rows.Err()
}

// ListChannels returns all notification channels
func (h *NotificationChannelHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT channel_type, channel_name, enabled, state,
		       last_test_at, last_success_at, last_error, failure_count,
		       created_at, updated_at
		FROM server_notification_channels
		ORDER BY channel_name ASC
	`)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.channels.failed_to_fetch_channels"))
		return
	}
	defer rows.Close()

	var channels []map[string]interface{}
	for rows.Next() {
		var channelType, channelName, state string
		var enabled bool
		var lastTestAt, lastSuccessAt, createdAt, updatedAt sql.NullTime
		var lastError sql.NullString
		var failureCount int

		err := rows.Scan(&channelType, &channelName, &enabled, &state,
			&lastTestAt, &lastSuccessAt, &lastError, &failureCount,
			&createdAt, &updatedAt)
		if err != nil {
			continue
		}

		channel := map[string]interface{}{
			"channel_type":    channelType,
			"channel_name":    channelName,
			"enabled":         enabled,
			"state":           state,
			"failure_count":   failureCount,
			"last_test_at":    nil,
			"last_success_at": nil,
			"last_error":      nil,
			"created_at":      nil,
			"updated_at":      nil,
		}

		if lastTestAt.Valid {
			channel["last_test_at"] = lastTestAt.Time
		}
		if lastSuccessAt.Valid {
			channel["last_success_at"] = lastSuccessAt.Time
		}
		if lastError.Valid {
			channel["last_error"] = lastError.String
		}
		if createdAt.Valid {
			channel["created_at"] = createdAt.Time
		}
		if updatedAt.Valid {
			channel["updated_at"] = updatedAt.Time
		}

		channels = append(channels, channel)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels": channels,
		"total":    len(channels),
	})
}

// GetChannel returns a specific channel
func (h *NotificationChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	var channelName, state, config string
	var enabled bool
	var lastTestAt, lastSuccessAt sql.NullTime
	var lastError sql.NullString
	var failureCount int

	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT channel_name, enabled, state, config,
		       last_test_at, last_success_at, last_error, failure_count
		FROM server_notification_channels
		WHERE channel_type = ?
	`, channelType).Scan(&channelName, &enabled, &state, &config,
		&lastTestAt, &lastSuccessAt, &lastError, &failureCount)

	if err != nil {
		NotFound(w, r, Translate(r, "errors.notifications.channels.channel_not_found"))
		return
	}

	channel := map[string]interface{}{
		"channel_type":    channelType,
		"channel_name":    channelName,
		"enabled":         enabled,
		"state":           state,
		"config":          config,
		"failure_count":   failureCount,
		"last_test_at":    nil,
		"last_success_at": nil,
		"last_error":      nil,
	}

	if lastTestAt.Valid {
		channel["last_test_at"] = lastTestAt.Time
	}
	if lastSuccessAt.Valid {
		channel["last_success_at"] = lastSuccessAt.Time
	}
	if lastError.Valid {
		channel["last_error"] = lastError.String
	}

	writeJSON(w, http.StatusOK, channel)
}

// UpdateChannel updates channel configuration
func (h *NotificationChannelHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	var req struct {
		Enabled bool                   `json:"enabled"`
		Config  map[string]interface{} `json:"config"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	if wantsFormSubmission(r) {
		req.Config = h.mergeChannelFormConfig(channelType, req.Config)
	}

	if err := h.saveChannel(channelType, req.Enabled, req.Config); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminConfigPagePath(r, "/config/channels"), "flash_channel_updated", "flash_channel_update_failed", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.notifications.channels.failed_to_update_channel"))
		return
	}

	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/channels"), "flash_channel_updated", "flash_channel_update_failed", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.notifications.channels.channel_updated_successfully"),
	})
}

// mergeChannelFormConfig folds a form post's config.* fields over the channel's
// stored config. A browser form carries only the fields on screen, so an
// update that omits a field would otherwise silently drop the stored value.
// The merge lives here rather than in the template so the form route and the
// JSON route agree on what a config update means.
func (h *NotificationChannelHandler) mergeChannelFormConfig(channelType string, submitted map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	var stored string
	if err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT config
		FROM server_notification_channels
		WHERE channel_type = ?
	`, channelType).Scan(&stored); err == nil {
		existing := make(map[string]interface{})
		if json.Unmarshal([]byte(stored), &existing) == nil {
			for key, value := range existing {
				merged[key] = value
			}
		}
	}

	for key, value := range submitted {
		merged[key] = value
	}

	return merged
}

// saveChannel persists the enabled flag and configuration for a channel. It is
// shared by the token-auth JSON route and the session-auth admin form route.
func (h *NotificationChannelHandler) saveChannel(channelType string, enabled bool, cfg map[string]interface{}) error {
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	// updated_at is bound as canonical UTC text instead of datetime('now'):
	// that spelling only exists on SQLite, and binding the value keeps this
	// writer in the single layout every reader parses.
	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE server_notification_channels
		SET enabled = ?, config = ?, updated_at = ?
		WHERE channel_type = ?
	`, enabled, string(configJSON), dbtime.FormatSQLTimestamp(time.Now()), channelType)

	return err
}

// EnableChannel enables a channel
func (h *NotificationChannelHandler) EnableChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	err := h.ChannelManager.EnableChannel(channelType)
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/channels"), "flash_channel_enabled", "flash_channel_update_failed", err)
		return
	}

	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.channels.failed_to_enable_channel"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.notifications.channels.channel_enabled_successfully"),
	})
}

// DisableChannel disables a channel
func (h *NotificationChannelHandler) DisableChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	err := h.ChannelManager.DisableChannel(channelType)
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/channels"), "flash_channel_disabled", "flash_channel_update_failed", err)
		return
	}

	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.channels.failed_to_disable_channel"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.notifications.channels.channel_disabled_successfully"),
	})
}

// TestChannel tests a channel configuration
func (h *NotificationChannelHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	var req struct {
		Recipient string `json:"recipient" binding:"required"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	smtpChannel, err := h.sendChannelTest(channelType, req.Recipient)
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/channels"), "flash_channel_test_sent", "flash_channel_test_failed", err)
		return
	}

	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.channels.test_notification_failed"))
		return
	}

	if smtpChannel {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": Translate(r, "success.notifications.channels.test_email_sent_successfully"),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.notifications.channels.test_notification_sent_successfully"),
	})
}

// sendChannelTest delivers a test notification through the named channel. The
// boolean reports whether the channel was the SMTP-backed email channel, which
// carries a different success message. The raw error is returned to the caller
// for logging only; it is never rendered into a response.
func (h *NotificationChannelHandler) sendChannelTest(channelType, recipient string) (bool, error) {
	if channelType == "email" {
		if err := h.SMTP.LoadConfig(); err != nil {
			return true, err
		}
		if err := h.SMTP.SendTestEmail(recipient); err != nil {
			return true, err
		}
		h.SMTP.EnableChannel()
		return true, nil
	}

	return false, h.ChannelManager.TestChannel(channelType, recipient)
}

// GetChannelStats returns statistics for a channel
func (h *NotificationChannelHandler) GetChannelStats(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	stats, err := h.ChannelManager.GetChannelStats(channelType)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.notifications.channels.channel_not_found"))
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// ListSMTPProviders returns available SMTP provider presets
func (h *NotificationChannelHandler) ListSMTPProviders(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	var providers []service.SMTPProviderPreset
	if category != "" {
		providers = service.ListProvidersByCategory(category)
	} else {
		providers = service.ListProviderPresets()
	}

	// Group by category
	grouped := make(map[string][]service.SMTPProviderPreset)
	for _, p := range providers {
		grouped[p.Category] = append(grouped[p.Category], p)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"providers": providers,
		"grouped":   grouped,
		"total":     len(providers),
	})
}

// AutoDetectSMTP attempts to auto-detect SMTP server
func (h *NotificationChannelHandler) AutoDetectSMTP(w http.ResponseWriter, r *http.Request) {
	found, err := h.SMTP.AutoDetect()
	if err != nil {
		NotFound(w, r, Translate(r, "errors.notifications.channels.no_smtp_server_detected"))
		return
	}

	if found {
		// Load the detected config
		h.SMTP.LoadConfig()
		config := h.SMTP.GetConfig()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": Translate(r, "success.notifications.channels.smtp_server_detected"),
			"host":    config.Host,
			"port":    config.Port,
		})
		return
	}

	NotFound(w, r, Translate(r, "errors.notifications.channels.no_smtp_server_detected"))
}

// InitializeChannels initializes all channels in database
func (h *NotificationChannelHandler) InitializeChannels(w http.ResponseWriter, r *http.Request) {
	err := h.ChannelManager.InitializeChannels()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.channels.failed_to_initialize_channels"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.notifications.channels.channels_initialized_successfully"),
		"total":   len(service.ChannelRegistry),
	})
}

// GetChannelDefinitions returns channel definitions from registry
func (h *NotificationChannelHandler) GetChannelDefinitions(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	var definitions []service.ChannelDefinition
	for _, def := range service.ChannelRegistry {
		if category == "" || def.Category == category {
			definitions = append(definitions, def)
		}
	}

	// Group by category
	grouped := make(map[string][]service.ChannelDefinition)
	for _, def := range definitions {
		grouped[def.Category] = append(grouped[def.Category], def)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"definitions": definitions,
		"grouped":     grouped,
		"total":       len(definitions),
	})
}

// GetQueueStats returns notification queue statistics
func (h *NotificationChannelHandler) GetQueueStats(w http.ResponseWriter, r *http.Request) {
	var stats struct {
		Total       int            `json:"total"`
		Pending     int            `json:"pending"`
		Sending     int            `json:"sending"`
		Delivered   int            `json:"delivered"`
		Failed      int            `json:"failed"`
		DeadLetters int            `json:"dead_letters"`
		ByChannel   map[string]int `json:"by_channel"`
	}

	// Total
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue").Scan(&stats.Total)

	// By state
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue WHERE state IN ('created', 'queued')").Scan(&stats.Pending)
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue WHERE state = 'sending'").Scan(&stats.Sending)
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue WHERE state = 'delivered'").Scan(&stats.Delivered)
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue WHERE state = 'failed'").Scan(&stats.Failed)
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM notification_queue WHERE state = 'dead_letter'").Scan(&stats.DeadLetters)

	// By channel
	stats.ByChannel = make(map[string]int)
	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutReport, "SELECT channel_type, COUNT(*) as count FROM notification_queue GROUP BY channel_type")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var channelType string
			var count int
			rows.Scan(&channelType, &count)
			stats.ByChannel[channelType] = count
		}
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetNotificationHistory returns notification history
func (h *NotificationChannelHandler) GetNotificationHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	channelType := r.URL.Query().Get("channel")
	status := r.URL.Query().Get("status")

	query := `
		SELECT id, queue_id, user_id, channel_type, status, subject,
		       created_at, delivered_at, error_message
		FROM notification_history
		WHERE 1=1
	`
	args := []interface{}{}

	if channelType != "" {
		query += " AND channel_type = ?"
		args = append(args, channelType)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, query, args...)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.notifications.channels.failed_to_fetch_history"))
		return
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var id, queueID sql.NullInt64
		var userID sql.NullInt64
		var channelType, status, subject string
		var createdAt sql.NullTime
		var deliveredAt sql.NullTime
		var errorMessage sql.NullString

		rows.Scan(&id, &queueID, &userID, &channelType, &status, &subject,
			&createdAt, &deliveredAt, &errorMessage)

		item := map[string]interface{}{
			"id":           id.Int64,
			"channel_type": channelType,
			"status":       status,
			"subject":      subject,
		}

		if queueID.Valid {
			item["queue_id"] = queueID.Int64
		}
		if userID.Valid {
			item["user_id"] = userID.Int64
		}
		if createdAt.Valid {
			item["created_at"] = createdAt.Time
		}
		if deliveredAt.Valid {
			item["delivered_at"] = deliveredAt.Time
		}
		if errorMessage.Valid {
			item["error_message"] = errorMessage.String
		}

		history = append(history, item)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"history": history,
		"total":   len(history),
	})
}
