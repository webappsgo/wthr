package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/util"
)

var contactRoutes chi.Router
var contactLocationExists func(string) bool
var contactEarthquakeExists func(string, string) bool

// SetContactRouteMatcher connects abuse reports to the application's registered routes.
func SetContactRouteMatcher(router chi.Router) { contactRoutes = router }

// SetContactLocationValidator checks dynamic location routes against local project data.
func SetContactLocationValidator(check func(string) bool) { contactLocationExists = check }

// SetContactEarthquakeValidator checks dynamic earthquake detail routes against cached data.
func SetContactEarthquakeValidator(check func(string, string) bool) { contactEarthquakeExists = check }

type contactFlowResult struct {
	Kind          string
	Field         string
	Reason        string
	RetryAfter    time.Duration
	Form          contactForm
	Values        ContactAntispamValues
	FormToken     string
	ReviewToken   string
	Drop          bool
	DeliverResult string
	TrackingID    string
}

var contactSnapshotFields = []string{
	"type", "name", "email", "subject", "message", "reported_url", "reason",
	"copyrighted_work", "good_faith", "accuracy", "signature", "request_kind",
}

var contactProtocolFields = []string{
	"csrf_token", "website", "form_token", "step", "review_token", "confirmed", "captcha_id", "captcha_answer",
	"captcha_response", "g-recaptcha-response", "h-captcha-response",
}

func allowedContactFields(reportType string) []string {
	fields := append([]string(nil), contactProtocolFields...)
	fields = append(fields, "type", "name", "email", "subject", "message")
	switch reportType {
	case "abuse":
		fields = append(fields, "reported_url", "reason")
	case "dmca":
		fields = append(fields, "reported_url", "copyrighted_work", "good_faith", "accuracy", "signature")
	case "privacy":
		fields = append(fields, "request_kind")
	}
	return fields
}

func contactFlowValues(raw map[string]string) ContactAntispamValues {
	values := make(ContactAntispamValues, len(contactSnapshotFields))
	for _, name := range contactSnapshotFields {
		values[name] = strings.TrimSpace(raw[name])
	}
	for _, name := range []string{"good_faith", "accuracy"} {
		if values[name] == "on" {
			values[name] = "true"
		}
	}
	return values
}

func contactFormFromValues(values ContactAntispamValues, raw map[string]string) contactForm {
	return contactForm{
		Type: values["type"], Name: values["name"], Email: values["email"], Subject: values["subject"], Message: values["message"],
		ReportedURL: values["reported_url"], Reason: values["reason"], CopyrightedWork: values["copyrighted_work"],
		GoodFaith: values["good_faith"] == "true", Accuracy: values["accuracy"] == "true", Signature: values["signature"], RequestKind: values["request_kind"],
		Website: raw["website"], FormToken: raw["form_token"], ReviewToken: raw["review_token"], Step: raw["step"], Confirmed: raw["confirmed"] == "true" || raw["confirmed"] == "on",
		CaptchaID: raw["captcha_id"], CaptchaAnswer: raw["captcha_answer"], CaptchaToken: firstNonEmpty(raw["captcha_response"], raw["g-recaptcha-response"], raw["h-captcha-response"]),
	}
}

func contactRawFromRequest(w http.ResponseWriter, r *http.Request, api bool) (map[string]string, error) {
	values := map[string]string{}
	if api {
		var raw map[string]json.RawMessage
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, contactAntispamBodyLimit))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		var trailing interface{}
		if err := decoder.Decode(&trailing); err != io.EOF {
			return nil, fmt.Errorf("multiple JSON values")
		}
		for key, value := range raw {
			var text string
			if err := json.Unmarshal(value, &text); err == nil {
				values[key] = text
				continue
			}
			var boolean bool
			if err := json.Unmarshal(value, &boolean); err == nil {
				if boolean {
					values[key] = "true"
				}
				continue
			}
			return nil, fmt.Errorf("invalid contact field")
		}
		return values, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, contactAntispamBodyLimit)
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for key, list := range r.PostForm {
		if len(list) > 1 {
			return nil, fmt.Errorf("repeated contact field")
		}
		if len(list) == 1 {
			values[key] = list[0]
		}
	}
	return values, nil
}

