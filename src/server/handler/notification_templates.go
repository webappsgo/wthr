package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
)

// NotificationTemplateHandler handles notification template management
type NotificationTemplateHandler struct {
	DB             *sql.DB
	TemplateEngine *service.TemplateEngine
}

// NewNotificationTemplateHandler creates a new template handler
func NewNotificationTemplateHandler(db *sql.DB) *NotificationTemplateHandler {
	return &NotificationTemplateHandler{
		DB:             db,
		TemplateEngine: service.NewTemplateEngine(db),
	}
}

// ListTemplates returns all templates
func (h *NotificationTemplateHandler) ListTemplates(c *gin.Context) {
	channelType := c.Query("channel_type")

	var query string
	var args []interface{}

	// server_notification_templates has no is_default column: the channel's
	// default template is the one named service.DefaultTemplateName, so the
	// CASE expression reproduces the old "default first" ordering.
	if channelType != "" {
		query = `
			SELECT id, channel_type, template_name, template_type,
			       subject, body, variables,
			       created_at, updated_at
			FROM server_notification_templates
			WHERE channel_type = ?
			ORDER BY CASE WHEN template_name = ? THEN 0 ELSE 1 END, template_name ASC
		`
		args = append(args, channelType, service.DefaultTemplateName)
	} else {
		query = `
			SELECT id, channel_type, template_name, template_type,
			       subject, body, variables,
			       created_at, updated_at
			FROM server_notification_templates
			ORDER BY channel_type, CASE WHEN template_name = ? THEN 0 ELSE 1 END, template_name ASC
		`
		args = append(args, service.DefaultTemplateName)
	}

	rows, err := database.QueryContext(context.Background(), h.DB, database.TimeoutSimpleSelect, query, args...)
	if err != nil {
		log.Printf("ERROR: ListTemplates: failed to query templates: %v", err)
		RespondError(c, http.StatusInternalServerError, ErrDatabaseError, "Failed to fetch templates")
		return
	}
	defer rows.Close()

	var templates []gin.H
	for rows.Next() {
		var id int
		var channelType, templateName, templateType string
		var subjectTemplate sql.NullString
		var bodyTemplate string
		var variables sql.NullString
		var createdAt, updatedAt sql.NullString

		err := rows.Scan(&id, &channelType, &templateName, &templateType,
			&subjectTemplate, &bodyTemplate, &variables,
			&createdAt, &updatedAt)
		if err != nil {
			continue
		}

		tmpl := gin.H{
			"id":               id,
			"channel_type":     channelType,
			"template_name":    templateName,
			"template_type":    templateType,
			"subject_template": subjectTemplate.String,
			"body_template":    bodyTemplate,
			"is_default":       templateName == service.DefaultTemplateName,
			"created_at":       nil,
			"updated_at":       nil,
		}

		if variables.Valid {
			var varsMap map[string]interface{}
			json.Unmarshal([]byte(variables.String), &varsMap)
			tmpl["variables"] = varsMap
		}

		if createdAt.Valid {
			if parsed, ok := dbtime.ParseStoredTimestamp(createdAt.String); ok {
				tmpl["created_at"] = parsed
			}
		}
		if updatedAt.Valid {
			if parsed, ok := dbtime.ParseStoredTimestamp(updatedAt.String); ok {
				tmpl["updated_at"] = parsed
			}
		}

		templates = append(templates, tmpl)
	}

	// Group by channel type
	grouped := make(map[string][]gin.H)
	for _, tmpl := range templates {
		ct := tmpl["channel_type"].(string)
		grouped[ct] = append(grouped[ct], tmpl)
	}

	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
		"grouped":   grouped,
		"total":     len(templates),
	})
}

