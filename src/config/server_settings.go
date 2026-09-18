package config

// This file holds the server.yml settings trees defined by AI.md PART 12:
// base URL, request limits, response compression, session, rate limiting,
// i18n, contact roles, analytics tracking, privacy/consent, and cache.

// LimitsConfig represents request limits per AI.md PART 12
type LimitsConfig struct {
	// Maximum request body size
	MaxBodySize string `yaml:"max_body_size"`
	// Read timeout
	ReadTimeout string `yaml:"read_timeout"`
	// Write timeout
	WriteTimeout string `yaml:"write_timeout"`
	// Idle connection timeout
	IdleTimeout string `yaml:"idle_timeout"`
}

// DefaultLimitsConfig returns the AI.md PART 12 request limit defaults
func DefaultLimitsConfig() LimitsConfig {
	return LimitsConfig{
		MaxBodySize:  "10MB",
		ReadTimeout:  "30s",
		WriteTimeout: "30s",
		IdleTimeout:  "120s",
	}
}

// CompressionConfig represents response compression per AI.md PART 12
type CompressionConfig struct {
	Enabled bool `yaml:"enabled"`
	// Compression level 1-9
	Level int `yaml:"level"`
	// MIME types to compress
	Types []string `yaml:"types"`
}

// DefaultCompressionConfig returns the AI.md PART 12 compression defaults
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{
		Enabled: true,
		Level:   5,
		Types: []string{
			"text/html",
			"text/css",
			"text/javascript",
			"application/json",
			"application/xml",
		},
	}
}

// SessionConfig represents session cookie settings per AI.md PART 12.
// Admin and user sessions are separate cookies with separate lifetimes; the
// remaining fields are shared by both.
type SessionConfig struct {
	Admin SessionRoleConfig `yaml:"admin"`
	User  SessionRoleConfig `yaml:"user"`
	// Refresh the session expiry on each request
	ExtendOnActivity bool `yaml:"extend_on_activity"`
	// auto = secure only when serving over HTTPS
	Secure string `yaml:"secure"`
	// Always true - never readable from JavaScript
	HTTPOnly bool `yaml:"http_only"`
	// strict, lax, or none
	SameSite string `yaml:"same_site"`
}

// SessionRoleConfig represents the per-account-type session settings
type SessionRoleConfig struct {
	CookieName string `yaml:"cookie_name"`
	// Absolute session lifetime
	MaxAge string `yaml:"max_age"`
	// Lifetime with no activity
	IdleTimeout string `yaml:"idle_timeout"`
}

// DefaultSessionConfig returns the AI.md PART 12 session defaults
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Admin: SessionRoleConfig{
			CookieName:  "admin_session",
			MaxAge:      "30d",
			IdleTimeout: "24h",
		},
		User: SessionRoleConfig{
			CookieName:  "user_session",
			MaxAge:      "7d",
			IdleTimeout: "24h",
		},
		ExtendOnActivity: true,
		Secure:           "auto",
		HTTPOnly:         true,
		SameSite:         "strict",
	}
}

// I18nConfig represents server language settings per AI.md PART 12
type I18nConfig struct {
	DefaultLanguage string   `yaml:"default_language"`
	Supported       []string `yaml:"supported"`
}

// DefaultI18nConfig returns the AI.md PART 12 i18n defaults
func DefaultI18nConfig() I18nConfig {
	return I18nConfig{
		DefaultLanguage: "en",
		Supported:       []string{"en"},
	}
}

// ContactConfig represents the notification recipient roles per AI.md PART 12.
// Empty role-specific values fall back through the documented chain, resolved
// per dispatch by ResolveContactEmail - never cached across requests.
type ContactConfig struct {
	Admin    ContactRoleConfig `yaml:"admin"`
	Security ContactRoleConfig `yaml:"security"`
	Abuse    ContactRoleConfig `yaml:"abuse"`
	General  ContactRoleConfig `yaml:"general"`
}