func processContactFlow(r *http.Request, db *database.DB, cfg *config.AppConfig, raw map[string]string, htmlForm bool) contactFlowResult {
	manager, err := getContactAntispam(db, cfg)
	if err != nil {
		return contactFlowResult{Kind: "unavailable"}
	}
	step := raw["step"]
	if step == "" {
		step = "submit"
	}
	values := contactFlowValues(raw)
	form := contactFormFromValues(values, raw)
	if step != "submit" && step != "confirm" && step != "edit" {
		return contactFlowResult{Kind: "invalid", Field: "step", Form: form}
	}
	if !validReportType(form.Type) {
		recordContactAntispamBlock(r, manager, "shape")
		return contactFlowResult{Kind: "invalid", Field: "type", Form: form, Values: values}
	}
	if shape := ValidateContactShape(r, ContactAntispamValues(raw), allowedContactFields(form.Type), []string{"type", "form_token"}, htmlForm, util.BuildURL(r, "/")); shape != "" {
		retry := recordContactAntispamBlock(r, manager, shape)
		return contactFlowResult{Kind: shape, Reason: shape, RetryAfter: retry, Form: form, Values: values}
	}
	if field := invalidContactField(&form); field != "" {
		recordContactAntispamBlock(r, manager, "shape")
		return contactFlowResult{Kind: "invalid", Field: field, Form: form, Values: values}
	}
	if step == "edit" {
		valid, drop := manager.VerifyContactReviewToken(form.ReviewToken, values, time.Now())
		if !valid {
			retry := recordContactAntispamBlock(r, manager, "review")
			return contactFlowResult{Kind: "review", Reason: "review", RetryAfter: retry, Form: form, Values: values}
		}
		fresh, tokenErr := manager.NewContactFormToken(time.Now())
		if tokenErr != nil {
			return contactFlowResult{Kind: "unavailable"}
		}
		return contactFlowResult{Kind: "edit", Form: form, Values: values, FormToken: fresh, Drop: drop}
	}
	ip := util.TrustedGetClientIP(r)
	overlay := util.OverlayHostFromRequest(r) != ""
	if step == "submit" {
		if reason, wait := manager.CheckRateLimit(ip, form.Email, time.Now(), false, overlay); reason != "" {
			retry := recordContactAntispamBlock(r, manager, reason)
			return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: maxDuration(wait, retry), Form: form, Values: values}
		}
		if form.Website != "" && contactHoneypotEnabled(cfg) {
			retry := recordContactAntispamBlock(r, manager, "honeypot")
			review, tokenErr := manager.NewContactReviewToken(values, time.Now(), true)
			fresh, _ := manager.NewContactFormToken(time.Now())
			if tokenErr != nil {
				return contactFlowResult{Kind: "unavailable"}
			}
			return contactFlowResult{Kind: "review", Form: form, Values: values, FormToken: fresh, ReviewToken: review, Drop: true, RetryAfter: retry}
		}
		valid, reason := manager.ConsumeContactFormToken(form.FormToken, time.Now())
		if !valid {
			retry := recordContactAntispamBlock(r, manager, reason)
			fresh, _ := manager.NewContactFormToken(time.Now())
			return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: retry, Form: form, Values: values, FormToken: fresh}
		}
		if captcha := checkContactCaptcha(r, cfg, form); captcha != "" {
			retry := recordContactAntispamBlock(r, manager, "captcha")
			return contactFlowResult{Kind: captcha, Reason: "captcha", RetryAfter: retry, Form: form, Values: values}
		}
		if reason := manager.CheckContactContent(values, contactAcceptedHosts(r, cfg), false); reason != "" {
			retry := recordContactAntispamBlock(r, manager, reason)
			return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: retry, Form: form, Values: values}
		}
		if form.Type == "abuse" || form.Type == "dmca" {
			if !contactReportedURLExists(form.ReportedURL, r, cfg) {
				retry := recordContactAntispamBlock(r, manager, "content")
				return contactFlowResult{Kind: "content", Reason: "content", RetryAfter: retry, Form: form, Values: values}
			}
		}
		fresh, tokenErr := manager.NewContactFormToken(time.Now())
		if tokenErr != nil {
			return contactFlowResult{Kind: "unavailable"}
		}
		review, tokenErr := manager.NewContactReviewToken(values, time.Now(), false)
		if tokenErr != nil {
			return contactFlowResult{Kind: "unavailable"}
		}
		return contactFlowResult{Kind: "review", Form: form, Values: values, FormToken: fresh, ReviewToken: review}
	}
	if !form.Confirmed {
		retry := recordContactAntispamBlock(r, manager, "review")
		return contactFlowResult{Kind: "invalid", Field: "confirmed", Reason: "review", RetryAfter: retry, Form: form, Values: values}
	}
	valid, drop := manager.VerifyContactReviewToken(form.ReviewToken, values, time.Now())
	if !valid {
		retry := recordContactAntispamBlock(r, manager, "review")
		return contactFlowResult{Kind: "invalid", Field: "review_token", Reason: "review", RetryAfter: retry, Form: form, Values: values}
	}
	if reason, wait := manager.CheckRateLimit(ip, form.Email, time.Now(), true, overlay); reason != "" {
		retry := recordContactAntispamBlock(r, manager, reason)
		return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: maxDuration(wait, retry), Form: form, Values: values}
	}
	if valid, _ = manager.ConsumeContactReviewToken(form.ReviewToken, values, time.Now()); !valid {
		retry := recordContactAntispamBlock(r, manager, "replay")
		return contactFlowResult{Kind: "invalid", Field: "review_token", Reason: "replay", RetryAfter: retry, Form: form, Values: values}
	}
	if drop {
		return contactFlowResult{Kind: "dropped", Form: form, Values: values}
	}
	if manager.IsContactDuplicate(values, time.Now()) {
		recordContactAntispamBlock(r, manager, "duplicate")
		return contactFlowResult{Kind: "dropped", Form: form, Values: values}
	}
	result := deliverContact(r, cfg, form)
	return contactFlowResult{Kind: "delivered", DeliverResult: result, Form: form, Values: values}
}