// GetTemplate returns a specific template
func (h *NotificationTemplateHandler) GetTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var channelType, templateName, templateType string
	var subjectTemplate sql.NullString
	var bodyTemplate string
	var variables sql.NullString
	var createdAt, updatedAt sql.NullString

	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT channel_type, template_name, template_type,
		       subject, body, variables,
		       created_at, updated_at
		FROM server_notification_templates
		WHERE id = ?
	`, id).Scan(&channelType, &templateName, &templateType,
		&subjectTemplate, &bodyTemplate, &variables,
		&createdAt, &updatedAt)

	if err != nil {
		RespondError(c, http.StatusNotFound, ErrNotFound, "Template not found")
		return
	}

	tmpl := gin.H{
		"id":               id,
		"channel_type":     channelType,
		"template_name":    templateName,
		"template_type":    templateType,
		"subject_template": subjectTemplate.String,
		"body_template":    bodyTemplate,
		"is_default":       templateName == service.DefaultTemplateName,
		"created_at":       nil,
		"updated_at":       nil,
	}

	if variables.Valid {
		var varsMap map[string]interface{}
		json.Unmarshal([]byte(variables.String), &varsMap)
		tmpl["variables"] = varsMap
	}

	if createdAt.Valid {
		if parsed, ok := dbtime.ParseStoredTimestamp(createdAt.String); ok {
			tmpl["created_at"] = parsed
		}
	}
	if updatedAt.Valid {
		if parsed, ok := dbtime.ParseStoredTimestamp(updatedAt.String); ok {
			tmpl["updated_at"] = parsed
		}
	}

	c.JSON(http.StatusOK, tmpl)
}

// CreateTemplate creates a new template
func (h *NotificationTemplateHandler) CreateTemplate(c *gin.Context) {
	var req struct {
		ChannelType     string                 `json:"channel_type" binding:"required"`
		TemplateName    string                 `json:"template_name" binding:"required"`
		TemplateType    string                 `json:"template_type" binding:"required"`
		SubjectTemplate string                 `json:"subject_template"`
		BodyTemplate    string                 `json:"body_template" binding:"required"`
		Variables       map[string]interface{} `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	// Validate template syntax
	if err := h.TemplateEngine.ValidateTemplate(req.BodyTemplate); err != nil {
		log.Printf("WARNING: CreateTemplate: invalid template syntax: %v", err)
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid template syntax")
		return
	}

	if req.SubjectTemplate != "" {
		if err := h.TemplateEngine.ValidateTemplate(req.SubjectTemplate); err != nil {
			log.Printf("WARNING: CreateTemplate: invalid subject template syntax: %v", err)
			RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid subject template syntax")
			return
		}
	}

	variablesJSON, _ := json.Marshal(req.Variables)
	now := dbtime.FormatSQLTimestamp(time.Now())

	// A channel's default template is the one named service.DefaultTemplateName
	// and the table's UNIQUE(channel_type, template_name, template_type) keeps
	// it singular, so there is no stored default flag to set or clear here.
	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO server_notification_templates
		(channel_type, template_name, template_type, subject,
		 body, variables, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ChannelType, req.TemplateName, req.TemplateType,
		req.SubjectTemplate, req.BodyTemplate, string(variablesJSON), now, now)

	if err != nil {
		log.Printf("ERROR: CreateTemplate: failed to insert template: %v", err)
		RespondError(c, http.StatusInternalServerError, ErrDatabaseError, "Failed to create template")
		return
	}

	id, _ := result.LastInsertId()
	RespondCreated(c, "Template created successfully", strconv.FormatInt(id, 10))
}

// UpdateTemplate updates a template
func (h *NotificationTemplateHandler) UpdateTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var req struct {
		TemplateName    string                 `json:"template_name"`
		TemplateType    string                 `json:"template_type"`
		SubjectTemplate string                 `json:"subject_template"`
		BodyTemplate    string                 `json:"body_template"`
		Variables       map[string]interface{} `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	// Validate template syntax
	if req.BodyTemplate != "" {
		if err := h.TemplateEngine.ValidateTemplate(req.BodyTemplate); err != nil {
			log.Printf("WARNING: UpdateTemplate: invalid template syntax: %v", err)
			RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid template syntax")
			return
		}
	}

	if req.SubjectTemplate != "" {
		if err := h.TemplateEngine.ValidateTemplate(req.SubjectTemplate); err != nil {
			log.Printf("WARNING: UpdateTemplate: invalid subject template syntax: %v", err)
			RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid subject template syntax")
			return
		}
	}

	// Confirm the template exists before updating it
	var channelType string
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT channel_type FROM server_notification_templates WHERE id = ?", id).Scan(&channelType)
	if err != nil {
		RespondError(c, http.StatusNotFound, ErrNotFound, "Template not found")
		return
	}

	variablesJSON, _ := json.Marshal(req.Variables)

	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		UPDATE server_notification_templates
		SET template_name = ?,
		    template_type = ?,
		    subject = ?,
		    body = ?,
		    variables = ?,
		    updated_at = ?
		WHERE id = ?
	`, req.TemplateName, req.TemplateType, req.SubjectTemplate,
		req.BodyTemplate, string(variablesJSON), dbtime.FormatSQLTimestamp(time.Now()), id)

	if err != nil {
		log.Printf("ERROR: UpdateTemplate: failed to update template: %v", err)
		RespondError(c, http.StatusInternalServerError, ErrDatabaseError, "Failed to update template")
		return
	}

	RespondSuccess(c, "Template updated successfully")
}

// DeleteTemplate deletes a template
func (h *NotificationTemplateHandler) DeleteTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	// A channel's default template is the one named service.DefaultTemplateName
	var templateName string
	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT template_name FROM server_notification_templates WHERE id = ?", id).Scan(&templateName)
	if err != nil {
		RespondError(c, http.StatusNotFound, ErrNotFound, "Template not found")
		return
	}

	if templateName == service.DefaultTemplateName {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Cannot delete default template")
		return
	}

	_, err = database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, "DELETE FROM server_notification_templates WHERE id = ?", id)
	if err != nil {
		log.Printf("ERROR: DeleteTemplate: failed to delete template: %v", err)
		RespondError(c, http.StatusInternalServerError, ErrDatabaseError, "Failed to delete template")
		return
	}

	RespondSuccess(c, "Template deleted successfully")
}