// ContactRoleConfig represents one contact role per AI.md PART 12.
// The webhooks map is open - any key is treated as a transport name.
type ContactRoleConfig struct {
	Email    string            `yaml:"email"`
	Webhooks map[string]string `yaml:"webhooks"`
}

// DefaultContactConfig returns the AI.md PART 12 contact defaults for a given
// FQDN. Only admin and security get an auto-populated address; abuse and
// general stay empty so an unprovisioned mailbox is never advertised.
func DefaultContactConfig(fqdn string) ContactConfig {
	return ContactConfig{
		Admin:    ContactRoleConfig{Email: "admin@" + fqdn, Webhooks: map[string]string{}},
		Security: ContactRoleConfig{Email: "security@" + fqdn, Webhooks: map[string]string{}},
		Abuse:    ContactRoleConfig{Email: "", Webhooks: map[string]string{}},
		General:  ContactRoleConfig{Email: "", Webhooks: map[string]string{}},
	}
}

// ResolveContactEmail returns the effective recipient for a role per the
// AI.md PART 12 resolution table. Computed per dispatch, never cached.
func (c ContactConfig) ResolveContactEmail(role string) string {
	switch role {
	case "security":
		if c.Security.Email != "" {
			return c.Security.Email
		}
	case "abuse":
		if c.Abuse.Email != "" {
			return c.Abuse.Email
		}
		if c.General.Email != "" {
			return c.General.Email
		}
	case "general":
		if c.General.Email != "" {
			return c.General.Email
		}
	}
	return c.Admin.Email
}

// ResolveContactWebhook returns the effective webhook URL for a role and
// transport, following the same fallback chain as ResolveContactEmail.
func (c ContactConfig) ResolveContactWebhook(role, transport string) string {
	switch role {
	case "security":
		if u := c.Security.Webhooks[transport]; u != "" {
			return u
		}
	case "abuse":
		if u := c.Abuse.Webhooks[transport]; u != "" {
			return u
		}
		if u := c.General.Webhooks[transport]; u != "" {
			return u
		}
	case "general":
		if u := c.General.Webhooks[transport]; u != "" {
			return u
		}
	}
	return c.Admin.Webhooks[transport]
}

// TrackingConfig represents analytics tracking settings per AI.md PART 12
type TrackingConfig struct {
	// google, matomo, piwik, owa, fathom, plausible, umami, simple,
	// cloudflare, or empty for none
	Type string `yaml:"type"`
	// Site/property identifier for the selected platform
	ID string `yaml:"id"`
	// Self-hosted instance URL - required for matomo, piwik, owa, umami
	URL string `yaml:"url"`
}

// ValidTrackingTypes lists the analytics platforms AI.md PART 12 recognizes
var ValidTrackingTypes = []string{
	"google", "matomo", "piwik", "owa", "fathom",
	"plausible", "umami", "simple", "cloudflare",
}

// trackingTypesRequiringURL are the self-hosted platforms that cannot render a
// tracking snippet without an instance URL
var trackingTypesRequiringURL = map[string]bool{
	"matomo": true,
	"piwik":  true,
	"owa":    true,
	"umami":  true,
}

// PrivacyConfig represents the privacy and consent settings per AI.md PART 12
type PrivacyConfig struct {
	Data       PrivacyDataConfig       `yaml:"data"`
	Retention  PrivacyRetentionConfig  `yaml:"retention"`
	Consent    PrivacyConsentConfig    `yaml:"consent"`
	Cookies    PrivacyCookiesConfig    `yaml:"cookies"`
	ThirdParty PrivacyThirdPartyConfig `yaml:"third_party"`
	Content    PrivacyContentConfig    `yaml:"content"`
}