func processSecurityContactFlow(r *http.Request, db *database.DB, cfg *config.AppConfig, securityID string, raw map[string]string, htmlForm bool) contactFlowResult {
	manager, err := getContactAntispam(db, cfg)
	if err != nil {
		return contactFlowResult{Kind: "unavailable"}
	}
	step := raw["step"]
	if step == "" {
		step = "submit"
	}
	if step != "submit" && step != "confirm" && step != "edit" {
		return contactFlowResult{Kind: "invalid", Field: "step"}
	}
	allowed := append([]string(nil), contactProtocolFields...)
	allowed = append(allowed, "security_id")
	values := ContactAntispamValues{"type": "security", "security_id": securityID}
	securityValues := map[string]string{}
	for _, spec := range securityFieldSpecs() {
		if spec.Type == "captcha" {
			continue
		}
		allowed = append(allowed, spec.Name)
		securityValues[spec.Name] = strings.TrimSpace(raw[spec.Name])
		values[spec.Name] = securityValues[spec.Name]
	}
	if shape := ValidateContactShape(r, ContactAntispamValues(raw), allowed, []string{"form_token"}, htmlForm, util.BuildURL(r, "/")); shape != "" {
		retry := recordContactAntispamBlock(r, manager, shape)
		return contactFlowResult{Kind: shape, Reason: shape, RetryAfter: retry, Values: values}
	}
	if field := invalidSecurityField(securityValues); field != "" {
		recordContactAntispamBlock(r, manager, "shape")
		return contactFlowResult{Kind: "invalid", Field: field, Values: values}
	}
	identity := &contactForm{Name: securityValues["name"], Email: securityValues["email"], Subject: "security", Message: securityValues["summary"]}
	if field := invalidContactIdentity(identity); field != "" {
		recordContactAntispamBlock(r, manager, "shape")
		return contactFlowResult{Kind: "invalid", Field: field, Values: values}
	}
	values["name"], values["email"] = identity.Name, identity.Email
	values["security_id"] = securityID
	form := contactForm{Type: "security", Name: identity.Name, Email: identity.Email, Subject: "Security report", Message: securityValues["summary"],
		FormToken: raw["form_token"], ReviewToken: raw["review_token"], Step: step, Confirmed: raw["confirmed"] == "true" || raw["confirmed"] == "on", Website: raw["website"],
		CaptchaID: raw["captcha_id"], CaptchaAnswer: raw["captcha_answer"], CaptchaToken: firstNonEmpty(raw["captcha_response"], raw["g-recaptcha-response"], raw["h-captcha-response"])}
	ip, overlay := util.TrustedGetClientIP(r), util.OverlayHostFromRequest(r) != ""
	if step == "submit" {
		if reason, wait := manager.CheckRateLimit(ip, form.Email, time.Now(), false, overlay); reason != "" {
			retry := recordContactAntispamBlock(r, manager, reason)
			return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: maxDuration(wait, retry), Form: form, Values: values}
		}
		if form.Website != "" && contactHoneypotEnabled(cfg) {
			retry := recordContactAntispamBlock(r, manager, "honeypot")
			review, tokenErr := manager.NewContactReviewToken(values, time.Now(), true)
			fresh, _ := manager.NewContactFormToken(time.Now())
			if tokenErr != nil {
				return contactFlowResult{Kind: "unavailable"}
			}
			return contactFlowResult{Kind: "review", Form: form, Values: values, FormToken: fresh, ReviewToken: review, Drop: true, RetryAfter: retry}
		}
		valid, reason := manager.ConsumeContactFormToken(form.FormToken, time.Now())
		if !valid {
			retry := recordContactAntispamBlock(r, manager, reason)
			fresh, _ := manager.NewContactFormToken(time.Now())
			return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: retry, Form: form, Values: values, FormToken: fresh}
		}
		if captcha := checkContactCaptcha(r, cfg, form); captcha != "" {
			retry := recordContactAntispamBlock(r, manager, "captcha")
			return contactFlowResult{Kind: captcha, Reason: "captcha", RetryAfter: retry, Form: form, Values: values}
		}
		fresh, tokenErr := manager.NewContactFormToken(time.Now())
		if tokenErr != nil {
			return contactFlowResult{Kind: "unavailable"}
		}
		review, tokenErr := manager.NewContactReviewToken(values, time.Now(), false)
		if tokenErr != nil {
			return contactFlowResult{Kind: "unavailable"}
		}
		return contactFlowResult{Kind: "review", Form: form, Values: values, FormToken: fresh, ReviewToken: review}
	}
	if step == "edit" {
		valid, drop := manager.VerifyContactReviewToken(form.ReviewToken, values, time.Now())
		if !valid {
			retry := recordContactAntispamBlock(r, manager, "review")
			return contactFlowResult{Kind: "review", Reason: "review", RetryAfter: retry, Form: form, Values: values}
		}
		fresh, tokenErr := manager.NewContactFormToken(time.Now())
		if tokenErr != nil {
			return contactFlowResult{Kind: "unavailable"}
		}
		return contactFlowResult{Kind: "edit", Form: form, Values: values, FormToken: fresh, ReviewToken: form.ReviewToken, Drop: drop}
	}
	if !form.Confirmed {
		retry := recordContactAntispamBlock(r, manager, "review")
		return contactFlowResult{Kind: "invalid", Field: "confirmed", Reason: "review", RetryAfter: retry, Form: form, Values: values}
	}
	valid, drop := manager.VerifyContactReviewToken(form.ReviewToken, values, time.Now())
	if !valid {
		retry := recordContactAntispamBlock(r, manager, "review")
		return contactFlowResult{Kind: "invalid", Field: "review_token", Reason: "review", RetryAfter: retry, Form: form, Values: values}
	}
	if reason, wait := manager.CheckRateLimit(ip, form.Email, time.Now(), true, overlay); reason != "" {
		retry := recordContactAntispamBlock(r, manager, reason)
		return contactFlowResult{Kind: reason, Reason: reason, RetryAfter: maxDuration(wait, retry), Form: form, Values: values}
	}
	if valid, _ = manager.ConsumeContactReviewToken(form.ReviewToken, values, time.Now()); !valid {
		retry := recordContactAntispamBlock(r, manager, "replay")
		return contactFlowResult{Kind: "invalid", Field: "review_token", Reason: "replay", RetryAfter: retry, Form: form, Values: values}
	}
	if drop {
		return contactFlowResult{Kind: "dropped", Form: form, Values: values}
	}
	result, tracking, field := storeSecurityReport(r, cfg, securityID, securityValues)
	if result != securityReceived {
		return contactFlowResult{Kind: "delivery_failed", Field: field, Form: form, Values: values}
	}
	return contactFlowResult{Kind: "delivered", DeliverResult: result, TrackingID: tracking, Form: form, Values: values}
}