// PreviewTemplate renders a template with sample data
func (h *NotificationTemplateHandler) PreviewTemplate(c *gin.Context) {
	var req struct {
		SubjectTemplate string                 `json:"subject_template"`
		BodyTemplate    string                 `json:"body_template" binding:"required"`
		Variables       map[string]interface{} `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	// Render subject
	var subject string
	var err error
	if req.SubjectTemplate != "" {
		subject, err = h.TemplateEngine.Render(req.SubjectTemplate, req.Variables)
		if err != nil {
			log.Printf("WARNING: PreviewTemplate: failed to render subject: %v", err)
			RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Failed to render subject")
			return
		}
	}

	// Render body
	body, err := h.TemplateEngine.Render(req.BodyTemplate, req.Variables)
	if err != nil {
		log.Printf("WARNING: PreviewTemplate: failed to render body: %v", err)
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Failed to render body")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subject": subject,
		"body":    body,
	})
}

// CloneTemplate creates a copy of a template
func (h *NotificationTemplateHandler) CloneTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var req struct {
		NewName string `json:"new_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid request")
		return
	}

	// Get original template
	var channelType, templateType, bodyTemplate string
	var subjectTemplate sql.NullString
	var variables sql.NullString

	err := database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, `
		SELECT channel_type, template_type, subject, body, variables
		FROM server_notification_templates
		WHERE id = ?
	`, id).Scan(&channelType, &templateType, &subjectTemplate, &bodyTemplate, &variables)

	if err != nil {
		RespondError(c, http.StatusNotFound, ErrNotFound, "Template not found")
		return
	}

	// A clone is never the channel default: it is stored under the caller's new
	// name, and only service.DefaultTemplateName marks the default template.
	if req.NewName == service.DefaultTemplateName {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Cannot clone a template onto the reserved default name")
		return
	}

	now := dbtime.FormatSQLTimestamp(time.Now())

	result, err := database.ExecContext(context.Background(), h.DB, database.TimeoutWrite, `
		INSERT INTO server_notification_templates
		(channel_type, template_name, template_type, subject,
		 body, variables, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, channelType, req.NewName, templateType, subjectTemplate.String, bodyTemplate,
		variables.String, now, now)

	if err != nil {
		log.Printf("ERROR: CloneTemplate: failed to clone template: %v", err)
		RespondError(c, http.StatusInternalServerError, ErrDatabaseError, "Failed to clone template")
		return
	}

	newID, _ := result.LastInsertId()
	RespondCreated(c, "Template cloned successfully", strconv.FormatInt(newID, 10))
}

// InitializeDefaults initializes default templates
func (h *NotificationTemplateHandler) InitializeDefaults(c *gin.Context) {
	err := h.TemplateEngine.InitializeDefaultTemplates()
	if err != nil {
		log.Printf("ERROR: InitializeDefaults: failed to initialize default templates: %v", err)
		RespondError(c, http.StatusInternalServerError, ErrInternal, "Failed to initialize default templates")
		return
	}

	RespondSuccess(c, "Default templates initialized successfully")
}

// GetTemplateVariables returns available template variables
func (h *NotificationTemplateHandler) GetTemplateVariables(c *gin.Context) {
	variables := gin.H{
		"common": []gin.H{
			{"name": "Title", "description": "Notification title"},
			{"name": "Subject", "description": "Notification subject"},
			{"name": "Body", "description": "Main notification body"},
			{"name": "Message", "description": "Message content"},
		},
		"weather": []gin.H{
			{"name": "Location", "description": "Location name"},
			{"name": "Temperature", "description": "Current temperature"},
			{"name": "Condition", "description": "Weather condition"},
			{"name": "AlertType", "description": "Type of weather alert"},
			{"name": "Severity", "description": "Alert severity level"},
			{"name": "IssuedAt", "description": "Alert issue time"},
			{"name": "ExpiresAt", "description": "Alert expiration time"},
		},
		"system": []gin.H{
			{"name": "Priority", "description": "Notification priority (low, medium, high, critical)"},
			{"name": "Component", "description": "System component name"},
			{"name": "Details", "description": "Additional details"},
		},
		"functions": []gin.H{
			{"name": "upper", "description": "Convert to uppercase", "example": "{{upper .Text}}"},
			{"name": "lower", "description": "Convert to lowercase", "example": "{{lower .Text}}"},
			{"name": "title", "description": "Convert to title case", "example": "{{title .Text}}"},
			{"name": "now", "description": "Current time", "example": "{{now}}"},
			{"name": "formatDate", "description": "Format date", "example": "{{formatDate .Date \"2006-01-02\"}}"},
			{"name": "default", "description": "Default value", "example": "{{default \"N/A\" .Value}}"},
		},
	}

	c.JSON(http.StatusOK, variables)
}
