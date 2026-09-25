package handler

import (
	"net/http"
	"strings"
)

// wantsFormSubmission reports whether the request is a browser HTML form
// submission rather than a JSON API call. AI.md PART 16 requires the admin
// panel to work with JavaScript disabled, so form posts bind through
// DecodeAndValidate and answer with a flash plus a POST-redirect-GET back to
// the originating page instead of a JSON body. The check is positive rather
// than negative so a token-auth POST that carries no body and no content type
// keeps its canonical JSON response.
func wantsFormSubmission(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
}

// adminConfigPagePath resolves the session-auth admin config page that owns a
// mutation endpoint, so a form post can redirect back to the page it came
// from. The admin path prefix is everything before the first `/config/`
// segment, which is what `/server/{admin_path}/config/*` routes share.
func adminConfigPagePath(r *http.Request, page string) string {
	idx := strings.Index(r.URL.Path, "/config/")
	if idx < 0 {
		return page
	}
	return r.URL.Path[:idx] + page
}

// redirectAdminForm performs the redirect half of the post redirect get flow
// used by the session-auth admin form routes, carrying the outcome to the
// next request through the one-shot flash cookie.
func redirectAdminForm(w http.ResponseWriter, r *http.Request, target, successKey, failureKey string, err error) {
	if err != nil {
		SetFlash(w, r, "error", failureKey)
	} else {
		SetFlash(w, r, "success", successKey)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
