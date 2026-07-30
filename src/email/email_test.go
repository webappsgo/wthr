package email

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew_Disabled verifies the service stays disabled when SMTP is not
// configured, per AI.md line 22786: "No SMTP configured -> Email
// functionality completely disabled". These cases never touch the network
// because New() only calls validateSMTP() when Host is set AND Port > 0.
func TestNew_Disabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  SMTPConfig
	}{
		{"empty config", SMTPConfig{}},
		{"host only, no port", SMTPConfig{Host: "smtp.example.com"}},
		{"port only, no host", SMTPConfig{Port: 587}},
		{"negative port", SMTPConfig{Host: "smtp.example.com", Port: -1}},
		{"zero port", SMTPConfig{Host: "smtp.example.com", Port: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := New(tt.cfg)
			if svc.IsEnabled() {
				t.Errorf("IsEnabled() = true, want false for config %+v", tt.cfg)
			}
		})
	}
}

// TestService_Send_Disabled verifies Send refuses to send (and therefore
// never dials the network) when the service was constructed without a
// reachable SMTP server.
func TestService_Send_Disabled(t *testing.T) {
	svc := New(SMTPConfig{})

	err := svc.Send([]string{"user@example.com"}, "subject", "body")
	if err == nil {
		t.Fatal("Send() error = nil, want error when SMTP disabled")
	}
	if !strings.Contains(err.Error(), "email disabled") {
		t.Errorf("Send() error = %q, want it to mention 'email disabled'", err.Error())
	}
}

// TestService_SendTemplate_Disabled mirrors TestService_Send_Disabled for
// the template-based send path.
func TestService_SendTemplate_Disabled(t *testing.T) {
	svc := New(SMTPConfig{})

	err := svc.SendTemplate([]string{"user@example.com"}, "welcome", nil)
	if err == nil {
		t.Fatal("SendTemplate() error = nil, want error when SMTP disabled")
	}
	if !strings.Contains(err.Error(), "email disabled") {
		t.Errorf("SendTemplate() error = %q, want it to mention 'email disabled'", err.Error())
	}
}

// TestService_SendTemplate_LoadTemplateFailure exercises the "enabled but
// bad template name" path. This is white-box: it builds a Service literal
// directly with enabled:true instead of going through New(), which would
// otherwise dial a real SMTP server. Since LoadTemplate fails before Send
// is ever called, this never touches the network.
func TestService_SendTemplate_LoadTemplateFailure(t *testing.T) {
	svc := &Service{enabled: true, config: SMTPConfig{Host: "smtp.example.com", Port: 587, From: "noreply@example.com"}}

	err := svc.SendTemplate([]string{"user@example.com"}, "does-not-exist", nil)
	if err == nil {
		t.Fatal("SendTemplate() error = nil, want error for missing template")
	}
	if !strings.Contains(err.Error(), "failed to load template") {
		t.Errorf("SendTemplate() error = %q, want it to mention 'failed to load template'", err.Error())
	}
}

// TestLoadTemplate_Embedded checks that every embedded template can be
// loaded and parsed into a non-empty subject and body.
func TestLoadTemplate_Embedded(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
	}{
		{"welcome", "welcome"},
		{"test", "test"},
		{"password_reset", "password_reset"},
		{"password_changed", "password_changed"},
		{"email_verify", "email_verify"},
		{"login_alert", "login_alert"},
		{"2fa_enabled", "2fa_enabled"},
		{"2fa_disabled", "2fa_disabled"},
		{"mfa_reminder", "mfa_reminder"},
		{"security_alert", "security_alert"},
		{"breach_notification", "breach_notification"},
		{"breach_admin_alert", "breach_admin_alert"},
		{"backup_complete", "backup_complete"},
		{"backup_failed", "backup_failed"},
		{"ssl_expiring", "ssl_expiring"},
		{"ssl_renewed", "ssl_renewed"},
		{"scheduler_error", "scheduler_error"},
		{"weather_alert", "weather_alert"},
		{"weather_alert_update", "weather_alert_update"},
		{"weather_alert_expired", "weather_alert_expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := LoadTemplate(tt.templateName)
			if err != nil {
				t.Fatalf("LoadTemplate(%q) error = %v, want nil", tt.templateName, err)
			}
			if tmpl.Name != tt.templateName {
				t.Errorf("Name = %q, want %q", tmpl.Name, tt.templateName)
			}
			if tmpl.Subject == "" {
				t.Errorf("Subject is empty for template %q", tt.templateName)
			}
			if tmpl.Body == "" {
				t.Errorf("Body is empty for template %q", tt.templateName)
			}
		})
	}
}

