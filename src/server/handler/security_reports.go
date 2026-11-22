package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	paths "github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// securityLogger receives security.log events; main wires it at startup.
var securityLogger *util.Logger

// SetSecurityLogger sets the logger that writes security.log and audit.log entries.
func SetSecurityLogger(l *util.Logger) { securityLogger = l }

// logSecurityEvent writes one security.log line. Callers never pass report content or PII.
func logSecurityEvent(r *http.Request, event, details string) {
	if securityLogger != nil {
		securityLogger.Security(util.TrustedGetClientIP(r), event, details)
	}
}

// logSecurityAudit writes one audit.log entry for an admin action on the disclosure pipeline.
func logSecurityAudit(r *http.Request, action, resource, detail string) {
	if securityLogger != nil {
		securityLogger.Audit(AdminUsername(r), action, resource, "", detail, util.TrustedGetClientIP(r), r.UserAgent(), true, "")
	}
}

// securityKeyDir is where the project keypair files live (PART 11: {config_dir}/security/).
var securityKeyDir = func() string { return filepath.Join(paths.GetConfigDir(), "security") }

// newSecurityStore builds the report store over the server database.
func newSecurityStore() *service.SecurityReportStore {
	db := database.GetServerDB()
	if db == nil {
		return nil
	}
	key := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		key = cfg.Server.Security.EncryptionKey
	}
	return &service.SecurityReportStore{DB: db, EncryptionKey: key, PGP: service.NewPGPKeyManager(db, securityKeyDir())}
}

// securityIDFromRequest returns the supplied security_id when it is currently valid, else "".
// An invalid or expired id silently falls back to the standard contact mode and is logged
// only as the first 8 hex characters of its SHA-256 (PART 11).
func securityIDFromRequest(r *http.Request, supplied string) string {
	supplied = strings.TrimSpace(supplied)
	if supplied == "" {
		return ""
	}
	db := database.GetServerDB()
	if db != nil && service.ValidSecurityID(db, supplied) {
		return supplied
	}
	logSecurityEvent(r, "security.security_id_invalid", fmt.Sprintf("ua=%q id_hash=%s", r.UserAgent(), service.SecurityIDFingerprint(supplied)))
	return ""
}

// securityComponentOptions are the affected-component suggestions, taken from the IDEA.md features.
var securityComponentOptions = []string{"weather", "alerts", "earthquakes", "hurricanes", "moon", "locations", "accounts", "admin", "api", "frontend", "other"}

var securitySeverities = []string{"critical", "high", "medium", "low", "informational"}
var securityCredits = []string{"real_name", "handle", "no", "anonymous"}

// securityFieldSpecs is the single definition of the security-report fields (PART 11
// "Security-mode form fields"): the HTML form, the API description and the validation
// all read it.
func securityFieldSpecs() []contactFieldSpec {
	return []contactFieldSpec{
		{Name: "name", Type: "text", Required: true, MaxLength: intPtr(contactNameMax)},
		{Name: "email", Type: "email", Required: true, MaxLength: intPtr(contactEmailMax)},
		{Name: "researcher_gpg", Type: "textarea", MaxLength: intPtr(16384), MaxLengthWhen: []contactMaxLengthWhen{{Prefix: "https://", MaxLength: 2048}}},
		{Name: "affected_component", Type: "text", Required: true, MaxLength: intPtr(255), Options: securityComponentOptions},
		{Name: "affected_endpoint", Type: "text", MaxLength: intPtr(2048)},
		{Name: "severity", Type: "enum", Required: true, Options: securitySeverities},
		{Name: "summary", Type: "text", Required: true, MaxLength: intPtr(200)},
		{Name: "steps", Type: "textarea", Required: true, MaxLength: intPtr(10000)},
		{Name: "impact", Type: "textarea", Required: true, MaxLength: intPtr(5000)},
		{Name: "suggested_fix", Type: "textarea", MaxLength: intPtr(5000)},
		{Name: "cve_requested", Type: "checkbox"},
		{Name: "disclosure_days", Type: "number"},
		{Name: "credit_preference", Type: "enum", Required: true, Options: securityCredits},
		{Name: "credit_handle", Type: "text", MaxLength: intPtr(100), RequiredWhen: &contactRequiredWhen{Field: "credit_preference", Equals: "handle"}},
		{Name: "disclosure_agreement", Type: "checkbox", Required: true},
		{Name: "captcha", Type: "captcha", Required: true},
	}
}

