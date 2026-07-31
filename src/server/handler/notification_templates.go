package handler

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

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

	if channelType != "" {
		query = `
			SELECT id, channel_type, template_name, template_type,
			       subject_template, body_template, variables, is_default,
			       created_at, updated_at
			FROM notification_templates
			WHERE channel_type = ?
			ORDER BY is_default DESC, template_name ASC
		`
		args = append(args, channelType)
	} else {
		query = `
			SELECT id, channel_type, template_name, template_type,
			       subject_template, body_template, variables, is_default,
			       created_at, updated_at
			FROM notification_templates
			ORDER BY channel_type, is_default DESC, template_name ASC
		`
	}

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch templates"})
		return
	}
	defer rows.Close()

	var templates []gin.H
	for rows.Next() {
		var id int
		var channelType, templateName, templateType string
		var subjectTemplate, bodyTemplate string
		var isDefault bool
		var variables sql.NullString
		var createdAt, updatedAt sql.NullTime

		err := rows.Scan(&id, &channelType, &templateName, &templateType,
			&subjectTemplate, &bodyTemplate, &variables, &isDefault,
			&createdAt, &updatedAt)
		if err != nil {
			continue
		}

		tmpl := gin.H{
			"id":               id,
			"channel_type":     channelType,
			"template_name":    templateName,
			"template_type":    templateType,
			"subject_template": subjectTemplate,
			"body_template":    bodyTemplate,
			"is_default":       isDefault,
			"created_at":       nil,
			"updated_at":       nil,
		}

		if variables.Valid {
			var varsMap map[string]interface{}
			json.Unmarshal([]byte(variables.String), &varsMap)
			tmpl["variables"] = varsMap
		}

		if createdAt.Valid {
			tmpl["created_at"] = createdAt.Time
		}
		if updatedAt.Valid {
			tmpl["updated_at"] = updatedAt.Time
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
	var subjectTemplate, bodyTemplate string
	var isDefault bool
	var variables sql.NullString
	var createdAt, updatedAt sql.NullTime

	err := h.DB.QueryRow(`
		SELECT channel_type, template_name, template_type,
		       subject_template, body_template, variables, is_default,
		       created_at, updated_at
		FROM notification_templates
		WHERE id = ?
	`, id).Scan(&channelType, &templateName, &templateType,
		&subjectTemplate, &bodyTemplate, &variables, &isDefault,
		&createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	tmpl := gin.H{
		"id":               id,
		"channel_type":     channelType,
		"template_name":    templateName,
		"template_type":    templateType,
		"subject_template": subjectTemplate,
		"body_template":    bodyTemplate,
		"is_default":       isDefault,
		"created_at":       nil,
		"updated_at":       nil,
	}

	if variables.Valid {
		var varsMap map[string]interface{}
		json.Unmarshal([]byte(variables.String), &varsMap)
		tmpl["variables"] = varsMap
	}

	if createdAt.Valid {
		tmpl["created_at"] = createdAt.Time
	}
	if updatedAt.Valid {
		tmpl["updated_at"] = updatedAt.Time
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
		IsDefault       bool                   `json:"is_default"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Validate template syntax
	if err := h.TemplateEngine.ValidateTemplate(req.BodyTemplate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template syntax: " + err.Error()})
		return
	}

	if req.SubjectTemplate != "" {
		if err := h.TemplateEngine.ValidateTemplate(req.SubjectTemplate); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subject template syntax: " + err.Error()})
			return
		}
	}

	// If setting as default, unset other defaults for this channel
	if req.IsDefault {
		if _, err := h.DB.Exec(`
			UPDATE notification_templates
			SET is_default = 0
			WHERE channel_type = ?
		`, req.ChannelType); err != nil {
			log.Printf("failed to clear existing default templates for channel %s: %v", req.ChannelType, err)
		}
	}

	variablesJSON, _ := json.Marshal(req.Variables)

	result, err := h.DB.Exec(`
		INSERT INTO notification_templates
		(channel_type, template_name, template_type, subject_template,
		 body_template, variables, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, req.ChannelType, req.TemplateName, req.TemplateType,
		req.SubjectTemplate, req.BodyTemplate, string(variablesJSON), req.IsDefault)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}

	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{
		"message": "Template created successfully",
		"id":      id,
	})
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
		IsDefault       bool                   `json:"is_default"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Validate template syntax
	if req.BodyTemplate != "" {
		if err := h.TemplateEngine.ValidateTemplate(req.BodyTemplate); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template syntax: " + err.Error()})
			return
		}
	}

	if req.SubjectTemplate != "" {
		if err := h.TemplateEngine.ValidateTemplate(req.SubjectTemplate); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subject template syntax: " + err.Error()})
			return
		}
	}

	// Get current template's channel type
	var channelType string
	err := h.DB.QueryRow("SELECT channel_type FROM notification_templates WHERE id = ?", id).Scan(&channelType)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	// If setting as default, unset other defaults
	if req.IsDefault {
		if _, err := h.DB.Exec(`
			UPDATE notification_templates
			SET is_default = 0
			WHERE channel_type = ? AND id != ?
		`, channelType, id); err != nil {
			log.Printf("failed to clear existing default templates for channel %s: %v", channelType, err)
		}
	}

	variablesJSON, _ := json.Marshal(req.Variables)

	_, err = h.DB.Exec(`
		UPDATE notification_templates
		SET template_name = ?,
		    template_type = ?,
		    subject_template = ?,
		    body_template = ?,
		    variables = ?,
		    is_default = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, req.TemplateName, req.TemplateType, req.SubjectTemplate,
		req.BodyTemplate, string(variablesJSON), req.IsDefault, id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template updated successfully"})
}

// DeleteTemplate deletes a template
func (h *NotificationTemplateHandler) DeleteTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	// Check if it's a default template
	var isDefault bool
	err := h.DB.QueryRow("SELECT is_default FROM notification_templates WHERE id = ?", id).Scan(&isDefault)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	if isDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete default template"})
		return
	}

	_, err = h.DB.Exec("DELETE FROM notification_templates WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted successfully"})
}

