package handler

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// PreferencesHandler serves the public guest preferences page and sync APIs.
// Preference values are stored only in the browser's allowlisted cookies.
type PreferencesHandler struct{}

// NewPreferencesHandler creates a preferences handler bound to the user database.
func NewPreferencesHandler(_ *sql.DB) *PreferencesHandler {
	return &PreferencesHandler{}
}

// ShowPreferences renders the preferences page with content negotiation.
// Route: GET /server/preferences
func (h *PreferencesHandler) ShowPreferences(w http.ResponseWriter, r *http.Request) {
	theme := server.ResolveTheme(r)
	_, authenticated := middleware.GetCurrentUser(r)
	state, shareURL, shareCode := exportPreferencePayload(r)
	consent := guestConsent(r)
	cfg := config.GetGlobalConfig()
	ccpaEnabled := cfg != nil && cfg.Server.Privacy.Data.Sold
	ccpaOptedOut := false
	if cookie, err := r.Cookie("ccpa_opt_out"); err == nil && cookie.Value == "true" {
		ccpaOptedOut = true
	}

	data := map[string]interface{}{
		"page":               "preferences",
		"theme":              theme,
		"next_theme":         server.NextTheme(theme),
		"themes":             []string{"dark", "light", "auto"},
		"authenticated":      authenticated,
		"preferences_url":    shareURL,
		"preferences_code":   shareCode,
		"preference_state":   state,
		"weather_units":      preferenceWeatherUnits(r),
		"cookie_preferences": consent,
		"ccpa_enabled":       ccpaEnabled,
		"ccpa_opted_out":     ccpaOptedOut,
		"gpc_received":       middleware.GPCOptOut(r),
	}

	NegotiateResponse(w, r, "page/preferences.tmpl", data)
}

// GuestConsent is the portable page's per-browser consent state.
type GuestConsent struct {
	Essential   bool  `json:"essential"`
	Preferences bool  `json:"preferences"`
	Analytics   bool  `json:"analytics"`
	Timestamp   int64 `json:"timestamp"`
}

func guestConsent(r *http.Request) GuestConsent {
	state := GuestConsent{Essential: true}
	if middleware.GPCOptOut(r) {
		return state
	}
	cookie, err := r.Cookie("cookie_consent")
	if err != nil {
		return state
	}
	value, err := url.QueryUnescape(cookie.Value)
	if err != nil || json.Unmarshal([]byte(value), &state) != nil {
		return GuestConsent{Essential: true}
	}
	state.Essential = true
	return state
}

// SaveConsent stores granular cookie consent for this browser only.
func (h *PreferencesHandler) SaveConsent(w http.ResponseWriter, r *http.Request) {
	choice := r.FormValue("choice")
	state := GuestConsent{Essential: true, Timestamp: time.Now().Unix()}
	if !middleware.GPCOptOut(r) {
		switch choice {
		case "accept":
			state.Preferences, state.Analytics = true, true
		case "decline":
		case "save":
			state.Preferences, state.Analytics = r.FormValue("preferences") == "on", r.FormValue("analytics") == "on"
		default:
			state.Preferences, state.Analytics = r.FormValue("preferences") == "on", r.FormValue("analytics") == "on"
		}
	}
	data, _ := json.Marshal(state)
	http.SetCookie(w, &http.Cookie{Name: "cookie_consent", Value: url.QueryEscape(string(data)), Path: "/", MaxAge: 31536000, Secure: util.TrustedIsHTTPS(r), SameSite: http.SameSiteLaxMode})
	writePreferencesRedirect(w, r)
}

