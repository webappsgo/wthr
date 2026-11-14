package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// validateConfig checks every loaded config value and replaces invalid ones
// with their defaults per AI.md PART 12 ("Config Validation Rule"): warn, do
// not error - the server must always start with sane defaults. It reports
// whether the listen port was replaced, which the caller must persist so the
// generated port survives the next restart.
func validateConfig(cfg *AppConfig) bool {
	portReplaced := validateServerIdentity(cfg)
	validateServerLimits(cfg)
	validateServerSession(cfg)
	validateServerRateLimit(cfg)
	validateServerI18n(cfg)
	validateServerPages(cfg)
	validateServerTracking(cfg)
	validateServerCache(cfg)
	validateServerUpdate(cfg)
	validateServerSEO(cfg)
	validateWebRobots(cfg)
	return portReplaced
}

// validateServerSEO bounds the sitemap and feed limits (AI.md PART 16): an
// out-of-range value is replaced by its default.
func validateServerSEO(cfg *AppConfig) {
	seo := &cfg.Server.SEO
	if seo.Sitemap.MaxURLs < 1 || seo.Sitemap.MaxURLs > DefaultSitemapMaxURLs {
		if seo.Sitemap.MaxURLs != 0 {
			warnConfig("invalid seo.sitemap.max_urls %d, using default %d", seo.Sitemap.MaxURLs, DefaultSitemapMaxURLs)
		}
		seo.Sitemap.MaxURLs = DefaultSitemapMaxURLs
	}
	if seo.Feed.MaxItems < 1 || seo.Feed.MaxItems > MaxFeedMaxItems {
		if seo.Feed.MaxItems != 0 {
			warnConfig("invalid seo.feed.max_items %d, using default %d", seo.Feed.MaxItems, DefaultFeedMaxItems)
		}
		seo.Feed.MaxItems = DefaultFeedMaxItems
	}
	seo.Feed.Title = strings.TrimSpace(seo.Feed.Title)
}

// validateWebRobots drops invalid robots paths with a warning and restores the
// defaults when nothing usable is left.
func validateWebRobots(cfg *AppConfig) {
	robots := &cfg.Web.Robots
	allow, allowErrs := NormalizeRobotsPaths(robots.Allow)
	deny, denyErrs := NormalizeRobotsPaths(robots.Deny)
	for _, message := range append(allowErrs, denyErrs...) {
		warnConfig("web.robots: %s", message)
	}
	robots.Allow = allow
	robots.Deny = deny
	if len(robots.Allow) == 0 && len(robots.Deny) == 0 {
		*robots = DefaultRobotsConfig()
	}
}

func validateServerUpdate(cfg *AppConfig) {
	branch := strings.ToLower(strings.TrimSpace(cfg.Server.Update.Branch))
	switch branch {
	case "daily", "rolling", "dev", "development":
		cfg.Server.Update.Branch = "devel"
	case "stable", "beta", "devel":
		cfg.Server.Update.Branch = branch
	default:
		warnConfig("invalid update.branch %q, using default %q", cfg.Server.Update.Branch, "stable")
		cfg.Server.Update.Branch = "stable"
	}
	if cfg.Server.Update.DeferDays < 0 || cfg.Server.Update.DeferDays > 365 {
		warnConfig("invalid update.defer_days %d, using default 0", cfg.Server.Update.DeferDays)
		cfg.Server.Update.DeferDays = 0
	}
}

