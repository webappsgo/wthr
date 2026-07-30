package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// NotificationChannel represents a notification channel interface
type NotificationChannel interface {
	GetType() string
	GetName() string
	IsEnabled() bool
	Send(recipient, subject, body string, metadata map[string]interface{}) error
	Test(recipient string) error
	ValidateConfig(config map[string]interface{}) error
}

// ChannelManager manages all notification channels
type ChannelManager struct {
	db       *sql.DB
	channels map[string]NotificationChannel
	smtp     *SMTPService
}

// ChannelDefinition defines a notification channel
type ChannelDefinition struct {
	Type         string
	Name         string
	Description  string
	Category     string
	Enabled      bool
	ConfigFields []ConfigField
}

// ConfigField defines a configuration field
type ConfigField struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	// text, password, number, boolean, select
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	DefaultValue string   `json:"default_value"`
	Placeholder  string   `json:"placeholder"`
	HelpText     string   `json:"help_text"`
	// For select type
	Options      []string `json:"options,omitempty"`
}

// ChannelRegistry contains definitions for 30+ notification channels
var ChannelRegistry = []ChannelDefinition{
	// Email Channels
	{
		Type: "email", Name: "Email (SMTP)", Category: "email",
		Description: "Send notifications via SMTP email",
		ConfigFields: []ConfigField{
			{Key: "host", Label: "SMTP Host", Type: "text", Required: true, Placeholder: "smtp.gmail.com"},
			{Key: "port", Label: "SMTP Port", Type: "number", Required: true, DefaultValue: "587"},
			{Key: "username", Label: "Username", Type: "text", Required: true},
			{Key: "password", Label: "Password", Type: "password", Required: true},
			{Key: "from_address", Label: "From Address", Type: "text", Required: true},
			{Key: "use_tls", Label: "Use TLS", Type: "boolean", DefaultValue: "true"},
		},
	},

	// Instant Messaging
	{Type: "slack", Name: "Slack", Category: "messaging", Description: "Send notifications to Slack channels"},
	{Type: "discord", Name: "Discord", Category: "messaging", Description: "Send notifications to Discord servers"},
	{Type: "telegram", Name: "Telegram", Category: "messaging", Description: "Send notifications via Telegram bot"},
	{Type: "whatsapp", Name: "WhatsApp Business", Category: "messaging", Description: "Send notifications via WhatsApp Business API"},
	{Type: "msteams", Name: "Microsoft Teams", Category: "messaging", Description: "Send notifications to Teams channels"},
	{Type: "rocketchat", Name: "Rocket.Chat", Category: "messaging", Description: "Send notifications to Rocket.Chat"},
	{Type: "mattermost", Name: "Mattermost", Category: "messaging", Description: "Send notifications to Mattermost"},
	{Type: "matrix", Name: "Matrix", Category: "messaging", Description: "Send notifications via Matrix protocol"},

	// SMS
	{Type: "twilio", Name: "Twilio SMS", Category: "sms", Description: "Send SMS via Twilio"},
	{Type: "nexmo", Name: "Vonage (Nexmo) SMS", Category: "sms", Description: "Send SMS via Vonage"},
	{Type: "aws_sns", Name: "AWS SNS", Category: "sms", Description: "Send SMS via Amazon SNS"},
	{Type: "plivo", Name: "Plivo SMS", Category: "sms", Description: "Send SMS via Plivo"},
	{Type: "messagebird", Name: "MessageBird SMS", Category: "sms", Description: "Send SMS via MessageBird"},

	// Push Notifications
	{Type: "fcm", Name: "Firebase Cloud Messaging", Category: "push", Description: "Send push notifications via FCM"},
	{Type: "apns", Name: "Apple Push Notification", Category: "push", Description: "Send push notifications to iOS devices"},
	{Type: "onesignal", Name: "OneSignal", Category: "push", Description: "Send push notifications via OneSignal"},
	{Type: "pushover", Name: "Pushover", Category: "push", Description: "Send push notifications via Pushover"},
	{Type: "pushbullet", Name: "Pushbullet", Category: "push", Description: "Send notifications via Pushbullet"},
	{Type: "gotify", Name: "Gotify", Category: "push", Description: "Send push notifications via Gotify"},

	// Webhooks & APIs
	{Type: "webhook", Name: "Generic Webhook", Category: "webhook", Description: "Send notifications to custom webhook URL"},
	{Type: "zapier", Name: "Zapier", Category: "webhook", Description: "Trigger Zapier workflows"},
	{Type: "ifttt", Name: "IFTTT", Category: "webhook", Description: "Trigger IFTTT applets"},
	{Type: "n8n", Name: "n8n", Category: "webhook", Description: "Trigger n8n workflows"},

	// Voice Calls
	{Type: "twilio_voice", Name: "Twilio Voice", Category: "voice", Description: "Make voice calls via Twilio"},
	{Type: "aws_connect", Name: "AWS Connect", Category: "voice", Description: "Make voice calls via Amazon Connect"},

	// Collaboration Tools
	{Type: "jira", Name: "Jira", Category: "collaboration", Description: "Create Jira tickets"},
	{Type: "trello", Name: "Trello", Category: "collaboration", Description: "Create Trello cards"},
	{Type: "asana", Name: "Asana", Category: "collaboration", Description: "Create Asana tasks"},
	{Type: "github", Name: "GitHub Issues", Category: "collaboration", Description: "Create GitHub issues"},
	{Type: "gitlab", Name: "GitLab Issues", Category: "collaboration", Description: "Create GitLab issues"},

	// Monitoring & Logging
	{Type: "pagerduty", Name: "PagerDuty", Category: "monitoring", Description: "Create PagerDuty incidents"},
	{Type: "opsgenie", Name: "Opsgenie", Category: "monitoring", Description: "Create Opsgenie alerts"},
	{Type: "datadog", Name: "Datadog", Category: "monitoring", Description: "Send events to Datadog"},
	{Type: "newrelic", Name: "New Relic", Category: "monitoring", Description: "Send events to New Relic"},
	{Type: "splunk", Name: "Splunk", Category: "monitoring", Description: "Send events to Splunk"},
	{Type: "elastic", Name: "Elasticsearch", Category: "monitoring", Description: "Index notifications in Elasticsearch"},

	// Social Media
	{Type: "twitter", Name: "Twitter", Category: "social", Description: "Post notifications to Twitter"},
	{Type: "mastodon", Name: "Mastodon", Category: "social", Description: "Post notifications to Mastodon"},
}

