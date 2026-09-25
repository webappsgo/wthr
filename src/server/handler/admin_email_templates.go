package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

type EmailTemplateHandler struct {
	templatesDir string
}

func NewEmailTemplateHandler(templatesDir string) *EmailTemplateHandler {
	return &EmailTemplateHandler{
		templatesDir: templatesDir,
	}
}

type EmailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// GetTemplate retrieves a specific email template
func (h *EmailTemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	templateName := chi.URLParam(r, "name")

	// Validate template name
	if !isValidTemplateName(templateName) {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.invalid_template_name"))
		return
	}

	templatePath := filepath.Join(h.templatesDir, "email", templateName+".tmpl")

	// Read template file
	content, err := os.ReadFile(templatePath)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.admin.email_templates.template_not_found"))
		return
	}

	// Parse template (format: Subject: ...\n---\nBody...)
	template := parseTemplate(string(content))

	writeJSON(w, http.StatusOK, template)
}

// resolveTemplateName returns the template a request targets. The token-auth
// API names it in the URL path; the session-auth admin form carries it in a
// form field so one editor page can drive every template from a single form.
// A submitted field wins because a form field is empty on every JSON API call.
func resolveTemplateName(r *http.Request) string {
	if name := r.FormValue("template"); name != "" {
		return name
	}

	return chi.URLParam(r, "name")
}

// UpdateTemplate updates a specific email template
func (h *EmailTemplateHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateName := resolveTemplateName(r)

	// Validate template name
	if !isValidTemplateName(templateName) {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.invalid_template_name"))
		return
	}

	var template EmailTemplate
	if !DecodeAndValidate(w, r, &template) {
		return
	}

	// Validate template
	if template.Subject == "" || template.Body == "" {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.subject_and_body_are_required"))
		return
	}

	// Format template content
	content := fmt.Sprintf("Subject: %s\n---\n%s\n", template.Subject, template.Body)

	// Write template file
	templatePath := filepath.Join(h.templatesDir, "email", templateName+".tmpl")
	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminConfigPagePath(r, "/config/email/templates"), "flash_email_template_saved", "flash_email_template_save_failed", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.admin.email_templates.failed_to_save_template"))
		return
	}

	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/email/templates"), "flash_email_template_saved", "flash_email_template_save_failed", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.email_templates.template_updated_successfully"),
	})
}

// ListTemplates returns all available email templates
func (h *EmailTemplateHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	emailDir := filepath.Join(h.templatesDir, "email")

	files, err := os.ReadDir(emailDir)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.email_templates.failed_to_read_templates_directory"))
		return
	}

	templates := make([]map[string]string, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".tmpl") {
			continue
		}

		name := strings.TrimSuffix(file.Name(), ".tmpl")
		templates = append(templates, map[string]string{
			"name": name,
			"file": file.Name(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": templates})
}

// TestTemplate sends a test email using the specified template
func (h *EmailTemplateHandler) TestTemplate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Template string `json:"template"`
	}

	if !DecodeAndValidate(w, r, &request) {
		return
	}

	templateName := request.Template
	if templateName == "" {
		templateName = resolveTemplateName(r)
	}

	// Validate template name
	if !isValidTemplateName(templateName) {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.invalid_template_name"))
		return
	}

	// In a real implementation, this would use the email service
	// For now, we'll just return success
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/email/templates"), "flash_test_email_sent", "flash_test_email_failed", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("%s: %s", Translate(r, "success.admin.email_templates.test_email_sent_using_template"), templateName),
	})
}

// Helper: Parse template file content
func parseTemplate(content string) EmailTemplate {
	lines := strings.Split(content, "\n")
	template := EmailTemplate{}

	separatorIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndex = i
			break
		}
		if strings.HasPrefix(line, "Subject:") {
			template.Subject = strings.TrimSpace(strings.TrimPrefix(line, "Subject:"))
		}
	}

	if separatorIndex > 0 && separatorIndex < len(lines)-1 {
		template.Body = strings.TrimSpace(strings.Join(lines[separatorIndex+1:], "\n"))
	}

	return template
}

// emailTemplateNames is the allow-list of editable email templates, in the
// order the editor page lists them. It is the single source for both name
// validation and the page's template picker.
var emailTemplateNames = []string{
	"welcome",
	"password_reset",
	"backup_complete",
	"backup_failed",
	"ssl_expiring",
	"ssl_renewed",
	"login_alert",
	"security_alert",
	"scheduler_error",
	"test",
	"2fa_disabled",
	"2fa_enabled",
	"account_disabled",
	"breach_admin_alert",
	"breach_notification",
	"email_verify",
	"mfa_reminder",
	"password_changed",
	"user_invite",
}

// Helper: Validate template name
func isValidTemplateName(name string) bool {
	for _, valid := range emailTemplateNames {
		if name == valid {
			return true
		}
	}
	return false
}

// EditorTemplate is one row of the session-auth template editor: the template
// name plus the content on disk. A template file that cannot be read is skipped
// rather than failing the whole page, so one bad file never blanks the editor.
type EditorTemplate struct {
	Name    string
	Subject string
	Body    string
}

// EditorTemplates returns every allow-listed template that has a readable file
// on disk, in allow-list order. The editor page renders the selected one
// server-side, which is what lets the panel edit templates with JavaScript
// disabled; the JSON API remains the source of truth for scripted clients.
func (h *EmailTemplateHandler) EditorTemplates() []EditorTemplate {
	editable := make([]EditorTemplate, 0, len(emailTemplateNames))

	for _, name := range emailTemplateNames {
		content, err := os.ReadFile(filepath.Join(h.templatesDir, "email", name+".tmpl"))
		if err != nil {
			continue
		}

		parsed := parseTemplate(string(content))
		editable = append(editable, EditorTemplate{Name: name, Subject: parsed.Subject, Body: parsed.Body})
	}

	return editable
}

// ExportTemplate exports a template as JSON
func (h *EmailTemplateHandler) ExportTemplate(w http.ResponseWriter, r *http.Request) {
	templateName := chi.URLParam(r, "name")

	if !isValidTemplateName(templateName) {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.invalid_template_name"))
		return
	}

	templatePath := filepath.Join(h.templatesDir, "email", templateName+".tmpl")

	content, err := os.ReadFile(templatePath)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.admin.email_templates.template_not_found"))
		return
	}

	template := parseTemplate(string(content))

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", templateName))
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(template)
}

// ImportTemplate imports a template from JSON
func (h *EmailTemplateHandler) ImportTemplate(w http.ResponseWriter, r *http.Request) {
	templateName := resolveTemplateName(r)

	if !isValidTemplateName(templateName) {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.invalid_template_name"))
		return
	}

	var template EmailTemplate
	if !DecodeAndValidate(w, r, &template) {
		return
	}

	if template.Subject == "" || template.Body == "" {
		BadRequest(w, r, Translate(r, "errors.admin.email_templates.subject_and_body_are_required"))
		return
	}

	content := fmt.Sprintf("Subject: %s\n---\n%s\n", template.Subject, template.Body)
	templatePath := filepath.Join(h.templatesDir, "email", templateName+".tmpl")

	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminConfigPagePath(r, "/config/email/templates"), "flash_email_template_saved", "flash_email_template_save_failed", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.admin.email_templates.failed_to_import_template"))
		return
	}

	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminConfigPagePath(r, "/config/email/templates"), "flash_email_template_saved", "flash_email_template_save_failed", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.email_templates.template_imported_successfully"),
	})
}
