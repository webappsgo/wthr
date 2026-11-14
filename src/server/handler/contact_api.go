package handler

import (
	"net/http"
	"strconv"

	"github.com/webappsgo/wthr/src/config"
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
	Options []string `json:"-"`
}

// contactRequiredWhen is the PART 11 required_when descriptor.
type contactRequiredWhen struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// contactMaxLengthWhen is one PART 11 max_length_when entry.
type contactMaxLengthWhen struct {
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"max_length"`
}

func intPtr(v int) *int { return &v }

// contactFields is the single definition behind the contact page and its API
// mirror: the same limits validateContactForm enforces.
func contactFields() []contactFieldSpec {
	return []contactFieldSpec{
		{Name: "name", Type: "text", Required: true, MaxLength: intPtr(contactNameMax)},
		{Name: "email", Type: "email", Required: true, MaxLength: intPtr(contactEmailMax)},
		{Name: "subject", Type: "text", Required: true, MaxLength: intPtr(contactSubjectMax)},
		{Name: "message", Type: "textarea", Required: true, MaxLength: intPtr(contactMessageMax)},
		{Name: "captcha", Type: "captcha", Required: true, MaxLength: nil},
	}
}

// GetContactFormAPI returns the contact form description as JSON: the field
// list with types, required flags and length limits, the captcha descriptor for
// the configured provider, the security reporting URL and the abuse address.
// Nothing else about recipients is exposed.
// GET /api/{api_version}/server/contact
func GetContactFormAPI(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := cfg.Server.Pages.Contact.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
		captcha := map[string]interface{}{"provider": provider}
		if provider == config.CaptchaSimple {
			id, a, b := contactCaptcha.New()
			captcha["challenge_id"] = id
			captcha["question"] = TranslateFormat(r, "contact.captcha.question", map[string]string{
				"a": strconv.Itoa(a),
				"b": strconv.Itoa(b),
			})
		} else {
			captcha["site_key"] = cfg.Server.Pages.Contact.CaptchaSiteKey
		}

		fields := contactFields()
		if securityIDFromRequest(r, r.URL.Query().Get("security_id")) != "" {
			fields = securityFieldSpecs()
		}
		RespondData(w, r, map[string]interface{}{
			"fields":       fields,
			"captcha":      captcha,
			"security_url": util.BuildURL(r, "/server/security"),
			"abuse_email":  cfg.Server.Contact.Abuse.Email,
		})
	}
}
