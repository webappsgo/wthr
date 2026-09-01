// Package server provides theme detection, switching, and persistence
// per AI.md PART 16 "Themes (NON-NEGOTIABLE - PROJECT-WIDE)".
package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/util"
)

// ThemeCookieName is the cookie that persists a guest's (or a logged-in
// user's mirrored) theme preference, per AI.md PART 16 Theme Detection Flow.
const ThemeCookieName = "theme"

// ThemeCookieMaxAge is one year, matching a long-lived UI preference cookie.
const ThemeCookieMaxAge = 365 * 24 * int(time.Hour/time.Second)

// DefaultTheme is the theme applied when no preference can be resolved,
// per AI.md PART 16 "Default to dark if no preference set".
const DefaultTheme = "dark"

// ValidThemes enumerates the three NON-NEGOTIABLE required themes.
var ValidThemes = map[string]bool{
	"dark":  true,
	"light": true,
	"auto":  true,
}

// IsValidTheme reports whether mode is one of the three required themes.
func IsValidTheme(mode string) bool {
	return ValidThemes[mode]
}

// ResolveTheme determines the theme to render on <html>, per the Theme
// Detection Flow: theme cookie (guests) or DB preference (logged-in users,
// mirrored into the cookie by the preference-save handlers) - defaulting to
// dark when no valid preference is present. It never inspects JavaScript
// state, so the correct theme class renders on first paint with zero JS.
func ResolveTheme(r *http.Request) string {
	if cookie, err := r.Cookie(ThemeCookieName); err == nil && IsValidTheme(cookie.Value) {
		return cookie.Value
	}
	return DefaultTheme
}

// SetThemeCookie persists a theme preference into the theme cookie so the
// server can render the correct theme-* class on the next request without
// JavaScript. Called by the theme-toggle route and by any handler that
// saves a logged-in user's/admin's theme preference to the database, so
// the cookie mirror always matches the stored preference.
func SetThemeCookie(w http.ResponseWriter, r *http.Request, mode string) {
	if !IsValidTheme(mode) {
		mode = DefaultTheme
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     ThemeCookieName,
		Value:    mode,
		Path:     "/",
		MaxAge:   ThemeCookieMaxAge,
		Secure:   secure,
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
	})
}

// NextTheme returns the next mode in the cycle: dark -> light -> auto -> dark,
// per AI.md PART 16. It is registered as the "nextTheme" template func so a
// toggle's POST value is always derived from the resolved .Theme rather than
// hardcoded - a fixed target only works on the first click. Input is trimmed
// and lowercased; anything unrecognised (including the empty string) is
// treated as unset and yields the default theme.
func NextTheme(current string) string {
	switch strings.ToLower(strings.TrimSpace(current)) {
	case "dark":
		return "light"
	case "light":
		return "auto"
	default:
		return DefaultTheme
	}
}

// ThemeClass returns the literal class attribute value to render on <html>
// for the given resolved mode ("theme-dark", "theme-light", "theme-auto").
func ThemeClass(mode string) string {
	if !IsValidTheme(mode) {
		mode = DefaultTheme
	}
	return "theme-" + mode
}

// SetThemeHandler is the POST form target for the theme toggle per AI.md
// PART 16 "Theme Switching" - works without JS, external JS may intercept
// the form submit and swap the CSS class instantly instead of reloading.
func SetThemeHandler(w http.ResponseWriter, r *http.Request) {
	mode := r.FormValue("theme")
	if !IsValidTheme(mode) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid theme"))
		return
	}
	SetThemeCookie(w, r, mode)

	// Only allow same-site, path-only redirects - util.SafeRedirectPath rejects
	// empty, non-rooted, protocol-relative ("//evil.com") and backslash-based
	// ("/\evil.com") targets, which browsers can interpret as scheme-qualified
	// URLs and enable an open redirect.
	redirectTo := util.SafeRedirectPath(r.FormValue("redirect"))
	if redirectTo == "" {
		redirectTo = "/"
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}
