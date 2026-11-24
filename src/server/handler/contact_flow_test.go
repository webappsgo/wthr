package handler

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

func TestContactReportedURLRejectsUnvalidatedDynamicRoutes(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/server/auth/invite/{code}", func(http.ResponseWriter, *http.Request) {})
	SetContactRouteMatcher(router)
	t.Cleanup(func() { SetContactRouteMatcher(nil) })

	r := httptest.NewRequest("GET", "https://weather.example/", nil)
	r.Host = "weather.example"
	if contactReportedURLExists("https://weather.example/server/auth/invite/guess", r, nil) {
		t.Fatal("accepted dynamic route without validating its resource")
	}
}

func TestContactPageHTMLHasExactFieldsAndRequiredInputs(t *testing.T) {
	templates, err := server.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	previousRenderer := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
		var body bytes.Buffer
		if err := templates.ExecuteTemplate(&body, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write(body.Bytes())
	}
	t.Cleanup(func() { middleware.RenderHTML = previousRenderer })

	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	cfg.Server.FQDN = "weather.example"

	tests := []struct {
		typeName string
		fields   []string
		required []string
	}{
		{typeName: "", fields: nil},
		{typeName: "abuse", fields: []string{"csrf_token", "type", "form_token", "step", "website", "name", "email", "reported_url", "reason", "subject", "message", "captcha_id", "captcha_answer"}, required: []string{"name", "email", "reported_url", "reason", "subject", "message", "captcha_answer"}},
		{typeName: "dmca", fields: []string{"csrf_token", "type", "form_token", "step", "website", "name", "email", "reported_url", "copyrighted_work", "good_faith", "accuracy", "signature", "subject", "message", "captcha_id", "captcha_answer"}, required: []string{"name", "email", "reported_url", "copyrighted_work", "good_faith", "accuracy", "signature", "subject", "message", "captcha_answer"}},
		{typeName: "privacy", fields: []string{"csrf_token", "type", "form_token", "step", "website", "name", "email", "request_kind", "subject", "message", "captcha_id", "captcha_answer"}, required: []string{"name", "email", "request_kind", "subject", "message", "captcha_answer"}},
	}
	namePattern := regexp.MustCompile(`\bname="([^"]+)"`)
	for _, tc := range tests {
		t.Run(tc.typeName, func(t *testing.T) {
			target := "/server/contact"
			if tc.typeName != "" {
				target += "?type=" + tc.typeName
			}
			r := httptest.NewRequest(http.MethodGet, target, nil)
			r.Header.Set("Accept", "text/html")
			r.Header.Set("User-Agent", "Mozilla/5.0")
			r.Host = "weather.example"
			w := httptest.NewRecorder()
			ShowContactPage(db, cfg)(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			main := contactMainElement(t, w.Body.String())
			assertContactHTMLFields(t, main, namePattern, tc.fields, tc.required)
			if tc.typeName == "" {
				if strings.Contains(main, "<input") || strings.Contains(main, "<textarea") || strings.Contains(main, "<select") {
					t.Error("chooser main contains form inputs")
				}
				if !strings.Contains(main, `href="/server/contact?type=abuse"`) || !strings.Contains(main, `href="/server/contact?type=dmca"`) || !strings.Contains(main, `href="/server/contact?type=privacy"`) {
					t.Error("chooser is missing a report-type link")
				}
				return
			}
		})
	}

	t.Run("security", func(t *testing.T) {
		cfg.Server.Security.EncryptionKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
		securityID, err := service.CurrentSecurityID(db.DB)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/server/contact?security_id="+securityID, nil)
		r.Header.Set("Accept", "text/html")
		r.Header.Set("User-Agent", "Mozilla/5.0")
		r.Host = "weather.example"
		w := httptest.NewRecorder()
		ShowContactPage(db, cfg)(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		main := contactMainElement(t, w.Body.String())
		assertContactHTMLFields(t, main, namePattern,
			[]string{"csrf_token", "security_id", "step", "form_token", "website", "name", "email", "researcher_gpg", "affected_component", "affected_endpoint", "severity", "summary", "steps", "impact", "suggested_fix", "cve_requested", "disclosure_days", "credit_preference", "credit_handle", "disclosure_agreement", "captcha_id", "captcha_answer"},
			[]string{"name", "email", "affected_component", "severity", "summary", "steps", "impact", "credit_preference", "disclosure_agreement", "captcha_answer"})
		if strings.Contains(main, `name="type"`) || strings.Contains(main, `name="subject"`) || strings.Contains(main, `name="message"`) {
			t.Error("security form contains standard-contact fields")
		}
	})
}

func contactMainElement(t *testing.T, page string) string {
	t.Helper()
	mainStart, mainEnd := strings.Index(page, "<main"), strings.Index(page, "</main>")
	if mainStart < 0 || mainEnd < mainStart {
		t.Fatalf("rendered page has no main element: %s", page)
	}
	return page[mainStart:mainEnd]
}

func assertContactHTMLFields(t *testing.T, main string, namePattern *regexp.Regexp, fields, required []string) {
	t.Helper()
	counts := map[string]int{}
	for _, match := range namePattern.FindAllStringSubmatch(main, -1) {
		counts[match[1]]++
	}
	got := make([]string, 0, len(counts))
	for field, count := range counts {
		if count != 1 {
			t.Errorf("field %q rendered %d times", field, count)
		}
		got = append(got, field)
	}
	want := append([]string(nil), fields...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("field names = %v, want %v", got, want)
	}
	for _, field := range required {
		pattern := regexp.MustCompile(`(?is)<(?:input|textarea|select)\b[^>]*\bname="` + regexp.QuoteMeta(field) + `"[^>]*>`)
		element := pattern.FindString(main)
		if element == "" || !regexp.MustCompile(`\brequired(?:\s|>|=)`).MatchString(element) {
			t.Errorf("required field %q lacks a required form control", field)
		}
	}
}

func TestHandleContactFormWebRendersReviewWithoutDelivery(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	cfg.Server.Pages.Contact.Antispam.ReviewMinSeconds = 0
	manager, err := getContactAntispam(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	formToken, err := manager.NewContactFormToken(time.Now().Add(-5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	captchaID, a, b := contactCaptcha.New()
	values := url.Values{
		"type": {"privacy"}, "request_kind": {"access"}, "name": {"Ada"}, "email": {"ada@example.com"},
		"subject": {"Privacy request"}, "message": {"Please provide my data"}, "form_token": {formToken},
		"captcha_id": {captchaID}, "captcha_answer": {strconv.Itoa(a + b)},
	}
	raw := map[string]string{}
	for key, list := range values {
		raw[key] = list[0]
	}
	probe := httptest.NewRequest(http.MethodPost, "http://localhost/server/contact", nil)
	setContactTestOrigin(probe)
	if flow := processContactFlow(probe, db, cfg, raw, true); flow.Kind != "review" {
		t.Fatalf("processContactFlow() = %#v, want review", flow)
	}
	formToken, err = manager.NewContactFormToken(time.Now().Add(-5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	captchaID, a, b = contactCaptcha.New()
	values.Set("form_token", formToken)
	values.Set("captcha_id", captchaID)
	values.Set("captcha_answer", strconv.Itoa(a+b))
	r := adminFormRequest("http://localhost/server/contact", values)
	setContactTestOrigin(r)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()
	var rendered map[string]interface{}
	previousRenderer := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, _ string, data map[string]interface{}) {
		rendered = data
		w.WriteHeader(status)
	}
	t.Cleanup(func() { middleware.RenderHTML = previousRenderer })
	HandleContactFormWeb(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if rendered["Mode"] != "review" || rendered["ReviewToken"] == "" {
		t.Fatalf("submission did not render the review step: %#v", rendered)
	}
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM contact_submissions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("step one delivered a contact message: %d rows", count)
	}
}

func TestHandleSecurityContactFormWebRendersReview(t *testing.T) {
	cfg, securityID := securityTestEnv(t)
	cfg.Server.Pages.Contact.Antispam.ReviewMinSeconds = 0
	manager, err := getContactAntispam(nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	formToken, err := manager.NewContactFormToken(time.Now().Add(-5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	captchaID, a, b := contactCaptcha.New()
	values := url.Values{"security_id": {securityID}, "form_token": {formToken}, "captcha_id": {captchaID}, "captcha_answer": {strconv.Itoa(a + b)}}
	for key, value := range validSecurityValues() {
		values.Set(key, value)
	}
	raw := map[string]string{}
	for key, list := range values {
		raw[key] = list[0]
	}
	probe := httptest.NewRequest(http.MethodPost, "http://localhost/server/contact", nil)
	setContactTestOrigin(probe)
	if flow := processSecurityContactFlow(probe, nil, cfg, securityID, raw, true); flow.Kind != "review" {
		t.Fatalf("processSecurityContactFlow() = %#v, want review", flow)
	}
	formToken, err = manager.NewContactFormToken(time.Now().Add(-5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	captchaID, a, b = contactCaptcha.New()
	values.Set("form_token", formToken)
	values.Set("captcha_id", captchaID)
	values.Set("captcha_answer", strconv.Itoa(a+b))
	r := adminFormRequest("http://localhost/server/contact", values)
	setContactTestOrigin(r)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()
	var rendered map[string]interface{}
	previousRenderer := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, _ string, data map[string]interface{}) {
		rendered = data
		w.WriteHeader(status)
	}
	t.Cleanup(func() { middleware.RenderHTML = previousRenderer })
	HandleContactFormWeb(nil, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if rendered["Mode"] != "review" || rendered["Type"] != "security" || rendered["ReviewToken"] == "" {
		t.Fatalf("security submission did not render the review step: %#v", rendered)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("security response headers = %#v", w.Header())
	}
}

func setContactTestOrigin(r *http.Request) {
	page, err := url.Parse(util.BuildURL(r, "/"))
	if err != nil {
		panic(err)
	}
	r.Header.Set("Origin", page.Scheme+"://"+page.Host)
}