func contactAcceptedHosts(r *http.Request, cfg *config.AppConfig) []string {
	hosts := []string{r.Host}
	if cfg != nil {
		hosts = append(hosts, cfg.Server.FQDN)
	}
	return hosts
}

func contactHoneypotEnabled(cfg *config.AppConfig) bool {
	if cfg == nil {
		return true
	}
	setting := cfg.Server.Pages.Contact.EffectiveAntispam().Honeypot
	return setting == nil || *setting
}

func contactReportedURLExists(raw string, r *http.Request, cfg *config.AppConfig) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || !contactHostAllowed(strings.ToLower(u.Hostname()), contactAcceptedHosts(r, cfg)) {
		return false
	}
	if u.Path == "" || u.Path == "/" || u.Path == "/server/contact" || strings.HasPrefix(u.Path, "/api/") {
		return false
	}
	if cfg != nil && cfg.Server.AdminPath != "" && strings.HasPrefix(u.Path, "/server/"+strings.Trim(cfg.Server.AdminPath, "/")) {
		return false
	}
	if contactRoutes == nil {
		return false
	}
	ctx := chi.NewRouteContext()
	if !contactRoutes.Match(ctx, http.MethodGet, u.Path) {
		return false
	}
	if strings.Contains(ctx.RoutePattern(), "{location}") {
		if contactLocationExists == nil {
			return false
		}
		for i, key := range ctx.URLParams.Keys {
			if key == "location" && i < len(ctx.URLParams.Values) {
				return contactLocationExists(ctx.URLParams.Values[i])
			}
		}
		return false
	}
	if strings.Contains(ctx.RoutePattern(), "/earthquakes/detail/{id}") {
		if contactEarthquakeExists == nil {
			return false
		}
		for i, key := range ctx.URLParams.Keys {
			if key == "id" && i < len(ctx.URLParams.Values) {
				feed := u.Query().Get("feed")
				if feed == "" {
					feed = "all_week"
				}
				return contactEarthquakeExists(feed, ctx.URLParams.Values[i])
			}
		}
		return false
	}
	// Dynamic routes must validate the captured resource against project data.
	if strings.ContainsAny(ctx.RoutePattern(), "{*") {
		return false
	}
	return true
}