// invalidSecurityField returns the first field of values that breaks the published rules, or "".
// Values are trimmed in place; limits count Unicode characters.
func invalidSecurityField(values map[string]string) string {
	for _, spec := range securityFieldSpecs() {
		if spec.Type == "captcha" {
			continue
		}
		value := strings.TrimSpace(values[spec.Name])
		values[spec.Name] = value
		required := spec.Required
		if spec.RequiredWhen != nil && values[spec.RequiredWhen.Field] == spec.RequiredWhen.Equals {
			required = true
		}
		if spec.Type == "checkbox" {
			if required && !config.IsTruthy(value) {
				return spec.Name
			}
			continue
		}
		if value == "" {
			if required {
				return spec.Name
			}
			continue
		}
		limit := 0
		if spec.MaxLength != nil {
			limit = *spec.MaxLength
		}
		for _, when := range spec.MaxLengthWhen {
			if strings.HasPrefix(value, when.Prefix) {
				limit = when.MaxLength
			}
		}
		if limit > 0 && utf8.RuneCountInString(value) > limit {
			return spec.Name
		}
		if (spec.Type == "text" || spec.Type == "email") && strings.ContainsAny(value, "\r\n") {
			return spec.Name
		}
		if spec.Type == "enum" && !stringIn(value, spec.Options) {
			return spec.Name
		}
		if spec.Type == "number" {
			if days, err := strconv.Atoi(value); err != nil || days < 1 || days > 3650 {
				return spec.Name
			}
		}
	}
	return ""
}

func stringIn(value string, options []string) bool {
	for _, o := range options {
		if o == value {
			return true
		}
	}
	return false
}

// Result keys of a security report submission.
const (
	securityReceived = "received"
	securityInvalid  = "invalid"
	securityFailed   = "failed"
)

// deliverSecurityReport validates, captcha-checks and stores one report. It returns a result
// key, the tracking id on success and the first invalid field on failure.
func deliverSecurityReport(r *http.Request, cfg *config.AppConfig, securityID string, values map[string]string, captcha contactForm) (result, trackingID, field string) {
	if field = invalidSecurityField(values); field != "" {
		return securityInvalid, "", field
	}
	if mail := (&contactForm{Name: values["name"], Email: values["email"], Subject: "x", Message: "x"}); invalidContactField(mail) != "" {
		return securityInvalid, "", invalidContactField(mail)
	}
	if res := checkContactCaptcha(r, cfg, captcha); res != "" {
		return res, "", "captcha"
	}
	store := newSecurityStore()
	if store == nil {
		return securityFailed, "", ""
	}
	armoredKey := ""
	if values["researcher_gpg"] != "" {
		key, err := service.FetchResearcherKey(r.Context(), values["researcher_gpg"])
		if err != nil {
			return securityInvalid, "", "researcher_gpg"
		}
		armoredKey = key
	}
	days, _ := strconv.Atoi(values["disclosure_days"])
	if days == 0 {
		days = 90
	}
	report := service.SecurityReport{
		ResearcherName: values["name"], Component: values["affected_component"], Endpoint: values["affected_endpoint"],
		Severity: values["severity"], Summary: values["summary"], Steps: values["steps"], Impact: values["impact"],
		SuggestedFix: values["suggested_fix"], CVERequested: config.IsTruthy(values["cve_requested"]), DisclosureDays: days,
		CreditPreference: values["credit_preference"], CreditHandle: values["credit_handle"],
		AppVersion: Version, CommitHash: CommitID, RequestUserAgent: r.UserAgent(), SubmittedAt: util.Now(),
	}
	tracking, token, err := store.Create(securityID, report, service.ReportContact{Name: values["name"], Email: values["email"], PGPKey: armoredKey})
	if err != nil {
		logSecurityEvent(r, "security.report_failed", "store error")
		return securityFailed, "", ""
	}
	logSecurityEvent(r, "security.report_received", fmt.Sprintf("tracking_id=%s severity=%s component=%s", tracking, report.Severity, sanitizeLogToken(report.Component)))
	notifySecurityReport(r, cfg, tracking, token, report, values["email"], armoredKey)
	return securityReceived, tracking, ""
}