// SaveCCPA records or clears the browser's CCPA opt-out and downgrades consent
// to essential-only when the visitor opts out.
func (h *PreferencesHandler) SaveCCPA(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	gpcOptOut := middleware.GPCOptOut(r)
	if cfg == nil || !cfg.Server.Privacy.Data.Sold && !gpcOptOut {
		http.NotFound(w, r)
		return
	}
	choice := r.FormValue("choice")
	if gpcOptOut || choice == "opt-out" {
		if cfg.Server.Privacy.Data.Sold {
			http.SetCookie(w, &http.Cookie{Name: "ccpa_opt_out", Value: "true", Path: "/", MaxAge: 31536000, Secure: util.TrustedIsHTTPS(r), SameSite: http.SameSiteLaxMode})
		}
		data, _ := json.Marshal(GuestConsent{Essential: true, Timestamp: time.Now().Unix()})
		http.SetCookie(w, &http.Cookie{Name: "cookie_consent", Value: url.QueryEscape(string(data)), Path: "/", MaxAge: 31536000, Secure: util.TrustedIsHTTPS(r), SameSite: http.SameSiteLaxMode})
	} else if choice == "opt-in" {
		http.SetCookie(w, &http.Cookie{Name: "ccpa_opt_out", Value: "", Path: "/", MaxAge: -1, Secure: util.TrustedIsHTTPS(r), SameSite: http.SameSiteLaxMode})
	}
	writePreferencesRedirect(w, r)
}

// SavePreferences applies a preference change and redirects back to the page
// the request came from, so the toggle works with JavaScript disabled.
// Route: POST /server/preferences
func (h *PreferencesHandler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	values := requestPreferenceValues(r)
	result, applied, ignored := applyGuestPreferences(w, r, values)
	w.Header().Set("Cache-Control", "no-store")
	if isPreferencesAPIRequest(r) {
		if len(applied) == 0 && !(middleware.GPCOptOut(r) && len(values) > 0) {
			RespondError(w, r, http.StatusBadRequest, ErrValidationFailed, "No supplied preference value was valid")
			return
		}
		writePreferenceData(w, preferenceResponse(result, applied, ignored))
		return
	}
	writePreferencesRedirect(w, r)
}

func writePreferencesRedirect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", preferencesRedirectTarget(r))
	w.WriteHeader(http.StatusSeeOther)
}

func writePreferenceData(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": data})
}

// PreferenceSyncData is the exact response data shape used by the preference
// import and partial-save endpoints.
type PreferenceSyncData struct {
	Theme   string            `json:"theme"`
	Lang    string            `json:"lang"`
	Prefs   map[string]string `json:"prefs"`
	Applied []string          `json:"applied"`
	Ignored []string          `json:"ignored"`
}

var preferenceKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var preferenceValuePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func isPreferencesAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

func requestPreferenceValues(r *http.Request) url.Values {
	values := make(url.Values)
	if r.Method == http.MethodPost && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body map[string]interface{}
		data, readErr := io.ReadAll(io.LimitReader(r.Body, 8193))
		if readErr == nil && len(data) <= 8192 && json.Unmarshal(data, &body) == nil {
			for k, v := range body {
				if s, ok := v.(string); ok {
					values.Set(k, s)
				} else {
					values.Set(k, "\x00")
				}
			}
		}
	} else {
		_ = r.ParseForm()
		for k, v := range r.Form {
			if len(v) > 0 {
				values.Set(k, v[0])
			}
		}
	}
	if len(values) > 32 {
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys[32:] {
			delete(values, k)
		}
	}
	return values
}

