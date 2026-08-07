// Package server provides theme detection, switching, and persistence
// per AI.md PART 16 "Themes (NON-NEGOTIABLE - PROJECT-WIDE)".
package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
func ResolveTheme(c *gin.Context) string {
	if cookieTheme, err := c.Cookie(ThemeCookieName); err == nil && IsValidTheme(cookieTheme) {
		return cookieTheme
	}
	return DefaultTheme
}

// SetThemeCookie persists a theme preference into the theme cookie so the
// server can render the correct theme-* class on the next request without
// JavaScript. Called by the theme-toggle route and by any handler that
// saves a logged-in user's/admin's theme preference to the database, so
// the cookie mirror always matches the stored preference.
func SetThemeCookie(c *gin.Context, mode string) {
	if !IsValidTheme(mode) {
		mode = DefaultTheme
	}
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(ThemeCookieName, mode, ThemeCookieMaxAge, "/", "", secure, false)
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
func SetThemeHandler(c *gin.Context) {
	mode := c.PostForm("theme")
	if !IsValidTheme(mode) {
		c.String(http.StatusBadRequest, "invalid theme")
		return
	}
	SetThemeCookie(c, mode)

	// Only allow same-site, path-only redirects - reject empty, non-rooted,
	// protocol-relative ("//evil.com") and backslash-based ("/\evil.com")
	// targets, which browsers can interpret as scheme-qualified URLs and
	// enable an open redirect.
	redirectTo := c.PostForm("redirect")
	if redirectTo == "" ||
		!strings.HasPrefix(redirectTo, "/") ||
		strings.HasPrefix(redirectTo, "//") ||
		strings.HasPrefix(redirectTo, "/\\") {
		redirectTo = "/"
	}
	c.Redirect(http.StatusSeeOther, redirectTo)
}
