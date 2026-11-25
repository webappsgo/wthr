package service

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/i18n"
	appconfig "github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// translate looks up a key via the global i18n instance in the server's
// default language, falling back to fallback when i18n hasn't been
// initialized yet (e.g. unit tests constructing SMTPService directly).
// SMTPService has no gin.Context (it also runs from schedulers/GraphQL/CLI
// paths), so per-request language selection isn't available here — per
// AI.md PART 31's CLI/Agent/Server Output Translation fallback chain, the
// server default language is the correct fallback for non-request output.
func translate(key, fallback string) string {
	inst := i18n.GetGlobalI18n()
	if inst == nil {
		return fallback
	}
	text := inst.T(inst.GetDefaultLanguage(), key)
	if text == key {
		return fallback
	}
	return text
}

// SMTPConfig represents SMTP configuration
type SMTPConfig struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          string `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	FromAddress   string `json:"from_address"`
	FromName      string `json:"from_name"`
	UseTLS        bool   `json:"use_tls"`
	TLSMode       string `json:"tls_mode"`
	AutoEnable    bool   `json:"auto_enable"`
	TestRecipient string `json:"test_recipient"`
}

// SMTPService handles email sending
type SMTPService struct {
	db     *database.Store
	config *SMTPConfig
	// verified records whether a working SMTP handshake was confirmed during
	// this process lifetime. AI.md PART 18 requires email features to be
	// enabled only while a real server answers, never from config presence
	// alone, so every IsEnabled check consults this flag.
	verified bool
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
func NewSMTPService(db *database.Store) *SMTPService {
	return &SMTPService{
		db: db,
	}
}

// sharedSMTPInstances maps a server.db handle to the single SMTPService bound
// to it. AI.md PART 18 requires the working-SMTP check to happen "once at
// startup and on config change", so every caller for a given database must
// share one instance: a caller that built its own SMTPService would never see
// the startup handshake and would wrongly report email as unavailable. Keying
// on the handle (rather than one process-wide slot) keeps test databases
// isolated from each other.
var sharedSMTPInstances sync.Map

// SharedSMTPService returns the single SMTPService bound to db, creating it on
// first use. Callers must use this instead of NewSMTPService so the verified
// state computed at startup is visible to every email-dependent feature.
func SharedSMTPService(db *database.Store) *SMTPService {
	if existing, ok := sharedSMTPInstances.Load(db); ok {
		return existing.(*SMTPService)
	}
	created, _ := sharedSMTPInstances.LoadOrStore(db, &SMTPService{db: db})
	return created.(*SMTPService)
}

// serverDB returns the server.db handle this SMTP service was constructed
// with. SMTP provider records live in server_notification_channels; settings
// are read from the owning configuration files.
// Fallback: when the injected handle is nil (unit tests, or construction
// before the global dual DB is wired) the process-global server handle is used
// instead, so a nil handle degrades to the previous behavior rather than
// panicking.
func (s *SMTPService) serverDB() *database.Store {
	if s.db != nil {
		return s.db
	}
	return database.GetServerStore()
}

// GetConfig returns the current SMTP configuration
func (s *SMTPService) GetConfig() *SMTPConfig {
	return s.config
}

// IsEnabled reports whether email features may be used right now.
// AI.md PART 18: "SMTP configured and working" is the only state that
// enables email — a configured-but-unreachable host, or no host at all,
// must leave every email feature disabled.
func (s *SMTPService) IsEnabled() bool {
	if s.config == nil {
		_ = s.LoadConfig()
	}
	return s.config != nil && s.config.Host != "" && s.config.FromAddress != "" && s.verified
}

// VerifyConfiguredConnection tests the configured SMTP host and records the
// result. AI.md PART 18 requires this check once at startup and again on
// every config change: success enables email, failure disables it with a
// warning and leaves the server running.
func (s *SMTPService) VerifyConfiguredConnection() error {
	// Always re-read rather than trusting the cached config: this is the
	// "on config change" half of AI.md PART 18's "check SMTP status once at
	// startup and on config change", and a stale cached host would verify a
	// server the admin has already replaced.
	if err := s.LoadConfig(); err != nil {
		s.verified = false
		return err
	}

	if s.config.Host == "" {
		s.verified = false
		return nil
	}

	if err := s.TestConnection(s.config); err != nil {
		s.verified = false
		return err
	}
	s.verified = true
	return nil
}

// LoadConfig loads SMTP configuration from file-backed settings and environment
func (s *SMTPService) LoadConfig() error {
	smtpCfg := &SMTPConfig{}

	// Load from configuration files first.
	enabled, _ := s.getSetting("smtp.enabled")
	host, _ := s.getSetting("smtp.host")
	port, _ := s.getSetting("smtp.port")
	username, _ := s.getSetting("smtp.username")
	password, secretErr := appconfig.ReadSecret("server.notifications.email.smtp.password")
	if secretErr != nil {
		return fmt.Errorf("read SMTP password secret: %w", secretErr)
	}
	fromAddr, _ := s.getSetting("from.email")
	if fromAddr == "" {
		fromAddr, _ = s.getSetting("smtp.from_address")
	}
	fromName, _ := s.getSetting("from.name")
	if fromName == "" {
		fromName, _ = s.getSetting("smtp.from_name")
	}
	tlsMode, _ := s.getSetting("smtp.tls")
	if tlsMode == "" {
		tlsMode, _ = s.getSetting("smtp.encryption")
	}
	useTLS, _ := s.getSetting("smtp.use_tls")
	autoEnable, _ := s.getSetting("smtp.auto_enable")
	testRecipient, _ := s.getSetting("smtp.test_recipient")

	// Environment variables override the config file, per AI.md PART 18's
	// Environment Variable Priority table. This is deliberately an override
	// (not a fallback): the table states SMTP_* env vars override config.
	if envHost := os.Getenv("SMTP_HOST"); envHost != "" {
		host = envHost
	}
	if envPort := os.Getenv("SMTP_PORT"); envPort != "" {
		port = envPort
	}
	if port == "" {
		port = "587"
	}
	if envUser := os.Getenv("SMTP_USERNAME"); envUser != "" {
		username = envUser
	}
	if envPass := os.Getenv("SMTP_PASSWORD"); envPass != "" {
		password = envPass
	}
	if envFrom := os.Getenv("SMTP_FROM_EMAIL"); envFrom != "" {
		fromAddr = envFrom
	}
	if envName := os.Getenv("SMTP_FROM_NAME"); envName != "" {
		fromName = envName
	}
	if tlsMode == "" {
		if truth, err := appconfig.ParseBool(useTLS, false); err == nil && truth {
			tlsMode = "starttls"
		} else {
			tlsMode = "auto"
		}
	}
	if envTLS := strings.TrimSpace(strings.ToLower(os.Getenv("SMTP_TLS"))); envTLS != "" {
		tlsMode = envTLS
	}
	if tlsMode == "true" || tlsMode == "1" || tlsMode == "yes" {
		tlsMode = "starttls"
	}
	if tlsMode == "false" || tlsMode == "0" || tlsMode == "no" {
		tlsMode = "none"
	}
	if !validSMTPMode(tlsMode) {
		return fmt.Errorf("invalid smtp.tls mode %q", tlsMode)
	}
	if fromName == "" {
		fromName = translate("app.name", "Weather")
	}

	smtpCfg.Enabled, _ = appconfig.ParseBool(enabled, false)
	smtpCfg.Host = host
	smtpCfg.Port = port
	smtpCfg.Username = username
	smtpCfg.Password = password
	smtpCfg.FromAddress = fromAddr
	smtpCfg.FromName = fromName
	smtpCfg.TLSMode = tlsMode
	smtpCfg.UseTLS = tlsMode == "tls" || tlsMode == "starttls" || (tlsMode == "auto" && port == "465")
	smtpCfg.AutoEnable, _ = appconfig.ParseBool(autoEnable, false)
	smtpCfg.TestRecipient = testRecipient

	s.config = smtpCfg
	return nil
}

// AutoDetect attempts to auto-detect SMTP server
// AI.md PART 18: reuse the shared host/port priority list and require a
// real EHLO handshake, so a non-SMTP listener is never reported as mail.
func (s *SMTPService) AutoDetect() (bool, error) {
	if s.config == nil {
		if err := s.LoadConfig(); err != nil {
			return false, err
		}
	}

	host, port := util.AutoDetectSMTP()
	if host == "" {
		return false, fmt.Errorf("no SMTP server detected")
	}

	// AI.md PART 18 auto-detection step 4: save the detected host:port and
	// enable email features. A successful detection is a verified handshake,
	// so the service is usable immediately.
	portText := strconv.Itoa(port)
	for _, setting := range [][2]string{{"smtp.host", host}, {"smtp.port", portText}, {"smtp.enabled", "true"}} {
		if err := s.saveSetting(setting[0], setting[1]); err != nil {
			return false, fmt.Errorf("save detected SMTP setting %s: %w", setting[0], err)
		}
	}
	s.config.Host = host
	s.config.Port = portText
	s.config.UseTLS = port == 465
	s.config.TLSMode = "auto"
	s.config.Enabled = true
	s.verified = true
	return true, nil
}

// TestConnection tests the SMTP connection
// AI.md PART 18: "Attempt SMTP handshake (EHLO)" — a bare TCP connect is
// not enough, any listener must actually answer EHLO with a 2xx reply.
func (s *SMTPService) TestConnection(config *SMTPConfig) error {
	if config == nil {
		config = s.config
	}

	if config == nil || config.Host == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	addr := net.JoinHostPort(config.Host, config.Port)

	mode := config.TLSMode
	if mode == "" {
		if config.UseTLS {
			mode = "tls"
		} else {
			mode = "none"
		}
	}
	if err := smtpHandshakeMode(addr, mode); err != nil {
		return fmt.Errorf("SMTP handshake with %s failed: %w", addr, err)
	}
	return nil
}

func validSMTPMode(mode string) bool {
	switch mode {
	case "auto", "starttls", "tls", "none":
		return true
	default:
		return false
	}
}

func smtpTLSConfig(host string) *tls.Config {
	settings := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	if address := net.ParseIP(host); address != nil && isPrivateSMTPAddress(address) {
		settings.InsecureSkipVerify = true //nolint:gosec // AI.md PART 18 permits private literal relays while retaining encryption.
	}
	return settings
}

func isPrivateSMTPAddress(address net.IP) bool {
	if address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return true
	}
	for _, network := range []string{"100.64.0.0/10"} {
		_, block, _ := net.ParseCIDR(network)
		if block.Contains(address) {
			return true
		}
	}
	return false
}

func smtpHandshakeMode(addr, mode string) error {
	host := hostFromAddr(addr)
	auto := mode == "auto"
	if auto {
		if _, port, err := net.SplitHostPort(addr); err == nil && port == "465" {
			mode = "tls"
		} else {
			mode = "starttls"
		}
	}
	if mode == "tls" {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, smtpTLSConfig(host))
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()
		return client.Hello("wthr")
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if err := client.Hello("wthr"); err != nil {
		return err
	}
	if mode == "none" {
		return nil
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		if auto {
			return nil
		}
		return fmt.Errorf("SMTP server does not offer STARTTLS")
	}
	return client.StartTLS(smtpTLSConfig(host))
}

// hostFromAddr strips the port from a host:port pair so TLS SNI and error
// messages never carry a port number.
func hostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// SendEmail sends an email
func (s *SMTPService) SendEmail(to, subject, body string) error {
	if s.config == nil {
		if err := s.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// AI.md PART 18: "ALL emails require a valid and working SMTP server. No
	// SMTP = No emails. Don't even try." A configured-but-unverified host must
	// not reach smtp.SendMail, so gate on the same check as every caller-facing
	// feature.
	if !s.IsEnabled() {
		return fmt.Errorf("SMTP not configured or not reachable")
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
	addr := net.JoinHostPort(s.config.Host, s.config.Port)

	var auth smtp.Auth
	if s.config.Username != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	err := sendSMTPMessage(addr, s.config.Host, s.config.TLSMode, auth, s.config.FromAddress, []string{to}, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// SendEmailAttachment sends a plain text message with one base64 encoded attachment.
func (s *SMTPService) SendEmailAttachment(to, subject, body, filename, contentType string, data []byte) error {
	if s.config == nil {
		if err := s.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}
	if !s.IsEnabled() {
		return fmt.Errorf("SMTP not configured or not reachable")
	}
	boundary := fmt.Sprintf("wthr-%d", time.Now().UnixNano())
	from := s.config.FromAddress
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddress)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	var msg strings.Builder
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\nDate: %s\r\n\r\n", from, to, subject, boundary, time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, body)
	fmt.Fprintf(&msg, "--%s\r\nContent-Type: %s; name=%q\r\nContent-Disposition: attachment; filename=%q\r\nContent-Transfer-Encoding: base64\r\n\r\n", boundary, contentType, filename, filename)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		msg.WriteString(encoded[i:end] + "\r\n")
	}
	fmt.Fprintf(&msg, "--%s--\r\n", boundary)
	addr := net.JoinHostPort(s.config.Host, s.config.Port)
	var auth smtp.Auth
	if s.config.Username != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}
	if err := sendSMTPMessage(addr, s.config.Host, s.config.TLSMode, auth, s.config.FromAddress, []string{to}, []byte(msg.String())); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

func sendSMTPMessage(addr, host, mode string, auth smtp.Auth, from string, recipients []string, message []byte) error {
	if mode == "" {
		mode = "auto"
	}
	implicitTLS := mode == "tls"
	auto := mode == "auto"
	if auto {
		if _, port, err := net.SplitHostPort(addr); err == nil && port == "465" {
			implicitTLS = true
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if implicitTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, smtpTLSConfig(host))
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if err := client.Hello("wthr"); err != nil {
		return err
	}
	if !implicitTLS && mode != "none" {
		supportsTLS, _ := client.Extension("STARTTLS")
		if supportsTLS {
			if err := client.StartTLS(smtpTLSConfig(host)); err != nil {
				return err
			}
		} else if mode == "starttls" {
			return fmt.Errorf("SMTP server does not offer STARTTLS")
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// SendTestEmail sends a test email
func (s *SMTPService) SendTestEmail(to string) error {
	appName := translate("app.name", "Weather")

	subject := strings.ReplaceAll(translate("email.subjects.smtp_test", "{app_name} SMTP Test"), "{app_name}", appName)
	heading := translate("email.body.smtp_test_heading", "SMTP Test Successful")
	intro := strings.ReplaceAll(translate("email.body.smtp_test_intro", "This is a test email from the {app_name} notification system."), "{app_name}", appName)
	success := translate("email.body.smtp_test_success", "If you received this email, your SMTP configuration is working correctly.")
	configLabel := translate("email.body.smtp_test_config_label", "Configuration:")
	hostLabel := translate("email.body.smtp_test_host_label", "Host")
	portLabel := translate("email.body.smtp_test_port_label", "Port")
	tlsLabel := translate("email.body.smtp_test_tls_label", "TLS")
	sentAt := strings.ReplaceAll(translate("email.body.smtp_test_sent_at", "Sent at {time}"), "{time}", time.Now().Format(time.RFC1123))

	body := `
	<html>
	<body>
		<h2>` + heading + `</h2>
		<p>` + intro + `</p>
		<p>` + success + `</p>
		<p><strong>` + configLabel + `</strong></p>
		<ul>
			<li>` + hostLabel + `: ` + s.config.Host + `</li>
			<li>` + portLabel + `: ` + s.config.Port + `</li>
			<li>` + tlsLabel + `: ` + fmt.Sprintf("%t", s.config.UseTLS) + `</li>
		</ul>
		<p><em>` + sentAt + `</em></p>
	</body>
	</html>
	`

	return s.SendEmail(to, subject, body)
}

// EnableChannel enables the SMTP notification channel
func (s *SMTPService) EnableChannel() error {
	// Update channel state to enabled
	_, err := database.ExecContext(context.Background(), s.serverDB(), database.TimeoutWrite, `
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
	value, exists, err := appconfig.ReadSetting(key)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", sql.ErrNoRows
	}
	return fmt.Sprint(value), nil
}

func (s *SMTPService) saveSetting(key, value string) error {
	var typedValue any = value
	switch key {
	case "smtp.enabled":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean setting %s: %w", key, err)
		}
		typedValue = parsed
	case "smtp.port":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid numeric setting %s: %w", key, err)
		}
		typedValue = parsed
	}
	return appconfig.WriteSetting(key, typedValue)
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
