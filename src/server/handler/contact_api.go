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

		RespondData(w, r, map[string]interface{}{
			"fields":       contactFields(),
			"captcha":      captcha,
			"security_url": util.BuildURL(r, "/server/security"),
			"abuse_email":  cfg.Server.Contact.Abuse.Email,
		})
	}
}
