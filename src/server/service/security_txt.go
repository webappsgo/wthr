package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	paths "github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/model"
)

// SecurityTxtService handles security.txt generation per RFC 9116
type SecurityTxtService struct {
	settingsModel *model.SettingsModel
	keyDir        string
}

// NewSecurityTxtService creates a new security.txt service
func NewSecurityTxtService(settingsModel *model.SettingsModel) *SecurityTxtService {
	return &SecurityTxtService{
		settingsModel: settingsModel,
		keyDir:        filepath.Join(paths.GetConfigDir(), "security"),
	}
}

// Generate creates a security.txt file following RFC 9116
func (s *SecurityTxtService) Generate(baseURL string) string {
	var lines []string

	// Contact (required): the repo's private vulnerability reporting URL, then the instance
	// security-report form with the current security_id, last the mailto address (PART 11)
	if reportURL := s.settingsModel.GetString("web.security.report_url", ""); reportURL != "" {
		lines = append(lines, fmt.Sprintf("Contact: %s", reportURL))
	}
	if s.settingsModel.DB != nil {
		if id, err := CurrentSecurityID(s.settingsModel.DB); err == nil {
			lines = append(lines, fmt.Sprintf("Contact: %s/server/contact?security_id=%s", baseURL, id))
		}
	}
	contact := s.settingsModel.GetString("security.contact", "")
	if contact == "" {
		contact = config.DefaultEmailAddress("security", config.GetGlobalConfig())
	}
	for _, c := range strings.Split(contact, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.HasPrefix(c, "mailto:") && !strings.HasPrefix(c, "http") && strings.Contains(c, "@") {
			c = "mailto:" + c
		}
		lines = append(lines, fmt.Sprintf("Contact: %s", c))
	}

	// Expires (required) - Auto-renew 1 year from now
	expiresStr := s.settingsModel.GetString("security.expires", "")
	var expires time.Time
	if expiresStr != "" {
		expires, _ = time.Parse(time.RFC3339, expiresStr)
	}
	if expires.IsZero() || expires.Before(time.Now()) {
		// Auto-renew 1 year from now
		expires = time.Now().AddDate(1, 0, 0)
		s.settingsModel.SetString("security.expires", expires.Format(time.RFC3339))
	}
	lines = append(lines, fmt.Sprintf("Expires: %s", expires.Format(time.RFC3339)))

	// Preferred-Languages (optional)
	languages := s.settingsModel.GetString("security.languages", "en")
	if languages != "" {
		lines = append(lines, fmt.Sprintf("Preferred-Languages: %s", languages))
	}

	// Canonical (recommended)
	canonical := s.settingsModel.GetString("security.canonical", "")
	if canonical == "" {
		canonical = baseURL + "/.well-known/security.txt"
	}
	lines = append(lines, fmt.Sprintf("Canonical: %s", canonical))

	// Encryption (optional)
	if s.settingsModel.DB != nil && NewPGPKeyManager(s.settingsModel.DB, s.keyDir).Exists() {
		lines = append(lines, fmt.Sprintf("Encryption: %s/.well-known/pgp-key.asc", baseURL))
	}

	// Acknowledgments (optional)
	lines = append(lines, fmt.Sprintf("Acknowledgments: %s/server/security/thanks", baseURL))

	// Policy (optional)
	lines = append(lines, fmt.Sprintf("Policy: %s/server/security", baseURL))

	// Hiring (optional)
	hiring := s.settingsModel.GetString("security.hiring", "")
	if hiring != "" {
		lines = append(lines, fmt.Sprintf("Hiring: %s", hiring))
	}

	// Build the final security.txt content
	var content strings.Builder

	// Add comment header
	content.WriteString("# Security Contact Information\n")
	content.WriteString(fmt.Sprintf("# Generated: %s\n", time.Now().Format(time.RFC3339)))
	content.WriteString("# This file follows RFC 9116\n")
	content.WriteString("# https://www.rfc-editor.org/rfc/rfc9116.html\n\n")

	// Add all fields
	for _, line := range lines {
		content.WriteString(line + "\n")
	}

	return content.String()
}

// GetConfig returns current security.txt configuration
func (s *SecurityTxtService) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"contact":    s.settingsModel.GetString("security.contact", ""),
		"expires":    s.settingsModel.GetString("security.expires", ""),
		"languages":  s.settingsModel.GetString("security.languages", "en"),
		"canonical":  s.settingsModel.GetString("security.canonical", ""),
		"report_url": s.settingsModel.GetString("web.security.report_url", ""),
		"keyservers": s.settingsModel.GetString("web.security.keyservers", ""),
		"hiring":     s.settingsModel.GetString("security.hiring", ""),
	}
}

// UpdateConfig updates security.txt configuration
func (s *SecurityTxtService) UpdateConfig(config map[string]string) error {
	for key, value := range config {
		settingKey := "security." + key
		if err := s.settingsModel.SetString(settingKey, value); err != nil {
			return err
		}
	}
	return nil
}

// CheckExpiry checks if security.txt needs renewal
func (s *SecurityTxtService) CheckExpiry() (bool, time.Duration) {
	expiresStr := s.settingsModel.GetString("security.expires", "")
	if expiresStr == "" {
		// Needs renewal
		return true, 0
	}

	expires, err := time.Parse(time.RFC3339, expiresStr)
	if err != nil {
		return true, 0
	}

	// If expired or expiring within 30 days, needs renewal
	until := time.Until(expires)
	needsRenewal := until < 30*24*time.Hour

	return needsRenewal, until
}

// AutoRenew automatically renews the expiry date
func (s *SecurityTxtService) AutoRenew() error {
	// 1 year from now
	expires := time.Now().AddDate(1, 0, 0)
	return s.settingsModel.SetString("security.expires", expires.Format(time.RFC3339))
}