// NewChannelManager creates a new channel manager
func NewChannelManager(db *sql.DB) *ChannelManager {
	return &ChannelManager{
		db:       db,
		channels: make(map[string]NotificationChannel),
		smtp:     NewSMTPService(db),
	}
}

// RegisterChannel registers a notification channel
func (cm *ChannelManager) RegisterChannel(channel NotificationChannel) {
	cm.channels[channel.GetType()] = channel
}

// InitializeChannels initializes all channels in the database (all disabled by default)
func (cm *ChannelManager) InitializeChannels() error {
	for _, def := range ChannelRegistry {
		// Check if channel already exists
		var exists bool
		err := cm.db.QueryRow("SELECT EXISTS(SELECT 1 FROM notification_channels WHERE channel_type = ?)", def.Type).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check channel existence: %w", err)
		}

		if !exists {
			// Insert channel with disabled state
			configJSON, _ := json.Marshal(map[string]interface{}{
				"description": def.Description,
				"category":    def.Category,
				"fields":      def.ConfigFields,
			})

			_, err = cm.db.Exec(`
				INSERT INTO notification_channels
				(channel_type, channel_name, enabled, state, config, created_at, updated_at)
				VALUES (?, ?, 0, 'disabled', ?, ?, ?)
			`, def.Type, def.Name, string(configJSON), time.Now(), time.Now())

			if err != nil {
				return fmt.Errorf("failed to insert channel %s: %w", def.Type, err)
			}
		}
	}

	return nil
}

// GetChannel returns a channel by type
func (cm *ChannelManager) GetChannel(channelType string) (NotificationChannel, error) {
	channel, exists := cm.channels[channelType]
	if !exists {
		return nil, fmt.Errorf("channel not found: %s", channelType)
	}
	return channel, nil
}

// ListChannels returns all registered channels
func (cm *ChannelManager) ListChannels() []string {
	var types []string
	for t := range cm.channels {
		types = append(types, t)
	}
	return types
}

