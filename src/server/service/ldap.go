// Package service provides LDAP authentication per AI.md PART 11.
package service

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"strings"

	ldap "github.com/go-ldap/ldap/v3"

	models "github.com/casapps/wthr/src/server/model"
)

// LDAPConfig holds LDAP connection settings read from server_config.
type LDAPConfig struct {
	Enabled    bool
	Server     string
	Port       int
	BindDN     string
	BindPass   string
	BaseDN     string
	UserFilter string
}

// LDAPService authenticates users against an LDAP directory.
type LDAPService struct {
	settings *models.SettingsModel
}

// NewLDAPService creates a new LDAPService backed by the given DB.
func NewLDAPService(db *sql.DB) *LDAPService {
	return &LDAPService{settings: &models.SettingsModel{DB: db}}
}

// Config reads LDAP settings from server_config.
func (s *LDAPService) Config() LDAPConfig {
	return LDAPConfig{
		Enabled:    s.settings.GetBool("server.auth.ldap.enabled", false),
		Server:     s.settings.GetString("server.auth.ldap.server", ""),
		Port:       s.settings.GetInt("server.auth.ldap.port", 389),
		BindDN:     s.settings.GetString("server.auth.ldap.bind_dn", ""),
		BindPass:   s.settings.GetString("server.auth.ldap.bind_password", ""),
		BaseDN:     s.settings.GetString("server.auth.ldap.base_dn", ""),
		UserFilter: s.settings.GetString("server.auth.ldap.user_filter", "(uid=%s)"),
	}
}

// Authenticate verifies the username and password against the LDAP directory.
// Returns the user's email and display name on success.
func (s *LDAPService) Authenticate(username, password string) (email, displayName string, err error) {
	cfg := s.Config()
	if !cfg.Enabled {
		return "", "", fmt.Errorf("LDAP authentication is not enabled")
	}
	if cfg.Server == "" || cfg.BaseDN == "" {
		return "", "", fmt.Errorf("LDAP is not fully configured")
	}
	if username == "" || password == "" {
		return "", "", fmt.Errorf("username and password are required")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server, cfg.Port)

	var conn *ldap.Conn
	if cfg.Port == 636 {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{ServerName: cfg.Server, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer conn.Close()

	conn.SetTimeout(10e9)

	if cfg.Port != 636 {
		if tlsErr := conn.StartTLS(&tls.Config{ServerName: cfg.Server, MinVersion: tls.VersionTLS12}); tlsErr != nil {
			return "", "", fmt.Errorf("STARTTLS failed: %w", tlsErr)
		}
	}

	if cfg.BindDN != "" {
		if bindErr := conn.Bind(cfg.BindDN, cfg.BindPass); bindErr != nil {
			return "", "", fmt.Errorf("LDAP service bind failed: %w", bindErr)
		}
	}

	filter := fmt.Sprintf(cfg.UserFilter, ldap.EscapeFilter(username))
	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1,
		10,
		false,
		filter,
		[]string{"dn", "mail", "cn", "displayName", "sn", "givenName"},
		nil,
	)

	result, err := conn.Search(searchReq)
	if err != nil {
		return "", "", fmt.Errorf("LDAP search failed: %w", err)
	}
	if len(result.Entries) == 0 {
		return "", "", fmt.Errorf("invalid credentials")
	}

	userDN := result.Entries[0].DN
	if err = conn.Bind(userDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return "", "", fmt.Errorf("invalid credentials")
		}
		return "", "", fmt.Errorf("invalid credentials")
	}

	entry := result.Entries[0]
	email = entry.GetAttributeValue("mail")
	displayName = entry.GetAttributeValue("displayName")
	if displayName == "" {
		cn := entry.GetAttributeValue("cn")
		given := entry.GetAttributeValue("givenName")
		sn := entry.GetAttributeValue("sn")
		switch {
		case cn != "":
			displayName = cn
		case given != "" || sn != "":
			displayName = strings.TrimSpace(given + " " + sn)
		default:
			displayName = username
		}
	}

	return email, displayName, nil
}
