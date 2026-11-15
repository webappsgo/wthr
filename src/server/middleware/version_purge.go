package middleware

import (
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/buildstamp"
)

// buildCookieName is the essential, non-sensitive cookie that carries the build stamp the
// browser last saw (AI.md PART 9 "Version-Change Purge": {project_name}_build).
const buildCookieName = "wthr_build"

// VersionPurge stamps every successful top-level HTML navigation with the running build
// and, when the browser last saw a different build, asks it to drop its HTTP cache and
// storage with `Clear-Site-Data: "cache", "storage"`. "cookies" is never included: that
// would end the session and erase cookie-stored preferences. The same response re-sets
// the cookie, so the purge cannot repeat, and a first visit (no cookie) never purges.
func VersionPurge() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !topLevelNavigation(r) {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(&purgeWriter{ResponseWriter: w, r: r}, r)
		})
	}
}

// topLevelNavigation is true for a document navigation, or an old browser that sends no
// Sec-Fetch headers; iframes, prefetches and fetch/XHR calls are excluded.
func topLevelNavigation(r *http.Request) bool {
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" && dest != "document" {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" && mode != "navigate" {
		return false
	}
	return true
}

type purgeWriter struct {
	http.ResponseWriter
	r    *http.Request
	done bool
}

func (p *purgeWriter) apply(status int) {
	if p.done {
		return
	}
	p.done = true
	if status < 200 || status > 299 || !strings.HasPrefix(p.Header().Get("Content-Type"), "text/html") {
		return
	}
	stamp := buildstamp.Stamp()
	if c, err := p.r.Cookie(buildCookieName); err == nil && c.Value != stamp {
		p.Header().Set("Clear-Site-Data", `"cache", "storage"`)
	}
	http.SetCookie(p.ResponseWriter, &http.Cookie{
		Name: buildCookieName, Value: stamp, Path: "/", MaxAge: 31536000,
		Secure:   p.r.TLS != nil || strings.EqualFold(p.r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (p *purgeWriter) WriteHeader(status int) {
	p.apply(status)
	p.ResponseWriter.WriteHeader(status)
}

func (p *purgeWriter) Write(b []byte) (int, error) {
	if !p.done {
		if p.Header().Get("Content-Type") == "" {
			p.Header().Set("Content-Type", http.DetectContentType(b))
		}
		p.apply(http.StatusOK)
	}
	return p.ResponseWriter.Write(b)
}

// Flush keeps streaming responses working through the wrapper.
func (p *purgeWriter) Flush() {
	if f, ok := p.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