func applyGuestPreferences(w http.ResponseWriter, r *http.Request, values url.Values) (PreferenceSyncData, []string, []string) {
	state := currentGuestPreferences(r)
	applied, ignored := []string{}, []string{}
	keys := make([]string, 0, len(values))
	for k := range values {
		if k != "code" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if middleware.GPCOptOut(r) {
		return state, applied, keys
	}
	for _, name := range keys {
		if name == "redirect" {
			ignored = append(ignored, name)
			continue
		}
		value := values.Get(name)
		if len(value) > 64 {
			ignored = append(ignored, name)
			continue
		}
		if name == "theme" {
			if !server.IsValidTheme(value) {
				ignored = append(ignored, name)
				continue
			}
			setGuestPreferenceCookie(w, r, name, value)
			state.Theme = value
			applied = append(applied, name)
			continue
		}
		if name == "lang" {
			lang, ok := canonicalPreferenceLanguage(value)
			if !ok {
				ignored = append(ignored, name)
				continue
			}
			setGuestPreferenceCookie(w, r, name, lang)
			state.Lang = lang
			applied = append(applied, name)
			continue
		}
		const prefix = "wthr_pref_"
		key := strings.TrimPrefix(name, prefix)
		validValue := preferenceValuePattern.MatchString(value)
		if key == "units" {
			validValue = value == "metric" || value == "imperial"
		}
		if !strings.HasPrefix(name, prefix) || !preferenceKeyPattern.MatchString(key) || !validValue {
			ignored = append(ignored, name)
			continue
		}
		setGuestPreferenceCookie(w, r, name, value)
		state.Prefs[key] = value
		applied = append(applied, name)
	}
	return state, applied, ignored
}

func currentGuestPreferences(r *http.Request) PreferenceSyncData {
	state := PreferenceSyncData{Theme: server.ResolveTheme(r), Lang: "en", Prefs: map[string]string{}}
	if lang, ok := reqctx.GetValue(r.Context(), "lang"); ok {
		if value, isString := lang.(string); isString {
			if canonical, valid := canonicalPreferenceLanguage(value); valid {
				state.Lang = canonical
			}
		}
	}
	if c, err := r.Cookie("lang"); err == nil {
		if lang, ok := canonicalPreferenceLanguage(c.Value); ok {
			state.Lang = lang
		}
	}
	for _, c := range r.Cookies() {
		key := strings.TrimPrefix(c.Name, "wthr_pref_")
		valid := strings.HasPrefix(c.Name, "wthr_pref_") && preferenceKeyPattern.MatchString(key) && preferenceValuePattern.MatchString(c.Value)
		if key == "units" {
			valid = strings.HasPrefix(c.Name, "wthr_pref_") && (c.Value == "metric" || c.Value == "imperial")
		}
		if valid {
			state.Prefs[key] = c.Value
		}
	}
	return state
}

func preferenceWeatherUnits(r *http.Request) string {
	if units := r.URL.Query().Get("units"); units == "metric" || units == "imperial" {
		return units
	}
	if c, err := r.Cookie("wthr_pref_units"); err == nil && (c.Value == "metric" || c.Value == "imperial") {
		return c.Value
	}
	return "imperial"
}

func canonicalPreferenceLanguage(value string) (string, bool) {
	langs := []string{"en"}
	if service := i18n.GetGlobalI18n(); service != nil {
		langs = service.GetSupportedLanguages()
	}
	value = strings.TrimSpace(value)
	for _, lang := range langs {
		if strings.EqualFold(lang, value) {
			return lang, true
		}
	}
	if base, _, ok := strings.Cut(value, "-"); ok {
		for _, lang := range langs {
			if strings.EqualFold(lang, base) {
				return lang, true
			}
		}
	}
	return "", false
}

func setGuestPreferenceCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: 31536000, Secure: util.TrustedIsHTTPS(r), HttpOnly: false, SameSite: http.SameSiteLaxMode})
}

func preferenceResponse(state PreferenceSyncData, applied, ignored []string) PreferenceSyncData {
	state.Applied = applied
	state.Ignored = ignored
	if state.Applied == nil {
		state.Applied = []string{}
	}
	if state.Ignored == nil {
		state.Ignored = []string{}
	}
	return state
}

func exportPreferencePayload(r *http.Request) (PreferenceSyncData, string, string) {
	state := currentGuestPreferences(r)
	values := url.Values{"theme": {state.Theme}, "lang": {state.Lang}}
	for key, value := range state.Prefs {
		values.Set("wthr_pref_"+key, value)
	}
	query := values.Encode()
	origin := util.BuildURL(r, "")
	fullURL := strings.TrimRight(origin, "/") + "/server/preferences/import?" + query
	code := base64.RawURLEncoding.EncodeToString([]byte(query))
	return state, fullURL, code
}

// ExportPreferences returns the canonical preference URL and portable code.
func (h *PreferencesHandler) ExportPreferences(w http.ResponseWriter, r *http.Request) {
	state, fullURL, code := exportPreferencePayload(r)
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case isPreferencesAPIRequest(r) || strings.Contains(r.Header.Get("Accept"), "application/json"):
		writePreferenceData(w, map[string]interface{}{"theme": state.Theme, "lang": state.Lang, "prefs": state.Prefs, "url": fullURL, "code": code})
	case strings.Contains(r.Header.Get("Accept"), "text/plain"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "URL: %s\nCode: %s\n", fullURL, code)
	default:
		w.Header().Set("Cache-Control", "no-store")
		NegotiateResponse(w, r, "page/preferences.tmpl", map[string]interface{}{"page": "preferences", "theme": state.Theme, "preferences_url": fullURL, "preferences_code": code})
	}
}

