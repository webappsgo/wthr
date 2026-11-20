package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/secret"
)

// issueSession runs a login-like handler through the signing middleware and returns the cookie the browser stores.
func issueSession(t *testing.T, name, token string) *http.Cookie {
	t.Helper()
	handler := SessionCookieSigning()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
		_, _ = w.Write([]byte("ok"))
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatal("no session cookie was issued")
	return nil
}

// seenToken runs one request carrying cookie and returns what the handler reads.
func seenToken(name string, cookie *http.Cookie) (string, bool) {
	var token string
	var found bool
	handler := SessionCookieSigning()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(name); err == nil {
			token, found = c.Value, true
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return token, found
}

func TestSessionCookieIsSignedOutAndVerifiedIn(t *testing.T) {
	csrfStoreFixture(t)
	issued := issueSession(t, UserSessionCookieName(), "tok_abc123")

	if issued.Value == "tok_abc123" || !strings.HasPrefix(issued.Value, "tok_abc123.") {
		t.Fatalf("the browser must receive a signed value, got %q", issued.Value)
	}
	if !issued.HttpOnly || issued.SameSite != http.SameSiteStrictMode || issued.MaxAge != 3600 {
		t.Errorf("the other cookie attributes must survive signing, got %+v", issued)
	}

	token, found := seenToken(UserSessionCookieName(), &http.Cookie{Name: issued.Name, Value: issued.Value})
	if !found || token != "tok_abc123" {
		t.Errorf("handlers must see the bare token, got %q, %v", token, found)
	}
}

func TestSessionCookieRejectsForgeries(t *testing.T) {
	csrfStoreFixture(t)
	issued := issueSession(t, UserSessionCookieName(), "tok_abc123")

	forgeries := map[string]string{
		"unsigned":         "tok_abc123",
		"empty signature":  "tok_abc123.",
		"wrong signature":  "tok_abc123.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"swapped token":    "tok_other" + issued.Value[strings.Index(issued.Value, "."):],
		"only a signature": "." + issued.Value[strings.Index(issued.Value, ".")+1:],
	}
	for label, value := range forgeries {
		if _, found := seenToken(UserSessionCookieName(), &http.Cookie{Name: UserSessionCookieName(), Value: value}); found {
			t.Errorf("%s: a forged session cookie must be dropped", label)
		}
	}
}

func TestSessionCookieIsBoundToItsName(t *testing.T) {
	csrfStoreFixture(t)
	user := issueSession(t, UserSessionCookieName(), "tok_abc123")
	if _, found := seenToken("admin_session", &http.Cookie{Name: "admin_session", Value: user.Value}); found {
		t.Error("a user session value replayed as the admin session cookie must be dropped")
	}
}

func TestSessionCookieSurvivesAKeyRotationDuringGrace(t *testing.T) {
	store := csrfStoreFixture(t)
	issued := issueSession(t, UserSessionCookieName(), "tok_abc123")

	if err := store.Rotate(secret.CookieSigningKey, secret.RotateOptions{}); err != nil {
		t.Fatal(err)
	}
	if token, found := seenToken(UserSessionCookieName(), &http.Cookie{Name: UserSessionCookieName(), Value: issued.Value}); !found || token != "tok_abc123" {
		t.Errorf("a cookie signed before the rotation must verify with the previous key, got %q, %v", token, found)
	}
}

func TestSessionCookieClearingAndOtherCookiesAreUntouched(t *testing.T) {
	csrfStoreFixture(t)
	handler := SessionCookieSigning()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: UserSessionCookieName(), Value: "", MaxAge: -1, Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "theme", Value: "dark", Path: "/"})
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/logout", nil))

	for _, cookie := range w.Result().Cookies() {
		switch cookie.Name {
		case UserSessionCookieName():
			if cookie.Value != "" || cookie.MaxAge >= 0 {
				t.Errorf("a clearing cookie must stay a clearing cookie, got %+v", cookie)
			}
		case "theme":
			if cookie.Value != "dark" {
				t.Errorf("a non-session cookie must not be signed, got %q", cookie.Value)
			}
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	var theme string
	SessionCookieSigning()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("theme"); err == nil {
			theme = c.Value
		}
	})).ServeHTTP(httptest.NewRecorder(), req)
	if theme != "light" {
		t.Errorf("a non-session cookie must pass through unchanged, got %q", theme)
	}
}

func TestSessionCookieSigningIsOffWithoutAStore(t *testing.T) {
	secret.SetDefault(nil)
	issued := issueSession(t, UserSessionCookieName(), "plain")
	if issued.Value != "plain" {
		t.Errorf("without a store the value must pass through, got %q", issued.Value)
	}
}
