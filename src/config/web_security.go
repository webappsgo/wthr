package config

import (
	"net/url"
	"strings"
)

// CSRFConfig is the web.csrf block (AI.md PART 16 "CSRF Protection" -> Configuration).
// CSRF applies to the frontend only; the backend (/api/**) is never subject to it.
type CSRFConfig struct {
	// Set false ONLY for API-only deployments (no browser forms at all)
	Enabled bool `yaml:"enabled"`
	// Token length in bytes
	TokenLength int    `yaml:"token_length"`
	CookieName  string `yaml:"cookie_name"`
	HeaderName  string `yaml:"header_name"`
	// auto, true or false: auto sets Secure when the protocol is https
	Secure string `yaml:"secure"`
	// Frontend (non-/api) paths exempt from CSRF, operator-declared, glob patterns supported
	ExemptPaths []string `yaml:"exempt_paths"`
}

const (
	// DefaultCORSSetting is the shipped web.cors value: "*" means unset and lets the
	// allow-list resolution continue (AI.md PART 16 "CORS Allow-list Resolution Order")
	DefaultCORSSetting = "*"
	// MinCSRFTokenLength and MaxCSRFTokenLength bound web.csrf.token_length in bytes
	MinCSRFTokenLength = 16
	MaxCSRFTokenLength = 128
)

// DefaultCSRFConfig returns the AI.md PART 16 web.csrf defaults.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		Enabled:     true,
		TokenLength: 32,
		CookieName:  "csrf_token",
		HeaderName:  "X-CSRF-Token",
		Secure:      "auto",
		ExemptPaths: []string{},
	}
}

// IsValidCORSOrigin reports whether one web.cors entry is a bare origin: an http or
// https scheme and a host with an optional port, and nothing else.
func IsValidCORSOrigin(entry string) bool {
	parsed, err := url.Parse(strings.TrimSpace(entry))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != "" && parsed.User == nil && (parsed.Path == "" || parsed.Path == "/") &&
		parsed.RawQuery == "" && parsed.Fragment == ""
}

// IsValidCORSSetting reports whether a web.cors value is usable: "" (CORS off), "*",
// or a comma-separated list of bare origins.
func IsValidCORSSetting(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == DefaultCORSSetting {
		return true
	}
	for _, entry := range strings.Split(trimmed, ",") {
		if !IsValidCORSOrigin(entry) {
			return false
		}
	}
	return true
}

// isValidHeaderToken reports whether a name is a valid HTTP header or cookie name.
func isValidHeaderToken(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// validateWebSecurity replaces an invalid web.cors or web.csrf value with its default and
// logs a warning, per AI.md PART 12: a bad value never stops startup.
func validateWebSecurity(cfg *AppConfig) {
	if !IsValidCORSSetting(cfg.Web.CORS) {
		warnConfig("invalid web.cors %q, using default %q", cfg.Web.CORS, DefaultCORSSetting)
		cfg.Web.CORS = DefaultCORSSetting
	}

	defaults := DefaultCSRFConfig()
	csrf := &cfg.Web.CSRF
	if csrf.TokenLength < MinCSRFTokenLength || csrf.TokenLength > MaxCSRFTokenLength {
		if csrf.TokenLength != 0 {
			warnConfig("invalid web.csrf.token_length %d, using default %d", csrf.TokenLength, defaults.TokenLength)
		}
		csrf.TokenLength = defaults.TokenLength
	}
	if !isValidHeaderToken(csrf.CookieName) {
		if csrf.CookieName != "" {
			warnConfig("invalid web.csrf.cookie_name %q, using default %q", csrf.CookieName, defaults.CookieName)
		}
		csrf.CookieName = defaults.CookieName
	}
	if !isValidHeaderToken(csrf.HeaderName) {
		if csrf.HeaderName != "" {
			warnConfig("invalid web.csrf.header_name %q, using default %q", csrf.HeaderName, defaults.HeaderName)
		}
		csrf.HeaderName = defaults.HeaderName
	}
	switch strings.ToLower(strings.TrimSpace(csrf.Secure)) {
	case "auto", "true", "false":
		csrf.Secure = strings.ToLower(strings.TrimSpace(csrf.Secure))
	default:
		if csrf.Secure != "" {
			warnConfig("invalid web.csrf.secure %q, using default %q", csrf.Secure, defaults.Secure)
		}
		csrf.Secure = defaults.Secure
	}
	kept := make([]string, 0, len(csrf.ExemptPaths))
	for _, pattern := range csrf.ExemptPaths {
		pattern = strings.TrimSpace(pattern)
		if !strings.HasPrefix(pattern, "/") {
			warnConfig("ignoring web.csrf.exempt_paths entry %q: a path must start with /", pattern)
			continue
		}
		kept = append(kept, pattern)
	}
	csrf.ExemptPaths = kept
}

// UpdateWebCORS validates and persists web.cors. Messages are translation keys.
func UpdateWebCORS(value string) []ValidationError {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return []ValidationError{{Field: "web.cors", Message: "errors.admin.web.config_unavailable"}}
	}
	value = strings.TrimSpace(value)
	if !IsValidCORSSetting(value) {
		return []ValidationError{{Field: "web.cors", Message: "errors.admin.web.cors_invalid"}}
	}
	cfg.Web.CORS = value
	if err := SaveConfig(cfg); err != nil {
		return []ValidationError{{Field: "web.cors", Message: "errors.admin.web.cors_update_failed"}}
	}
	return nil
}
