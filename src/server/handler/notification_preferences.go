package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/reqctx"
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
func (h *NotificationPreferencesHandler) GetUserPreferences(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT id, channel_type, enabled, priority,
		       quiet_hours_start, quiet_hours_end, config
		FROM user_notification_channel_preferences
		WHERE user_id = ?
		ORDER BY priority DESC
	`, userID)

	if err != nil {
		log.Printf("ERROR: GetUserPreferences: failed to query preferences for user %d: %v", userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to fetch preferences")
		return
	}
	defer rows.Close()

	var preferences []map[string]interface{}
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

		pref := map[string]interface{}{
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"preferences": preferences,
		"total":       len(preferences),
	})
}

// UpdatePreference updates a user's channel preference
func (h *NotificationPreferencesHandler) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")
	prefID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid preference id")
		return
	}

	var req struct {
		Enabled         bool                   `json:"enabled"`
		Priority        int                    `json:"priority"`
		QuietHoursStart *string                `json:"quiet_hours_start"`
		QuietHoursEnd   *string                `json:"quiet_hours_end"`
		Config          map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: UpdatePreference: failed to marshal config: %v", err)
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid config")
		return
	}

	// updated_at is bound as canonical UTC text rather than produced by SQLite's
	// datetime('now'), which does not exist on PostgreSQL or MySQL.
	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE user_notification_channel_preferences
		SET enabled = ?, priority = ?, quiet_hours_start = ?,
		    quiet_hours_end = ?, config = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, req.Enabled, req.Priority, req.QuietHoursStart, req.QuietHoursEnd,
		string(configJSON), dbtime.FormatSQLTimestamp(time.Now()), prefID, userID)

	if err != nil {
		log.Printf("ERROR: UpdatePreference: failed to update preference %d for user %d: %v", prefID, userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to update preference")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("ERROR: UpdatePreference: failed to read affected rows for preference %d: %v", prefID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to update preference")
		return
	}
	if rowsAffected == 0 {
		RespondError(w, r, http.StatusNotFound, ErrNotFound, "Preference not found")
		return
	}

	RespondSuccess(w, r, "Preference updated successfully")
}

// CreatePreference creates a new channel preference for user
func (h *NotificationPreferencesHandler) CreatePreference(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")

	var req struct {
		ChannelType     string                 `json:"channel_type" binding:"required"`
		Enabled         bool                   `json:"enabled"`
		Priority        int                    `json:"priority"`
		QuietHoursStart *string                `json:"quiet_hours_start"`
		QuietHoursEnd   *string                `json:"quiet_hours_end"`
		Config          map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	// binding:"required" equivalent
	if req.ChannelType == "" {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: CreatePreference: failed to marshal config: %v", err)
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid config")
		return
	}

	// created_at/updated_at are bound as canonical UTC text rather than produced
	// by SQLite's datetime('now'), which does not exist on PostgreSQL or MySQL.
	now := dbtime.FormatSQLTimestamp(time.Now())

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO user_notification_channel_preferences
		(user_id, channel_type, enabled, priority, quiet_hours_start,
		 quiet_hours_end, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, channel_type) DO UPDATE SET
		    enabled = excluded.enabled,
		    priority = excluded.priority,
		    quiet_hours_start = excluded.quiet_hours_start,
		    quiet_hours_end = excluded.quiet_hours_end,
		    config = excluded.config,
		    updated_at = ?
	`, userID, req.ChannelType, req.Enabled, req.Priority,
		req.QuietHoursStart, req.QuietHoursEnd, string(configJSON), now, now, now)

	if err != nil {
		log.Printf("ERROR: CreatePreference: failed to upsert preference for user %d: %v", userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to create preference")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		log.Printf("WARNING: CreatePreference: failed to read inserted id for user %d: %v", userID, err)
	}
	RespondCreated(w, r, "Preference created successfully", strconv.FormatInt(id, 10))
}

