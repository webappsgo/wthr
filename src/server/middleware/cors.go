package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/rs/cors"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

// corsAllowAll is the wildcard origin.
const corsAllowAll = "*"

// corsMaxAgeSeconds is Access-Control-Max-Age: 24 hours (AI.md PART 16 "CORS Headers").
const corsMaxAgeSeconds = 86400

// corsAllowMethods and corsAllowHeaders are the AI.md PART 16 "CORS Headers" lists. The header
// list names every header a browser client may send, because the Fetch wildcard does not
// cover Authorization.
var (
	corsAllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	corsAllowHeaders = []string{
		"Content-Type", "Accept", "X-Requested-With", "Authorization", "X-CSRF-Token", "X-XSRF-Token",
		"MCP-Protocol-Version", "Mcp-Method", "Mcp-Name",
	}
)

var (
	learnedCORSMutex sync.Mutex
	learnedCORSHosts = map[string]bool{}
)

// CORSPolicy is the resolved allow-list for one request (AI.md PART 16 "CORS Allow-list
// Resolution Order"): either every origin without credentials, or an explicit list.
type CORSPolicy struct {
	// Disabled is true when web.cors is "" (same-origin only: no CORS headers at all)
	Disabled bool
	// AllowAll is true when no source produced an explicit list
	AllowAll bool
	// Origins is the explicit list; credentials are allowed only for these
	Origins []string
}

// normalizeCORSOrigin lowercases an origin and drops a trailing slash so the request
// Origin header and the configured entry compare equal.
func normalizeCORSOrigin(origin string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(origin)), "/")
}

// LearnCORSHost records a hostname seen in X-Forwarded-Host from a trusted proxy as an
// https origin (resolution step 3). Untrusted peers never reach this function.
func LearnCORSHost(host string) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || strings.ContainsAny(host, " /\\@") {
		return
	}
	learnedCORSMutex.Lock()
	learnedCORSHosts["https://"+host] = true
	learnedCORSMutex.Unlock()
}

// ResetLearnedCORSHosts forgets every learned host; tests use it for isolation.
func ResetLearnedCORSHosts() {
	learnedCORSMutex.Lock()
	learnedCORSHosts = map[string]bool{}
	learnedCORSMutex.Unlock()
}

// LearnedCORSOrigins returns the origins learned from trusted proxies, for the CSP
// connect-src list that shares this resolved allow-list (AI.md PART 11).
func LearnedCORSOrigins() []string {
	learnedCORSMutex.Lock()
	defer learnedCORSMutex.Unlock()
	origins := make([]string, 0, len(learnedCORSHosts))
	for origin := range learnedCORSHosts {
		origins = append(origins, origin)
	}
	return origins
}

// ResolveCORSPolicy builds the allow-list from, in order: web.cors (explicit entries),
// the DOMAIN environment variable, hostnames learned from trusted proxies, and finally
// the `*` fallback. The shipped default "*" is not explicit config, it means unset and
// lets resolution continue; "" disables CORS entirely and stops resolution.
func ResolveCORSPolicy(webCORS string) CORSPolicy {
	trimmed := strings.TrimSpace(webCORS)
	if trimmed == "" {
		return CORSPolicy{Disabled: true}
	}

	seen := map[string]bool{}
	var origins []string
	add := func(origin string) {
		origin = normalizeCORSOrigin(origin)
		if origin == "" || origin == corsAllowAll || seen[origin] {
			return
		}
		seen[origin] = true
		origins = append(origins, origin)
	}

	if trimmed != corsAllowAll {
		for _, entry := range strings.Split(trimmed, ",") {
			add(entry)
		}
	}
	for _, domain := range strings.Split(os.Getenv("DOMAIN"), ",") {
		domain = strings.TrimSpace(domain)
		if domain != "" {
			add("https://" + domain)
		}
	}
	for _, learned := range LearnedCORSOrigins() {
		add(learned)
	}

	if len(origins) == 0 {
		return CORSPolicy{AllowAll: true}
	}
	return CORSPolicy{Origins: origins}
}

// allows reports whether an explicit list contains the request origin.
func (p CORSPolicy) allows(origin string) bool {
	origin = normalizeCORSOrigin(origin)
	for _, candidate := range p.Origins {
		if candidate == origin {
			return true
		}
	}
	return false
}

// key identifies a policy so the middleware can reuse its rs/cors handler.
func (p CORSPolicy) key() string {
	switch {
	case p.Disabled:
		return "disabled"
	case p.AllowAll:
		return "all"
	}
	return "explicit:" + strings.Join(p.Origins, ",")
}

var (
	corsHandlerMutex sync.Mutex
	corsHandlerKey   string
	corsHandler      *cors.Cors
)

// corsHandlerFor returns the rs/cors handler of a policy, rebuilding it only when the
// resolved allow-list changes. `*` sends no credentials; an explicit list echoes the
// matching origin and allows credentials (AI.md PART 16 "CORS" -> "Behavior").
func corsHandlerFor(policy CORSPolicy) *cors.Cors {
	corsHandlerMutex.Lock()
	defer corsHandlerMutex.Unlock()

	if corsHandler != nil && corsHandlerKey == policy.key() {
		return corsHandler
	}

	options := cors.Options{
		AllowedMethods: corsAllowMethods,
		AllowedHeaders: corsAllowHeaders,
		MaxAge:         corsMaxAgeSeconds,
	}
	if policy.AllowAll {
		options.AllowedOrigins = []string{corsAllowAll}
	} else {
		options.AllowOriginFunc = policy.allows
		options.AllowCredentials = true
	}
	corsHandler = cors.New(options)
	corsHandlerKey = policy.key()
	return corsHandler
}

// CORS applies the one CORS policy to every route (AI.md PART 16 "CORS"): API routes
// follow the same policy. The policy is resolved from the live config on every request,
// so an admin change applies without a restart; the headers themselves come from rs/cors.
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			webCORS := corsAllowAll
			if cfg := config.GetGlobalConfig(); cfg != nil {
				webCORS = cfg.Web.CORS
			}

			if host := r.Header.Get("X-Forwarded-Host"); host != "" && util.IsTrustedProxyPeer(r) {
				LearnCORSHost(host)
			}

			policy := ResolveCORSPolicy(webCORS)
			if policy.Disabled {
				next.ServeHTTP(w, r)
				return
			}
			corsHandlerFor(policy).Handler(next).ServeHTTP(w, r)
		})
	}
}

// browserRequestHeaderList formats header names the way a browser sends them in
// Access-Control-Request-Headers: lowercase, sorted and comma-separated (Fetch standard).
func browserRequestHeaderList(names []string) string {
	lowered := make([]string, len(names))
	for i, name := range names {
		lowered[i] = strings.ToLower(name)
	}
	sort.Strings(lowered)
	return strings.Join(lowered, ",")
}

// CORSHeaderPreview returns the headers a browser at origin receives from a preflight
// under the given web.cors value. It runs the real middleware against a synthetic
// preflight, so the admin preview can never drift from what the server sends.
func CORSHeaderPreview(webCORS, origin string) http.Header {
	policy := ResolveCORSPolicy(webCORS)
	if policy.Disabled {
		return http.Header{}
	}

	request := httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", origin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", browserRequestHeaderList(corsAllowHeaders))

	recorder := httptest.NewRecorder()
	corsHandlerFor(policy).Handler(http.NotFoundHandler()).ServeHTTP(recorder, request)
	return recorder.Header()
}