// sanitizeLogToken keeps a free-text component safe for one log line.
func sanitizeLogToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/' {
			b.WriteRune(r)
		}
		if b.Len() >= 40 {
			break
		}
	}
	return b.String()
}

// notifySecurityReport emails the maintainers and the researcher when SMTP works. The
// maintainer mail never carries report content (it is read in the admin panel); the
// researcher mail carries only the tracking id and the one-shot status link.
func notifySecurityReport(r *http.Request, cfg *config.AppConfig, tracking, token string, report service.SecurityReport, researcherEmail, researcherKey string) {
	smtp := GetSMTPService(r)
	if smtp == nil {
		return
	}
	if cfg != nil && database.GetUsersDB() != nil {
		rows, err := database.GetUsersDB().Query(`SELECT email, COALESCE(personal_pgp_public_key, '') FROM admins WHERE is_active = 1`)
		if err == nil {
			defer func() { _ = rows.Close() }()
			content, _ := json.Marshal(struct {
				Tracking string                 `json:"tracking_id"`
				Report   service.SecurityReport `json:"report"`
			}{tracking, report})
			var recipients []struct{ email, key string }
			for rows.Next() {
				var rec struct{ email, key string }
				if rows.Scan(&rec.email, &rec.key) == nil {
					recipients = append(recipients, rec)
				}
			}
			for _, rec := range recipients {
				if rec.email == "" || strings.TrimSpace(rec.key) == "" {
					continue
				}
				sealed, sealErr := service.EncryptToArmoredKey(rec.key, content)
				if sealErr == nil {
					_ = smtp.SendEmail(rec.email, Translate(r, "email.security_report.maintainer_subject")+" "+tracking, string(sealed))
				}
			}
			noAdminKeys := true
			for _, rec := range recipients {
				if strings.TrimSpace(rec.key) != "" {
					noAdminKeys = false
					break
				}
			}
			if noAdminKeys {
				sealed, sealErr := util.EncryptAtRest(cfg.Server.Security.EncryptionKey, string(content))
				if sealErr == nil {
					fallback := Translate(r, "email.security_report.maintainer_warning")
					for _, rec := range recipients {
						if rec.email != "" {
							_ = smtp.SendEmailAttachment(rec.email, Translate(r, "email.security_report.maintainer_subject")+" "+tracking, fallback, tracking+".enc", "application/octet-stream", []byte(sealed))
						}
					}
				}
			}
		}
	}
	link := util.BuildURL(r, "/server/security/report/"+tracking) + "?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n\n%s", Translate(r, "email.security_report.ack_intro"),
		Translate(r, "email.security_report.label_tracking"), tracking,
		Translate(r, "email.security_report.label_status_link"), link, Translate(r, "email.security_report.ack_footer"))
	if researcherKey != "" {
		if sealed, err := service.EncryptToArmoredKey(researcherKey, []byte(body)); err == nil {
			body = string(sealed)
		}
	}
	_ = smtp.SendEmail(researcherEmail, Translate(r, "email.security_report.ack_subject"), body)
}

// securityFormValues reads the security fields from a parsed form.
func securityFormValues(r *http.Request) map[string]string {
	values := map[string]string{}
	for _, spec := range securityFieldSpecs() {
		if spec.Type != "captcha" {
			values[spec.Name] = r.PostFormValue(spec.Name)
		}
	}
	return values
}

// renderSecurityReceived renders the confirmation after a web submission.
func renderSecurityReceived(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, tracking string) {
	data := map[string]interface{}{
		"page":       "security_report_received",
		"server":     pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
		"HostInfo":   util.GetHostInfo(r),
		"TrackingID": tracking,
	}
	NegotiateResponse(w, r, "page/security_report_received.tmpl", data)
}

// securityReportFormData builds the template data of the security-mode contact form.
func securityReportFormData(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, securityID string, captcha map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"page":       "contact",
		"server":     pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
		"HostInfo":   util.GetHostInfo(r),
		"flash":      TakeFlash(w, r),
		"Captcha":    captcha,
		"SecurityID": securityID,
		"Components": securityComponentOptions,
		"Severities": securitySeverities,
		"Credits":    securityCredits,
		"Fields":     securityFieldSpecs(),
	}
}

