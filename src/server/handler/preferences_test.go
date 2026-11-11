package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
)

// setPreferencesTestRenderer installs a stub RenderHTML so the HTML branch of
// NegotiateResponse is exercised instead of panicking on a nil func value.
func setPreferencesTestRenderer(t *testing.T) {
	t.Helper()
	orig := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(name))
	}
	t.Cleanup(func() { middleware.RenderHTML = orig })
}

// newPreferencesBrowserRequest builds a request that content negotiation
// classifies as a browser, so the HTML branch is taken.
func newPreferencesBrowserRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req
}

func TestShowPreferencesRendersPage(t *testing.T) {
	setPreferencesTestRenderer(t)

	h := NewPreferencesHandler(nil)
	req := newPreferencesBrowserRequest(http.MethodGet, "/server/preferences", "")
	rec := httptest.NewRecorder()

	h.ShowPreferences(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ShowPreferences() status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "page/preferences.tmpl" {
		t.Fatalf("ShowPreferences() rendered %q, want %q", got, "page/preferences.tmpl")
	}
}

func TestShowPreferencesMarksGPCSignal(t *testing.T) {
	oldRenderer := middleware.RenderHTML
	var rendered map[string]interface{}
	middleware.RenderHTML = func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
		rendered = data
		w.WriteHeader(status)
	}
	t.Cleanup(func() { middleware.RenderHTML = oldRenderer })

	req := newPreferencesBrowserRequest(http.MethodGet, "/server/preferences", "")
	req.Header.Set("Sec-GPC", "1")
	NewPreferencesHandler(nil).ShowPreferences(httptest.NewRecorder(), req)
	if received, _ := rendered["gpc_received"].(bool); !received {
		t.Fatal("preferences page did not expose the received GPC signal")
	}
}

func TestSaveCCPAHonorsGPCAndExplicitChoices(t *testing.T) {
	oldConfig := config.GetGlobalConfig()
	cfg := &config.AppConfig{Server: config.ServerConfig{Privacy: config.PrivacyConfig{Data: config.PrivacyDataConfig{Sold: true}}}}
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(oldConfig) })
	handler := NewPreferencesHandler(nil)

	t.Run("GPC cannot be overridden by opt-in", func(t *testing.T) {
		req := newPreferencesBrowserRequest(http.MethodPost, "/server/ccpa", "choice=opt-in")
		req.Header.Set("Sec-GPC", "1")
		rec := httptest.NewRecorder()
		handler.SaveCCPA(rec, req)
		if got := responseCookieValue(rec, "ccpa_opt_out"); got != "true" {
			t.Fatalf("GPC opt-out cookie = %q, want true", got)
		}
	})

	t.Run("explicit opt-in clears opt-out", func(t *testing.T) {
		req := newPreferencesBrowserRequest(http.MethodPost, "/server/ccpa", "choice=opt-in")
		rec := httptest.NewRecorder()
		handler.SaveCCPA(rec, req)
		for _, cookie := range rec.Result().Cookies() {
			if cookie.Name == "ccpa_opt_out" && cookie.MaxAge != -1 {
				t.Fatalf("opt-in cookie = %#v, want deletion", cookie)
			}
		}
	})

	t.Run("unknown choice preserves opt-out", func(t *testing.T) {
		req := newPreferencesBrowserRequest(http.MethodPost, "/server/ccpa", "choice=unknown")
		rec := httptest.NewRecorder()
		handler.SaveCCPA(rec, req)
		if len(rec.Result().Cookies()) != 0 {
			t.Fatalf("unknown choice changed cookies: %#v", rec.Result().Cookies())
		}
	})
}

