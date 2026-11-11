// Server-rendered one-shot flash messages per AI.md PART 16
package handler

import (
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/util"
)

// FlashCookieName is the short-lived cookie carrying a one-shot flash message.
const FlashCookieName = "flash"

// Flash is a one-shot message rendered as a static dismissible alert on the
// page reached by a POST-redirect-GET, so form feedback works without JavaScript.
type Flash struct {
	Kind string
	Key  string
}

// flashKinds are the alert styles a flash may request.
var flashKinds = map[string]bool{
	"success": true,
	"error":   true,
	"info":    true,
	"warning": true,
}

// flashKeys is the allow-list of translation keys a flash may name. The cookie
// value is attacker-controllable, so only keys listed here are ever rendered.
var flashKeys = map[string]bool{
	"flash.settings_saved":              true,
	"flash.settings_save_failed":        true,
	"flash.channel_updated":             true,
	"flash.channel_update_failed":       true,
	"flash.channel_enabled":             true,
	"flash.channel_disabled":            true,
	"flash.channel_test_sent":           true,
	"flash.channel_test_failed":         true,
	"flash.email_template_saved":        true,
	"flash.email_template_save_failed":  true,
	"flash.test_email_sent":             true,
	"flash.test_email_failed":           true,
	"flash.contact_sent":                true,
	"flash.contact_saved":               true,
	"flash.contact_invalid":             true,
	"flash.contact_failed":              true,
	"flash.contact_captcha_failed":      true,
	"flash.contact_captcha_unavailable": true,
}

// SetFlash stores a one-shot flash message for the next rendered page.
func SetFlash(w http.ResponseWriter, r *http.Request, kind, key string) {
	if middleware.GPCOptOut(r) {
		return
	}
	if !flashKinds[kind] || !flashKeys[key] {
		return
	}

	secure := util.TrustedIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     FlashCookieName,
		Value:    kind + ":" + key,
		Path:     "/",
		MaxAge:   120,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// TakeFlash returns the pending flash message, if any, and clears the cookie so
// the message is displayed exactly once.
func TakeFlash(w http.ResponseWriter, r *http.Request) *Flash {
	if middleware.GPCOptOut(r) {
		return nil
	}
	cookie, err := r.Cookie(FlashCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}

	secure := util.TrustedIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     FlashCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	kind, key, found := strings.Cut(cookie.Value, ":")
	if !found || !flashKinds[kind] || !flashKeys[key] {
		return nil
	}

	return &Flash{Kind: kind, Key: key}
}