func contactSnapshot(values ContactAntispamValues) []map[string]string {
	fields := make([]map[string]string, 0, len(contactSnapshotFields))
	for _, name := range contactSnapshotFields {
		fields = append(fields, map[string]string{"Name": name, "Value": values[name]})
	}
	return fields
}

func securitySnapshot(values ContactAntispamValues) []map[string]string {
	fields := make([]map[string]string, 0, len(values))
	for _, spec := range securityFieldSpecs() {
		if spec.Type == "captcha" {
			continue
		}
		fields = append(fields, map[string]string{"Name": spec.Name, "Value": values[spec.Name]})
	}
	return fields
}

func contactPageData(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, reportType, mode string, values ContactAntispamValues, formToken, reviewToken string) map[string]interface{} {
	provider := cfg.Server.Pages.Contact.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
	captcha := map[string]interface{}{"Provider": provider}
	if provider == config.CaptchaSimple {
		id, a, b := contactCaptcha.New()
		captcha["ID"], captcha["A"], captcha["B"] = id, a, b
	} else {
		captcha["SiteKey"] = cfg.Server.Pages.Contact.CaptchaSiteKey
		id, a, b := contactCaptcha.New()
		captcha["Fallback"] = map[string]interface{}{"Provider": config.CaptchaSimple, "ID": id, "A": a, "B": b}
	}
	if values == nil {
		values = ContactAntispamValues{}
	}
	anti := cfg.Server.Pages.Contact.EffectiveAntispam()
	honeypot := anti.Honeypot == nil || *anti.Honeypot
	return map[string]interface{}{
		"page": "contact", "server": pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
		"HostInfo": util.GetHostInfo(r), "flash": TakeFlash(w, r), "Mode": mode, "Type": reportType,
		"Values": map[string]string(values), "SnapshotFields": contactSnapshot(values), "FormToken": formToken, "ReviewToken": reviewToken,
		"Captcha": captcha, "FQDN": cfg.Server.FQDN, "AbuseEmail": cfg.Server.Contact.Abuse.Email,
		"ReasonOptions": contactReasonOptions(), "RequestKindOptions": contactRequestKindOptions(), "HoneypotEnabled": honeypot,
	}
}

