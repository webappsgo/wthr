package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
)

// securityTestEnv wires a server database, a global config with an encryption key and returns
// the current security_id.
func securityTestEnv(t *testing.T) (*config.AppConfig, string) {
	t.Helper()
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(nil) })
	id, err := service.CurrentSecurityID(serverDB)
	if err != nil {
		t.Fatalf("CurrentSecurityID: %v", err)
	}
	return cfg, id
}

func validSecurityValues() map[string]string {
	return map[string]string{
		"name": "Ada", "email": "ada@example.com", "affected_component": "api", "severity": "high",
		"summary": "reflected xss", "steps": "1. open", "impact": "script runs", "credit_preference": "handle",
		"credit_handle": "ada77", "disclosure_agreement": "true", "disclosure_days": "90",
	}
}

func TestInvalidSecurityField(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name   string
		mutate func(map[string]string)
		want   string
	}{
		{"valid", func(map[string]string) {}, ""},
		{"missing summary", func(v map[string]string) { delete(v, "summary") }, "summary"},
		{"summary too long", func(v map[string]string) { v["summary"] = long(201) }, "summary"},
		{"summary multi line", func(v map[string]string) { v["summary"] = "a\nb" }, "summary"},
		{"steps at the limit", func(v map[string]string) { v["steps"] = long(10000) }, ""},
		{"steps over the limit", func(v map[string]string) { v["steps"] = long(10001) }, "steps"},
		{"unknown severity", func(v map[string]string) { v["severity"] = "urgent" }, "severity"},
		{"handle required for handle credit", func(v map[string]string) { delete(v, "credit_handle") }, "credit_handle"},
		{"handle not required otherwise", func(v map[string]string) { v["credit_preference"] = "anonymous"; delete(v, "credit_handle") }, ""},
		{"agreement is required", func(v map[string]string) { delete(v, "disclosure_agreement") }, "disclosure_agreement"},
		{"agreement unchecked", func(v map[string]string) { v["disclosure_agreement"] = "false" }, "disclosure_agreement"},
		{"https key url over 2048", func(v map[string]string) { v["researcher_gpg"] = "https://example.com/" + long(2040) }, "researcher_gpg"},
		{"pasted key may be longer than 2048", func(v map[string]string) { v["researcher_gpg"] = "-----BEGIN" + long(5000) }, ""},
		{"pasted key over 16384", func(v map[string]string) { v["researcher_gpg"] = "-----BEGIN" + long(16400) }, "researcher_gpg"},
		{"days out of range", func(v map[string]string) { v["disclosure_days"] = "0" }, "disclosure_days"},
		{"days not a number", func(v map[string]string) { v["disclosure_days"] = "soon" }, "disclosure_days"},
		{"characters not bytes", func(v map[string]string) { v["summary"] = strings.Repeat("é", 200) }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validSecurityValues()
			tc.mutate(v)
			if got := invalidSecurityField(v); got != tc.want {
				t.Errorf("invalidSecurityField = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSecurityFieldDescriptorsPublishConditionalRules(t *testing.T) {
	raw, err := json.Marshal(securityFieldSpecs())
	if err != nil {
		t.Fatal(err)
	}
	var fields []map[string]interface{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	byName := map[string]map[string]interface{}{}
	for _, f := range fields {
		byName[f["name"].(string)] = f
		for _, key := range []string{"name", "type", "required", "max_length", "required_when", "max_length_when"} {
			if _, ok := f[key]; !ok {
				t.Errorf("%v lacks %q", f["name"], key)
			}
		}
	}
	rw := byName["credit_handle"]["required_when"].(map[string]interface{})
	if rw["field"] != "credit_preference" || rw["equals"] != "handle" || byName["credit_handle"]["required"] != false {
		t.Errorf("credit_handle required_when = %v", rw)
	}
	mw := byName["researcher_gpg"]["max_length_when"].([]interface{})[0].(map[string]interface{})
	if mw["prefix"] != "https://" || mw["max_length"].(float64) != 2048 || byName["researcher_gpg"]["max_length"].(float64) != 16384 {
		t.Errorf("researcher_gpg descriptors = %v / %v", mw, byName["researcher_gpg"]["max_length"])
	}
	if byName["severity"]["max_length"] != nil || byName["cve_requested"]["max_length"] != nil {
		t.Error("enum and checkbox fields publish max_length null")
	}
}

func TestSecurityIDFromRequest(t *testing.T) {
	_, id := securityTestEnv(t)
	r := httptest.NewRequest(http.MethodGet, "/server/contact", nil)
	if got := securityIDFromRequest(r, id); got != id {
		t.Errorf("valid id = %q", got)
	}
	if got := securityIDFromRequest(r, "0000000000000000"); got != "" {
		t.Errorf("invalid id must fall back to standard mode, got %q", got)
	}
	if got := securityIDFromRequest(r, ""); got != "" {
		t.Errorf("empty id = %q", got)
	}
}

func TestSecurityReportSubmissionStoresEncryptedReport(t *testing.T) {
	cfg, id := securityTestEnv(t)
	cfg.Server.Pages.Contact.Captcha = config.CaptchaSimple
	captchaID, a, b := contactCaptcha.New()

	form := url.Values{"security_id": {id}, "captcha_id": {captchaID}, "captcha_answer": {strconv.Itoa(a + b)}}
	for k, v := range validSecurityValues() {
		form.Set(k, v)
	}
	req := httptest.NewRequest(http.MethodPost, "/server/contact", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	result, tracking, field := deliverSecurityReport(req, cfg, id, securityFormValues(req), contactForm{CaptchaID: captchaID, CaptchaAnswer: strconv.Itoa(a + b)})
	if result != securityReceived || field != "" || !strings.HasPrefix(tracking, "sec_") {
		t.Fatalf("deliverSecurityReport = %q %q %q", result, tracking, field)
	}
	store := newSecurityStore()
	report, contact, err := store.Body(tracking)
	if err != nil || report.Summary != "reflected xss" || contact.Email != "ada@example.com" {
		t.Fatalf("Body = %+v %+v %v", report, contact, err)
	}
	var stored string
	_ = store.DB.QueryRow(`SELECT body_enc FROM security_reports WHERE tracking_id = ?`, tracking).Scan(&stored)
	if strings.Contains(stored, "reflected xss") || strings.Contains(stored, "ada@example.com") {
		t.Error("plaintext must never be persisted")
	}

	// a wrong captcha answer stores nothing
	badID, _, _ := contactCaptcha.New()
	res, _, _ := deliverSecurityReport(req, cfg, id, validSecurityValues(), contactForm{CaptchaID: badID, CaptchaAnswer: "0"})
	if res != contactCaptchaFailed {
		t.Errorf("wrong captcha result = %q", res)
	}
	rows, _ := store.List("")
	if len(rows) != 1 {
		t.Errorf("only the valid report is stored, got %d", len(rows))
	}
	// an invalid field is reported by name
	invalid := validSecurityValues()
	invalid["severity"] = "nope"
	if res, _, f := deliverSecurityReport(req, cfg, id, invalid, contactForm{}); res != securityInvalid || f != "severity" {
		t.Errorf("invalid result = %q %q", res, f)
	}
}

func TestSecurityReportAPISubmission(t *testing.T) {
	cfg, id := securityTestEnv(t)
	captchaID, a, b := contactCaptcha.New()
	body := map[string]interface{}{"security_id": id, "captcha_id": captchaID, "captcha_answer": strconv.Itoa(a + b), "cve_requested": true}
	for k, v := range validSecurityValues() {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	HandleContactFormSubmission(nil, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		OK   bool `json:"ok"`
		Data struct {
			TrackingID string `json:"tracking_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || !out.OK || !strings.HasPrefix(out.Data.TrackingID, "sec_") {
		t.Fatalf("response = %s", w.Body.String())
	}

	// a missing required field is a field-level VALIDATION_FAILED
	bad := map[string]interface{}{"security_id": id, "name": "Ada", "email": "ada@example.com"}
	raw, _ = json.Marshal(bad)
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	HandleContactFormSubmission(nil, cfg)(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "VALIDATION_FAILED") {
		t.Fatalf("invalid response = %d %s", w.Code, w.Body.String())
	}

	// an invalid security_id silently falls back to the standard contact form
	raw, _ = json.Marshal(map[string]interface{}{"security_id": "0000000000000000", "name": "x"})
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	HandleContactFormSubmission(nil, cfg)(w, r)
	if strings.Contains(w.Body.String(), "tracking_id") {
		t.Error("an invalid id must not reach the security pipeline")
	}

	// the API description switches to the security fields only for a valid id
	for query, wantSecurity := range map[string]bool{"?security_id=" + id: true, "?security_id=bad": false, "": false} {
		w = httptest.NewRecorder()
		r = httptest.NewRequest(http.MethodGet, "/api/v1/server/contact"+query, nil)
		r.Header.Set("Accept", "application/json")
		GetContactFormAPI(cfg)(w, r)
		if got := strings.Contains(w.Body.String(), "credit_preference"); got != wantSecurity {
			t.Errorf("fields for %q include security fields = %v, want %v", query, got, wantSecurity)
		}
		if strings.Contains(w.Body.String(), id) {
			t.Error("the contact description never echoes the security id")
		}
	}
}

func TestSecurityReportStatusPage(t *testing.T) {
	cfg, id := securityTestEnv(t)
	store := newSecurityStore()
	tracking, token, err := store.Create(id, service.SecurityReport{Severity: "low", Component: "api", CreditPreference: "no"}, service.ReportContact{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/server/security/report/{tracking_id}", ShowSecurityReportStatus(cfg))

	get := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}
	if w := get("/server/security/report/" + tracking + "?token=wrong"); w.Code != http.StatusNotFound {
		t.Errorf("wrong token = %d", w.Code)
	}
	if w := get("/server/security/report/sec_missing?token=" + token); w.Code != http.StatusNotFound {
		t.Errorf("unknown report = %d", w.Code)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/server/security/report/"+tracking+"?token="+token, nil)
	req.Header.Set("Accept", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tracking) {
		t.Errorf("valid token = %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("one-shot page headers = %v", w.Header())
	}
}

func TestAdminSecurityReportUpdateAndThanks(t *testing.T) {
	_, id := securityTestEnv(t)
	store := newSecurityStore()
	tracking, _, _ := store.Create(id, service.SecurityReport{Severity: "high", Component: "api", CreditPreference: "handle", CreditHandle: "ada77", ResearcherName: "Ada"}, service.ReportContact{})

	router := chi.NewRouter()
	router.Post("/server/administration/security/reports/{tracking_id}", UpdateSecurityReport)
	router.Post("/server/administration/security/thanks", UpdateSecurityThanks)
	router.Post("/server/administration/security/policy", UpdateSecurityPolicy)

	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(w, r)
		return w
	}
	w := post("/server/administration/security/reports/"+tracking, url.Values{"status": {"triaged"}, "researcher_comment": {"confirmed receipt"}, "disclosure_date": {"2026-12-01"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/server/administration/security/reports/"+tracking {
		t.Fatalf("update = %d %s", w.Code, w.Header().Get("Location"))
	}
	row, _ := store.Get(tracking)
	if row.Status != "triaged" || row.ResearcherComment != "confirmed receipt" || row.DisclosureDate != "2026-12-01" {
		t.Fatalf("row = %+v", row)
	}
	post("/server/administration/security/reports/"+tracking, url.Values{"status": {"received"}})
	if row, _ = store.Get(tracking); row.Status != "triaged" {
		t.Error("a backward move must be refused")
	}
	post("/server/administration/security/reports/"+tracking, url.Values{"status": {"disclosed"}})
	entries, _ := store.Thanks(false)
	if len(entries) != 1 || entries[0].Credit != "ada77" {
		t.Fatalf("disclosing adds the hall-of-fame entry: %+v", entries)
	}
	post("/server/administration/security/thanks", url.Values{"id": {strconv.FormatInt(entries[0].ID, 10)}, "credit": {"Ada L."}, "sort_order": {"1"}})
	if hidden, _ := store.Thanks(true); len(hidden) != 0 {
		t.Error("an unchecked visible box hides the entry")
	}
	if publicThanks(); len(publicThanks()) != 0 {
		t.Error("hidden entries stay off the public page")
	}
	post("/server/administration/security/policy", url.Values{"policy": {"Report in good faith."}})
	if securityPolicyText() != "Report in good faith." {
		t.Errorf("policy = %q", securityPolicyText())
	}
	post("/server/administration/security/policy", url.Values{"policy": {strings.Repeat("a", 20001)}})
	if securityPolicyText() != "Report in good faith." {
		t.Error("an oversized policy must be refused")
	}
}

func TestServePGPKeyIs404WithoutKeypair(t *testing.T) {
	securityTestEnv(t)
	w := httptest.NewRecorder()
	ServePGPKey()(w, httptest.NewRequest(http.MethodGet, "/.well-known/pgp-key.asc", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d", w.Code)
	}
}

func TestSecurityOverviewListsContactsInPreferenceOrder(t *testing.T) {
	cfg, id := securityTestEnv(t)
	cfg.Server.Contact.Security.Email = "sec@example.com"
	t.Setenv("DOMAIN", "wthr.example")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/security", nil)
	overview := securityOverview(r, cfg)
	contacts := overview["contacts"].([]map[string]string)
	if len(contacts) != 2 || contacts[0]["type"] != "form" || !strings.HasSuffix(contacts[0]["url"], "/server/contact?security_id="+id) || contacts[1]["address"] != "sec@example.com" {
		t.Fatalf("contacts = %+v", contacts)
	}
	if pgp := overview["pgp"].(map[string]interface{}); pgp["available"] != false {
		t.Errorf("pgp = %v", pgp)
	}
}

func TestSanitizeLogToken(t *testing.T) {
	if got := sanitizeLogToken("api\nINJECT user=admin"); got != "apiINJECTuseradmin" {
		t.Errorf("sanitizeLogToken = %q", got)
	}
	if got := sanitizeLogToken(strings.Repeat("a", 100)); len(got) != 40 {
		t.Errorf("sanitizeLogToken length = %d", len(got))
	}
}

// adminKeypairRequest builds a keypair-action POST as the signed-in admin "ops".
func adminKeypairRequest(form url.Values) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest(http.MethodPost, "/server/administration/security/keypair", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = r.WithContext(reqctx.SetValue(r.Context(), "admin_username", "ops"))
	return httptest.NewRecorder(), r
}

func TestSecurityKeypairActions(t *testing.T) {
	cfg, _ := securityTestEnv(t)
	cfg.Server.Branding.Title = "Weather"
	dir := t.TempDir()
	oldDir := securityKeyDir
	securityKeyDir = func() string { return dir }
	t.Cleanup(func() { securityKeyDir = oldDir })
	db := database.GetServerDB()
	settings := &model.SettingsModel{DB: db}
	_ = settings.SetString("web.security.keyservers", " ")
	hash, err := model.HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO server_admin_credentials (username, email, password_hash) VALUES ('ops', 'ops@example.com', ?)`, hash); err != nil {
		t.Fatal(err)
	}
	store := newSecurityStore()
	do := func(form url.Values) *httptest.ResponseRecorder {
		w, r := adminKeypairRequest(form)
		SecurityKeypairAction(w, r)
		return w
	}

	if w := do(url.Values{"action": {"generate"}}); w.Code != http.StatusSeeOther || !store.PGP.Exists() {
		t.Fatalf("generate = %d exists=%v", w.Code, store.PGP.Exists())
	}
	first, _ := store.PGP.Info()
	do(url.Values{"action": {"rotate"}})
	if second, _ := store.PGP.Info(); second.Fingerprint == first.Fingerprint || second.LastRotatedAt == nil {
		t.Error("rotate must replace the keypair")
	}
	do(url.Values{"action": {"publish"}})
	do(url.Values{"action": {"bogus"}})

	// sensitive operations: the password is asked again
	if w := do(url.Values{"action": {"export_private"}, "password": {"wrong"}, "reason": {"backup"}}); strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal("a wrong password must not export the key")
	}
	if w := do(url.Values{"action": {"export_private"}, "password": {"correct horse battery"}}); strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal("a missing reason must not export the key")
	}
	w := do(url.Values{"action": {"export_private"}, "password": {"correct horse battery"}, "reason": {"migrating to a new host"}})
	if !strings.Contains(w.Body.String(), "BEGIN PGP PRIVATE KEY BLOCK") || !strings.Contains(w.Header().Get("Content-Disposition"), "pgp.priv.asc") {
		t.Fatalf("export = %d %q", w.Code, w.Header())
	}
	if w := do(url.Values{"action": {"export_private"}, "password": {"correct horse battery"}, "reason": {"again"}}); strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal("the export is limited to once per hour")
	}
	exported, _ := store.PGP.PrivateKey()

	if do(url.Values{"action": {"delete"}, "password": {"correct horse battery"}, "confirm": {"yes"}}); !store.PGP.Exists() {
		t.Fatal("delete needs the typed confirmation")
	}
	do(url.Values{"action": {"delete"}, "password": {"correct horse battery"}, "confirm": {"DELETE"}})
	if store.PGP.Exists() {
		t.Fatal("delete must remove the keypair")
	}
	do(url.Values{"action": {"import_private"}, "password": {"correct horse battery"}, "private_key": {exported}})
	if !store.PGP.Exists() {
		t.Fatal("import must restore the keypair")
	}

	// public key download
	pw := httptest.NewRecorder()
	DownloadSecurityPublicKey(pw, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(pw.Body.String(), "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Error("public key download")
	}
	pub := httptest.NewRecorder()
	ServePGPKey()(pub, httptest.NewRequest(http.MethodGet, "/.well-known/pgp-key.asc", nil))
	if pub.Code != http.StatusOK || pub.Header().Get("Content-Type") != "application/pgp-keys" {
		t.Errorf("well-known key = %d %v", pub.Code, pub.Header())
	}
	if !verifyAdminPassword(adminReq("ops"), "correct horse battery") || verifyAdminPassword(adminReq("ops"), "nope") || verifyAdminPassword(adminReq(""), "x") {
		t.Error("verifyAdminPassword")
	}
	if n, err := PurgeSecurityReports(db, 0); err != nil || n != 0 {
		t.Errorf("PurgeSecurityReports = %d %v", n, err)
	}
}

func adminReq(name string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if name != "" {
		r = r.WithContext(reqctx.SetValue(r.Context(), "admin_username", name))
	}
	return r
}
