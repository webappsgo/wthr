// Package handler provides HTTP handlers
// Persistent admin sidebar section state per AI.md PART 17
package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AdminNavCookieName is the long-lived cookie carrying the admin sidebar's
// open/closed section state across page loads.
const AdminNavCookieName = "admin_nav"

// AdminNavSections is the allow-list of sidebar section names whose state may
// be persisted. The cookie value is attacker-controllable, so only these
// names are ever echoed back into the rendered `open` attribute.
var AdminNavSections = []string{"server", "security", "network", "users", "cluster", "help"}

// adminNavSectionAllowed reports whether name is a known sidebar section.
func adminNavSectionAllowed(name string) bool {
	for _, section := range AdminNavSections {
		if section == name {
			return true
		}
	}
	return false
}

// AdminNavOpenSections returns the set of sidebar sections that should render
// open. Sections are open by default; a section is closed only when the cookie
// explicitly names it as collapsed.
func AdminNavOpenSections(r *http.Request) map[string]bool {
	open := make(map[string]bool, len(AdminNavSections))
	cookie, err := r.Cookie(AdminNavCookieName)
	if err != nil || cookie.Value == "" {
		for _, section := range AdminNavSections {
			open[section] = true
		}
		return open
	}

	collapsed := map[string]bool{}
	for _, name := range strings.Split(cookie.Value, ",") {
		name = strings.TrimSpace(name)
		if adminNavSectionAllowed(name) {
			collapsed[name] = true
		}
	}
	for _, section := range AdminNavSections {
		open[section] = !collapsed[section]
	}
	return open
}

// SetAdminNavCollapsed persists the collapsed section names as a cookie. The
// name is allow-listed before it is written, and an empty list clears the
// cookie so the sidebar falls back to all-open.
func SetAdminNavCollapsed(w http.ResponseWriter, r *http.Request, collapsed []string) {
	kept := make([]string, 0, len(collapsed))
	for _, name := range collapsed {
		name = strings.TrimSpace(name)
		if adminNavSectionAllowed(name) {
			kept = append(kept, name)
		}
	}
	if len(kept) == 0 {
		http.SetCookie(w, &http.Cookie{
			Name:     AdminNavCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		return
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     AdminNavCookieName,
		Value:    strings.Join(kept, ","),
		Path:     "/",
		MaxAge:   31536000,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// UpdateAdminNavState stores the admin sidebar's collapsed-section list. The
// request body may be JSON ({"collapsed":["users"]}) or form-encoded
// (collapsed=users&collapsed=help); unknown names are dropped before the
// cookie is written. This is the AI.md PART 17 "JS may only enhance what
// already works" path — the cookie stays HttpOnly and the next server render
// reads it back through AdminNavOpenSections.
func UpdateAdminNavState(w http.ResponseWriter, r *http.Request) {
	var collapsed []string

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var body struct {
			Collapsed []string `json:"collapsed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, Translate(r, "errors.invalid_request_body"))
			return
		}
		collapsed = body.Collapsed
	} else {
		if err := r.ParseForm(); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, Translate(r, "errors.invalid_request_body"))
			return
		}
		// AI.md PART 17: the collapsed list is request state, so it comes from
		// the POST body only. r.PostForm keeps a crafted query string from
		// overriding what the form itself submitted.
		collapsed = r.PostForm["collapsed"]
	}

	SetAdminNavCollapsed(w, r, collapsed)
	RespondSuccess(w, r, "")
}
