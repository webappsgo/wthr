package middleware

import (
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/mode"
	"github.com/webappsgo/wthr/src/util"
)

// AI.md PART 11 "Security Headers": two years, subdomains included, preload on.
const hstsValue = "max-age=63072000; includeSubDomains; preload"

// cspDirective is one directive of the generated Content-Security-Policy. The
// policy is built from an ordered slice rather than a map so the emitted header
// is byte-stable across requests and testable.
type cspDirective struct {
	name  string
	value string
}

// permissionsFeature is one entry of the generated Permissions-Policy header.
type permissionsFeature struct {
	name  string
	value string
}

// CSPConfig holds the per-directive Content-Security-Policy configuration from
// AI.md PART 11 "Content Security Policy". Operators never redefine the whole
// policy: each *Extra string is appended to the spec default for that
// directive.
type CSPConfig struct {
	Enabled bool
	// Mode is "enforce" or "report-only". Development mode falls back to
	// report-only unless enforce is set explicitly.
	Mode            string
	ScriptSrcExtra  string
	StyleSrcExtra   string
	ImgSrcExtra     string
	FontSrcExtra    string
	ConnectSrcExtra string
	FrameSrcExtra   string
	FormActionExtra string
}

// NELConfig holds the Network Error Logging reporting configuration.
type NELConfig struct {
	Enabled          bool
	MaxAgeSeconds    int
	IncludeSubdomain bool
}

// SecurityHeadersConfig holds the web.headers.* configuration from AI.md
// PART 11 "Security Header Config".
type SecurityHeadersConfig struct {
	COOP                string
	COEP                string
	CORP                string
	OriginAgentCluster  bool
	CrossDomainPolicies string
	CSP                 CSPConfig
	NEL                 NELConfig
	PermissionsPolicy   []permissionsFeature
	// APIVersion is the {api_version} segment of the reports endpoints the
	// Reporting-Endpoints/Report-To/NEL headers point at.
	APIVersion string
}

// defaultPermissionsPolicy is the AI.md PART 11 "Permissions-Policy
// Configuration" default set: sensor/hardware features locked, advertising and
// tracking proposals always locked, spec-used features scoped to self.
// geolocation is "(self)" because IDEA.md declares location detection as a
// product feature, which is exactly the IDEA.md-driven enablement the spec
// describes.
func defaultPermissionsPolicy() []permissionsFeature {
	return []permissionsFeature{
		{"accelerometer", "()"},
		{"ambient-light-sensor", "()"},
		{"battery", "()"},
		{"camera", "()"},
		{"display-capture", "()"},
		{"geolocation", "(self)"},
		{"gyroscope", "()"},
		{"hid", "()"},
		{"idle-detection", "()"},
		{"magnetometer", "()"},
		{"microphone", "()"},
		{"midi", "()"},
		{"screen-wake-lock", "()"},
		{"serial", "()"},
		{"usb", "()"},
		{"xr-spatial-tracking", "()"},
		{"attribution-reporting", "()"},
		{"browsing-topics", "()"},
		{"interest-cohort", "()"},
		{"autoplay", "(self)"},
		{"encrypted-media", "(self)"},
		{"fullscreen", "(self)"},
		{"payment", "(self)"},
		{"picture-in-picture", "(self)"},
		{"publickey-credentials-get", "(self)"},
		{"storage-access", "(self)"},
		{"web-share", "(self)"},
	}
}

// DefaultSecurityHeadersConfig returns the AI.md PART 11 defaults. The CSP
// extras carry the origins this project's own templates need (Leaflet assets,
// OpenStreetMap tiles, same-origin WebSockets) through the per-directive append
// mechanism instead of hardcoding them into the policy itself.
func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		COOP:                "unsafe-none",
		COEP:                "unsafe-none",
		CORP:                "cross-origin",
		OriginAgentCluster:  true,
		CrossDomainPolicies: "none",
		CSP: CSPConfig{
			Enabled:         true,
			Mode:            "enforce",
			ScriptSrcExtra:  "https://unpkg.com",
			StyleSrcExtra:   "https://unpkg.com",
			FontSrcExtra:    "https://unpkg.com",
			ConnectSrcExtra: "https://*.tile.openstreetmap.org wss: ws:",
		},
		NEL: NELConfig{
			Enabled:          true,
			MaxAgeSeconds:    2592000,
			IncludeSubdomain: true,
		},
		PermissionsPolicy: defaultPermissionsPolicy(),
		APIVersion:        "v1",
	}
}

// SecurityHeaders adds the AI.md PART 11 security headers to every response.
func SecurityHeaders() func(http.Handler) http.Handler {
	return SecurityHeadersWithConfig(DefaultSecurityHeadersConfig())
}

// SecurityHeadersWithConfig is SecurityHeaders with an explicit configuration.
func SecurityHeadersWithConfig(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secure := util.TrustedIsHTTPS(r)
			applyBaseHeaders(w, r, cfg, secure)

			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if cfg.CSP.Enabled {
				w.Header().Set(cspHeaderName(cfg.CSP), buildCSP(cfg, secure))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersAPI adds security headers tuned for API endpoints. The
// mandatory PART 11 header set still applies — only the CSP, framing and
// caching policy differ, because API responses never render HTML.
func SecurityHeadersAPI() func(http.Handler) http.Handler {
	return SecurityHeadersAPIWithConfig(DefaultSecurityHeadersConfig())
}

// SecurityHeadersAPIWithConfig is SecurityHeadersAPI with an explicit
// configuration.
func SecurityHeadersAPIWithConfig(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			applyBaseHeaders(w, r, cfg, util.TrustedIsHTTPS(r))

			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

			// AI.md PART 9 HTTP caching: private/authenticated API responses
			// are never cached.
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")

			next.ServeHTTP(w, r)
		})
	}
}

