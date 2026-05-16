package service

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/apimgr/weather/src/database"
	"github.com/apimgr/weather/src/util"
)

// SMTPConfig represents SMTP configuration
type SMTPConfig struct {
	Host          string `json:"host"`
	Port          string `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	FromAddress   string `json:"from_address"`
	FromName      string `json:"from_name"`
	UseTLS        bool   `json:"use_tls"`
	AutoEnable    bool   `json:"auto_enable"`
	TestRecipient string `json:"test_recipient"`
}

// SMTPService handles email sending
type SMTPService struct {
	db     *sql.DB
	config *SMTPConfig
}

// SMTPProviderPreset represents a known SMTP provider configuration
type SMTPProviderPreset struct {
	Name     string
	Host     string
	Port     string
	UseTLS   bool
	Category string
}

// SMTPProviderPresets contains 40+ known SMTP providers
var SMTPProviderPresets = []SMTPProviderPreset{
	// Popular Email Services
	{"Gmail", "smtp.gmail.com", "587", true, "popular"},
	{"Outlook/Office365", "smtp.office365.com", "587", true, "popular"},
	{"Yahoo Mail", "smtp.mail.yahoo.com", "587", true, "popular"},
	{"iCloud", "smtp.mail.me.com", "587", true, "popular"},
	{"ProtonMail", "smtp.protonmail.com", "587", true, "popular"},

	// Business Email Services
	{"Zoho Mail", "smtp.zoho.com", "587", true, "business"},
	{"FastMail", "smtp.fastmail.com", "587", true, "business"},
	{"Mailgun", "smtp.mailgun.org", "587", true, "business"},
	{"SendGrid", "smtp.sendgrid.net", "587", true, "business"},
	{"Amazon SES (US East)", "email-smtp.us-east-1.amazonaws.com", "587", true, "business"},
	{"Amazon SES (EU West)", "email-smtp.eu-west-1.amazonaws.com", "587", true, "business"},
	{"Postmark", "smtp.postmarkapp.com", "587", true, "business"},
	{"SparkPost", "smtp.sparkpostmail.com", "587", true, "business"},
	{"Mailjet", "in-v3.mailjet.com", "587", true, "business"},
	{"Elastic Email", "smtp.elasticemail.com", "2525", true, "business"},

	// Transactional Email Services
	{"Mandrill (Mailchimp)", "smtp.mandrillapp.com", "587", true, "transactional"},
	{"Sendinblue", "smtp-relay.sendinblue.com", "587", true, "transactional"},
	{"Pepipost", "smtp.pepipost.com", "587", true, "transactional"},
	{"SocketLabs", "smtp.socketlabs.com", "587", true, "transactional"},

	// Hosting Providers
	{"GoDaddy", "smtpout.secureserver.net", "587", true, "hosting"},
	{"Bluehost", "mail.yourdomain.com", "587", true, "hosting"},
	{"HostGator", "mail.yourdomain.com", "587", true, "hosting"},
	{"Namecheap", "mail.privateemail.com", "587", true, "hosting"},
	{"DreamHost", "mail.yourdomain.com", "587", true, "hosting"},

	// European Providers
	{"GMX", "smtp.gmx.com", "587", true, "european"},
	{"Mail.ru", "smtp.mail.ru", "587", true, "european"},
	{"Yandex", "smtp.yandex.com", "587", true, "european"},
	{"1&1 IONOS", "smtp.ionos.com", "587", true, "european"},

	// Regional Providers
	{"AOL", "smtp.aol.com", "587", true, "regional"},
	{"AT&T", "smtp.att.yahoo.com", "587", true, "regional"},
	{"Comcast", "smtp.comcast.net", "587", true, "regional"},
	{"Verizon", "smtp.verizon.net", "587", true, "regional"},

	// Open Source / Self-Hosted
	{"Postfix (Default)", "localhost", "25", false, "self-hosted"},
	{"Postfix (Submission)", "localhost", "587", true, "self-hosted"},
	{"Sendmail", "localhost", "25", false, "self-hosted"},
	{"Exim", "localhost", "25", false, "self-hosted"},

	// Docker / Local Development
	{"Mailhog", "localhost", "1025", false, "development"},
	{"MailDev", "localhost", "1025", false, "development"},
	{"Mailcatcher", "localhost", "1025", false, "development"},
	{"FakeSMTP", "localhost", "2525", false, "development"},

	// Cloud Providers
	{"Google Workspace", "smtp-relay.gmail.com", "587", true, "cloud"},
	{"Azure Communication Services", "smtp.azurecomm.net", "587", true, "cloud"},
}

// NewSMTPService creates a new SMTP service
func NewSMTPService(db *sql.DB) *SMTPService {
	return &SMTPService{
		db: db,
	}
}

// GetConfig returns the current SMTP configuration
func (s *SMTPService) GetConfig() *SMTPConfig {
	return s.config
}

// IsEnabled returns whether SMTP is configured and ready to send
func (s *SMTPService) IsEnabled() bool {
	if s.config == nil {
		_ = s.LoadConfig()
	}
	return s.config != nil && s.config.Host != "" && s.config.FromAddress != ""
}

// LoadConfig loads SMTP configuration from database and environment
func (s *SMTPService) LoadConfig() error {
	config := &SMTPConfig{}

	// Load from database first
	host, _ := s.getSetting("smtp.host")
	port, _ := s.getSetting("smtp.port")
	username, _ := s.getSetting("smtp.username")
	password, _ := s.getSetting("smtp.password")
	fromAddr, _ := s.getSetting("smtp.from_address")
	fromName, _ := s.getSetting("smtp.from_name")
	useTLS, _ := s.getSetting("smtp.use_tls")
	autoEnable, _ := s.getSetting("smtp.auto_enable")
	testRecipient, _ := s.getSetting("smtp.test_recipient")

	// Environment variables as hints (database takes precedence)
	if host == "" {
		host = os.Getenv("SMTP_HOST")
	}
	if port == "" {
		port = os.Getenv("SMTP_PORT")
		if port == "" {
			port = "587"
		}
	}
	if username == "" {
		username = os.Getenv("SMTP_USERNAME")
	}
	if password == "" {
		password = os.Getenv("SMTP_PASSWORD")
	}
	if fromAddr == "" {
		fromAddr = os.Getenv("SMTP_FROM_ADDRESS")
	}
	if fromName == "" {
		fromName = os.Getenv("SMTP_FROM_NAME")
		if fromName == "" {
			fromName = "Weather Service"
		}
	}

	config.Host = host
	config.Port = port
	config.Username = username
	config.Password = password
	config.FromAddress = fromAddr
	config.FromName = fromName
	config.UseTLS = useTLS == "true"
	config.AutoEnable = autoEnable == "true"
	config.TestRecipient = testRecipient

	s.config = config
	return nil
}

// AutoDetect attempts to auto-detect SMTP server
// AI.md: Host-specific values detected at runtime
func (s *SMTPService) AutoDetect() (bool, error) {
	// Build list of SMTP servers to try
	candidates := []struct {
		host string
		port string
	}{
		{"localhost", "25"},
		{"127.0.0.1", "25"},
	}

	// AI.md: Detect Docker gateway at runtime, not hardcoded
	if gwIP := utils.GetDockerGatewayIP(); gwIP != "" {
		candidates = append(candidates, struct {
			host string
			port string
		}{gwIP, "25"})
	}

	// Add remaining candidates
	candidates = append(candidates, []struct {
		host string
		port string
	}{
		// Docker Desktop
		{"host.docker.internal", "25"},
		{"localhost", "587"},
		// Mailhog/MailDev
		{"localhost", "1025"},
	}...)

	for _, candidate := range candidates {
		conn, err := net.DialTimeout("tcp", candidate.host+":"+candidate.port, 2*time.Second)
		if err == nil {
			conn.Close()

			// Found a listening SMTP server
			s.config.Host = candidate.host
			s.config.Port = candidate.port
			s.config.UseTLS = candidate.port == "587"

			// Save to database
			s.saveSetting("smtp.host", candidate.host)
			s.saveSetting("smtp.port", candidate.port)

			return true, nil
		}
	}

	return false, fmt.Errorf("no SMTP server detected")
}

// TestConnection tests the SMTP connection
func (s *SMTPService) TestConnection(config *SMTPConfig) error {
	if config == nil {
		config = s.config
	}

	if config.Host == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	// Try to connect
	addr := fmt.Sprintf("%s:%s", config.Host, config.Port)

	if config.UseTLS {
		// TLS connection
		tlsConfig := &tls.Config{
			ServerName: config.Host,
		}

		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("TLS connection failed: %w", err)
		}
		defer conn.Close()
	} else {
		// Plain connection
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		defer conn.Close()
	}

	return nil
}

// SendEmail sends an email
func (s *SMTPService) SendEmail(to, subject, body string) error {
	if s.config == nil {
		if err := s.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	if s.config.Host == "" {
		return fmt.Errorf("SMTP not configured")
	}

	// Build message
	from := s.config.FromAddress
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddress)
	}

	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"
	headers["Date"] = time.Now().Format(time.RFC1123Z)

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	// Send email
	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	var auth smtp.Auth
	if s.config.Username != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	err := smtp.SendMail(addr, auth, s.config.FromAddress, []string{to}, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// SendTestEmail sends a test email
func (s *SMTPService) SendTestEmail(to string) error {
	subject := "Weather Service SMTP Test"
	body := `
	<html>
	<body>
		<h2>SMTP Test Successful</h2>
		<p>This is a test email from Weather Service notification system.</p>
		<p>If you received this email, your SMTP configuration is working correctly.</p>
		<p><strong>Configuration:</strong></p>
		<ul>
			<li>Host: ` + s.config.Host + `</li>
			<li>Port: ` + s.config.Port + `</li>
			<li>TLS: ` + fmt.Sprintf("%t", s.config.UseTLS) + `</li>
		</ul>
		<p><em>Sent at ` + time.Now().Format(time.RFC1123) + `</em></p>
	</body>
	</html>
	`

	return s.SendEmail(to, subject, body)
}

// EnableChannel enables the SMTP notification channel
func (s *SMTPService) EnableChannel() error {
	// Update channel state to enabled
	_, err := database.GetServerDB().Exec(`
		INSERT INTO server_notification_channels (channel_type, channel_name, enabled, state, config, updated_at)
		VALUES ('email', 'Email (SMTP)', 1, 'enabled', ?, ?)
		ON CONFLICT(channel_type) DO UPDATE SET
			enabled = 1,
			state = 'enabled',
			last_success_at = ?,
			updated_at = ?
	`, s.configToJSON(), time.Now(), time.Now(), time.Now())

	return err
}

// Helper methods

func (s *SMTPService) getSetting(key string) (string, error) {
	var value string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = ?", key).Scan(&value)
	return value, err
}

func (s *SMTPService) saveSetting(key, value string) error {
	_, err := database.GetServerDB().Exec(`
		INSERT INTO server_config (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, key, value, time.Now(), value, time.Now())
	return err
}

func (s *SMTPService) configToJSON() string {
	if s.config == nil {
		return "{}"
	}
	data, _ := json.Marshal(s.config)
	return string(data)
}

// GetProviderPreset returns preset configuration for a known provider
func GetProviderPreset(name string) (*SMTPProviderPreset, error) {
	for _, preset := range SMTPProviderPresets {
		if strings.EqualFold(preset.Name, name) {
			return &preset, nil
		}
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

// ListProviderPresets returns all provider presets
func ListProviderPresets() []SMTPProviderPreset {
	return SMTPProviderPresets
}

// ListProvidersByCategory returns providers filtered by category
func ListProvidersByCategory(category string) []SMTPProviderPreset {
	var result []SMTPProviderPreset
	for _, preset := range SMTPProviderPresets {
		if preset.Category == category {
			result = append(result, preset)
		}
	}
	return result
}