// warnConfig reports a replaced config value on stderr
func warnConfig(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

// validateServerIdentity validates port, base URL, mode, and route prefixes.
// It reports whether the port was replaced.
func validateServerIdentity(cfg *AppConfig) bool {
	portReplaced := false
	if !isValidPortValue(cfg.Server.Port) {
		p := randomPort()
		warnConfig("invalid port %v, using random port %d", cfg.Server.Port, p)
		cfg.Server.Port = p
		portReplaced = true
	}

	cfg.Server.BaseURL = normalizeBaseURL(cfg.Server.BaseURL)

	switch strings.ToLower(strings.TrimSpace(cfg.Server.Mode)) {
	case "production", "prod", "development", "devel", "dev", "debug":
	default:
		warnConfig("invalid mode %q, using default %q", cfg.Server.Mode, "production")
		cfg.Server.Mode = "production"
	}

	if !IsValidAdminPath(cfg.Server.AdminPath) {
		warnConfig("invalid admin_path %q, using default %q", cfg.Server.AdminPath, DefaultAdminPath)
		cfg.Server.AdminPath = DefaultAdminPath
	}

	if strings.TrimSpace(cfg.Server.APIVersion) == "" || strings.Contains(cfg.Server.APIVersion, "/") {
		warnConfig("invalid api_version %q, using default %q", cfg.Server.APIVersion, "v1")
		cfg.Server.APIVersion = "v1"
	}

	return portReplaced
}

// isValidPortValue reports whether a configured port is usable. A port may be
// an int or a "http,https" dual-port string per AI.md PART 5.
func isValidPortValue(v interface{}) bool {
	switch p := v.(type) {
	case int:
		return p >= 0 && p <= 65535
	case string:
		parts := strings.Split(p, ",")
		if len(parts) > 2 {
			return false
		}
		for _, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 0 || n > 65535 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// normalizeBaseURL applies the AI.md PART 12 base URL rules: empty is "/",
// a leading slash is required, and the trailing slash is stripped.
func normalizeBaseURL(raw string) string {
	b := strings.TrimSpace(raw)
	if b == "" {
		return "/"
	}
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	b = strings.TrimRight(b, "/")
	if b == "" {
		return "/"
	}
	return b
}

// validateServerLimits validates request limits and response compression
func validateServerLimits(cfg *AppConfig) {
	defaults := DefaultLimitsConfig()
	limits := &cfg.Server.Limits

	for _, field := range []struct {
		name  string
		value *string
		def   string
	}{
		{"limits.max_body_size", &limits.MaxBodySize, defaults.MaxBodySize},
		{"limits.read_timeout", &limits.ReadTimeout, defaults.ReadTimeout},
		{"limits.write_timeout", &limits.WriteTimeout, defaults.WriteTimeout},
		{"limits.idle_timeout", &limits.IdleTimeout, defaults.IdleTimeout},
	} {
		if strings.TrimSpace(*field.value) == "" {
			warnConfig("invalid %s %q, using default %q", field.name, *field.value, field.def)
			*field.value = field.def
		}
	}

	if cfg.Server.Compression.Level < 1 || cfg.Server.Compression.Level > 9 {
		def := DefaultCompressionConfig().Level
		warnConfig("invalid compression.level %d, using default %d", cfg.Server.Compression.Level, def)
		cfg.Server.Compression.Level = def
	}

	if len(cfg.Server.Compression.Types) == 0 {
		cfg.Server.Compression.Types = DefaultCompressionConfig().Types
	}
}

// validateServerSession validates the admin and user session settings
func validateServerSession(cfg *AppConfig) {
	defaults := DefaultSessionConfig()
	validateSessionRole(&cfg.Server.Session.Admin, defaults.Admin, "admin")
	validateSessionRole(&cfg.Server.Session.User, defaults.User, "user")

	switch strings.ToLower(strings.TrimSpace(cfg.Server.Session.SameSite)) {
	case "strict", "lax", "none":
	default:
		warnConfig("invalid session.same_site %q, using default %q", cfg.Server.Session.SameSite, defaults.SameSite)
		cfg.Server.Session.SameSite = defaults.SameSite
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Server.Session.Secure)) {
	case "auto", "true", "false":
	default:
		warnConfig("invalid session.secure %q, using default %q", cfg.Server.Session.Secure, defaults.Secure)
		cfg.Server.Session.Secure = defaults.Secure
	}

	// AI.md PART 12: session cookies are never readable from JavaScript
	cfg.Server.Session.HTTPOnly = true
}

// validateSessionRole validates one account type's session settings
func validateSessionRole(role *SessionRoleConfig, def SessionRoleConfig, name string) {
	if strings.TrimSpace(role.CookieName) == "" {
		warnConfig("invalid session.%s.cookie_name %q, using default %q", name, role.CookieName, def.CookieName)
		role.CookieName = def.CookieName
	}
	if strings.TrimSpace(role.MaxAge) == "" {
		warnConfig("invalid session.%s.max_age %q, using default %q", name, role.MaxAge, def.MaxAge)
		role.MaxAge = def.MaxAge
	}
	if strings.TrimSpace(role.IdleTimeout) == "" {
		warnConfig("invalid session.%s.idle_timeout %q, using default %q", name, role.IdleTimeout, def.IdleTimeout)
		role.IdleTimeout = def.IdleTimeout
	}
}

// validateServerRateLimit validates every rate limit bucket
func validateServerRateLimit(cfg *AppConfig) {
	defaults := DefaultRateLimitConfig()
	rl := &cfg.Server.RateLimit

	validateRateLimitBucket(&rl.Read, defaults.Read, "rate_limit.read")
	validateRateLimitBucket(&rl.Write, defaults.Write, "rate_limit.write")
	validateRateLimitBucket(&rl.Health, defaults.Health, "rate_limit.health")
	validateRateLimitBucket(&rl.Auth.Login, defaults.Auth.Login, "rate_limit.auth.login")
	validateRateLimitBucket(&rl.Auth.PasswordReset, defaults.Auth.PasswordReset, "rate_limit.auth.password_reset")
	validateRateLimitBucket(&rl.Auth.Registration, defaults.Auth.Registration, "rate_limit.auth.registration")

	if rl.GlobalBurst < 1 {
		warnConfig("invalid rate_limit.global_burst %d, using default %d", rl.GlobalBurst, defaults.GlobalBurst)
		rl.GlobalBurst = defaults.GlobalBurst
	}
}

// validateRateLimitBucket validates one sliding-window limit
func validateRateLimitBucket(bucket *RateLimitBucket, def RateLimitBucket, name string) {
	if bucket.Requests < 1 {
		warnConfig("invalid %s.requests %d, using default %d", name, bucket.Requests, def.Requests)
		bucket.Requests = def.Requests
	}
	if bucket.Window < 1 {
		warnConfig("invalid %s.window %d, using default %d", name, bucket.Window, def.Window)
		bucket.Window = def.Window
	} else if int64(bucket.Window) > int64((time.Duration(1<<63-1))/time.Second) {
		warnConfig("invalid %s.window %d overflows time.Duration, using default %d", name, bucket.Window, def.Window)
		bucket.Window = def.Window
	}
}

// validateServerI18n validates the language settings
func validateServerI18n(cfg *AppConfig) {
	defaults := DefaultI18nConfig()

	if strings.TrimSpace(cfg.Server.I18n.DefaultLanguage) == "" {
		warnConfig("invalid i18n.default_language %q, using default %q",
			cfg.Server.I18n.DefaultLanguage, defaults.DefaultLanguage)
		cfg.Server.I18n.DefaultLanguage = defaults.DefaultLanguage
	}

	// AI.md PART 31: the operator may narrow the list but never below the required six
	enabled := make(map[string]bool, len(cfg.Server.I18n.AvailableLanguages))
	for _, lang := range cfg.Server.I18n.AvailableLanguages {
		enabled[strings.ToLower(strings.TrimSpace(lang))] = true
	}
	for _, lang := range RequiredLanguages {
		if !enabled[lang] {
			warnConfig("i18n.available_languages is missing required language %q, restoring it", lang)
			cfg.Server.I18n.AvailableLanguages = append(cfg.Server.I18n.AvailableLanguages, lang)
		}
	}

	// AI.md PART 31: an unsupported default language silently falls back to
	// English rather than erroring
	found := false
	for _, lang := range cfg.Server.I18n.AvailableLanguages {
		if lang == cfg.Server.I18n.DefaultLanguage {
			found = true
			break
		}
	}
	if !found {
		warnConfig("i18n.default_language %q is not in i18n.available_languages, using %q",
			cfg.Server.I18n.DefaultLanguage, defaults.DefaultLanguage)
		cfg.Server.I18n.DefaultLanguage = defaults.DefaultLanguage
	}
}

// validateServerPages validates the contact captcha settings (AI.md PART 16 "Captcha providers")
func validateServerPages(cfg *AppConfig) {
	c := &cfg.Server.Pages.Contact
	switch c.Captcha {
	case "":
		c.Captcha = CaptchaSimple
	case CaptchaSimple:
	case CaptchaReCaptcha, CaptchaHCaptcha:
		if c.CaptchaSiteKey == "" || c.CaptchaSecretKey == "" {
			warnConfig("pages.contact.captcha %q needs captcha_site_key and captcha_secret_key, using %q", c.Captcha, CaptchaSimple)
			c.Captcha = CaptchaSimple
		}
	default:
		warnConfig("invalid pages.contact.captcha %q, using %q", c.Captcha, CaptchaSimple)
		c.Captcha = CaptchaSimple
	}
}

// validateServerTracking validates the analytics platform settings
func validateServerTracking(cfg *AppConfig) {
	t := &cfg.Server.Tracking

	kind := strings.ToLower(strings.TrimSpace(t.Type))
	if kind == "" || kind == "none" {
		t.Type = ""
		return
	}

	valid := false
	for _, known := range ValidTrackingTypes {
		if kind == known {
			valid = true
			break
		}
	}
	if !valid {
		warnConfig("unknown tracking.type %q, analytics disabled", t.Type)
		t.Type = ""
		return
	}
	t.Type = kind

	if t.ID == "" {
		warnConfig("tracking.type %q has no tracking.id, analytics disabled", t.Type)
		t.Type = ""
		return
	}

	if trackingTypesRequiringURL[t.Type] && strings.TrimSpace(t.URL) == "" {
		warnConfig("tracking.type %q requires tracking.url, analytics disabled", t.Type)
		t.Type = ""
	}
}

// validateServerCache validates the cache backend settings
func validateServerCache(cfg *AppConfig) {
	defaults := DefaultCacheConfig()
	c := &cfg.Server.Cache

	switch strings.ToLower(strings.TrimSpace(c.Type)) {
	case "none", "memory", "valkey", "redis":
		c.Type = strings.ToLower(strings.TrimSpace(c.Type))
	default:
		warnConfig("invalid cache.type %q, using default %q", c.Type, defaults.Type)
		c.Type = defaults.Type
	}

	if c.Port < 1 || c.Port > 65535 {
		warnConfig("invalid cache.port %d, using default %d", c.Port, defaults.Port)
		c.Port = defaults.Port
	}

	if c.PoolSize < 1 {
		warnConfig("invalid cache.pool_size %d, using default %d", c.PoolSize, defaults.PoolSize)
		c.PoolSize = defaults.PoolSize
	}

	if c.MinIdle < 0 || c.MinIdle > c.PoolSize {
		warnConfig("invalid cache.min_idle %d, using default %d", c.MinIdle, defaults.MinIdle)
		c.MinIdle = defaults.MinIdle
	}

	if strings.TrimSpace(c.Timeout) == "" {
		c.Timeout = defaults.Timeout
	} else if d, err := time.ParseDuration(strings.TrimSpace(c.Timeout)); err != nil || d <= 0 {
		warnConfig("invalid cache.timeout, using default %q", defaults.Timeout)
		c.Timeout = defaults.Timeout
	}

	if strings.TrimSpace(c.TTL) == "" {
		c.TTL = defaults.TTL
	} else if d, err := time.ParseDuration(strings.TrimSpace(c.TTL)); err != nil || d <= 0 {
		warnConfig("invalid cache.ttl, using default %q", defaults.TTL)
		c.TTL = defaults.TTL
	}

	if strings.TrimSpace(c.Prefix) == "" {
		c.Prefix = defaults.Prefix
	}

	if rawURL := strings.TrimSpace(c.URL); rawURL != "" {
		u, err := url.Parse(rawURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "redis" && u.Scheme != "rediss" && u.Scheme != "valkey") {
			warnConfig("invalid cache.url, using host and port settings")
			c.URL = defaults.URL
		}
	}

	if c.DB < 0 {
		warnConfig("invalid cache.db %d, using default %d", c.DB, defaults.DB)
		c.DB = defaults.DB
	}
	if c.Cluster {
		validNodes := len(c.ClusterNodes) > 0
		for _, node := range c.ClusterNodes {
			if _, _, err := net.SplitHostPort(strings.TrimSpace(node)); err != nil {
				validNodes = false
				break
			}
		}
		if !validNodes {
			warnConfig("invalid cache.cluster_nodes, disabling cache cluster mode")
			c.Cluster = defaults.Cluster
			c.ClusterNodes = defaults.ClusterNodes
		}
	}
}

// reservedAdminPaths are route segments an admin_path may never take (AI.md PART 17).
// DefaultAdminPath is deliberately absent: the revert-on-error fallback depends on it.
var reservedAdminPaths = map[string]bool{
	"api": true, "health": true, "healthz": true, "metrics": true, "version": true,
	".well-known": true, "about": true, "privacy": true, "contact": true, "help": true,
	"terms": true, "preferences": true, "docs": true, "auth": true, "security": true,
	"static": true, "assets": true,
}

// IsValidAdminPath reports whether v is an acceptable admin_path: one segment of
// [a-z0-9-], 2-32 characters, no leading or trailing hyphen, not reserved.
func IsValidAdminPath(v string) bool {
	if len(v) < 2 || len(v) > 32 || reservedAdminPaths[v] {
		return false
	}
	if v[0] == '-' || v[len(v)-1] == '-' {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}