// PreviewTemplate renders a template with sample data
func (h *NotificationTemplateHandler) PreviewTemplate(c *gin.Context) {
	var req struct {
		SubjectTemplate string                 `json:"subject_template"`
		BodyTemplate    string                 `json:"body_template" binding:"required"`
		Variables       map[string]interface{} `json:"variables"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Render subject
	var subject string
	var err error
	if req.SubjectTemplate != "" {
		subject, err = h.TemplateEngine.Render(req.SubjectTemplate, req.Variables)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to render subject: " + err.Error()})
			return
		}
	}

	// Render body
	body, err := h.TemplateEngine.Render(req.BodyTemplate, req.Variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to render body: " + err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Get original template
	var channelType, templateType, subjectTemplate, bodyTemplate string
	var variables sql.NullString

	err := h.DB.QueryRow(`
		SELECT channel_type, template_type, subject_template, body_template, variables
		FROM notification_templates
		WHERE id = ?
	`, id).Scan(&channelType, &templateType, &subjectTemplate, &bodyTemplate, &variables)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	// Create clone (never set as default)
	result, err := h.DB.Exec(`
		INSERT INTO notification_templates
		(channel_type, template_name, template_type, subject_template,
		 body_template, variables, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, datetime('now'), datetime('now'))
	`, channelType, req.NewName, templateType, subjectTemplate, bodyTemplate,
		variables.String)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clone template"})
		return
	}

	newID, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{
		"message": "Template cloned successfully",
		"id":      newID,
	})
}

// InitializeDefaults initializes default templates
func (h *NotificationTemplateHandler) InitializeDefaults(c *gin.Context) {
	err := h.TemplateEngine.InitializeDefaultTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Default templates initialized successfully",
	})
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