// TestLoadTemplate_NotFound verifies the "template not found" error path
// for a template name that exists neither as a custom nor an embedded file.
func TestLoadTemplate_NotFound(t *testing.T) {
	// Ensure no CONFIG_DIR override interferes with this lookup.
	t.Setenv("CONFIG_DIR", t.TempDir())

	_, err := LoadTemplate("this-template-does-not-exist")
	if err == nil {
		t.Fatal("LoadTemplate() error = nil, want error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "template not found") {
		t.Errorf("LoadTemplate() error = %q, want it to mention 'template not found'", err.Error())
	}
}

// TestLoadTemplate_CustomOverridesEmbedded verifies the documented
// precedence: a custom template under {CONFIG_DIR}/template/email/ wins
// over the embedded fallback (AI.md lines 22768-22778: "Check custom
// first, fallback to embedded").
func TestLoadTemplate_CustomOverridesEmbedded(t *testing.T) {
	dir := t.TempDir()
	emailDir := filepath.Join(dir, "template", "email")
	if err := os.MkdirAll(emailDir, 0o755); err != nil {
		t.Fatalf("failed to create template dir: %v", err)
	}

	custom := "Subject: Custom Welcome {app_name}\n---\nCUSTOM BODY {app_name}\n"
	if err := os.WriteFile(filepath.Join(emailDir, "welcome.txt"), []byte(custom), 0o644); err != nil {
		t.Fatalf("failed to write custom template: %v", err)
	}

	t.Setenv("CONFIG_DIR", dir)

	tmpl, err := LoadTemplate("welcome")
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v, want nil", err)
	}
	if tmpl.Subject != "Custom Welcome {app_name}" {
		t.Errorf("Subject = %q, want custom override", tmpl.Subject)
	}
	if !strings.Contains(tmpl.Body, "CUSTOM BODY") {
		t.Errorf("Body = %q, want custom override content", tmpl.Body)
	}
}

// TestLoadTemplate_FallsBackWhenCustomMissing verifies that when CONFIG_DIR
// is set but no matching custom template file exists there, LoadTemplate
// still falls back to the embedded template rather than erroring.
func TestLoadTemplate_FallsBackWhenCustomMissing(t *testing.T) {
	t.Setenv("CONFIG_DIR", t.TempDir())

	tmpl, err := LoadTemplate("welcome")
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v, want nil (embedded fallback)", err)
	}
	if tmpl.Subject == "" {
		t.Error("Subject is empty, want embedded fallback content")
	}
}

// TestParseTemplate covers the "Subject: ...\n---\n{body}" format rules
// documented at AI.md lines 22926-22930, including malformed inputs.
func TestParseTemplate(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		wantSubject string
		wantBody    string
	}{
		{
			name:        "valid template",
			content:     "Subject: Hello {name}\n---\nBody line 1\nBody line 2",
			wantErr:     false,
			wantSubject: "Hello {name}",
			wantBody:    "Body line 1\nBody line 2",
		},
		{
			name:        "subject has surrounding whitespace",
			content:     "Subject:   Hello World   \n---\nbody",
			wantErr:     false,
			wantSubject: "Hello World",
			wantBody:    "body",
		},
		{
			name:        "empty body after separator",
			content:     "Subject: Hi\n---\n",
			wantErr:     false,
			wantSubject: "Hi",
			wantBody:    "",
		},
		{
			name:    "too few lines",
			content: "Subject: Hi\n---",
			wantErr: true,
		},
		{
			name:    "missing Subject prefix",
			content: "Hello\n---\nbody",
			wantErr: true,
		},
		{
			name:    "missing separator",
			content: "Subject: Hi\nno separator here\nmore body",
			wantErr: true,
		},
		{
			name:    "empty content",
			content: "",
			wantErr: true,
		},
		{
			name:        "separator with surrounding whitespace",
			content:     "Subject: Hi\n   ---   \nbody",
			wantErr:     false,
			wantSubject: "Hi",
			wantBody:    "body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := parseTemplate("test-name", tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTemplate() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTemplate() error = %v, want nil", err)
			}
			if tmpl.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", tmpl.Subject, tt.wantSubject)
			}
			if tmpl.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", tmpl.Body, tt.wantBody)
			}
			if tmpl.Name != "test-name" {
				t.Errorf("Name = %q, want %q", tmpl.Name, "test-name")
			}
		})
	}
}