func responseCookieValue(rec *httptest.ResponseRecorder, name string) string {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func TestSavePreferencesSetsCookieAndRedirects(t *testing.T) {
	h := NewPreferencesHandler(nil)
	req := newPreferencesBrowserRequest(http.MethodPost, "/server/preferences", "theme=light")
	req.Header.Set("Referer", "http://example.test/moon")
	req.Host = "example.test"
	rec := httptest.NewRecorder()

	h.SavePreferences(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("SavePreferences() status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/moon" {
		t.Fatalf("SavePreferences() Location = %q, want %q", loc, "/moon")
	}

	var themeCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "theme" {
			themeCookie = c
		}
	}
	if themeCookie == nil {
		t.Fatal("SavePreferences() did not set the theme cookie")
	}
	if themeCookie.Value != "light" {
		t.Fatalf("theme cookie = %q, want %q", themeCookie.Value, "light")
	}
}

func TestSavePreferencesDropsInvalidThemeAndRedirects(t *testing.T) {
	h := NewPreferencesHandler(nil)
	req := newPreferencesBrowserRequest(http.MethodPost, "/server/preferences", "theme=sepia")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	h.SavePreferences(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("SavePreferences() status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "theme" {
			t.Fatalf("invalid theme must not set the theme cookie (got %q)", c.Value)
		}
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("invalid theme redirect = %q, want /", loc)
	}
}

func TestPreferenceExportImportRoundTrip(t *testing.T) {
	h := NewPreferencesHandler(nil)
	exportReq := httptest.NewRequest(http.MethodGet, "/server/preferences/export", nil)
	exportReq.Header.Set("Accept", "application/json")
	exportReq.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	exportReq.AddCookie(&http.Cookie{Name: "lang", Value: "fr"})
	exportReq.AddCookie(&http.Cookie{Name: "wthr_pref_sort", Value: "name"})
	exportReq.AddCookie(&http.Cookie{Name: "cookie_consent", Value: "accepted"})
	exportRec := httptest.NewRecorder()
	h.ExportPreferences(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("ExportPreferences() status = %d", exportRec.Code)
	}

	var payload struct {
		Data struct {
			URL  string `json:"url"`
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(exportRec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload.Data.URL, "cookie_consent") {
		t.Fatal("export included excluded cookie")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload.Data.Code)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "lang=fr&theme=light&wthr_pref_sort=name" {
		t.Fatalf("canonical code payload = %q", decoded)
	}

	query := url.Values{"code": {payload.Data.Code}, "theme": {"dark"}}
	importReq := httptest.NewRequest(http.MethodGet, "/server/preferences/import?"+query.Encode(), nil)
	importRec := httptest.NewRecorder()
	h.ImportPreferences(importRec, importReq)
	if importRec.Code != http.StatusSeeOther {
		t.Fatalf("ImportPreferences() status = %d", importRec.Code)
	}
	if got := importRec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if importRec.Body.Len() != 0 {
		t.Fatalf("web import response body = %q, want empty", importRec.Body.String())
	}
	cookies := map[string]string{}
	for _, cookie := range importRec.Result().Cookies() {
		cookies[cookie.Name] = cookie.Value
	}
	if cookies["theme"] != "dark" || cookies["lang"] != "fr" || cookies["wthr_pref_sort"] != "name" {
		t.Fatalf("import cookies = %#v", cookies)
	}
	for _, cookie := range importRec.Result().Cookies() {
		if cookie.Name == "theme" {
			if cookie.Path != "/" || cookie.MaxAge != 31536000 || cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("theme cookie attributes = %#v", cookie)
			}
		}
	}
	if _, exists := cookies["cookie_consent"]; exists {
		t.Fatal("import set excluded cookie")
	}
}

func TestPreferenceCodeFormsAndWeatherUnits(t *testing.T) {
	query := "theme=dark"
	padded := base64.URLEncoding.EncodeToString([]byte(query))
	if got := decodePreferenceCode(padded); got != query {
		t.Fatalf("padded code decoded as %q", got)
	}
	if got := decodePreferenceCode(query); got != query {
		t.Fatalf("plain query decoded as %q", got)
	}
	r := httptest.NewRequest(http.MethodGet, "/?units=metric", nil)
	r.AddCookie(&http.Cookie{Name: "wthr_pref_units", Value: "imperial"})
	if got := preferenceWeatherUnits(r); got != "metric" {
		t.Fatalf("query units = %q, want metric", got)
	}
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "wthr_pref_units", Value: "metric"})
	if got := preferenceWeatherUnits(r); got != "metric" {
		t.Fatalf("cookie units = %q, want metric", got)
	}
}

func TestPreferenceImportAcceptsSupportedCodeForms(t *testing.T) {
	h := NewPreferencesHandler(nil)
	query := "theme=light&lang=fr"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(query))
	fullURL := "https://example.test/server/preferences/import?" + query
	tests := []struct {
		name string
		code string
	}{
		{"plain query", query},
		{"unpadded base64url", encoded},
		{"padded base64url", base64.URLEncoding.EncodeToString([]byte(query))},
		{"pasted full URL", fullURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := url.Values{"code": {tt.code}}
			req := httptest.NewRequest(http.MethodGet, "/server/preferences/import?"+values.Encode(), nil)
			rec := httptest.NewRecorder()
			h.ImportPreferences(rec, req)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", rec.Code)
			}
			got := map[string]string{}
			for _, cookie := range rec.Result().Cookies() {
				got[cookie.Name] = cookie.Value
			}
			if got["theme"] != "light" || got["lang"] != "fr" {
				t.Fatalf("imported cookies = %#v", got)
			}
		})
	}
}