// handleSecurityFormWeb is the no-JavaScript submission of the security-mode form (POST /server/contact with security_id).
func handleSecurityFormWeb(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, securityID string) {
	result, tracking, field := deliverSecurityReport(r, cfg, securityID, securityFormValues(r), contactForm{
		CaptchaID: r.PostFormValue("captcha_id"), CaptchaAnswer: r.PostFormValue("captcha_answer"),
		CaptchaToken: firstNonEmpty(r.PostFormValue("g-recaptcha-response"), r.PostFormValue("h-captcha-response")),
	})
	switch result {
	case securityReceived:
		renderSecurityReceived(w, r, cfg, tracking)
		return
	case securityInvalid:
		SetFlash(w, r, "error", "flash.security_report_invalid")
	case contactCaptchaFailed:
		SetFlash(w, r, "error", "flash.contact_captcha_failed")
	case contactCaptchaOffline:
		SetFlash(w, r, "error", "flash.contact_captcha_unavailable")
	default:
		SetFlash(w, r, "error", "flash.security_report_failed")
	}
	_ = field
	http.Redirect(w, r, "/server/contact?security_id="+url.QueryEscape(securityID), http.StatusSeeOther)
}

// handleSecurityFormAPI is the JSON submission (POST /api/{api_version}/server/contact with security_id).
func handleSecurityFormAPI(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, securityID string, raw map[string]interface{}) {
	values := map[string]string{}
	for k, v := range raw {
		switch typed := v.(type) {
		case string:
			values[k] = typed
		case bool:
			values[k] = strconv.FormatBool(typed)
		case float64:
			values[k] = strconv.FormatFloat(typed, 'f', -1, 64)
		}
	}
	captcha := contactForm{CaptchaID: values["captcha_id"], CaptchaAnswer: values["captcha_answer"], CaptchaToken: values["captcha_response"]}
	result, tracking, field := deliverSecurityReport(r, cfg, securityID, values, captcha)
	switch result {
	case securityReceived:
		RespondSuccess(w, r, Translate(r, "success.security_report.received"), map[string]interface{}{"tracking_id": tracking})
	case securityInvalid:
		ValidationFailed(w, r, Translate(r, "errors.contact.invalid"), map[string]interface{}{"field": field, "rule": "invalid"})
	case contactCaptchaFailed:
		RespondError(w, r, http.StatusBadRequest, "CAPTCHA_FAILED", Translate(r, "errors.contact.captcha_failed"))
	case contactCaptchaOffline:
		w.Header().Set("Retry-After", "30")
		RespondError(w, r, http.StatusServiceUnavailable, "CAPTCHA_UNAVAILABLE", Translate(r, "errors.contact.captcha_unavailable"))
	default:
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.contact.failed_to_send_message"))
	}
}

// decodeSecurityBody reads a JSON body for the security API path.
func decodeSecurityBody(w http.ResponseWriter, r *http.Request) (map[string]interface{}, bool) {
	var raw map[string]interface{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&raw); err != nil {
		return nil, false
	}
	return raw, true
}

// ShowSecurityReportStatus is the researcher status page.
// GET /server/security/report/{tracking_id}?token=... (no API mirror: the emailed one-shot link stays off the API surface)
func ShowSecurityReportStatus(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		store := newSecurityStore()
		if store == nil {
			http.NotFound(w, r)
			return
		}
		row, err := store.ResearcherView(chi.URLParam(r, "tracking_id"), r.URL.Query().Get("token"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data := map[string]interface{}{
			"page":     "security_report_status",
			"server":   pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
			"HostInfo": util.GetHostInfo(r),
			"Report":   row,
		}
		NegotiateResponse(w, r, "page/security_report_status.tmpl", data)
	}
}

// ServePGPKey serves the project public key. GET /.well-known/pgp-key.asc
func ServePGPKey() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := newSecurityStore()
		if store == nil || !store.PGP.Exists() {
			http.NotFound(w, r)
			return
		}
		pub, err := store.PGP.PublicKey()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pgp-keys")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(pub))
	}
}
