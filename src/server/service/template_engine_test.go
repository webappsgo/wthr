package service

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"

	_ "modernc.org/sqlite"
)

// setupTemplateEngineTestDB creates an in-memory server SQLite database with
// the real production ServerSchema applied, which is the only definition of
// server_notification_templates the TemplateEngine uses.
func setupTemplateEngineTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}

	return db
}

// TestTemplateEngine_Render covers happy-path rendering, template helper
// functions (upper/lower/title/default/formatDate/now), and error paths for
// malformed templates and missing variables.
func TestTemplateEngine_Render(t *testing.T) {
	te := NewTemplateEngine(nil)

	tests := []struct {
		name        string
		template    string
		vars        map[string]interface{}
		want        string
		wantErr     bool
		containsAll []string
	}{
		{
			name:     "basic variable substitution",
			template: "Hello {{.Name}}!",
			vars:     map[string]interface{}{"Name": "World"},
			want:     "Hello World!",
		},
		{
			name:     "empty template",
			template: "",
			vars:     map[string]interface{}{"Name": "World"},
			want:     "",
		},
		{
			name:     "nil variables map",
			template: "Static text only",
			vars:     nil,
			want:     "Static text only",
		},
		{
			name:     "upper func",
			template: "{{upper .Name}}",
			vars:     map[string]interface{}{"Name": "weather"},
			want:     "WEATHER",
		},
		{
			name:     "lower func",
			template: "{{lower .Name}}",
			vars:     map[string]interface{}{"Name": "WEATHER"},
			want:     "weather",
		},
		{
			name:     "title func",
			template: "{{title .Name}}",
			vars:     map[string]interface{}{"Name": "weather alert"},
			want:     "Weather Alert",
		},
		{
			name:     "default func uses provided value",
			template: "{{default \"fallback\" .Title}}",
			vars:     map[string]interface{}{"Title": "Real Title"},
			want:     "Real Title",
		},
		{
			name:     "default func falls back on missing key",
			template: "{{default \"fallback\" .Missing}}",
			vars:     map[string]interface{}{},
			want:     "fallback",
		},
		{
			name:     "default func falls back on empty string",
			template: "{{default \"fallback\" .Title}}",
			vars:     map[string]interface{}{"Title": ""},
			want:     "fallback",
		},
		{
			name:        "formatDate func",
			template:    `{{formatDate .When "2006-01-02"}}`,
			vars:        map[string]interface{}{"When": time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
			want:        "2024-03-15",
			containsAll: nil,
		},
		{
			name:        "now func produces non-empty output",
			template:    `{{now.Format "2006"}}`,
			vars:        map[string]interface{}{},
			containsAll: []string{time.Now().UTC().Format("2006")[:2]},
		},
		{
			name:     "conditional block renders when field set",
			template: "{{if .ExpiresAt}}Expires: {{.ExpiresAt}}{{end}}",
			vars:     map[string]interface{}{"ExpiresAt": "tomorrow"},
			want:     "Expires: tomorrow",
		},
		{
			name:     "conditional block skipped when field absent",
			template: "{{if .ExpiresAt}}Expires: {{.ExpiresAt}}{{end}}",
			vars:     map[string]interface{}{},
			want:     "",
		},
		{
			name:     "malformed template syntax errors on parse",
			template: "{{.Name",
			vars:     map[string]interface{}{"Name": "World"},
			wantErr:  true,
		},
		{
			name:     "unknown function errors on parse",
			template: "{{notAFunction .Name}}",
			vars:     map[string]interface{}{"Name": "World"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := te.Render(tt.template, tt.vars)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Render() expected error, got nil (result=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if tt.containsAll != nil {
				for _, want := range tt.containsAll {
					if !strings.Contains(got, want) {
						t.Errorf("Render() = %q, want substring %q", got, want)
					}
				}
				return
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTemplateEngine_ValidateTemplate covers valid templates, syntax errors,
// and boundary cases (empty template, whitespace-only).
func TestTemplateEngine_ValidateTemplate(t *testing.T) {
	te := NewTemplateEngine(nil)

	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{name: "valid simple template", template: "Hello {{.Name}}", wantErr: false},
		{name: "valid template with helper funcs", template: "{{upper .Name}} {{lower .Other}} {{title .X}} {{default 1 .Y}}", wantErr: false},
		{name: "empty template is valid", template: "", wantErr: false},
		{name: "unterminated action errors", template: "{{.Name", wantErr: true},
		{name: "unknown function errors", template: "{{doesNotExist .X}}", wantErr: true},
		{name: "unbalanced if errors", template: "{{if .X}}no end", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := te.ValidateTemplate(tt.template)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateTemplate(%q) expected error, got nil", tt.template)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateTemplate(%q) unexpected error: %v", tt.template, err)
			}
		})
	}
}

// TestTemplateEngine_CreateGetUpdateDeleteTemplate exercises the full CRUD
// lifecycle against an in-memory SQLite database, including the Variables
// map round-trip through JSON.
func TestTemplateEngine_CreateGetUpdateDeleteTemplate(t *testing.T) {
	db := setupTemplateEngineTestDB(t)
	te := NewTemplateEngine(db)

	tmpl := &NotificationTemplate{
		ChannelType:     "email",
		TemplateName:    "custom",
		TemplateType:    "general",
		SubjectTemplate: "Subject {{.X}}",
		BodyTemplate:    "Body {{.X}}",
		Variables:       map[string]string{"X": "example"},
		IsDefault:       false,
	}

	if err := te.CreateTemplate(tmpl); err != nil {
		t.Fatalf("CreateTemplate() unexpected error: %v", err)
	}

	got, err := te.GetTemplate("email", "custom")
	if err != nil {
		t.Fatalf("GetTemplate() unexpected error: %v", err)
	}
	if got.SubjectTemplate != tmpl.SubjectTemplate || got.BodyTemplate != tmpl.BodyTemplate {
		t.Errorf("GetTemplate() = %+v, want subject/body matching %+v", got, tmpl)
	}
	if got.Variables["X"] != "example" {
		t.Errorf("GetTemplate() Variables[X] = %q, want %q", got.Variables["X"], "example")
	}
	if got.ID == 0 {
		t.Errorf("GetTemplate() ID = 0, want a non-zero autoincrement id")
	}

	// Update the template and verify changes persist.
	got.BodyTemplate = "Updated body {{.X}}"
	got.TemplateType = "updated"
	if err := te.UpdateTemplate(got); err != nil {
		t.Fatalf("UpdateTemplate() unexpected error: %v", err)
	}

	updated, err := te.GetTemplate("email", "custom")
	if err != nil {
		t.Fatalf("GetTemplate() after update unexpected error: %v", err)
	}
	if updated.BodyTemplate != "Updated body {{.X}}" {
		t.Errorf("BodyTemplate after update = %q, want %q", updated.BodyTemplate, "Updated body {{.X}}")
	}
	if updated.TemplateType != "updated" {
		t.Errorf("TemplateType after update = %q, want %q", updated.TemplateType, "updated")
	}

	// Delete and verify GetTemplate falls back (no default exists, so it errors).
	if err := te.DeleteTemplate(updated.ID); err != nil {
		t.Fatalf("DeleteTemplate() unexpected error: %v", err)
	}
	if _, err := te.GetDefaultTemplate("email"); err == nil {
		t.Errorf("GetDefaultTemplate() expected error after delete with no default template, got nil")
	}
}

// TestTemplateEngine_GetTemplate_FallbackToDefault verifies that requesting a
// non-existent template name falls back to the channel's default template.
func TestTemplateEngine_GetTemplate_FallbackToDefault(t *testing.T) {
	db := setupTemplateEngineTestDB(t)
	te := NewTemplateEngine(db)

	def := &NotificationTemplate{
		ChannelType:     "slack",
		TemplateName:    "default",
		TemplateType:    "general",
		SubjectTemplate: "",
		BodyTemplate:    "default body",
		IsDefault:       true,
	}
	if err := te.CreateTemplate(def); err != nil {
		t.Fatalf("CreateTemplate() unexpected error: %v", err)
	}

	got, err := te.GetTemplate("slack", "does-not-exist")
	if err != nil {
		t.Fatalf("GetTemplate() expected fallback to default, got error: %v", err)
	}
	if got.TemplateName != "default" || got.BodyTemplate != "default body" {
		t.Errorf("GetTemplate() fallback = %+v, want the default template", got)
	}
}

// TestTemplateEngine_GetDefaultTemplate_NotFound covers the error path when
// no default template exists for a channel type, and the boundary case of an
// unknown channel type entirely.
func TestTemplateEngine_GetDefaultTemplate_NotFound(t *testing.T) {
	db := setupTemplateEngineTestDB(t)
	te := NewTemplateEngine(db)

	tests := []string{"email", "", "unknown-channel"}
	for _, channel := range tests {
		t.Run("channel="+channel, func(t *testing.T) {
			if _, err := te.GetDefaultTemplate(channel); err == nil {
				t.Errorf("GetDefaultTemplate(%q) expected error, got nil", channel)
			}
		})
	}
}

// TestTemplateEngine_ListTemplates verifies ordering (default first, then
// alphabetical by name) and the empty-result boundary case.
func TestTemplateEngine_ListTemplates(t *testing.T) {
	db := setupTemplateEngineTestDB(t)
	te := NewTemplateEngine(db)

	// Boundary: no templates yet.
	empty, err := te.ListTemplates("email")
	if err != nil {
		t.Fatalf("ListTemplates() on empty set unexpected error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListTemplates() on empty set = %d items, want 0", len(empty))
	}

	templates := []*NotificationTemplate{
		{ChannelType: "email", TemplateName: "zzz_last", TemplateType: "general", BodyTemplate: "b1"},
		{ChannelType: "email", TemplateName: "aaa_first", TemplateType: "general", BodyTemplate: "b2"},
		{ChannelType: "email", TemplateName: "default", TemplateType: "general", BodyTemplate: "b3", IsDefault: true},
		// Different channel type must not appear in the "email" results.
		{ChannelType: "sms", TemplateName: "default", TemplateType: "general", BodyTemplate: "b4", IsDefault: true},
	}
	for _, tmpl := range templates {
		if err := te.CreateTemplate(tmpl); err != nil {
			t.Fatalf("CreateTemplate(%q) unexpected error: %v", tmpl.TemplateName, err)
		}
	}

	list, err := te.ListTemplates("email")
	if err != nil {
		t.Fatalf("ListTemplates() unexpected error: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListTemplates() = %d items, want 3", len(list))
	}
	if list[0].TemplateName != "default" {
		t.Errorf("ListTemplates()[0].TemplateName = %q, want %q (default sorts first)", list[0].TemplateName, "default")
	}
	if list[1].TemplateName != "aaa_first" || list[2].TemplateName != "zzz_last" {
		t.Errorf("ListTemplates() ordering = [%q, %q], want alphabetical after default", list[1].TemplateName, list[2].TemplateName)
	}
}

// TestTemplateEngine_RenderTemplate covers the full render pipeline (subject
// + body) and its error paths: missing template with no default, and a body
// template with invalid syntax.
func TestTemplateEngine_RenderTemplate(t *testing.T) {
	db := setupTemplateEngineTestDB(t)
	te := NewTemplateEngine(db)

	tmpl := &NotificationTemplate{
		ChannelType:     "email",
		TemplateName:    "alert",
		TemplateType:    "alert",
		SubjectTemplate: "Alert: {{.AlertType}}",
		BodyTemplate:    "{{.AlertType}} in {{.Location}}",
	}
	if err := te.CreateTemplate(tmpl); err != nil {
		t.Fatalf("CreateTemplate() unexpected error: %v", err)
	}

	subject, body, err := te.RenderTemplate("email", "alert", map[string]interface{}{
		"AlertType": "Tornado",
		"Location":  "Kansas",
	})
	if err != nil {
		t.Fatalf("RenderTemplate() unexpected error: %v", err)
	}
	if subject != "Alert: Tornado" {
		t.Errorf("RenderTemplate() subject = %q, want %q", subject, "Alert: Tornado")
	}
	if body != "Tornado in Kansas" {
		t.Errorf("RenderTemplate() body = %q, want %q", body, "Tornado in Kansas")
	}

	t.Run("missing template with no default errors", func(t *testing.T) {
		if _, _, err := te.RenderTemplate("webhook", "nope", nil); err == nil {
			t.Errorf("RenderTemplate() expected error for missing template/default, got nil")
		}
	})

	t.Run("invalid body template errors on render", func(t *testing.T) {
		bad := &NotificationTemplate{
			ChannelType:  "push",
			TemplateName: "bad",
			TemplateType: "general",
			BodyTemplate: "{{.Unterminated",
		}
		if err := te.CreateTemplate(bad); err != nil {
			t.Fatalf("CreateTemplate() unexpected error: %v", err)
		}
		if _, _, err := te.RenderTemplate("push", "bad", nil); err == nil {
			t.Errorf("RenderTemplate() expected error for invalid body template, got nil")
		}
	})
}

// TestTemplateEngine_InitializeDefaultTemplates verifies that calling it
// populates the expected default templates and that calling it a second time
// is idempotent (no duplicate rows, no errors).
func TestTemplateEngine_InitializeDefaultTemplates(t *testing.T) {
	db := setupTemplateEngineTestDB(t)
	te := NewTemplateEngine(db)

	if err := te.InitializeDefaultTemplates(); err != nil {
		t.Fatalf("InitializeDefaultTemplates() unexpected error: %v", err)
	}

	var countAfterFirst int
	if err := db.QueryRow("SELECT COUNT(*) FROM server_notification_templates").Scan(&countAfterFirst); err != nil {
		t.Fatalf("failed to count templates: %v", err)
	}
	if countAfterFirst == 0 {
		t.Fatalf("InitializeDefaultTemplates() created 0 templates, want > 0")
	}

	// Spot-check a well-known default exists and is marked default.
	emailDefault, err := te.GetDefaultTemplate("email")
	if err != nil {
		t.Fatalf("GetDefaultTemplate(email) unexpected error: %v", err)
	}
	if emailDefault.TemplateName != "default" || !emailDefault.IsDefault {
		t.Errorf("GetDefaultTemplate(email) = %+v, want the default=true 'default' template", emailDefault)
	}

	// Idempotency: running again must not create duplicates or error.
	if err := te.InitializeDefaultTemplates(); err != nil {
		t.Fatalf("InitializeDefaultTemplates() second call unexpected error: %v", err)
	}

	var countAfterSecond int
	if err := db.QueryRow("SELECT COUNT(*) FROM server_notification_templates").Scan(&countAfterSecond); err != nil {
		t.Fatalf("failed to count templates: %v", err)
	}
	if countAfterSecond != countAfterFirst {
		t.Errorf("InitializeDefaultTemplates() second call changed row count: %d -> %d, want unchanged (idempotent)", countAfterFirst, countAfterSecond)
	}
}
