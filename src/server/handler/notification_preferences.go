package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/database"
)

// NotificationPreferencesHandler handles user notification preferences
type NotificationPreferencesHandler struct {
	DB *sql.DB
}

// NewNotificationPreferencesHandler creates a new handler
func NewNotificationPreferencesHandler(db *sql.DB) *NotificationPreferencesHandler {
	return &NotificationPreferencesHandler{DB: db}
}

// GetUserPreferences returns user's notification preferences
func (h *NotificationPreferencesHandler) GetUserPreferences(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT id, channel_type, enabled, priority,
		       quiet_hours_start, quiet_hours_end, config
		FROM user_notification_preferences
		WHERE user_id = ?
		ORDER BY priority DESC
	`, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch preferences"})
		return
	}
	defer rows.Close()

	var preferences []gin.H
	for rows.Next() {
		var id int
		var channelType string
		var enabled bool
		var priority int
		var quietStart, quietEnd, config sql.NullString

		if err := rows.Scan(&id, &channelType, &enabled, &priority, &quietStart, &quietEnd, &config); err != nil {
			log.Printf("WARNING: GetUserPreferences: failed to scan row for user %d: %v", userID, err)
			continue
		}

		pref := gin.H{
			"id":           id,
			"channel_type": channelType,
			"enabled":      enabled,
			"priority":     priority,
		}

		if quietStart.Valid {
			pref["quiet_hours_start"] = quietStart.String
		}
		if quietEnd.Valid {
			pref["quiet_hours_end"] = quietEnd.String
		}
		if config.Valid {
			var configMap map[string]interface{}
			if err := json.Unmarshal([]byte(config.String), &configMap); err != nil {
				log.Printf("WARNING: GetUserPreferences: failed to unmarshal config for preference %d: %v", id, err)
			}
			pref["config"] = configMap
		}

		preferences = append(preferences, pref)
	}

	c.JSON(http.StatusOK, gin.H{
		"preferences": preferences,
		"total":       len(preferences),
	})
}

// UpdatePreference updates a user's channel preference
func (h *NotificationPreferencesHandler) UpdatePreference(c *gin.Context) {
	userID := c.GetInt("user_id")
	prefID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid preference id"})
		return
	}

	var req struct {
		Enabled         bool                   `json:"enabled"`
		Priority        int                    `json:"priority"`
		QuietHoursStart *string                `json:"quiet_hours_start"`
		QuietHoursEnd   *string                `json:"quiet_hours_end"`
		Config          map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: UpdatePreference: failed to marshal config: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config"})
		return
	}

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE user_notification_preferences
		SET enabled = ?, priority = ?, quiet_hours_start = ?,
		    quiet_hours_end = ?, config = ?, updated_at = datetime('now')
		WHERE id = ? AND user_id = ?
	`, req.Enabled, req.Priority, req.QuietHoursStart, req.QuietHoursEnd,
		string(configJSON), prefID, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update preference"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update preference"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Preference not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Preference updated successfully"})
}

// CreatePreference creates a new channel preference for user
func (h *NotificationPreferencesHandler) CreatePreference(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req struct {
		ChannelType     string                 `json:"channel_type" binding:"required"`
		Enabled         bool                   `json:"enabled"`
		Priority        int                    `json:"priority"`
		QuietHoursStart *string                `json:"quiet_hours_start"`
		QuietHoursEnd   *string                `json:"quiet_hours_end"`
		Config          map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: CreatePreference: failed to marshal config: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config"})
		return
	}

	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO user_notification_preferences
		(user_id, channel_type, enabled, priority, quiet_hours_start,
		 quiet_hours_end, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(user_id, channel_type) DO UPDATE SET
		    enabled = excluded.enabled,
		    priority = excluded.priority,
		    quiet_hours_start = excluded.quiet_hours_start,
		    quiet_hours_end = excluded.quiet_hours_end,
		    config = excluded.config,
		    updated_at = datetime('now')
	`, userID, req.ChannelType, req.Enabled, req.Priority,
		req.QuietHoursStart, req.QuietHoursEnd, string(configJSON))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create preference"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Preference created successfully"})
}

// DeletePreference deletes a user's channel preference
func (h *NotificationPreferencesHandler) DeletePreference(c *gin.Context) {
	userID := c.GetInt("user_id")
	prefID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid preference id"})
		return
	}

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		DELETE FROM user_notification_preferences
		WHERE id = ? AND user_id = ?
	`, prefID, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete preference"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete preference"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Preference not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Preference deleted successfully"})
}

// GetSubscriptions returns user's notification subscriptions
func (h *NotificationPreferencesHandler) GetSubscriptions(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT id, subscription_type, subscription_category, enabled, config
		FROM notification_subscriptions
		WHERE user_id = ?
		ORDER BY subscription_type, subscription_category
	`, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscriptions"})
		return
	}
	defer rows.Close()

	var subscriptions []gin.H
	for rows.Next() {
		var id int
		var subType, subCategory string
		var enabled bool
		var config sql.NullString

		if err := rows.Scan(&id, &subType, &subCategory, &enabled, &config); err != nil {
			log.Printf("WARNING: GetSubscriptions: failed to scan row for user %d: %v", userID, err)
			continue
		}

		sub := gin.H{
			"id":                    id,
			"subscription_type":     subType,
			"subscription_category": subCategory,
			"enabled":               enabled,
		}

		if config.Valid {
			var configMap map[string]interface{}
			if err := json.Unmarshal([]byte(config.String), &configMap); err != nil {
				log.Printf("WARNING: GetSubscriptions: failed to unmarshal config for subscription %d: %v", id, err)
			}
			sub["config"] = configMap
		}

		subscriptions = append(subscriptions, sub)
	}

	c.JSON(http.StatusOK, gin.H{
		"subscriptions": subscriptions,
		"total":         len(subscriptions),
	})
}

// UpdateSubscription updates a subscription
func (h *NotificationPreferencesHandler) UpdateSubscription(c *gin.Context) {
	userID := c.GetInt("user_id")
	subID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subscription id"})
		return
	}

	var req struct {
		Enabled bool                   `json:"enabled"`
		Config  map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: UpdateSubscription: failed to marshal config: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config"})
		return
	}

	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE notification_subscriptions
		SET enabled = ?, config = ?, updated_at = datetime('now')
		WHERE id = ? AND user_id = ?
	`, req.Enabled, string(configJSON), subID, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription updated successfully"})
}

// CreateSubscription creates a new subscription
func (h *NotificationPreferencesHandler) CreateSubscription(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req struct {
		SubscriptionType     string                 `json:"subscription_type" binding:"required"`
		SubscriptionCategory string                 `json:"subscription_category" binding:"required"`
		Enabled              bool                   `json:"enabled"`
		Config               map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: CreateSubscription: failed to marshal config: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config"})
		return
	}

	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO notification_subscriptions
		(user_id, subscription_type, subscription_category, enabled, config,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(user_id, subscription_type, subscription_category) DO UPDATE SET
		    enabled = excluded.enabled,
		    config = excluded.config,
		    updated_at = datetime('now')
	`, userID, req.SubscriptionType, req.SubscriptionCategory,
		req.Enabled, string(configJSON))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subscription"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Subscription created successfully"})
}
