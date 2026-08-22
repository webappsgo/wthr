package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// DefaultTemplateName is the template_name that marks a channel's fallback
// template. server_notification_templates carries no is_default column, so
// default-ness is derived from this reserved name instead of stored.
const DefaultTemplateName = "default"

// TemplateEngine handles notification template processing
type TemplateEngine struct {
	db *sql.DB
}

// NotificationTemplate represents a notification template. SubjectTemplate and
// BodyTemplate map to the subject and body columns of
// server_notification_templates; IsDefault is derived on read from
// TemplateName and is never persisted.
type NotificationTemplate struct {
	ID              int
	ChannelType     string
	TemplateName    string
	TemplateType    string
	SubjectTemplate string
	BodyTemplate    string
	Variables       map[string]string
	IsDefault       bool
}

// NewTemplateEngine creates a new template engine
func NewTemplateEngine(db *sql.DB) *TemplateEngine {
	return &TemplateEngine{db: db}
}

// Render renders a template with the given variables
func (te *TemplateEngine) Render(templateContent string, variables map[string]interface{}) (string, error) {
	// Add helper functions
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(s string) string { return cases.Title(language.English).String(s) },
		"now":   time.Now,
		"formatDate": func(t time.Time, format string) string {
			return t.Format(format)
		},
		"default": func(defaultValue, value interface{}) interface{} {
			if value == nil || value == "" {
				return defaultValue
			}
			return value
		},
	}

	tmpl, err := template.New("notification").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GetTemplate retrieves a template from the database
func (te *TemplateEngine) GetTemplate(channelType, templateName string) (*NotificationTemplate, error) {
	var t NotificationTemplate
	var subject sql.NullString
	var variablesJSON sql.NullString

	err := database.QueryRowContext(context.Background(), te.db, database.TimeoutSimpleSelect, `
		SELECT id, channel_type, template_name, template_type,
		       subject, body, variables
		FROM server_notification_templates
		WHERE channel_type = ? AND template_name = ?
	`, channelType, templateName).Scan(
		&t.ID, &t.ChannelType, &t.TemplateName, &t.TemplateType,
		&subject, &t.BodyTemplate, &variablesJSON,
	)

	if err != nil {
		// Try to get default template
		return te.GetDefaultTemplate(channelType)
	}

	t.SubjectTemplate = subject.String
	t.IsDefault = t.TemplateName == DefaultTemplateName

	if variablesJSON.Valid {
		if err := json.Unmarshal([]byte(variablesJSON.String), &t.Variables); err != nil {
			log.Printf("WARNING: GetTemplate: failed to unmarshal variables for template %q (%s): %v", templateName, channelType, err)
		}
	}

	return &t, nil
}

// GetDefaultTemplate retrieves the default template for a channel type
func (te *TemplateEngine) GetDefaultTemplate(channelType string) (*NotificationTemplate, error) {
	var t NotificationTemplate
	var subject sql.NullString
	var variablesJSON sql.NullString

	err := database.QueryRowContext(context.Background(), te.db, database.TimeoutSimpleSelect, `
		SELECT id, channel_type, template_name, template_type,
		       subject, body, variables
		FROM server_notification_templates
		WHERE channel_type = ? AND template_name = ?
		LIMIT 1
	`, channelType, DefaultTemplateName).Scan(
		&t.ID, &t.ChannelType, &t.TemplateName, &t.TemplateType,
		&subject, &t.BodyTemplate, &variablesJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("no default template found for channel %s", channelType)
	}

	t.SubjectTemplate = subject.String
	t.IsDefault = true

	if variablesJSON.Valid {
		if err := json.Unmarshal([]byte(variablesJSON.String), &t.Variables); err != nil {
			log.Printf("WARNING: GetDefaultTemplate: failed to unmarshal variables for channel %s: %v", channelType, err)
		}
	}

	return &t, nil
}