// DeletePreference deletes a user's channel preference
func (h *NotificationPreferencesHandler) DeletePreference(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")
	prefID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid preference id")
		return
	}

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		DELETE FROM user_notification_channel_preferences
		WHERE id = ? AND user_id = ?
	`, prefID, userID)

	if err != nil {
		log.Printf("ERROR: DeletePreference: failed to delete preference %d for user %d: %v", prefID, userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to delete preference")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("ERROR: DeletePreference: failed to read affected rows for preference %d: %v", prefID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to delete preference")
		return
	}
	if rowsAffected == 0 {
		RespondError(w, r, http.StatusNotFound, ErrNotFound, "Preference not found")
		return
	}

	RespondSuccess(w, r, "Preference deleted successfully")
}

// GetSubscriptions returns user's notification subscriptions
func (h *NotificationPreferencesHandler) GetSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT id, subscription_type, subscription_category, enabled, config
		FROM notification_subscriptions
		WHERE user_id = ?
		ORDER BY subscription_type, subscription_category
	`, userID)

	if err != nil {
		log.Printf("ERROR: GetSubscriptions: failed to query subscriptions for user %d: %v", userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to fetch subscriptions")
		return
	}
	defer rows.Close()

	var subscriptions []map[string]interface{}
	for rows.Next() {
		var id int
		var subType, subCategory string
		var enabled bool
		var config sql.NullString

		if err := rows.Scan(&id, &subType, &subCategory, &enabled, &config); err != nil {
			log.Printf("WARNING: GetSubscriptions: failed to scan row for user %d: %v", userID, err)
			continue
		}

		sub := map[string]interface{}{
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscriptions": subscriptions,
		"total":         len(subscriptions),
	})
}

// UpdateSubscription updates a subscription
func (h *NotificationPreferencesHandler) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")
	subID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid subscription id")
		return
	}

	var req struct {
		Enabled bool                   `json:"enabled"`
		Config  map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: UpdateSubscription: failed to marshal config: %v", err)
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid config")
		return
	}

	// updated_at is bound as canonical UTC text rather than produced by SQLite's
	// datetime('now'), which does not exist on PostgreSQL or MySQL.
	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE notification_subscriptions
		SET enabled = ?, config = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, req.Enabled, string(configJSON), dbtime.FormatSQLTimestamp(time.Now()), subID, userID)

	if err != nil {
		log.Printf("ERROR: UpdateSubscription: failed to update subscription %d for user %d: %v", subID, userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to update subscription")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("ERROR: UpdateSubscription: failed to read affected rows for subscription %d: %v", subID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to update subscription")
		return
	}
	if rowsAffected == 0 {
		RespondError(w, r, http.StatusNotFound, ErrNotFound, "Subscription not found")
		return
	}

	RespondSuccess(w, r, "Subscription updated successfully")
}

// CreateSubscription creates a new subscription
func (h *NotificationPreferencesHandler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	userID := reqctx.GetInt(r.Context(), "user_id")

	var req struct {
		SubscriptionType     string                 `json:"subscription_type" binding:"required"`
		SubscriptionCategory string                 `json:"subscription_category" binding:"required"`
		Enabled              bool                   `json:"enabled"`
		Config               map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	// binding:"required" equivalent
	if req.SubscriptionType == "" || req.SubscriptionCategory == "" {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		log.Printf("WARNING: CreateSubscription: failed to marshal config: %v", err)
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Invalid config")
		return
	}

	// created_at/updated_at are bound as canonical UTC text rather than produced
	// by SQLite's datetime('now'), which does not exist on PostgreSQL or MySQL.
	now := dbtime.FormatSQLTimestamp(time.Now())

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO notification_subscriptions
		(user_id, subscription_type, subscription_category, enabled, config,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, subscription_type, subscription_category) DO UPDATE SET
		    enabled = excluded.enabled,
		    config = excluded.config,
		    updated_at = ?
	`, userID, req.SubscriptionType, req.SubscriptionCategory,
		req.Enabled, string(configJSON), now, now, now)

	if err != nil {
		log.Printf("ERROR: CreateSubscription: failed to upsert subscription for user %d: %v", userID, err)
		RespondError(w, r, http.StatusInternalServerError, ErrDatabaseError, "Failed to create subscription")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		log.Printf("WARNING: CreateSubscription: failed to read inserted id for user %d: %v", userID, err)
	}
	RespondCreated(w, r, "Subscription created successfully", strconv.FormatInt(id, 10))
}
