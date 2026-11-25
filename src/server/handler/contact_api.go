package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// contactFieldSpec describes one contact form field for the API mirror.
type contactFieldSpec struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	// MaxLength counts Unicode characters after trimming; null means no text limit
	MaxLength *int `json:"max_length"`
	// RequiredWhen makes the field required when another field has a value; null when unconditional
	RequiredWhen *contactRequiredWhen `json:"required_when"`
	// MaxLengthWhen narrows the limit for values starting with a prefix; null when there is none
	MaxLengthWhen []contactMaxLengthWhen `json:"max_length_when"`
	// Options lists the allowed values of an enum field or the suggestions of a text field; not part of the published descriptor
	Options []string `json:"options,omitempty"`
}

// contactRequiredWhen is the PART 11 required_when descriptor.
type contactRequiredWhen struct {
	Field  string   `json:"field"`
	Equals string   `json:"equals,omitempty"`
	In     []string `json:"in,omitempty"`
}

// contactMaxLengthWhen is one PART 11 max_length_when entry.
type contactMaxLengthWhen struct {
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"max_length"`
}

func intPtr(v int) *int { return &v }

func validReportType(value string) bool {
	switch value {
	case "abuse", "dmca", "privacy":
		return true
	}
	return false
}

func contactReasonOptions() []map[string]string {
	return []map[string]string{
		{"Value": "illegal_content", "LabelKey": "contact.report.reason.illegal_content"},
		{"Value": "terms_violation", "LabelKey": "contact.report.reason.terms_violation"},
		{"Value": "harassment", "LabelKey": "contact.report.reason.harassment"},
		{"Value": "malware_phishing", "LabelKey": "contact.report.reason.malware_phishing"},
		{"Value": "other", "LabelKey": "contact.report.reason.other"},
	}
}

func contactRequestKindOptions() []map[string]string {
	return []map[string]string{
		{"Value": "access", "LabelKey": "contact.report.request_kind.access"},
		{"Value": "export", "LabelKey": "contact.report.request_kind.export"},
		{"Value": "deletion", "LabelKey": "contact.report.request_kind.deletion"},
		{"Value": "correction", "LabelKey": "contact.report.request_kind.correction"},
		{"Value": "opt_out", "LabelKey": "contact.report.request_kind.opt_out"},
		{"Value": "other", "LabelKey": "contact.report.request_kind.other"},
	}
}

func contactCaptchaDescriptor(r *http.Request, cfg *config.AppConfig) map[string]interface{} {
	provider := config.CaptchaSimple
	if cfg != nil {
		provider = cfg.Server.Pages.Contact.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
	}
	descriptor := map[string]interface{}{"provider": provider}
	newSimple := func() map[string]interface{} {
		id, a, b := contactCaptcha.New()
		return map[string]interface{}{"provider": config.CaptchaSimple, "captcha_id": id, "question": TranslateFormat(r, "contact.captcha.question", map[string]string{"a": strconv.Itoa(a), "b": strconv.Itoa(b)})}
	}
	if provider == config.CaptchaSimple {
		for key, value := range newSimple() {
			if key != "provider" {
				descriptor[key] = value
			}
		}
	} else {
		descriptor["site_key"] = cfg.Server.Pages.Contact.CaptchaSiteKey
		descriptor["fallback"] = newSimple()
	}
	return descriptor
}

// contactFields is the single definition behind the contact page and its API
// mirror: the same limits validateContactForm enforces.
func contactFields() []contactFieldSpec {
	return []contactFieldSpec{
		{Name: "name", Type: "text", Required: true, MaxLength: intPtr(contactNameMax)},
		{Name: "email", Type: "email", Required: true, MaxLength: intPtr(contactEmailMax)},
		{Name: "subject", Type: "text", Required: true, MaxLength: intPtr(contactSubjectMax)},
		{Name: "message", Type: "textarea", Required: true, MaxLength: intPtr(contactMessageMax)},
		{Name: "type", Type: "enum", Required: true, Options: []string{"abuse", "dmca", "privacy"}},
		{Name: "reported_url", Type: "url", RequiredWhen: &contactRequiredWhen{Field: "type", In: []string{"abuse", "dmca"}}, MaxLength: intPtr(2048)},
		{Name: "reason", Type: "enum", RequiredWhen: &contactRequiredWhen{Field: "type", Equals: "abuse"}, Options: []string{"illegal_content", "terms_violation", "harassment", "malware_phishing", "other"}},
		{Name: "copyrighted_work", Type: "textarea", RequiredWhen: &contactRequiredWhen{Field: "type", Equals: "dmca"}, MaxLength: intPtr(2000)},
		{Name: "good_faith", Type: "checkbox", RequiredWhen: &contactRequiredWhen{Field: "type", Equals: "dmca"}},
		{Name: "accuracy", Type: "checkbox", RequiredWhen: &contactRequiredWhen{Field: "type", Equals: "dmca"}},
		{Name: "signature", Type: "text", RequiredWhen: &contactRequiredWhen{Field: "type", Equals: "dmca"}, MaxLength: intPtr(100)},
		{Name: "request_kind", Type: "enum", RequiredWhen: &contactRequiredWhen{Field: "type", Equals: "privacy"}, Options: []string{"access", "export", "deletion", "correction", "opt_out", "other"}},
		{Name: "captcha", Type: "captcha", Required: true, MaxLength: nil},
	}
}

// GetContactFormAPI returns the contact form description as JSON: the field
// list with types, required flags and length limits, the captcha descriptor for
// the configured provider, the security reporting URL and the abuse address.
// Nothing else about recipients is exposed.
// GET /api/{api_version}/server/contact
func GetContactFormAPI(db *database.Store, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg == nil {
			cfg = config.GetGlobalConfig()
		}
		if cfg == nil || !cfg.Server.Pages.Contact.Enabled {
			http.NotFound(w, r)
			return
		}
		antispam, err := getContactAntispam(db, cfg)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "CONTACT_FORM_UNAVAILABLE"})
			return
		}
		formToken, err := antispam.NewContactFormToken(time.Now())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR"})
			return
		}
		captcha := contactCaptchaDescriptor(r, cfg)

		fields := contactFields()
		types := []string{"abuse", "dmca", "privacy"}
		if securityIDFromRequest(r, r.URL.Query().Get("security_id")) != "" {
			fields = securityFieldSpecs()
			types = []string{"security"}
		}
		RespondData(w, r, map[string]interface{}{
			"fields":       fields,
			"types":        types,
			"captcha":      captcha,
			"form_token":   formToken,
			"security_url": util.BuildURL(r, "/server/security"),
			"abuse_email":  cfg.Server.Contact.Abuse.Email,
		})
	}
}