// PrivacyDataConfig represents the data-handling disclosure per AI.md PART 12
type PrivacyDataConfig struct {
	// Whether user data is sold or shared for value - drives the dynamic
	// consent and privacy page messaging
	Sold bool `yaml:"sold"`
	// Whether data is stored on this server rather than a third party
	StoredOnServer bool                 `yaml:"stored_on_server"`
	Sharing        []PrivacySharingItem `yaml:"sharing"`
}

// PrivacySharingItem represents one row of the data-sharing disclosure table
type PrivacySharingItem struct {
	Condition string `yaml:"condition"`
	When      string `yaml:"when"`
	Data      string `yaml:"data"`
}

// PrivacyRetentionConfig represents the retention statement per AI.md PART 12
type PrivacyRetentionConfig struct {
	Period            string `yaml:"period"`
	ExportAvailable   bool   `yaml:"export_available"`
	DeletionAvailable bool   `yaml:"deletion_available"`
}

// PrivacyConsentConfig represents the consent banner per AI.md PART 12
type PrivacyConsentConfig struct {
	ShowUntilAcknowledged bool                       `yaml:"show_until_acknowledged"`
	DefaultEnabled        bool                       `yaml:"default_enabled"`
	Message               string                     `yaml:"message"`
	MessageIfSold         string                     `yaml:"message_if_sold"`
	Policy                PrivacyConsentPolicyConfig `yaml:"policy"`
	Buttons               PrivacyConsentButtonConfig `yaml:"buttons"`
	// top or bottom
	Position        string `yaml:"position"`
	ShowPreferences bool   `yaml:"show_preferences"`
	PreferencesText string `yaml:"preferences_text"`
}

// PrivacyConsentPolicyConfig represents the policy link in the consent banner
type PrivacyConsentPolicyConfig struct {
	Text string `yaml:"text"`
	URL  string `yaml:"url"`
}

// PrivacyConsentButtonConfig represents the consent banner button labels
type PrivacyConsentButtonConfig struct {
	Decline string `yaml:"decline"`
	Accept  string `yaml:"accept"`
}

// PrivacyCookiesConfig represents the per-category cookie disclosure
type PrivacyCookiesConfig struct {
	Essential   PrivacyCookieCategory `yaml:"essential"`
	Preferences PrivacyCookieCategory `yaml:"preferences"`
	Analytics   PrivacyCookieCategory `yaml:"analytics"`
}

// PrivacyCookieCategory represents one cookie category disclosure.
// The analytics category additionally varies its description by data.sold.
type PrivacyCookieCategory struct {
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description"`
	// Analytics only - appended when data.sold is false
	DescriptionSuffixNotSold string `yaml:"description_suffix_not_sold,omitempty"`
	// Analytics only - appended when data.sold is true
	DescriptionSuffixSold string `yaml:"description_suffix_sold,omitempty"`
}

// PrivacyThirdPartyConfig lists third-party services that receive user data
type PrivacyThirdPartyConfig struct {
	Services []PrivacyThirdPartyService `yaml:"services"`
}

// PrivacyThirdPartyService represents one third-party data recipient
type PrivacyThirdPartyService struct {
	Name    string `yaml:"name"`
	Purpose string `yaml:"purpose"`
	Data    string `yaml:"data"`
	URL     string `yaml:"url"`
}

// PrivacyContentConfig holds the privacy page prose (Markdown supported)
type PrivacyContentConfig struct {
	DataCollection string `yaml:"data_collection"`
	// Used when data.sold is false
	DataUsage string `yaml:"data_usage"`
	// Used when data.sold is true
	DataUsageIfSold string `yaml:"data_usage_if_sold"`
	DataSecurity    string `yaml:"data_security"`
}