// applyBaseHeaders sets the headers AI.md PART 11 requires on all responses,
// browser-facing and API alike.
func applyBaseHeaders(w http.ResponseWriter, r *http.Request, cfg SecurityHeadersConfig, secure bool) {
	h := w.Header()

	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-XSS-Protection", "1; mode=block")
	h.Set("X-Permitted-Cross-Domain-Policies", cfg.CrossDomainPolicies)
	if cfg.OriginAgentCluster {
		h.Set("Origin-Agent-Cluster", "?1")
	}
	h.Set("Cross-Origin-Opener-Policy", cfg.COOP)
	h.Set("Cross-Origin-Embedder-Policy", cfg.COEP)
	h.Set("Cross-Origin-Resource-Policy", cfg.CORP)
	h.Set("Permissions-Policy", buildPermissionsPolicy(cfg.PermissionsPolicy))

	if endpoint := reportsEndpoint(r, cfg.APIVersion, "default"); endpoint != "" {
		h.Set("Reporting-Endpoints", `default="`+endpoint+`"`)
		h.Set("Report-To", `{"group":"default","max_age":10886400,"endpoints":[{"url":"`+endpoint+`"}]}`)
		if cfg.NEL.Enabled {
			h.Set("NEL", buildNEL(cfg.NEL))
		}
	}

	if secure {
		h.Set("Strict-Transport-Security", hstsValue)
	}

	// Never advertise the server software (AI.md PART 11 Tier 1 disclosure).
	h.Del("Server")
}

// cspHeaderName returns the enforcing or report-only CSP header name. AI.md
// PART 11: development mode reports instead of enforcing unless the operator
// explicitly asked for enforcement.
func cspHeaderName(cfg CSPConfig) string {
	if strings.EqualFold(cfg.Mode, "report-only") {
		return "Content-Security-Policy-Report-Only"
	}
	if mode.IsAppModeDev() && !strings.EqualFold(cfg.Mode, "enforce") {
		return "Content-Security-Policy-Report-Only"
	}
	return "Content-Security-Policy"
}

// buildCSP renders the AI.md PART 11 default policy with the configured
// per-directive extras appended.
func buildCSP(cfg SecurityHeadersConfig, secure bool) string {
	c := cfg.CSP
	directives := []cspDirective{
		{"default-src", "'self'"},
		{"script-src", appendSources("'self'", c.ScriptSrcExtra)},
		{"style-src", appendSources("'self' 'unsafe-inline'", c.StyleSrcExtra)},
		{"img-src", appendSources("'self' data: blob: https:", c.ImgSrcExtra)},
		{"font-src", appendSources("'self' https:", c.FontSrcExtra)},
		{"connect-src", appendSources("'self'", c.ConnectSrcExtra)},
		{"media-src", "'self' blob:"},
		{"worker-src", "'self' blob:"},
		{"manifest-src", "'self'"},
		{"frame-src", appendSources("'self'", c.FrameSrcExtra)},
		{"frame-ancestors", "'self'"},
		{"base-uri", "'self'"},
		{"form-action", appendSources("'self'", c.FormActionExtra)},
		{"object-src", "'none'"},
	}

	parts := make([]string, 0, len(directives)+3)
	for _, d := range directives {
		parts = append(parts, d.name+" "+d.value)
	}
	if secure {
		parts = append(parts, "upgrade-insecure-requests")
	}
	parts = append(parts, "report-to default")
	if path := reportsPath(cfg.APIVersion, "csp"); path != "" {
		parts = append(parts, "report-uri "+path)
	}

	return strings.Join(parts, "; ")
}

// appendSources appends operator-configured extra sources to a directive's
// spec default, ignoring an empty extra.
func appendSources(base, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	return base + " " + extra
}

// buildPermissionsPolicy joins the configured features into the header value.
// A feature with an empty value is skipped entirely so the browser default
// applies, per the AI.md PART 11 generation rule.
func buildPermissionsPolicy(features []permissionsFeature) string {
	parts := make([]string, 0, len(features))
	for _, f := range features {
		if f.value == "" {
			continue
		}
		parts = append(parts, f.name+"="+f.value)
	}
	return strings.Join(parts, ", ")
}

// buildNEL renders the Network Error Logging policy pointing at the default
// reporting group.
func buildNEL(cfg NELConfig) string {
	value := `{"report_to":"default","max_age":` + itoa(cfg.MaxAgeSeconds)
	if cfg.IncludeSubdomain {
		value += `,"include_subdomains":true`
	}
	return value + "}"
}

// reportsPath returns the origin-relative reports path for name.
func reportsPath(apiVersion, name string) string {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return ""
	}
	return "/api/" + apiVersion + "/server/reports/" + name
}

// reportsEndpoint returns the absolute reports URL for name, derived from the
// request. The Host header is attacker-controlled, so a host that is not a
// plausible authority is dropped rather than reflected into a response header.
func reportsEndpoint(r *http.Request, apiVersion, name string) string {
	path := reportsPath(apiVersion, name)
	if path == "" || r == nil || !isSafeHostHeader(r.Host) {
		return ""
	}

	scheme := "http"
	if util.TrustedIsHTTPS(r) {
		scheme = "https"
	}

	return scheme + "://" + r.Host + path
}

// isSafeHostHeader reports whether host is safe to interpolate into a response
// header value: hostname, IPv6 literal or host:port characters only.
func isSafeHostHeader(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, c := range host {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '-', c == ':', c == '[', c == ']':
		default:
			return false
		}
	}
	return true
}

// itoa renders a non-negative int without pulling in strconv for one call
// site's fixed, in-range value.
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
