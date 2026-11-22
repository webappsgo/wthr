package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/secret"
)

// sessionCookieSignatureSeparator splits the session token from its signature.
const sessionCookieSignatureSeparator = "."

// signSessionCookieValue returns token + "." + HMAC-SHA256 over the cookie name and token
// under cookie_signing_key (AI.md PART 11 "Other Project-Level Secrets"). Binding the name
// stops a value from one session cookie being replayed as another.
func signSessionCookieValue(key []byte, name, token string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(name + "|" + token))
	return token + sessionCookieSignatureSeparator + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifySessionCookieValue returns the bare token when a signed value verifies under the
// current cookie_signing_key or, during rotation, the previous one.
func verifySessionCookieValue(store *secret.Store, name, signed string) (string, bool) {
	cut := strings.LastIndex(signed, sessionCookieSignatureSeparator)
	if cut <= 0 || cut == len(signed)-1 {
		return "", false
	}
	token := signed[:cut]

	candidates, err := store.Candidates(secret.CookieSigningKey)
	if err != nil {
		return "", false
	}
	for _, key := range candidates {
		if subtle.ConstantTimeCompare([]byte(signSessionCookieValue(key, name, token)), []byte(signed)) == 1 {
			return token, true
		}
	}
	return "", false
}

// isSessionCookieName reports whether a cookie is a browser session cookie.
func isSessionCookieName(name string) bool {
	for _, candidate := range sessionCookieNames() {
		if candidate == name {
			return true
		}
	}
	return false
}

// SessionCookieSigning signs the session cookies of every response and verifies them on every
// request, so a tampered or forged session cookie is dropped before any handler sees it. The
// handlers keep reading and writing the bare token: outbound Set-Cookie values are signed here
// and inbound Cookie values are verified and stripped of the signature here. A request whose
// session cookie is unsigned or does not verify arrives without that cookie, as anonymous.
func SessionCookieSigning() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			store := secret.Default()
			if store == nil {
				next.ServeHTTP(w, r)
				return
			}

			verifyInboundSessionCookies(store, r)
			next.ServeHTTP(&sessionCookieSigningWriter{ResponseWriter: w, store: store}, r)
		})
	}
}

// verifyInboundSessionCookies rewrites the Cookie header: a session cookie keeps only its
// verified bare token, and one that fails verification is removed.
func verifyInboundSessionCookies(store *secret.Store, r *http.Request) {
	cookies := r.Cookies()
	if len(cookies) == 0 {
		return
	}

	r.Header.Del("Cookie")
	for _, cookie := range cookies {
		if isSessionCookieName(cookie.Name) {
			token, ok := verifySessionCookieValue(store, cookie.Name, cookie.Value)
			if !ok {
				continue
			}
			cookie.Value = token
		}
		r.AddCookie(cookie)
	}
}

// sessionCookieSigningWriter signs session cookies as the response headers go out.
type sessionCookieSigningWriter struct {
	http.ResponseWriter
	store       *secret.Store
	wroteHeader bool
}

// WriteHeader signs the session cookies once, just before the status line is sent.
func (w *sessionCookieSigningWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.signSessionCookies()
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write sends the implicit 200 through WriteHeader so the signing also runs for it.
func (w *sessionCookieSigningWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap exposes the underlying writer so http.ResponseController, Flush and Hijack still work.
func (w *sessionCookieSigningWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// signSessionCookies replaces the value of every Set-Cookie for a session cookie that sets a
// value. A cookie that clears the session (empty value, or a negative MaxAge) is left alone.
func (w *sessionCookieSigningWriter) signSessionCookies() {
	header := w.Header()
	values := header.Values("Set-Cookie")
	if len(values) == 0 {
		return
	}

	key, err := w.store.Current(secret.CookieSigningKey)
	if err != nil {
		return
	}

	rewritten := make([]string, 0, len(values))
	for _, value := range values {
		cookie, parseErr := http.ParseSetCookie(value)
		if parseErr != nil || !isSessionCookieName(cookie.Name) || cookie.Value == "" || cookie.MaxAge < 0 {
			rewritten = append(rewritten, value)
			continue
		}
		cookie.Value = signSessionCookieValue(key, cookie.Name, cookie.Value)
		rewritten = append(rewritten, cookie.String())
	}

	header.Del("Set-Cookie")
	for _, value := range rewritten {
		header.Add("Set-Cookie", value)
	}
}
