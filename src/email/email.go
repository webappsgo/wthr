// Package email - SMTP email sending per AI.md PART 26
// AI.md Reference: Lines 22754-24087
package email

import (
	"crypto/tls"
	"embed"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Embedded templates per AI.md line 22772
//
//go:embed template/*.txt
var embeddedTemplates embed.FS

// Service handles email sending
// Per AI.md line 22782: "No SMTP = No emails. Don't even try."
type Service struct {
	enabled bool
	config  SMTPConfig
}

// SMTPConfig holds SMTP server configuration
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	TLS      bool
}

// New creates a new email service
// Per AI.md line 22808: "Check SMTP status once at startup and on config change"
func New(cfg SMTPConfig) *Service {
	svc := &Service{
		config:  cfg,
		enabled: false,
	}

	// Check if SMTP is configured per AI.md line 22786
	if cfg.Host != "" && cfg.Port > 0 {
		// Validate SMTP connection
		if err := svc.validateSMTP(); err == nil {
			svc.enabled = true
		}
	}

	return svc
}

// IsEnabled returns whether email functionality is available
// Per AI.md line 22809: "Completely disable email features if no SMTP"
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// validateSMTP checks if SMTP server is reachable and working
// Per AI.md line 22787: "SMTP configured but invalid → Validate on save, reject invalid config"
func (s *Service) validateSMTP() error {
	// Try to connect to SMTP server
	addr := net.JoinHostPort(s.config.Host, fmt.Sprintf("%d", s.config.Port))

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("SMTP connection failed: %w", err)
	}
	defer conn.Close()

	// If using TLS, verify TLS connection
	if s.config.TLS {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: s.config.Host,
		})
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("SMTP TLS handshake failed: %w", err)
		}
	}

	return nil
}

// Send sends an email
// Per AI.md lines 22802-22804: Only send if SMTP is enabled
func (s *Service) Send(to []string, subject, body string) error {
	// Per AI.md line 22786: "No SMTP configured → Email functionality completely disabled"
	if !s.enabled {
		return fmt.Errorf("email disabled: SMTP not configured")
	}

	// Construct message
	from := s.config.From
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.From)
	}

	// Build email message per RFC 5322
	msg := fmt.Sprintf("From: %s\r\n", from)
	msg += fmt.Sprintf("To: %s\r\n", strings.Join(to, ","))
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	msg += "Content-Type: text/plain; charset=UTF-8\r\n"
	msg += "\r\n"
	msg += body

	// Setup authentication
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)

	// Send email
	addr := net.JoinHostPort(s.config.Host, fmt.Sprintf("%d", s.config.Port))
	if err := smtp.SendMail(addr, auth, s.config.From, to, []byte(msg)); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// SendTemplate sends an email using a template
func (s *Service) SendTemplate(to []string, templateName string, vars map[string]string) error {
	// Per AI.md line 22786: "No SMTP configured → Email functionality completely disabled"
	if !s.enabled {
		return fmt.Errorf("email disabled: SMTP not configured")
	}

	// Load template
	tmpl, err := LoadTemplate(templateName)
	if err != nil {
		return fmt.Errorf("failed to load template: %w", err)
	}

	// Add global variables per AI.md lines 22932-22944
	if vars == nil {
		vars = make(map[string]string)
	}

	// Add timestamp and year
	now := time.Now()
	vars["timestamp"] = now.Format("2006-01-02 15:04:05 MST")
	vars["year"] = fmt.Sprintf("%d", now.Year())

	// Render template
	subject, body, err := tmpl.Render(vars)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	// Send email
	return s.Send(to, subject, body)
}

// Template represents an email template
type Template struct {
	Name    string
	Subject string
	Body    string
}

// LoadTemplate loads an email template
// Per AI.md lines 22768-22778: Check custom first, fallback to embedded
func LoadTemplate(name string) (*Template, error) {
	// Try custom template first
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "/etc/casapps/wthr"
	}
	customPath := filepath.Join(configDir, "template", "email", name+".txt")
	if content, err := os.ReadFile(customPath); err == nil {
		return parseTemplate(name, string(content))
	}

	// Fall back to embedded template per AI.md line 22772
	embeddedPath := "template/" + name + ".txt"
	content, err := embeddedTemplates.ReadFile(embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("template not found: %s", name)
	}

	return parseTemplate(name, string(content))
}

// parseTemplate parses template content
// Per AI.md lines 22926-22930: "Subject: ...\n---\n{body}"
func parseTemplate(name, content string) (*Template, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("invalid template format")
	}

	// First line should be "Subject: ..."
	if !strings.HasPrefix(lines[0], "Subject:") {
		return nil, fmt.Errorf("template must start with 'Subject:'")
	}

	subject := strings.TrimSpace(strings.TrimPrefix(lines[0], "Subject:"))

	// Find separator "---"
	bodyStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			bodyStart = i + 1
			break
		}
	}

	if bodyStart == -1 {
		return nil, fmt.Errorf("template missing '---' separator")
	}

	// Everything after separator is body
	body := strings.Join(lines[bodyStart:], "\n")

	return &Template{
		Name:    name,
		Subject: subject,
		Body:    body,
	}, nil
}

// Render renders the template with variables
// Per AI.md line 22908: Variables use {variable} syntax
func (t *Template) Render(vars map[string]string) (string, string, error) {
	subject := t.Subject
	body := t.Body

	// Replace all variables
	for key, value := range vars {
		placeholder := "{" + key + "}"
		subject = strings.ReplaceAll(subject, placeholder, value)
		body = strings.ReplaceAll(body, placeholder, value)
	}

	return subject, body, nil
}
