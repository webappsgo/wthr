package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

// SessionAccount names the account type a session cookie belongs to. Server Admin and
// regular user sessions are separate cookies with separate lifetimes (AI.md PART 12).
type SessionAccount int

const (
	// SessionAccountUser is a regular user session
	SessionAccountUser SessionAccount = iota
	// SessionAccountAdmin is a Server Admin session
	SessionAccountAdmin
)

// SessionSettings are the resolved server.session values of one account type.
type SessionSettings struct {
	// CookieName is the session cookie name
	CookieName string
	// MaxAge is the absolute session lifetime
	MaxAge time.Duration
	// IdleTimeout is the lifetime with no activity
	IdleTimeout time.Duration
}

// sessionConfig returns the live server.session block, or the AI.md PART 12 defaults when no
// configuration is loaded yet.
func sessionConfig() config.SessionConfig {
	if cfg := config.GetGlobalConfig(); cfg != nil {
		return cfg.Server.Session
	}

	return config.DefaultSessionConfig()
}

// SessionSettingsFor resolves the cookie name and lifetimes of one account type. A value that
// is empty or does not parse falls back to the default, so a bad server.yml never breaks login.
func SessionSettingsFor(account SessionAccount) SessionSettings {
	defaults := config.DefaultSessionConfig()
	configured, fallback := sessionConfig().User, defaults.User
	if account == SessionAccountAdmin {
		configured, fallback = sessionConfig().Admin, defaults.Admin
	}

	settings := SessionSettings{CookieName: strings.TrimSpace(configured.CookieName)}
	if settings.CookieName == "" {
		settings.CookieName = fallback.CookieName
	}

	settings.MaxAge = parseSessionDuration(configured.MaxAge, fallback.MaxAge)
	settings.IdleTimeout = parseSessionDuration(configured.IdleTimeout, fallback.IdleTimeout)

	return settings
}

// parseSessionDuration parses a configured duration and falls back to the default text.
func parseSessionDuration(value, fallback string) time.Duration {
	if parsed, err := config.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}

	parsed, _ := config.ParseDuration(fallback)

	return parsed
}

// UserSessionCookieName is the cookie that carries a regular user session.
func UserSessionCookieName() string {
	return SessionSettingsFor(SessionAccountUser).CookieName
}

// AdminSessionCookieName is the cookie that carries a Server Admin session.
func AdminSessionCookieName() string {
	return SessionSettingsFor(SessionAccountAdmin).CookieName
}

// sessionSameSite maps server.session.same_site to the cookie mode; strict is the default and
// the fallback for an unknown value.
func sessionSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteStrictMode
	}
}

// sessionSecure resolves server.session.secure: auto follows the request scheme (honoring a
// trusted proxy), true and false are explicit.
func sessionSecure(r *http.Request, value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		return util.TrustedIsHTTPS(r)
	}
}

// NewSessionCookie builds the session cookie for an account type from server.session: the
// configured name, SameSite and Secure rules, HttpOnly always, and the lifetime given (never
// longer than the configured absolute maximum).
func NewSessionCookie(r *http.Request, account SessionAccount, value string, lifetime time.Duration) *http.Cookie {
	settings := SessionSettingsFor(account)
	if lifetime <= 0 || lifetime > settings.MaxAge {
		lifetime = settings.MaxAge
	}

	block := sessionConfig()

	return &http.Cookie{
		Name:     settings.CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(lifetime.Seconds()),
		HttpOnly: true,
		Secure:   sessionSecure(r, block.Secure),
		SameSite: sessionSameSite(block.SameSite),
	}
}

// ClearSessionCookie builds the cookie that removes an account type's session cookie.
func ClearSessionCookie(r *http.Request, account SessionAccount) *http.Cookie {
	cookie := NewSessionCookie(r, account, "", time.Second)
	cookie.MaxAge = -1

	return cookie
}