// ListEnabledChannels returns all enabled channels from database
func (cm *ChannelManager) ListEnabledChannels() ([]string, error) {
	rows, err := cm.db.Query("SELECT channel_type FROM notification_channels WHERE enabled = 1 AND state = 'enabled'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []string
	for rows.Next() {
		var channelType string
		if err := rows.Scan(&channelType); err != nil {
			continue
		}
		channels = append(channels, channelType)
	}

	return channels, nil
}

// GetChannelState returns the state of a channel from database
func (cm *ChannelManager) GetChannelState(channelType string) (string, error) {
	var state string
	err := cm.db.QueryRow("SELECT state FROM notification_channels WHERE channel_type = ?", channelType).Scan(&state)
	return state, err
}

// UpdateChannelState updates the state of a channel
func (cm *ChannelManager) UpdateChannelState(channelType, state string) error {
	_, err := cm.db.Exec(`
		UPDATE notification_channels
		SET state = ?, updated_at = ?
		WHERE channel_type = ?
	`, state, time.Now(), channelType)
	return err
}

// EnableChannel enables a channel
func (cm *ChannelManager) EnableChannel(channelType string) error {
	_, err := cm.db.Exec(`
		UPDATE notification_channels
		SET enabled = 1, state = 'enabled', updated_at = ?
		WHERE channel_type = ?
	`, time.Now(), channelType)
	return err
}

// DisableChannel disables a channel
func (cm *ChannelManager) DisableChannel(channelType string) error {
	_, err := cm.db.Exec(`
		UPDATE notification_channels
		SET enabled = 0, state = 'disabled', updated_at = ?
		WHERE channel_type = ?
	`, time.Now(), channelType)
	return err
}

// TestChannel tests a channel configuration
func (cm *ChannelManager) TestChannel(channelType, recipient string) error {
	channel, err := cm.GetChannel(channelType)
	if err != nil {
		return err
	}

	// Update state to testing
	cm.UpdateChannelState(channelType, "testing")

	// Run test
	err = channel.Test(recipient)

	// Update state based on result
	now := time.Now()
	if err != nil {
		_, dbErr := cm.db.Exec(`
			UPDATE notification_channels
			SET state = 'failed',
				last_test_at = ?,
				last_test_result = 'failed',
				last_error = ?,
				failure_count = failure_count + 1,
				updated_at = ?
			WHERE channel_type = ?
		`, now, err.Error(), now, channelType)
		if dbErr != nil {
			return fmt.Errorf("test failed: %w, db update error: %v", err, dbErr)
		}
		return err
	}

	// Test succeeded
	_, err = cm.db.Exec(`
		UPDATE notification_channels
		SET state = 'enabled',
			enabled = 1,
			last_test_at = ?,
			last_test_result = 'success',
			last_success_at = ?,
			last_error = NULL,
			failure_count = 0,
			updated_at = ?
		WHERE channel_type = ?
	`, now, now, now, channelType)

	return err
}

// RecordSuccess records a successful delivery
func (cm *ChannelManager) RecordSuccess(channelType string) error {
	_, err := cm.db.Exec(`
		UPDATE notification_channels
		SET last_success_at = ?,
			failure_count = 0,
			updated_at = ?
		WHERE channel_type = ?
	`, time.Now(), time.Now(), channelType)
	return err
}

// RecordFailure records a failed delivery
func (cm *ChannelManager) RecordFailure(channelType string, errorMsg string) error {
	_, err := cm.db.Exec(`
		UPDATE notification_channels
		SET last_error = ?,
			failure_count = failure_count + 1,
			state = CASE
				WHEN failure_count + 1 >= 5 THEN 'failed'
				ELSE state
			END,
			updated_at = ?
		WHERE channel_type = ?
	`, errorMsg, time.Now(), channelType)
	return err
}

// GetChannelStats returns statistics for a channel
func (cm *ChannelManager) GetChannelStats(channelType string) (map[string]interface{}, error) {
	var enabled bool
	var state, lastError sql.NullString
	var lastTestAt, lastSuccessAt sql.NullTime
	var failureCount int

	err := cm.db.QueryRow(`
		SELECT enabled, state, last_test_at, last_success_at, last_error, failure_count
		FROM notification_channels
		WHERE channel_type = ?
	`, channelType).Scan(&enabled, &state, &lastTestAt, &lastSuccessAt, &lastError, &failureCount)

	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"enabled":         enabled,
		"state":           state.String,
		"failure_count":   failureCount,
		"last_test_at":    nil,
		"last_success_at": nil,
		"last_error":      nil,
	}

	if lastTestAt.Valid {
		stats["last_test_at"] = lastTestAt.Time
	}
	if lastSuccessAt.Valid {
		stats["last_success_at"] = lastSuccessAt.Time
	}
	if lastError.Valid {
		stats["last_error"] = lastError.String
	}

	return stats, nil
}
