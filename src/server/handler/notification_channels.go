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
	smtp := service.NewSMTPService(db)

	return &NotificationChannelHandler{
		DB:             db,
		ChannelManager: cm,
		SMTP:           smtp,
	}
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
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to fetch channels"})
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
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Channel not found"})
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid request"})
		return
	}

	// Convert config to JSON
	configJSON, _ := json.Marshal(req.Config)

	// Update channel.
	//
	// updated_at is bound as canonical UTC text instead of datetime('now'):
	// that spelling only exists on SQLite, and binding the value keeps this
	// writer in the single layout every reader parses.
	_, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE server_notification_channels
		SET enabled = ?, config = ?, updated_at = ?
		WHERE channel_type = ?
	`, req.Enabled, string(configJSON), dbtime.FormatSQLTimestamp(time.Now()), channelType)

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to update channel"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Channel updated successfully"})
}

// EnableChannel enables a channel
func (h *NotificationChannelHandler) EnableChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	err := h.ChannelManager.EnableChannel(channelType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to enable channel"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Channel enabled successfully"})
}

// DisableChannel disables a channel
func (h *NotificationChannelHandler) DisableChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	err := h.ChannelManager.DisableChannel(channelType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to disable channel"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Channel disabled successfully"})
}

// TestChannel tests a channel configuration
func (h *NotificationChannelHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	var req struct {
		Recipient string `json:"recipient" binding:"required"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Recipient required"})
		return
	}

	if req.Recipient == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Recipient required"})
		return
	}

	// Special handling for SMTP/email channel
	if channelType == "email" {
		// Load config and send test
		if err := h.SMTP.LoadConfig(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to load SMTP config"})
			return
		}

		if err := h.SMTP.SendTestEmail(req.Recipient); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}

		// Auto-enable if configured
		h.SMTP.EnableChannel()

		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Test email sent successfully"})
		return
	}

	// Generic channel test
	err := h.ChannelManager.TestChannel(channelType, req.Recipient)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Test notification sent successfully"})
}

// GetChannelStats returns statistics for a channel
func (h *NotificationChannelHandler) GetChannelStats(w http.ResponseWriter, r *http.Request) {
	channelType := chi.URLParam(r, "type")

	stats, err := h.ChannelManager.GetChannelStats(channelType)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "Channel not found"})
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
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": err.Error()})
		return
	}

	if found {
		// Load the detected config
		h.SMTP.LoadConfig()
		config := h.SMTP.GetConfig()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "SMTP server detected",
			"host":    config.Host,
			"port":    config.Port,
		})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "No SMTP server detected"})
}

// InitializeChannels initializes all channels in database
func (h *NotificationChannelHandler) InitializeChannels(w http.ResponseWriter, r *http.Request) {
	err := h.ChannelManager.InitializeChannels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Channels initialized successfully",
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
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "Failed to fetch history"})
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
