package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// navStateCookie returns the admin_nav cookie from a response, or nil.
func navStateCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == AdminNavCookieName {
			return cookie
		}
	}
	return nil
}

func TestAdminNavSectionAllowed(t *testing.T) {
	for _, name := range AdminNavSections {
		if !adminNavSectionAllowed(name) {
			t.Errorf("expected %q to be allowed", name)
		}
	}
	for _, name := range []string{"", "../etc", "Server", "unknown"} {
		if adminNavSectionAllowed(name) {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestAdminNavOpenSectionsDefaultsToAllOpen(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/server/admin", nil)

	open := AdminNavOpenSections(r)
	for _, section := range AdminNavSections {
		if !open[section] {
			t.Errorf("expected %q open with no cookie", section)
		}
	}
}

func TestAdminNavOpenSectionsEmptyCookieIsAllOpen(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	r.AddCookie(&http.Cookie{Name: AdminNavCookieName, Value: ""})

	open := AdminNavOpenSections(r)
	for _, section := range AdminNavSections {
		if !open[section] {
			t.Errorf("expected %q open with an empty cookie", section)
		}
	}
}

func TestAdminNavOpenSectionsDropsUnknownNames(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	r.AddCookie(&http.Cookie{Name: AdminNavCookieName, Value: "users, evil\"><script>, help"})

	open := AdminNavOpenSections(r)
	if open["users"] {
		t.Error("expected users to be collapsed")
	}
	if open["help"] {
		t.Error("expected help to be collapsed")
	}
	if !open["server"] {
		t.Error("expected unnamed sections to stay open")
	}
}

func TestSetAdminNavCollapsedFiltersAndPersists(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", nil)

	SetAdminNavCollapsed(rec, r, []string{" users ", "bogus", "help"})

	cookie := navStateCookie(t, rec)
	if cookie == nil {
		t.Fatal("expected the admin_nav cookie to be set")
	}
	if cookie.Value != "users,help" {
		t.Errorf("unexpected cookie value %q", cookie.Value)
	}
	if cookie.MaxAge != 31536000 {
		t.Errorf("unexpected MaxAge %d", cookie.MaxAge)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie must be HttpOnly and SameSite=Strict, got HttpOnly=%v SameSite=%v", cookie.HttpOnly, cookie.SameSite)
	}
	if cookie.Secure {
		t.Error("plain HTTP request must not set Secure")
	}
}

func TestSetAdminNavCollapsedSecureBehindHTTPSProxy(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://example.invalid/server/admin/config/nav-state", nil)
	r.Header.Set("X-Forwarded-Proto", "https")

	SetAdminNavCollapsed(rec, r, []string{"users"})

	cookie := navStateCookie(t, rec)
	if cookie == nil {
		t.Fatal("expected the admin_nav cookie to be set")
	}
	if !cookie.Secure {
		t.Error("expected Secure when the request arrived over HTTPS")
	}
}

func TestSetAdminNavCollapsedClearsCookieWhenEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", nil)

	SetAdminNavCollapsed(rec, r, []string{"bogus"})

	cookie := navStateCookie(t, rec)
	if cookie == nil {
		t.Fatal("expected a clearing cookie")
	}
	if cookie.MaxAge != -1 || cookie.Value != "" {
		t.Errorf("expected a cleared cookie, got value=%q MaxAge=%d", cookie.Value, cookie.MaxAge)
	}
}

func TestAdminNavStateRoundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", nil)

	SetAdminNavCollapsed(rec, r, []string{"security", "network"})

	next := httptest.NewRequest(http.MethodGet, "/server/admin/config/settings", nil)
	for _, cookie := range rec.Result().Cookies() {
		next.AddCookie(cookie)
	}

	open := AdminNavOpenSections(next)
	if open["security"] || open["network"] {
		t.Error("expected the persisted sections to render collapsed")
	}
	if !open["server"] || !open["users"] || !open["help"] {
		t.Error("expected unnamed sections to stay open")
	}
}

func TestUpdateAdminNavStateJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", strings.NewReader(`{"collapsed":["users","bogus"]}`))
	r.Header.Set("Content-Type", "application/json")

	UpdateAdminNavState(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := navStateCookie(t, rec)
	if cookie == nil || cookie.Value != "users" {
		t.Fatalf("expected only the allow-listed name in the cookie, got %#v", cookie)
	}
}

func TestUpdateAdminNavStateFormEncoded(t *testing.T) {
	rec := httptest.NewRecorder()
	form := strings.NewReader("collapsed=help&collapsed=users")
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	UpdateAdminNavState(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := navStateCookie(t, rec)
	if cookie == nil || cookie.Value != "help,users" {
		t.Fatalf("unexpected cookie %#v", cookie)
	}
}

// TestUpdateAdminNavStateIgnoresQueryParams confirms a crafted query string
// cannot override the collapsed list in the form body. AI.md PART 17 treats
// this as request state written by the sidebar, so only the POST body counts.
func TestUpdateAdminNavStateIgnoresQueryParams(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state?collapsed=security", strings.NewReader("collapsed=users"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	UpdateAdminNavState(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := navStateCookie(t, rec)
	if cookie == nil || cookie.Value != "users" {
		t.Fatalf("query param leaked into the cookie: %#v", cookie)
	}
}

func TestUpdateAdminNavStateRejectsMalformedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", strings.NewReader("{not json"))
	r.Header.Set("Content-Type", "application/json")

	UpdateAdminNavState(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if navStateCookie(t, rec) != nil {
		t.Error("no cookie may be written when the body is invalid")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["ok"] != false || body["error"] != ErrInvalidInput {
		t.Errorf("unexpected error body %#v", body)
	}
	if message, _ := body["message"].(string); strings.Contains(message, "json") || message == "" {
		t.Errorf("expected a translated message, got %q", message)
	}
}

func TestUpdateAdminNavStateSuccessShape(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/nav-state", strings.NewReader(`{"collapsed":[]}`))
	r.Header.Set("Content-Type", "application/json")

	UpdateAdminNavState(rec, r)

	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Errorf("unexpected Content-Type %q", got)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["ok"] != true {
		t.Errorf("expected ok=true, got %#v", body)
	}
}
