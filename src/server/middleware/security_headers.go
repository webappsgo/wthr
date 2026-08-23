package middleware

import (
	"net/http"
)

// SecurityHeaders adds security headers to all responses per TEMPLATE.md requirements
// These headers protect against common web vulnerabilities:
// - XSS attacks
// - Clickjacking
// - MIME type sniffing
// - Information disclosure
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent MIME type sniffing
			// Browsers should not try to detect content type, trust the Content-Type header
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Prevent clickjacking attacks
			// Deny embedding this site in frames/iframes from other origins
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")

			// XSS Protection (legacy browsers)
			// Modern browsers use CSP instead, but this helps older browsers
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Content Security Policy
			// Allows resources needed for maps (Leaflet from unpkg.com, tiles from OpenStreetMap)
			// Allows inline styles for Dracula theme; all JS lives in static/js/app.js
			// bound via data-action delegation, so script-src no longer needs unsafe-inline
			csp := "default-src 'self'; " +
				"script-src 'self' https://unpkg.com; " +
				"style-src 'self' 'unsafe-inline' https://unpkg.com; " +
				"img-src 'self' data: https: blob:; " +
				"font-src 'self' https://unpkg.com; " +
				"connect-src 'self' https://*.tile.openstreetmap.org wss: ws:; " +
				"frame-ancestors 'self'; " +
				"base-uri 'self'; " +
				"form-action 'self'"
			w.Header().Set("Content-Security-Policy", csp)

			// Referrer Policy
			// Only send referrer for same-origin requests
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Permissions Policy (formerly Feature-Policy)
			// Disable potentially dangerous browser features
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")

			// Strict Transport Security (HSTS)
			// Force HTTPS for 1 year, include subdomains
			// Only set if connection is HTTPS
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			// Remove server identification header for security
			// Don't leak server software version
			w.Header().Del("Server")

			// Cross-Origin policies
			// Use credentialless instead of require-corp to allow loading external resources
			// (maps, CDN scripts) while still providing isolation benefits
			w.Header().Set("Cross-Origin-Embedder-Policy", "credentialless")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")

			// Continue processing request
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersAPI adds security headers optimized for API endpoints
// Less restrictive CSP since API endpoints don't serve HTML
func SecurityHeadersAPI() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Same basic security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			// API responses should never be framed
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Simpler CSP for API (no inline scripts/styles needed for JSON responses)
			w.Header().Set("Content-Security-Policy", "default-src 'none'")

			// API-specific headers
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")

			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Del("Server")

			// HSTS for HTTPS API requests
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
