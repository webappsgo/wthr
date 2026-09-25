package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// newEmailTemplateTestRequest builds a request/recorder pair with a
// JSON-encoded body and the given chi URL params attached, mirroring the
// pattern the other converted handlers/tests in this package use.
func newEmailTemplateTestRequest(t *testing.T, method, target string, body interface{}, params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()

	var raw []byte
	switch v := body.(type) {
	case nil:
		raw = nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}

	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	return r, httptest.NewRecorder()
}

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

	r, w := newEmailTemplateTestRequest(t, http.MethodPost, "/server/admin/config/email-templates/welcome",
		map[string]string{"Subject": "Welcome!", "Body": "Hello there."}, map[string]string{"name": "welcome"})

	h.UpdateTemplate(w, r)

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

	r, w := newEmailTemplateTestRequest(t, http.MethodPost, "/server/admin/config/email-templates/nope",
		map[string]string{"Subject": "x", "Body": "y"}, map[string]string{"name": "nope"})

	h.UpdateTemplate(w, r)

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
	r, w := newEmailTemplateTestRequest(t, http.MethodGet, "/server/admin/config/email-templates", nil, nil)

	h.ListTemplates(w, r)

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

// TestEmailTemplateHandler_GetTemplate_Success verifies a valid template is
// retrieved and returned with parsed subject/body.
func TestEmailTemplateHandler_GetTemplate_Success(t *testing.T) {
	dir := t.TempDir()
	emailDir := filepath.Join(dir, "email")
	if err := os.MkdirAll(emailDir, 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	content := "Subject: Hello\n---\nHello world"
	if err := os.WriteFile(filepath.Join(emailDir, "welcome.tmpl"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write fixture template: %v", err)
	}

	h := &EmailTemplateHandler{templatesDir: dir}
	r, w := newEmailTemplateTestRequest(t, http.MethodGet, "/server/admin/config/email-templates/welcome",
		nil, map[string]string{"name": "welcome"})

	h.GetTemplate(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp EmailTemplate
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Subject != "Hello" {
		t.Errorf("Subject = %q, want %q", resp.Subject, "Hello")
	}
	if resp.Body != "Hello world" {
		t.Errorf("Body = %q, want %q", resp.Body, "Hello world")
	}
}

// TestEmailTemplateHandler_ExportTemplate verifies a template is exported
// as JSON with the correct subject and body.
func TestEmailTemplateHandler_ExportTemplate(t *testing.T) {
	dir := t.TempDir()
	emailDir := filepath.Join(dir, "email")
	if err := os.MkdirAll(emailDir, 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emailDir, "test.tmpl"), []byte("Subject: Test\n---\nBody"), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	h := &EmailTemplateHandler{templatesDir: dir}
	r, w := newEmailTemplateTestRequest(t, http.MethodGet, "/server/admin/config/email-templates/test/export",
		nil, map[string]string{"name": "test"})

	h.ExportTemplate(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	contentDisposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "test.json") {
		t.Errorf("Content-Disposition = %q, want filename test.json", contentDisposition)
	}
}

// TestEmailTemplateHandler_ImportTemplate verifies a JSON template is imported
// correctly and written to disk.
func TestEmailTemplateHandler_ImportTemplate(t *testing.T) {
	dir := t.TempDir()
	emailDir := filepath.Join(dir, "email")
	if err := os.MkdirAll(emailDir, 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}

	h := &EmailTemplateHandler{templatesDir: dir}
	template := EmailTemplate{Subject: "Imported", Body: "Imported body"}
	r, w := newEmailTemplateTestRequest(t, http.MethodPost, "/server/admin/config/email-templates/test/import",
		template, map[string]string{"name": "test"})

	h.ImportTemplate(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(emailDir, "test.tmpl"))
	if err != nil {
		t.Fatalf("failed to read imported template: %v", err)
	}
	if !strings.Contains(string(written), "Subject: Imported") || !strings.Contains(string(written), "Imported body") {
		t.Errorf("unexpected template content: %s", written)
	}
}

// newEmailTemplateFormRequest builds a form-encoded POST carrying the given
// fields plus a chi URL param set, mirroring the session-auth admin form
// routes. csrf_token is omitted because these handler tests exercise the
// handler below the CSRF middleware layer.
func newEmailTemplateFormRequest(t *testing.T, target string, form url.Values, params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	return r, httptest.NewRecorder()
}

// emailTemplateFlashValue returns the flash cookie value set on the recorder,
// matching the helper used by the form-redirect tests.
func emailTemplateFlashValue(t *testing.T, w *httptest.ResponseRecorder) (string, bool) {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == FlashCookieName {
			return c.Value, true
		}
	}
	return "", false
}

// TestResolveTemplateName verifies the session-auth form field takes priority
// over the URL path parameter, and that the URL param is the fallback when no
// form field is present.
func TestResolveTemplateName(t *testing.T) {
	t.Run("form field beats URL param", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/server/admin/config/email/templates",
			strings.NewReader(url.Values{"template": {"welcome"}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("name", "password_reset")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		if got := resolveTemplateName(r); got != "welcome" {
			t.Fatalf("resolveTemplateName = %q, want %q", got, "welcome")
		}
	})

	t.Run("URL param fallback when no form field", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/server/admin/config/email-templates/welcome", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("name", "welcome")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		if got := resolveTemplateName(r); got != "welcome" {
			t.Fatalf("resolveTemplateName = %q, want %q", got, "welcome")
		}
	})
}

// TestResolveTemplateName_JSONBodyNotConsumed is the regression test for the
// session-auth/token-auth split: resolveTemplateName calls r.FormValue, which
// must not consume the body of an application/json request, so the token-auth
// JSON route that follows still decodes its body correctly.
func TestResolveTemplateName_JSONBodyNotConsumed(t *testing.T) {
	payload := `{"subject":"Kept","body":"Kept body"}`
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/email-templates/welcome",
		strings.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "welcome")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	// FormValue must return empty and must not drain the JSON body.
	if name := resolveTemplateName(r); name != "welcome" {
		t.Fatalf("resolveTemplateName on JSON request = %q, want URL param %q", name, "welcome")
	}

	var decoded EmailTemplate
	if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
		t.Fatalf("JSON body was consumed by resolveTemplateName: %v", err)
	}
	if decoded.Subject != "Kept" || decoded.Body != "Kept body" {
		t.Fatalf("decoded = %+v, want subject/body preserved", decoded)
	}
}