// TestTemplate_Render covers variable substitution using the {variable}
// syntax documented at AI.md line 22908, including the documented (or
// implied) behavior for missing/extra variables.
func TestTemplate_Render(t *testing.T) {
	tests := []struct {
		name        string
		tmpl        Template
		vars        map[string]string
		wantSubject string
		wantBody    string
	}{
		{
			name:        "single variable substituted in subject and body",
			tmpl:        Template{Subject: "Hi {name}", Body: "Welcome, {name}!"},
			vars:        map[string]string{"name": "Alice"},
			wantSubject: "Hi Alice",
			wantBody:    "Welcome, Alice!",
		},
		{
			name:        "multiple variables",
			tmpl:        Template{Subject: "{greeting} {name}", Body: "{greeting}, {name}. Visit {url}."},
			vars:        map[string]string{"greeting": "Hello", "name": "Bob", "url": "https://example.com"},
			wantSubject: "Hello Bob",
			wantBody:    "Hello, Bob. Visit https://example.com.",
		},
		{
			name:        "missing variable left as literal placeholder",
			tmpl:        Template{Subject: "Hi {name}", Body: "Token: {missing_var}"},
			vars:        map[string]string{"name": "Alice"},
			wantSubject: "Hi Alice",
			wantBody:    "Token: {missing_var}",
		},
		{
			name:        "no variables provided",
			tmpl:        Template{Subject: "Static subject", Body: "Static body"},
			vars:        nil,
			wantSubject: "Static subject",
			wantBody:    "Static body",
		},
		{
			name:        "repeated placeholder replaced everywhere",
			tmpl:        Template{Subject: "{name} {name}", Body: "{name}-{name}-{name}"},
			vars:        map[string]string{"name": "X"},
			wantSubject: "X X",
			wantBody:    "X-X-X",
		},
		{
			name:        "case sensitive placeholder keys",
			tmpl:        Template{Subject: "{Name}", Body: "{name}"},
			vars:        map[string]string{"name": "lower"},
			wantSubject: "{Name}",
			wantBody:    "lower",
		},
		{
			name:        "empty string value clears placeholder",
			tmpl:        Template{Subject: "Hi {name}!", Body: "[{name}]"},
			vars:        map[string]string{"name": ""},
			wantSubject: "Hi !",
			wantBody:    "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSubject, gotBody, err := tt.tmpl.Render(tt.vars)
			if err != nil {
				t.Fatalf("Render() error = %v, want nil", err)
			}
			if gotSubject != tt.wantSubject {
				t.Errorf("subject = %q, want %q", gotSubject, tt.wantSubject)
			}
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
		})
	}
}

// TestLoadTemplate_RenderRoundTrip loads a real embedded template and
// renders it end to end, confirming the load -> parse -> render pipeline
// works together (not just each stage in isolation).
func TestLoadTemplate_RenderRoundTrip(t *testing.T) {
	tmpl, err := LoadTemplate("welcome")
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}

	vars := map[string]string{
		"app_name":           "WTHR",
		"recipient_email":    "user@example.com",
		"recipient_username": "user",
		"fqdn":               "wthr.example.com",
		"login_url":          "https://wthr.example.com/login",
		"profile_url":        "https://wthr.example.com/profile",
		"admin_email":        "admin@example.com",
		"app_url":            "https://wthr.example.com",
	}

	subject, body, err := tmpl.Render(vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(subject, "WTHR") {
		t.Errorf("subject = %q, want it to contain app_name", subject)
	}
	if !strings.Contains(body, "user@example.com") {
		t.Errorf("body does not contain recipient_email: %q", body)
	}
	if strings.Contains(body, "{app_name}") {
		t.Errorf("body still contains unreplaced {app_name} placeholder: %q", body)
	}
}