// DefaultPrivacyConfig returns the AI.md PART 12 privacy defaults
func DefaultPrivacyConfig() PrivacyConfig {
	return PrivacyConfig{
		Data: PrivacyDataConfig{
			Sold:           false,
			StoredOnServer: true,
			Sharing: []PrivacySharingItem{
				{
					Condition: "Legal requirement",
					When:      "Court order, subpoena, or law enforcement request",
					Data:      "Only what is legally compelled",
				},
				{
					Condition: "Service providers",
					When:      "Email delivery, error reporting, or hosting",
					Data:      "Only what the provider needs to perform the service",
				},
				{
					Condition: "With your consent",
					When:      "You explicitly authorize the transfer",
					Data:      "Only what you authorize",
				},
			},
		},
		Retention: PrivacyRetentionConfig{
			Period:            "Account data is retained while the account is active and deleted within 30 days of account deletion.",
			ExportAvailable:   true,
			DeletionAvailable: true,
		},
		Consent: PrivacyConsentConfig{
			ShowUntilAcknowledged: true,
			DefaultEnabled:        true,
			Message:               "We use cookies for essential functionality and, with your consent, analytics. Your data is never sold.",
			MessageIfSold:         "We use cookies for essential functionality and, with your consent, analytics. Your data may be shared with or sold to third parties.",
			Policy: PrivacyConsentPolicyConfig{
				Text: "Privacy Policy",
				URL:  "/server/privacy",
			},
			Buttons: PrivacyConsentButtonConfig{
				Decline: "Decline",
				Accept:  "I Agree",
			},
			Position:        "bottom",
			ShowPreferences: true,
			PreferencesText: "Manage Preferences",
		},
		Cookies: PrivacyCookiesConfig{
			Essential: PrivacyCookieCategory{
				Enabled:     true,
				Description: "Required for login sessions, security, and core functionality. Cannot be disabled.",
			},
			Preferences: PrivacyCookieCategory{
				Enabled:     true,
				Description: "Remembers your theme, language, and display preferences.",
			},
			Analytics: PrivacyCookieCategory{
				Enabled:                  true,
				Description:              "Helps us understand how the service is used.",
				DescriptionSuffixNotSold: " This data is never sold.",
				DescriptionSuffixSold:    " This data may be shared with or sold to third parties.",
			},
		},
		ThirdParty: PrivacyThirdPartyConfig{
			Services: []PrivacyThirdPartyService{},
		},
		Content: PrivacyContentConfig{
			DataCollection:  defaultPrivacyDataCollection,
			DataUsage:       defaultPrivacyDataUsage,
			DataUsageIfSold: defaultPrivacyDataUsageIfSold,
			DataSecurity:    defaultPrivacyDataSecurity,
		},
	}
}

// Privacy page prose defaults per AI.md PART 12
const (
	defaultPrivacyDataCollection = `**We collect only what is necessary to provide our service:**

**Account Information:**
- Email address (for account recovery and notifications)
- Username (for identification)
- Password (stored securely hashed, never in plain text)

**Usage Information (with consent):**
- Pages visited and features used
- Browser type and device information
- Approximate location (country/region from IP, not precise)

**Technical Information:**
- IP address (for security and abuse prevention)
- Session data (for keeping you logged in)

**We do NOT collect:**
- Payment information (unless explicitly required by the service)
- Precise location data
- Data from other websites or apps
`

	defaultPrivacyDataUsage = `**Your data is used solely to:**

- **Provide the service:** Account management, authentication, core functionality
- **Improve the experience:** Performance optimization, bug fixes, feature improvements
- **Ensure security:** Prevent abuse, detect fraud, protect your account
- **Communicate:** Service updates, security alerts, and (with consent) product news

**Your data is NEVER:**
- Sold to third parties
- Used for targeted advertising
- Shared without your explicit consent (except as required by law)
`

	defaultPrivacyDataUsageIfSold = `**Your data may be used to:**

- **Provide the service:** Account management, authentication, core functionality
- **Improve the experience:** Performance optimization, bug fixes, feature improvements
- **Ensure security:** Prevent abuse, detect fraud, protect your account
- **Communicate:** Service updates, security alerts, and (with consent) product news
- **Third-party sharing:** Your data may be shared with or sold to third parties for analytics, advertising, or other purposes as described below

**Your rights:**
- You can opt out of data sales via your account privacy settings
- You can request deletion of your data at any time
- See "Your Rights" section below for details
`

	defaultPrivacyDataSecurity = `**How we protect your data:**

- All data is stored on our servers (not third-party cloud services unless specified)
- Passwords are hashed using Argon2id (industry-standard, memory-hard algorithm)
- All connections are encrypted (HTTPS/TLS)
- Regular security audits and updates
- Access controls and audit logging for admin actions
`
)