// ExportCurrentPreferences returns the effective preferences as a JSON body.
func (h *PreferencesHandler) ExportCurrentPreferences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	state := currentGuestPreferences(r)
	writePreferenceData(w, map[string]interface{}{"theme": state.Theme, "lang": state.Lang, "prefs": state.Prefs})
}

// ImportPreferences applies a share URL or code using the same validator as save.
func (h *PreferencesHandler) ImportPreferences(w http.ResponseWriter, r *http.Request) {
	direct := cloneValues(r.URL.Query())
	values := cloneValues(direct)
	if code := strings.TrimSpace(direct.Get("code")); len(code) <= 2048 && code != "" {
		if i := strings.IndexByte(code, '?'); i >= 0 {
			code = code[i+1:]
		}
		decoded := decodePreferenceCode(code)
		if parsed, err := url.ParseQuery(decoded); err == nil {
			for k, v := range parsed {
				if _, exists := direct[k]; !exists {
					values[k] = v
				}
			}
		}
	}
	if len(direct) > 32 {
		direct = limitPreferenceValues(direct, 32)
	}
	if len(values) > 32 {
		codeKeys := make([]string, 0, len(values))
		for k := range values {
			if _, fromDirect := direct[k]; !fromDirect {
				codeKeys = append(codeKeys, k)
			}
		}
		sort.Strings(codeKeys)
		for len(values) > 32 && len(codeKeys) > 0 {
			delete(values, codeKeys[len(codeKeys)-1])
			codeKeys = codeKeys[:len(codeKeys)-1]
		}
	}
	state, applied, ignored := applyGuestPreferences(w, r, values)
	if isPreferencesAPIRequest(r) {
		writePreferenceData(w, preferenceResponse(state, applied, ignored))
		return
	}
	writePreferencesRedirect(w, r)
}

func decodePreferenceCode(code string) string {
	if strings.HasSuffix(code, "=") {
		if decoded, err := base64.URLEncoding.DecodeString(code); err == nil {
			if parsed, parseErr := url.ParseQuery(string(decoded)); parseErr == nil && len(parsed) > 0 {
				return string(decoded)
			}
		}
	}
	if strings.Contains(code, "=") {
		return code
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(code); err == nil {
		return string(decoded)
	}
	return ""
}

func cloneValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for k, v := range values {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func limitPreferenceValues(values url.Values, limit int) url.Values {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys[limit:] {
		delete(values, k)
	}
	return values
}

// preferencesRedirectTarget resolves where to send the browser after a
// preference change: an explicit redirect field, else the referring page,
// else the preferences page itself. Only same-site, path-rooted targets are
// accepted - protocol-relative ("//evil.com") and backslash ("/\evil.com")
// forms are rejected because browsers read them as scheme-qualified URLs.
func preferencesRedirectTarget(r *http.Request) string {
	if target := validPreferenceRedirect(r.FormValue("redirect")); target != "" {
		return target
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		if parsed, err := url.Parse(ref); err == nil {
			requestScheme := "http"
			if util.TrustedIsHTTPS(r) {
				requestScheme = "https"
			}
			if parsed.Host == r.Host && parsed.Scheme == requestScheme {
				candidate := parsed.EscapedPath()
				if parsed.RawQuery != "" {
					candidate += "?" + parsed.RawQuery
				}
				if target := validPreferenceRedirect(candidate); target != "" {
					return target
				}
			}
		}
	}
	return "/"
}

func validPreferenceRedirect(candidate string) string {
	if strings.ContainsAny(candidate, "\r\n") {
		return ""
	}
	target := util.SafeRedirectPath(candidate)
	if target == "" {
		return ""
	}
	for _, blocked := range []string{"/server/preferences/import", "/server/preferences/export", "/server/preferences"} {
		if strings.HasPrefix(target, blocked) {
			return ""
		}
	}
	return target
}