func securityFlowPageData(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, securityID, mode string, flow contactFlowResult) map[string]interface{} {
	provider := cfg.Server.Pages.Contact.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
	captcha := map[string]interface{}{"Provider": provider}
	if provider == config.CaptchaSimple {
		id, a, b := contactCaptcha.New()
		captcha["ID"], captcha["A"], captcha["B"] = id, a, b
	} else {
		captcha["SiteKey"] = cfg.Server.Pages.Contact.CaptchaSiteKey
		id, a, b := contactCaptcha.New()
		captcha["Fallback"] = map[string]interface{}{"Provider": config.CaptchaSimple, "ID": id, "A": a, "B": b}
	}
	data := securityReportFormData(w, r, cfg, securityID, captcha)
	values := map[string]string{}
	for key, value := range flow.Values {
		values[key] = value
	}
	anti := cfg.Server.Pages.Contact.EffectiveAntispam()
	data["Mode"], data["Type"], data["Values"], data["FormToken"], data["ReviewToken"] = mode, "security", values, flow.FormToken, flow.ReviewToken
	data["SnapshotFields"] = securitySnapshot(flow.Values)
	data["HoneypotEnabled"] = anti.Honeypot == nil || *anti.Honeypot
	return data
}

func handleSecurityContactFlowWeb(w http.ResponseWriter, r *http.Request, db *database.DB, cfg *config.AppConfig, securityID string, raw map[string]string) {
	flow := processSecurityContactFlow(r, db, cfg, securityID, raw, true)
	if flow.Kind == "review" || flow.Kind == "edit" {
		mode := "form"
		if flow.Kind == "review" {
			mode = "review"
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		NegotiateResponse(w, r, "page/security_report.tmpl", securityFlowPageData(w, r, cfg, securityID, mode, flow))
		return
	}
	if flow.Kind == "delivered" {
		renderSecurityReceived(w, r, cfg, flow.TrackingID)
		return
	}
	if flow.Kind == "dropped" {
		renderSecurityReceived(w, r, cfg, "")
		return
	}
	renderContactFlowFailure(w, r, cfg, db, flow, securityID)
}

func handleSecurityContactFlowAPI(w http.ResponseWriter, r *http.Request, db *database.DB, cfg *config.AppConfig, securityID string, raw map[string]string) {
	flow := processSecurityContactFlow(r, db, cfg, securityID, raw, false)
	if flow.FormToken == "" {
		if manager, err := getContactAntispam(db, cfg); err == nil {
			flow.FormToken, _ = manager.NewContactFormToken(time.Now())
		}
	}
	if flow.Kind == "review" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{
			"status": "review", "review_token": flow.ReviewToken, "form_token": flow.FormToken,
			"confirm_after_seconds": cfg.Server.Pages.Contact.EffectiveAntispam().ReviewMinSeconds,
			"review":                map[string]string{"type": "security", "email": flow.Values["email"], "subject": flow.Values["summary"]},
		}})
		return
	}
	if flow.Kind == "delivered" {
		RespondSuccess(w, r, Translate(r, "success.security_report.received"), map[string]interface{}{"tracking_id": flow.TrackingID, "form_token": flow.FormToken})
		return
	}
	if flow.Kind == "dropped" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"status": "received", "form_token": flow.FormToken}})
		return
	}
	writeContactFlowError(w, r, cfg, db, flow)
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func writeContactFlowError(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, db *database.DB, flow contactFlowResult) {
	formToken := flow.FormToken
	if formToken == "" {
		if manager, err := getContactAntispam(db, cfg); err == nil {
			formToken, _ = manager.NewContactFormToken(time.Now())
		}
	}
	status, code, message := http.StatusBadRequest, "VALIDATION_FAILED", "Your message could not be sent"
	details := map[string]string{"field": "form_token", "rule": flow.Kind}
	if flow.Kind == "content" || flow.Kind == "markup" {
		details = map[string]string{"field": "message", "rule": "invalid"}
	}
	switch flow.Kind {
	case "invalid":
		details["field"], details["rule"] = flow.Field, "invalid"
	case "captcha_failed":
		details["field"], details["rule"] = "captcha", "invalid"
		code = "VALIDATION_FAILED"
		message = Translate(r, "errors.contact.captcha_failed")
	case "captcha_unavailable":
		status, code, message = http.StatusServiceUnavailable, "CAPTCHA_UNAVAILABLE", Translate(r, "errors.contact.captcha_unavailable")
	case "rate_ip", "rate_email", "rate_global", "banned":
		status, code, message = http.StatusTooManyRequests, "RATE_LIMITED", "Please try again later"
		seconds := int(flow.RetryAfter.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", fmt.Sprint(seconds))
	case "origin":
		message = "Please submit the form from this site"
	case "unavailable":
		status, code, message = http.StatusServiceUnavailable, "CONTACT_FORM_UNAVAILABLE", "Contact form is unavailable"
	case "delivery_failed":
		status, code, message = http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.contact.failed_to_send_message")
	}
	data := map[string]interface{}{"form_token": formToken, "captcha": contactCaptchaDescriptor(r, cfg)}
	writeJSON(w, status, map[string]interface{}{"ok": false, "error": code, "message": message, "details": details, "data": data})
}

func renderContactFlowFailure(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, db *database.DB, flow contactFlowResult, securityID string) {
	if flow.FormToken == "" {
		if manager, err := getContactAntispam(db, cfg); err == nil {
			flow.FormToken, _ = manager.NewContactFormToken(time.Now())
		}
	}
	status := http.StatusBadRequest
	key := "flash.contact_invalid"
	if flow.Kind == "captcha_failed" {
		key = "flash.contact_captcha_failed"
	}
	if flow.Kind == "captcha_unavailable" {
		key, status = "flash.contact_captcha_unavailable", http.StatusServiceUnavailable
	}
	if flow.Kind == "rate_ip" || flow.Kind == "rate_email" || flow.Kind == "rate_global" || flow.Kind == "banned" {
		status = http.StatusTooManyRequests
		seconds := int(flow.RetryAfter.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", fmt.Sprint(seconds))
	}
	if flow.Kind == "unavailable" {
		status = http.StatusServiceUnavailable
	}
	if flow.Kind == "delivery_failed" {
		status = http.StatusInternalServerError
	}
	SetFlash(w, r, "error", key)
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json") {
		if securityID != "" {
			data := securityFlowPageData(w, r, cfg, securityID, "form", flow)
			middleware.RenderHTML(w, r, status, "page/security_report.tmpl", util.TemplateData(r, data))
			return
		}
		data := contactPageData(w, r, cfg, flow.Form.Type, "form", flow.Values, flow.FormToken, "")
		middleware.RenderHTML(w, r, status, "page/contact.tmpl", util.TemplateData(r, data))
		return
	}
	writeContactFlowError(w, r, cfg, db, flow)
}