// CacheConfig represents the cache backend settings per AI.md PART 12.
// memory works standalone for single-instance mode; valkey/redis is required
// for cluster mode. URL takes precedence over the discrete fields.
type CacheConfig struct {
	// memory, valkey, or redis
	Type string `yaml:"type"`
	// Connection URL - takes precedence over host/port/username/password/db
	URL      string `yaml:"url"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	// Connect over TLS
	TLS bool `yaml:"tls"`
	// Skip TLS certificate verification
	TLSSkipVerify bool `yaml:"tls_skip_verify"`
	PoolSize      int  `yaml:"pool_size"`
	MinIdle       int  `yaml:"min_idle"`
	// Connection timeout
	Timeout string `yaml:"timeout"`
	// Key prefix - must be unique per app
	Prefix string `yaml:"prefix"`
	// Default entry lifetime
	TTL string `yaml:"ttl"`
	// Native Valkey/Redis cluster mode
	Cluster      bool     `yaml:"cluster"`
	ClusterNodes []string `yaml:"cluster_nodes"`
}

// DefaultCacheConfig returns the AI.md PART 12 cache defaults
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Type:          "memory",
		URL:           "",
		Host:          "localhost",
		Port:          6379,
		Username:      "",
		Password:      "",
		DB:            0,
		TLS:           false,
		TLSSkipVerify: false,
		PoolSize:      10,
		MinIdle:       2,
		Timeout:       "5s",
		Prefix:        "wthr:",
		TTL:           "1h",
		Cluster:       false,
		ClusterNodes:  []string{},
	}
}

// RateLimitConfig represents rate limiting configuration per AI.md PART 12.
// Limits are enforced as a sliding window per client IP.
type RateLimitConfig struct {
	Enabled bool `yaml:"enabled"`
	// GET and other safe methods
	Read RateLimitBucket `yaml:"read"`
	// POST, PUT, PATCH, DELETE
	Write RateLimitBucket `yaml:"write"`
	// Health endpoints
	Health RateLimitBucket `yaml:"health"`
	// Maximum requests per minute across all buckets
	GlobalBurst int                 `yaml:"global_burst"`
	Auth        RateLimitAuthConfig `yaml:"auth"`
}

// RateLimitBucket represents one sliding-window limit per AI.md PART 12
type RateLimitBucket struct {
	Requests int `yaml:"requests"`
	// Window in seconds
	Window int `yaml:"window"`
}

// RateLimitAuthConfig represents the auth-specific limits per AI.md PART 12
type RateLimitAuthConfig struct {
	Login         RateLimitBucket `yaml:"login"`
	PasswordReset RateLimitBucket `yaml:"password_reset"`
	Registration  RateLimitBucket `yaml:"registration"`
}

// DefaultRateLimitConfig returns the AI.md PART 12 rate limit defaults
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:     true,
		Read:        RateLimitBucket{Requests: 120, Window: 60},
		Write:       RateLimitBucket{Requests: 10, Window: 60},
		Health:      RateLimitBucket{Requests: 120, Window: 60},
		GlobalBurst: 240,
		Auth: RateLimitAuthConfig{
			Login:         RateLimitBucket{Requests: 5, Window: 900},
			PasswordReset: RateLimitBucket{Requests: 3, Window: 3600},
			Registration:  RateLimitBucket{Requests: 5, Window: 3600},
		},
	}
}