// TestEditorTemplates verifies the editor lists only allow-listed templates
// that have a readable file on disk, in allow-list order, skipping unreadable
// entries rather than failing.
func TestEditorTemplates(t *testing.T) {
	dir := t.TempDir()
	emailDir := filepath.Join(dir, "email")
	if err := os.MkdirAll(emailDir, 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	// welcome is allow-list first; test is allow-list last. Write them in
	// reverse order so a filesystem-ordered read would catch a missing sort.
	if err := os.WriteFile(filepath.Join(emailDir, "test.tmpl"), []byte("Subject: Test\n---\nTest body"), 0644); err != nil {
		t.Fatalf("write test.tmpl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emailDir, "welcome.tmpl"), []byte("Subject: Welcome\n---\nWelcome body"), 0644); err != nil {
		t.Fatalf("write welcome.tmpl: %v", err)
	}
	// A file that is not in the allow-list must be ignored.
	if err := os.WriteFile(filepath.Join(emailDir, "not_a_template.tmpl"), []byte("Subject: x\n---\ny"), 0644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	h := &EmailTemplateHandler{templatesDir: dir}
	got := h.EditorTemplates()

	if len(got) != 2 {
		t.Fatalf("EditorTemplates returned %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Name != "welcome" || got[1].Name != "test" {
		t.Fatalf("EditorTemplates order = %q,%q, want welcome,test", got[0].Name, got[1].Name)
	}
	if got[0].Subject != "Welcome" || got[0].Body != "Welcome body" {
		t.Fatalf("welcome template parsed wrong: %+v", got[0])
	}

	// Removing the only readable file must shrink the list, not fail it.
	if err := os.Remove(filepath.Join(emailDir, "welcome.tmpl")); err != nil {
		t.Fatalf("remove welcome.tmpl: %v", err)
	}
	got = h.EditorTemplates()
	if len(got) != 1 || got[0].Name != "test" {
		t.Fatalf("EditorTemplates after removing welcome = %+v, want only test", got)
	}
}

// TestEmailTemplateHandler_UpdateTemplate_FormPRG verifies a successful
// form-encoded update redirects (303) to the editor page with a success flash
// and writes the template to disk.
func TestEmailTemplateHandler_UpdateTemplate_FormPRG(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "email"), 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	h := &EmailTemplateHandler{templatesDir: dir}

	form := url.Values{
		"template": {"welcome"},
		"subject":  {"Welcome aboard"},
		"body":     {"Hello there."},
	}
	r, w := newEmailTemplateFormRequest(t, "/server/admin/config/email/templates", form, nil)

	h.UpdateTemplate(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.HasSuffix(loc, "/config/email/templates") {
		t.Fatalf("Location = %q, want editor page", loc)
	}
	val, ok := emailTemplateFlashValue(t, w)
	if !ok || val != "success:flash_email_template_saved" {
		t.Fatalf("flash = %q (ok=%v), want success:flash_email_template_saved", val, ok)
	}

	written, err := os.ReadFile(filepath.Join(dir, "email", "welcome.tmpl"))
	if err != nil {
		t.Fatalf("template not written: %v", err)
	}
	if !strings.Contains(string(written), "Subject: Welcome aboard") {
		t.Fatalf("written content = %q, want updated subject", written)
	}
}

// TestEmailTemplateHandler_UpdateTemplate_FormWriteFail verifies a form-encoded
// update that fails to write redirects (303) with a failure flash rather than a
// bare 500.
func TestEmailTemplateHandler_UpdateTemplate_FormWriteFail(t *testing.T) {
	// Point templatesDir at a path whose email/ subdirectory does not exist so
	// the write fails.
	h := &EmailTemplateHandler{templatesDir: filepath.Join(t.TempDir(), "nonexistent")}

	form := url.Values{
		"template": {"welcome"},
		"subject":  {"x"},
		"body":     {"y"},
	}
	r, w := newEmailTemplateFormRequest(t, "/server/admin/config/email/templates", form, nil)

	h.UpdateTemplate(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", w.Code, w.Body.String())
	}
	val, ok := emailTemplateFlashValue(t, w)
	if !ok || val != "error:flash_email_template_save_failed" {
		t.Fatalf("flash = %q (ok=%v), want error:flash_email_template_save_failed", val, ok)
	}
}

// TestEmailTemplateHandler_TestTemplate_FormPRG verifies a form-encoded test
// submission redirects (303) with the test-sent success flash.
func TestEmailTemplateHandler_TestTemplate_FormPRG(t *testing.T) {
	h := &EmailTemplateHandler{templatesDir: t.TempDir()}

	form := url.Values{"template": {"welcome"}}
	r, w := newEmailTemplateFormRequest(t, "/server/admin/config/email/templates/test", form, nil)

	h.TestTemplate(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", w.Code, w.Body.String())
	}
	val, ok := emailTemplateFlashValue(t, w)
	if !ok || val != "success:flash_test_email_sent" {
		t.Fatalf("flash = %q (ok=%v), want success:flash_test_email_sent", val, ok)
	}
}

// TestEmailTemplateHandler_ImportTemplate_FormPRG verifies a successful
// form-encoded import redirects (303) with a success flash and writes the
// imported content to disk.
func TestEmailTemplateHandler_ImportTemplate_FormPRG(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "email"), 0755); err != nil {
		t.Fatalf("failed to create email dir: %v", err)
	}
	h := &EmailTemplateHandler{templatesDir: dir}

	form := url.Values{
		"template": {"test"},
		"subject":  {"Imported"},
		"body":     {"Imported body"},
	}
	r, w := newEmailTemplateFormRequest(t, "/server/admin/config/email/templates/import", form, nil)

	h.ImportTemplate(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", w.Code, w.Body.String())
	}
	val, ok := emailTemplateFlashValue(t, w)
	if !ok || val != "success:flash_email_template_saved" {
		t.Fatalf("flash = %q (ok=%v), want success:flash_email_template_saved", val, ok)
	}

	written, err := os.ReadFile(filepath.Join(dir, "email", "test.tmpl"))
	if err != nil {
		t.Fatalf("template not written: %v", err)
	}
	if !strings.Contains(string(written), "Subject: Imported") {
		t.Fatalf("written content = %q, want imported subject", written)
	}
}

// TestEmailTemplateHandler_ImportTemplate_FormWriteFail verifies a form-encoded
// import that fails to write redirects (303) with a failure flash.
func TestEmailTemplateHandler_ImportTemplate_FormWriteFail(t *testing.T) {
	h := &EmailTemplateHandler{templatesDir: filepath.Join(t.TempDir(), "nonexistent")}

	form := url.Values{
		"template": {"test"},
		"subject":  {"x"},
		"body":     {"y"},
	}
	r, w := newEmailTemplateFormRequest(t, "/server/admin/config/email/templates/import", form, nil)

	h.ImportTemplate(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", w.Code, w.Body.String())
	}
	val, ok := emailTemplateFlashValue(t, w)
	if !ok || val != "error:flash_email_template_save_failed" {
		t.Fatalf("flash = %q (ok=%v), want error:flash_email_template_save_failed", val, ok)
	}
}
