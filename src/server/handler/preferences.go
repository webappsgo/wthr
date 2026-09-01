package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/util"
)

// preferencesPath is the canonical guest-facing preferences route per AI.md
// PART 16 and the redirect target of last resort after a POST.
const preferencesPath = "/server/preferences"

// PreferencesHandler serves the public preferences page (AI.md PART 16).
// It owns no theme storage of its own: guests are persisted through the theme
// cookie, and logged-in users go through the same settings pipeline that backs
// /users/settings/appearance so both surfaces stay in sync.
type PreferencesHandler struct {
	DB *sql.DB
}

// NewPreferencesHandler creates a preferences handler bound to the user database.
func NewPreferencesHandler(db *sql.DB) *PreferencesHandler {
	return &PreferencesHandler{DB: db}
}

// ShowPreferences renders the preferences page with content negotiation.
// Route: GET /server/preferences
func (h *PreferencesHandler) ShowPreferences(w http.ResponseWriter, r *http.Request) {
	theme := server.ResolveTheme(r)
	_, authenticated := middleware.GetCurrentUser(r)

	data := map[string]interface{}{
		"page":          "preferences",
		"theme":         theme,
		"next_theme":    server.NextTheme(theme),
		"themes":        []string{"dark", "light", "auto"},
		"authenticated": authenticated,
	}

	NegotiateResponse(w, r, "page/preferences.tmpl", data)
}

// SavePreferences applies a preference change and redirects back to the page
// the request came from, so the toggle works with JavaScript disabled.
// Route: POST /server/preferences
func (h *PreferencesHandler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	theme := strings.ToLower(strings.TrimSpace(r.FormValue("theme")))
	if !server.IsValidTheme(theme) {
		BadRequest(w, r, Translate(r, "errors.preferences.invalid_theme"))
		return
	}

	// The cookie is always written so the server can render the correct
	// theme-* class on <html> for the very next request, with zero JS.
	server.SetThemeCookie(w, r, theme)

	// Logged-in users additionally persist to the database through the same
	// update path as /users/settings/appearance, never a parallel query.
	if user, ok := middleware.GetCurrentUser(r); ok && h.DB != nil {
		req := &UpdateSettingsRequest{Appearance: &AppearanceSettings{Theme: theme}}
		if err := ApplyUserSettingsUpdate(h.DB, user.ID, req); err != nil {
			InternalError(w, r, Translate(r, "errors.preferences.save_failed"))
			return
		}
	}

	http.Redirect(w, r, preferencesRedirectTarget(r), http.StatusSeeOther)
}

// preferencesRedirectTarget resolves where to send the browser after a
// preference change: an explicit redirect field, else the referring page,
// else the preferences page itself. Only same-site, path-rooted targets are
// accepted - protocol-relative ("//evil.com") and backslash ("/\evil.com")
// forms are rejected because browsers read them as scheme-qualified URLs.
func preferencesRedirectTarget(r *http.Request) string {
	if target := util.SafeRedirectPath(r.FormValue("redirect")); target != "" {
		return target
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		if parsed, err := url.Parse(ref); err == nil {
			if parsed.Host == "" || parsed.Host == r.Host {
				candidate := parsed.EscapedPath()
				if parsed.RawQuery != "" {
					candidate += "?" + parsed.RawQuery
				}
				if target := util.SafeRedirectPath(candidate); target != "" {
					return target
				}
			}
		}
	}
	return preferencesPath
}
