package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestNewEmailTemplateHandler verifies the constructor wires the
// templatesDir field as passed.
func TestNewEmailTemplateHandler(t *testing.T) {
	h := NewEmailTemplateHandler("/tmp/example/templates")
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.templatesDir != "/tmp/example/templates" {
		t.Errorf("templatesDir = %q, want %q", h.templatesDir, "/tmp/example/templates")
	}
}

// TestIsValidTemplateName verifies the allow-list check accepts every
// known template name and rejects anything else, including attempts at
// path traversal through the name parameter.
func TestIsValidTemplateName(t *testing.T) {
	valid := []string{
		"welcome", "password_reset", "backup_complete", "backup_failed",
		"ssl_expiring", "ssl_renewed", "login_alert", "security_alert",
		"scheduler_error", "test",
	}
	for _, name := range valid {
		if !isValidTemplateName(name) {
			t.Errorf("isValidTemplateName(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "does_not_exist", "../../etc/passwd", "Welcome", "welcome "}
	for _, name := range invalid {
		if isValidTemplateName(name) {
			t.Errorf("isValidTemplateName(%q) = true, want false", name)
		}
	}
}

// TestParseTemplate verifies the "Subject: ...\n---\nbody" format is split
// correctly, and that a body without the "---" separator yields an empty
// Body while still capturing the Subject line.
func TestParseTemplate(t *testing.T) {
	content := "Subject: Welcome to {app_name}\n---\nHello {username},\n\nWelcome aboard.\n"

	got := parseTemplate(content)
	if got.Subject != "Welcome to {app_name}" {
		t.Errorf("Subject = %q, want %q", got.Subject, "Welcome to {app_name}")
	}
	if got.Body != "Hello {username},\n\nWelcome aboard." {
		t.Errorf("Body = %q, want %q", got.Body, "Hello {username},\n\nWelcome aboard.")
	}
}

// TestParseTemplate_NoSeparator verifies a missing "---" separator leaves
// Body empty rather than panicking or including the whole content.
func TestParseTemplate_NoSeparator(t *testing.T) {
	got := parseTemplate("Subject: Only a subject\nNo separator here")
	if got.Subject != "Only a subject" {
		t.Errorf("Subject = %q, want %q", got.Subject, "Only a subject")
	}
	if got.Body != "" {
		t.Errorf("Body = %q, want empty", got.Body)
	}
}

// TestEmailTemplateHandler_UpdateTemplate_Success verifies a valid template
// body is written to disk in the "Subject: ...\n---\n..." format.
func TestEmailTemplateHandler_UpdateTemplate_Success(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "email"), 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	h := &EmailTemplateHandler{templatesDir: dir}

	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/email-templates/welcome",
		map[string]string{"Subject": "Welcome!", "Body": "Hello there."})
	c.Params = []gin.Param{{Key: "name", Value: "welcome"}}

	h.UpdateTemplate(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(dir, "email", "welcome.tmpl"))
	if err != nil {
		t.Fatalf("failed to read written template: %v", err)
	}
	if !strings.Contains(string(written), "Subject: Welcome!") || !strings.Contains(string(written), "Hello there.") {
		t.Errorf("unexpected template content: %s", written)
	}
}

// TestEmailTemplateHandler_UpdateTemplate_InvalidName verifies an
// unrecognized template name is rejected with 400 before any write.
func TestEmailTemplateHandler_UpdateTemplate_InvalidName(t *testing.T) {
	h := &EmailTemplateHandler{templatesDir: t.TempDir()}

	c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/config/email-templates/nope",
		map[string]string{"Subject": "x", "Body": "y"})
	c.Params = []gin.Param{{Key: "name", Value: "nope"}}

	h.UpdateTemplate(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

// TestEmailTemplateHandler_ListTemplates verifies templates in the email
// subdirectory are listed and non-.tmpl files are ignored.
func TestEmailTemplateHandler_ListTemplates(t *testing.T) {
	dir := t.TempDir()
	emailDir := filepath.Join(dir, "email")
	if err := os.MkdirAll(emailDir, 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emailDir, "welcome.tmpl"), []byte("Subject: x\n---\ny\n"), 0644); err != nil {
		t.Fatalf("failed to write fixture template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emailDir, "readme.txt"), []byte("not a template"), 0644); err != nil {
		t.Fatalf("failed to write fixture non-template: %v", err)
	}

	h := &EmailTemplateHandler{templatesDir: dir}
	c, w := newAPITestContext("/server/admin/config/email-templates")

	h.ListTemplates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "welcome") {
		t.Errorf("expected templates list to contain 'welcome', got: %s", body)
	}
	if strings.Contains(body, "readme.txt") {
		t.Errorf("expected non-.tmpl file to be excluded, got: %s", body)
	}
}