func TestPreferenceImportFiltersAndKeepsExistingCookies(t *testing.T) {
	h := NewPreferencesHandler(nil)
	query := url.Values{
		"theme": {"sepia"}, "lang": {"xx"}, "wthr_pref_units": {"kelvin"},
		"cookie_consent": {"accepted"}, "ccpa_opt_out": {"true"}, "wthr_build": {"build"}, "session": {"secret"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/preferences/import?"+query.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: "wthr_pref_units", Value: "metric"})
	rec := httptest.NewRecorder()
	h.ImportPreferences(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("API import status = %d", rec.Code)
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Theme   string            `json:"theme"`
			Prefs   map[string]string `json:"prefs"`
			Applied []string          `json:"applied"`
			Ignored []string          `json:"ignored"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Data.Theme != "dark" || body.Data.Prefs["units"] != "metric" || len(body.Data.Applied) != 0 {
		t.Fatalf("import response = %#v", body)
	}
	for _, name := range []string{"theme", "lang", "wthr_pref_units", "cookie_consent", "ccpa_opt_out", "wthr_build", "session"} {
		found := false
		for _, ignored := range body.Data.Ignored {
			if ignored == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ignored does not contain %q: %#v", name, body.Data.Ignored)
		}
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "wthr_pref_units" && cookie.Value != "kelvin" {
			t.Fatalf("invalid pref overwrote existing value: %q", cookie.Value)
		}
	}
}

func TestPreferencesRedirectTarget(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		referer string
		want    string
	}{
		{"explicit redirect field wins", "theme=dark&redirect=/earthquake", "http://example.test/moon", "/earthquake"},
		{"referer path is used when no field", "theme=dark", "http://example.test/moon?zoom=3", "/moon?zoom=3"},
		{"no referer falls back to root", "theme=dark", "", "/"},
		{"cross-host referer is rejected", "theme=dark", "http://evil.example/moon", "/"},
		{"protocol-relative redirect field is rejected", "theme=dark&redirect=//evil.example", "", "/"},
		{"backslash redirect field is rejected", "theme=dark&redirect=/\\evil.example", "", "/"},
		{"non-rooted redirect field is rejected", "theme=dark&redirect=evil.example", "", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newPreferencesBrowserRequest(http.MethodPost, "/server/preferences", tt.body)
			req.Host = "example.test"
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			if got := preferencesRedirectTarget(req); got != tt.want {
				t.Fatalf("preferencesRedirectTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