// CreateTemplate creates a new template
func (te *TemplateEngine) CreateTemplate(t *NotificationTemplate) error {
	variablesJSON, err := json.Marshal(t.Variables)
	if err != nil {
		return fmt.Errorf("failed to marshal template variables: %w", err)
	}

	now := dbtime.FormatSQLTimestamp(time.Now())

	_, err = database.ExecContext(context.Background(), te.db, database.TimeoutWrite, `
		INSERT INTO server_notification_templates
		(channel_type, template_name, template_type, subject,
		 body, variables, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ChannelType, t.TemplateName, t.TemplateType, t.SubjectTemplate,
		t.BodyTemplate, string(variablesJSON), now, now)

	return err
}

// UpdateTemplate updates an existing template
func (te *TemplateEngine) UpdateTemplate(t *NotificationTemplate) error {
	variablesJSON, err := json.Marshal(t.Variables)
	if err != nil {
		return fmt.Errorf("failed to marshal template variables: %w", err)
	}

	_, err = database.ExecContext(context.Background(), te.db, database.TimeoutWrite, `
		UPDATE server_notification_templates
		SET template_name = ?,
		    template_type = ?,
		    subject = ?,
		    body = ?,
		    variables = ?,
		    updated_at = ?
		WHERE id = ?
	`, t.TemplateName, t.TemplateType, t.SubjectTemplate,
		t.BodyTemplate, string(variablesJSON), dbtime.FormatSQLTimestamp(time.Now()), t.ID)

	return err
}

// DeleteTemplate deletes a template
func (te *TemplateEngine) DeleteTemplate(id int) error {
	_, err := database.ExecContext(context.Background(), te.db, database.TimeoutWrite, "DELETE FROM server_notification_templates WHERE id = ?", id)
	return err
}

// ListTemplates lists all templates for a channel type
func (te *TemplateEngine) ListTemplates(channelType string) ([]*NotificationTemplate, error) {
	// The channel's default template is the one named DefaultTemplateName, so
	// the CASE expression reproduces the "default first, then alphabetical"
	// ordering the stored is_default column used to provide.
	rows, err := database.QueryContext(context.Background(), te.db, database.TimeoutSimpleSelect, `
		SELECT id, channel_type, template_name, template_type,
		       subject, body, variables
		FROM server_notification_templates
		WHERE channel_type = ?
		ORDER BY CASE WHEN template_name = ? THEN 0 ELSE 1 END, template_name ASC
	`, channelType, DefaultTemplateName)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*NotificationTemplate
	for rows.Next() {
		var t NotificationTemplate
		var subject sql.NullString
		var variablesJSON sql.NullString

		err := rows.Scan(
			&t.ID, &t.ChannelType, &t.TemplateName, &t.TemplateType,
			&subject, &t.BodyTemplate, &variablesJSON,
		)
		if err != nil {
			continue
		}

		t.SubjectTemplate = subject.String
		t.IsDefault = t.TemplateName == DefaultTemplateName

		if variablesJSON.Valid {
			if err := json.Unmarshal([]byte(variablesJSON.String), &t.Variables); err != nil {
				log.Printf("WARNING: ListTemplates: failed to unmarshal variables for template %q (%s): %v", t.TemplateName, channelType, err)
			}
		}

		templates = append(templates, &t)
	}

	return templates, nil
}

// RenderTemplate renders a template with variables
func (te *TemplateEngine) RenderTemplate(channelType, templateName string, variables map[string]interface{}) (subject, body string, err error) {
	tmpl, err := te.GetTemplate(channelType, templateName)
	if err != nil {
		return "", "", err
	}

	// Render subject
	if tmpl.SubjectTemplate != "" {
		subject, err = te.Render(tmpl.SubjectTemplate, variables)
		if err != nil {
			return "", "", fmt.Errorf("failed to render subject: %w", err)
		}
	}

	// Render body
	body, err = te.Render(tmpl.BodyTemplate, variables)
	if err != nil {
		return "", "", fmt.Errorf("failed to render body: %w", err)
	}

	return subject, body, nil
}

// InitializeDefaultTemplates creates default templates for common notification types
func (te *TemplateEngine) InitializeDefaultTemplates() error {
	defaultTemplates := []NotificationTemplate{
		// Email default template
		{
			ChannelType:     "email",
			TemplateName:    "default",
			TemplateType:    "general",
			SubjectTemplate: "{{.Subject}}",
			BodyTemplate: `
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
		.container { max-width: 600px; margin: 0 auto; padding: 20px; }
		.header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
		.content { padding: 20px; background-color: #f9f9f9; }
		.footer { padding: 10px; text-align: center; font-size: 12px; color: #666; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>{{default "Notification" .Title}}</h1>
		</div>
		<div class="content">
			{{.Body}}
		</div>
		<div class="footer">
			<p>Sent from Weather at {{now.Format "2006-01-02 15:04:05"}}</p>
		</div>
	</div>
</body>
</html>
`,
			IsDefault: true,
		},

		// Weather alert template
		{
			ChannelType:     "email",
			TemplateName:    "weather_alert",
			TemplateType:    "alert",
			SubjectTemplate: "Weather Alert: {{.AlertType}} for {{.Location}}",
			BodyTemplate: `
<html>
<body>
	<h2>⚠️ Weather Alert</h2>
	<p><strong>Type:</strong> {{.AlertType}}</p>
	<p><strong>Location:</strong> {{.Location}}</p>
	<p><strong>Severity:</strong> {{.Severity}}</p>
	<p><strong>Message:</strong></p>
	<p>{{.Message}}</p>
	<p><em>Issued: {{.IssuedAt}}</em></p>
	{{if .ExpiresAt}}<p><em>Expires: {{.ExpiresAt}}</em></p>{{end}}
</body>
</html>
`,
			IsDefault: false,
		},

		// System notification template
		{
			ChannelType:     "email",
			TemplateName:    "system",
			TemplateType:    "system",
			SubjectTemplate: "[{{upper .Priority}}] {{.Subject}}",
			BodyTemplate: `
<html>
<body>
	<h2>System Notification</h2>
	<p><strong>Priority:</strong> {{upper .Priority}}</p>
	<p><strong>Component:</strong> {{.Component}}</p>
	<p><strong>Message:</strong></p>
	<p>{{.Message}}</p>
	{{if .Details}}<pre>{{.Details}}</pre>{{end}}
	<p><em>{{now.Format "2006-01-02 15:04:05 MST"}}</em></p>
</body>
</html>
`,
			IsDefault: false,
		},

		// Daily forecast email template
		{
			ChannelType:     "email",
			TemplateName:    "daily_forecast",
			TemplateType:    "forecast",
			SubjectTemplate: "🌤️ Daily Forecast for {{.Location}}",
			BodyTemplate: `
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; margin: 0; padding: 0; background-color: #f4f4f4; }
		.container { max-width: 600px; margin: 20px auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		.header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
		.emoji { font-size: 48px; margin: 10px 0; }
		.content { padding: 30px; }
		.weather-info { background: #f8f9fa; padding: 20px; border-radius: 8px; margin: 15px 0; }
		.temp { font-size: 36px; font-weight: bold; color: #667eea; }
		.footer { background: #f8f9fa; padding: 20px; text-align: center; color: #666; font-size: 12px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Daily Weather Forecast</h1>
			<div class="emoji">{{.Emoji}}</div>
			<h2>{{.Location}}</h2>
			<p>{{.Date}}</p>
		</div>
		<div class="content">
			<div class="weather-info">
				<p class="temp">{{.Temperature}}</p>
				<p><strong>Conditions:</strong> {{.Condition}}</p>
			</div>
			<p>Have a great day! 🌈</p>
		</div>
		<div class="footer">
			<p>Weather • {{now.Format "Monday, January 2, 2006"}}</p>
		</div>
	</div>
</body>
</html>
`,
			IsDefault: false,
		},

		// Welcome email template
		{
			ChannelType:     "email",
			TemplateName:    "welcome",
			TemplateType:    "account",
			SubjectTemplate: "Welcome to Weather, {{.Name}}!",
			BodyTemplate: `
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0; }
		.container { max-width: 600px; margin: 0 auto; }
		.header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 40px 20px; text-align: center; }
		.content { padding: 40px 20px; background: white; }
		.button { display: inline-block; padding: 12px 30px; background: #667eea; color: white; text-decoration: none; border-radius: 5px; margin: 20px 0; }
		.features { background: #f8f9fa; padding: 20px; border-radius: 8px; margin: 20px 0; }
		.feature-item { margin: 10px 0; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Welcome to Weather! 🌤️</h1>
		</div>
		<div class="content">
			<p>Hi {{.Name}},</p>
			<p>Thanks for signing up! We're excited to help you stay informed about the weather in your area.</p>

			<div class="features">
				<h3>What you can do:</h3>
				<div class="feature-item">☀️ Get real-time weather updates</div>
				<div class="feature-item">📍 Track multiple locations</div>
				<div class="feature-item">⚠️ Receive severe weather alerts</div>
				<div class="feature-item">📧 Daily forecast emails</div>
			</div>

			<p style="text-align: center;">
				<a href="{{.DashboardURL}}" class="button">Go to Dashboard</a>
			</p>

			<p>Need help getting started? Check out our <a href="{{.HelpURL}}">help center</a>.</p>

			<p>Happy weather tracking!<br>The Weather Team</p>
		</div>
	</div>
</body>
</html>
`,
			IsDefault: false,
		},

		// Password reset email template
		{
			ChannelType:     "email",
			TemplateName:    "password_reset",
			TemplateType:    "account",
			SubjectTemplate: "Reset Your Password - Weather",
			BodyTemplate: `
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
		.container { max-width: 600px; margin: 0 auto; padding: 20px; }
		.header { background: #667eea; color: white; padding: 20px; text-align: center; border-radius: 8px 8px 0 0; }
		.content { background: white; padding: 30px; border: 1px solid #ddd; border-top: none; border-radius: 0 0 8px 8px; }
		.button { display: inline-block; padding: 12px 30px; background: #667eea; color: white; text-decoration: none; border-radius: 5px; margin: 20px 0; }
		.warning { background: #fff3cd; border-left: 4px solid #ffc107; padding: 15px; margin: 20px 0; }
		.code { background: #f8f9fa; padding: 15px; font-family: monospace; font-size: 24px; text-align: center; letter-spacing: 3px; border-radius: 5px; margin: 20px 0; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>🔐 Password Reset Request</h1>
		</div>
		<div class="content">
			<p>Hi {{.Name}},</p>
			<p>We received a request to reset your password for your Weather account.</p>

			{{if .ResetURL}}
			<p style="text-align: center;">
				<a href="{{.ResetURL}}" class="button">Reset Password</a>
			</p>
			<p style="color: #666; font-size: 12px;">Or copy and paste this link: {{.ResetURL}}</p>
			{{end}}

			{{if .ResetCode}}
			<p>Use this code to reset your password:</p>
			<div class="code">{{.ResetCode}}</div>
			{{end}}

			<div class="warning">
				<strong>⚠️ Security Notice:</strong> This link will expire in {{default "1 hour" .ExpiresIn}}.
				If you didn't request this reset, please ignore this email and your password will remain unchanged.
			</div>

			<p style="color: #666; font-size: 12px;">
				Sent at {{now.Format "2006-01-02 15:04:05 MST"}}
			</p>
		</div>
	</div>
</body>
</html>
`,
			IsDefault: false,
		},

		// Slack default template
		{
			ChannelType:     "slack",
			TemplateName:    "default",
			TemplateType:    "general",
			SubjectTemplate: "",
			BodyTemplate: `{
	"text": "{{.Subject}}",
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "{{default "Notification" .Title}}"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "{{.Body}}"
			}
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "mrkdwn",
					"text": "Sent at {{now.Format "15:04:05 MST"}}"
				}
			]
		}
	]
}`,
			IsDefault: true,
		},

		// Slack weather alert template
		{
			ChannelType:     "slack",
			TemplateName:    "weather_alert",
			TemplateType:    "alert",
			SubjectTemplate: "",
			BodyTemplate: `{
	"text": "⚠️ Weather Alert: {{.AlertType}} for {{.Location}}",
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "⚠️ Weather Alert",
				"emoji": true
			}
		},
		{
			"type": "section",
			"fields": [
				{
					"type": "mrkdwn",
					"text": "*Type:*\n{{.AlertType}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Location:*\n{{.Location}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Severity:*\n{{upper .Severity}}"
				},
				{
					"type": "mrkdwn",
					"text": "*Issued:*\n{{.IssuedAt}}"
				}
			]
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*Message:*\n{{.Message}}"
			}
		}
	]
}`,
			IsDefault: false,
		},

		// Discord default template
		{
			ChannelType:     "discord",
			TemplateName:    "default",
			TemplateType:    "general",
			SubjectTemplate: "",
			BodyTemplate: `{
	"embeds": [{
		"title": "{{default "Notification" .Title}}",
		"description": "{{.Body}}",
		"color": 6737151,
		"timestamp": "{{now.Format "2006-01-02T15:04:05Z07:00"}}",
		"footer": {
			"text": "Weather"
		}
	}]
}`,
			IsDefault: true,
		},

		// Discord weather alert template
		{
			ChannelType:     "discord",
			TemplateName:    "weather_alert",
			TemplateType:    "alert",
			SubjectTemplate: "",
			BodyTemplate: `{
	"embeds": [{
		"title": "⚠️ Weather Alert: {{.AlertType}}",
		"description": "{{.Message}}",
		"color": 16744192,
		"fields": [
			{
				"name": "Location",
				"value": "{{.Location}}",
				"inline": true
			},
			{
				"name": "Severity",
				"value": "{{upper .Severity}}",
				"inline": true
			},
			{
				"name": "Coordinates",
				"value": "{{.Coordinates}}",
				"inline": false
			}
		],
		"timestamp": "{{now.Format "2006-01-02T15:04:05Z07:00"}}",
		"footer": {
			"text": "Issued at {{.IssuedAt}}"
		}
	}]
}`,
			IsDefault: false,
		},

		// SMS default template
		{
			ChannelType:     "sms",
			TemplateName:    "default",
			TemplateType:    "general",
			SubjectTemplate: "",
			BodyTemplate:    `{{.Subject}}: {{.Body}}`,
			IsDefault:       true,
		},

		// SMS weather alert template
		{
			ChannelType:     "sms",
			TemplateName:    "weather_alert",
			TemplateType:    "alert",
			SubjectTemplate: "",
			BodyTemplate:    `⚠️ {{.AlertType}} alert for {{.Location}}. {{.Message}}`,
			IsDefault:       false,
		},

		// SMS daily forecast template
		{
			ChannelType:     "sms",
			TemplateName:    "daily_forecast",
			TemplateType:    "forecast",
			SubjectTemplate: "",
			BodyTemplate:    `{{.Emoji}} Weather for {{.Location}}: {{.Temperature}}, {{.Condition}}. Have a great day!`,
			IsDefault:       false,
		},

		// Push notification default template
		{
			ChannelType:     "push",
			TemplateName:    "default",
			TemplateType:    "general",
			SubjectTemplate: "{{.Title}}",
			BodyTemplate:    `{{.Body}}`,
			IsDefault:       true,
		},

		// Push weather alert template
		{
			ChannelType:     "push",
			TemplateName:    "weather_alert",
			TemplateType:    "alert",
			SubjectTemplate: "⚠️ {{.AlertType}} Alert",
			BodyTemplate:    `{{.Message}} ({{.Location}})`,
			IsDefault:       false,
		},

		// Webhook default template
		{
			ChannelType:     "webhook",
			TemplateName:    "default",
			TemplateType:    "general",
			SubjectTemplate: "",
			BodyTemplate: `{
	"event": "notification",
	"timestamp": "{{now.Format "2006-01-02T15:04:05Z07:00"}}",
	"subject": "{{.Subject}}",
	"body": "{{.Body}}",
	"priority": {{default "2" .Priority}}
}`,
			IsDefault: true,
		},

		// Webhook weather alert template
		{
			ChannelType:     "webhook",
			TemplateName:    "weather_alert",
			TemplateType:    "alert",
			SubjectTemplate: "",
			BodyTemplate: `{
	"event": "weather_alert",
	"timestamp": "{{now.Format "2006-01-02T15:04:05Z07:00"}}",
	"alert": {
		"type": "{{.AlertType}}",
		"severity": "{{.Severity}}",
		"location": "{{.Location}}",
		"message": "{{.Message}}",
		"coordinates": "{{.Coordinates}}",
		"issued_at": "{{.IssuedAt}}"
		{{if .ExpiresAt}},"expires_at": "{{.ExpiresAt}}"{{end}}
	}
}`,
			IsDefault: false,
		},
	}

	for _, tmpl := range defaultTemplates {
		// Check if template already exists
		var exists bool
		err := database.QueryRowContext(context.Background(), te.db, database.TimeoutSimpleSelect, `
			SELECT EXISTS(
				SELECT 1 FROM server_notification_templates
				WHERE channel_type = ? AND template_name = ?
			)
		`, tmpl.ChannelType, tmpl.TemplateName).Scan(&exists)

		if err != nil {
			return err
		}

		if !exists {
			if err := te.CreateTemplate(&tmpl); err != nil {
				return fmt.Errorf("failed to create template %s/%s: %w",
					tmpl.ChannelType, tmpl.TemplateName, err)
			}
		}
	}

	return nil
}

// ValidateTemplate validates a template for syntax errors
func (te *TemplateEngine) ValidateTemplate(templateContent string) error {
	funcMap := template.FuncMap{
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      func(s string) string { return cases.Title(language.English).String(s) },
		"now":        time.Now,
		"formatDate": func(t time.Time, format string) string { return "" },
		"default":    func(a, b interface{}) interface{} { return a },
	}

	_, err := template.New("test").Funcs(funcMap).Parse(templateContent)
	return err
}
