package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/casapps/wthr/src/server/service"
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
func (h *NotificationChannelHandler) ListChannels(c *gin.Context) {
	rows, err := h.DB.Query(`
		SELECT channel_type, channel_name, enabled, state,
		       last_test_at, last_success_at, last_error, failure_count,
		       created_at, updated_at
		FROM notification_channels
		ORDER BY channel_name ASC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch channels"})
		return
	}
	defer rows.Close()

	var channels []gin.H
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

		channel := gin.H{
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

	c.JSON(http.StatusOK, gin.H{
		"channels": channels,
		"total":    len(channels),
	})
}

// GetChannel returns a specific channel
func (h *NotificationChannelHandler) GetChannel(c *gin.Context) {
	channelType := c.Param("type")

	var channelName, state, config string
	var enabled bool
	var lastTestAt, lastSuccessAt sql.NullTime
	var lastError sql.NullString
	var failureCount int

	err := h.DB.QueryRow(`
		SELECT channel_name, enabled, state, config,
		       last_test_at, last_success_at, last_error, failure_count
		FROM notification_channels
		WHERE channel_type = ?
	`, channelType).Scan(&channelName, &enabled, &state, &config,
		&lastTestAt, &lastSuccessAt, &lastError, &failureCount)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found"})
		return
	}

	channel := gin.H{
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

	c.JSON(http.StatusOK, channel)
}

// UpdateChannel updates channel configuration
func (h *NotificationChannelHandler) UpdateChannel(c *gin.Context) {
	channelType := c.Param("type")

	var req struct {
		Enabled bool                   `json:"enabled"`
		Config  map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Convert config to JSON
	configJSON, _ := json.Marshal(req.Config)

	// Update channel
	_, err := h.DB.Exec(`
		UPDATE notification_channels
		SET enabled = ?, config = ?, updated_at = datetime('now')
		WHERE channel_type = ?
	`, req.Enabled, string(configJSON), channelType)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update channel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Channel updated successfully"})
}

// EnableChannel enables a channel
func (h *NotificationChannelHandler) EnableChannel(c *gin.Context) {
	channelType := c.Param("type")

	err := h.ChannelManager.EnableChannel(channelType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable channel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Channel enabled successfully"})
}

// DisableChannel disables a channel
func (h *NotificationChannelHandler) DisableChannel(c *gin.Context) {
	channelType := c.Param("type")

	err := h.ChannelManager.DisableChannel(channelType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable channel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Channel disabled successfully"})
}

// TestChannel tests a channel configuration
func (h *NotificationChannelHandler) TestChannel(c *gin.Context) {
	channelType := c.Param("type")

	var req struct {
		Recipient string `json:"recipient" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Recipient required"})
		return
	}

	// Special handling for SMTP/email channel
	if channelType == "email" {
		// Load config and send test
		if err := h.SMTP.LoadConfig(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load SMTP config"})
			return
		}

		if err := h.SMTP.SendTestEmail(req.Recipient); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Auto-enable if configured
		h.SMTP.EnableChannel()

		c.JSON(http.StatusOK, gin.H{"message": "Test email sent successfully"})
		return
	}

	// Generic channel test
	err := h.ChannelManager.TestChannel(channelType, req.Recipient)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Test notification sent successfully"})
}

// GetChannelStats returns statistics for a channel
func (h *NotificationChannelHandler) GetChannelStats(c *gin.Context) {
	channelType := c.Param("type")

	stats, err := h.ChannelManager.GetChannelStats(channelType)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ListSMTPProviders returns available SMTP provider presets
func (h *NotificationChannelHandler) ListSMTPProviders(c *gin.Context) {
	category := c.Query("category")

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

	c.JSON(http.StatusOK, gin.H{
		"providers": providers,
		"grouped":   grouped,
		"total":     len(providers),
	})
}

// AutoDetectSMTP attempts to auto-detect SMTP server
func (h *NotificationChannelHandler) AutoDetectSMTP(c *gin.Context) {
	found, err := h.SMTP.AutoDetect()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if found {
		// Load the detected config
		h.SMTP.LoadConfig()
		config := h.SMTP.GetConfig()
		c.JSON(http.StatusOK, gin.H{
			"message": "SMTP server detected",
			"host":    config.Host,
			"port":    config.Port,
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "No SMTP server detected"})
}

// InitializeChannels initializes all channels in database
func (h *NotificationChannelHandler) InitializeChannels(c *gin.Context) {
	err := h.ChannelManager.InitializeChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Channels initialized successfully",
		"total":   len(service.ChannelRegistry),
	})
}

// GetChannelDefinitions returns channel definitions from registry
func (h *NotificationChannelHandler) GetChannelDefinitions(c *gin.Context) {
	category := c.Query("category")

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

	c.JSON(http.StatusOK, gin.H{
		"definitions": definitions,
		"grouped":     grouped,
		"total":       len(definitions),
	})
}

// GetQueueStats returns notification queue statistics
func (h *NotificationChannelHandler) GetQueueStats(c *gin.Context) {
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
	h.DB.QueryRow("SELECT COUNT(*) FROM notification_queue").Scan(&stats.Total)

	// By state
	h.DB.QueryRow("SELECT COUNT(*) FROM notification_queue WHERE state IN ('created', 'queued')").Scan(&stats.Pending)
	h.DB.QueryRow("SELECT COUNT(*) FROM notification_queue WHERE state = 'sending'").Scan(&stats.Sending)
	h.DB.QueryRow("SELECT COUNT(*) FROM notification_queue WHERE state = 'delivered'").Scan(&stats.Delivered)
	h.DB.QueryRow("SELECT COUNT(*) FROM notification_queue WHERE state = 'failed'").Scan(&stats.Failed)
	h.DB.QueryRow("SELECT COUNT(*) FROM notification_queue WHERE state = 'dead_letter'").Scan(&stats.DeadLetters)

	// By channel
	stats.ByChannel = make(map[string]int)
	rows, err := h.DB.Query("SELECT channel_type, COUNT(*) as count FROM notification_queue GROUP BY channel_type")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var channelType string
			var count int
			rows.Scan(&channelType, &count)
			stats.ByChannel[channelType] = count
		}
	}

	c.JSON(http.StatusOK, stats)
}

// GetNotificationHistory returns notification history
func (h *NotificationChannelHandler) GetNotificationHistory(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	channelType := c.Query("channel")
	status := c.Query("status")

	query := `
		SELECT id, queue_id, user_id, channel_type, status, subject,
		       sent_at, delivered_at, error_message
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

	query += " ORDER BY sent_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch history"})
		return
	}
	defer rows.Close()

	var history []gin.H
	for rows.Next() {
		var id, queueID sql.NullInt64
		var userID sql.NullInt64
		var channelType, status, subject string
		var sentAt sql.NullTime
		var deliveredAt sql.NullTime
		var errorMessage sql.NullString

		rows.Scan(&id, &queueID, &userID, &channelType, &status, &subject,
			&sentAt, &deliveredAt, &errorMessage)

		item := gin.H{
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
		if sentAt.Valid {
			item["sent_at"] = sentAt.Time
		}
		if deliveredAt.Valid {
			item["delivered_at"] = deliveredAt.Time
		}
		if errorMessage.Valid {
			item["error_message"] = errorMessage.String
		}

		history = append(history, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"history": history,
		"total":   len(history),
	})
}
