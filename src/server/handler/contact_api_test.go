package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestContactFieldsMatchValidationLimits(t *testing.T) {
	limits := map[string]int{"name": contactNameMax, "email": contactEmailMax, "subject": contactSubjectMax, "message": contactMessageMax}
	for _, field := range contactFields() {
		want, limited := limits[field.Name]
		if !limited {
			if field.MaxLength != nil {
				t.Errorf("%s max_length = %d, want null", field.Name, *field.MaxLength)
			}
			continue
		}
		if field.MaxLength == nil || *field.MaxLength != want {
			t.Errorf("%s max_length = %v, want %d", field.Name, field.MaxLength, want)
		}
	}
}

func TestValidateContactFormCountsCharactersNotBytes(t *testing.T) {
	valid := contactForm{Name: strings.Repeat("é", contactNameMax), Email: "a@example.com", Subject: "s", Message: "m"}
	if !validateContactForm(&valid) {
		t.Error("100 two-byte characters must fit a 100-character name limit")
	}
	tooLong := contactForm{Name: strings.Repeat("é", contactNameMax+1), Email: "a@example.com", Subject: "s", Message: "m"}
	if validateContactForm(&tooLong) {
		t.Error("101 characters must exceed the name limit")
	}
	longEmail := contactForm{Name: "n", Email: strings.Repeat("a", 250) + "@example.com", Subject: "s", Message: "m"}
	if validateContactForm(&longEmail) {
		t.Error("an email longer than 254 characters must be rejected")
	}
}

func TestGetContactFormAPIDescribesTheForm(t *testing.T) {
	t.Setenv("DOMAIN", "wthr.example")
	cfg := &config.AppConfig{}
	cfg.Server.Contact.Abuse.Email = "abuse@example.com"
	cfg.Server.Contact.Admin.Email = "private-admin@example.com"

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/contact", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	GetContactFormAPI(cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-admin@example.com") {
		t.Fatal("the contact API must not expose any recipient other than the abuse address")
	}

	var body struct {
		Fields []struct {
			Name      string `json:"name"`
			Required  bool   `json:"required"`
			MaxLength *int   `json:"max_length"`
		} `json:"fields"`
		Captcha     map[string]interface{} `json:"captcha"`
		SecurityURL string                 `json:"security_url"`
		AbuseEmail  string                 `json:"abuse_email"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v\n%s", err, w.Body.String())
	}
	if len(body.Fields) != 5 || body.Fields[0].Name != "name" || body.Fields[3].MaxLength == nil || *body.Fields[3].MaxLength != 5000 {
		t.Errorf("fields = %+v", body.Fields)
	}
	if body.Captcha["provider"] != "simple" || body.Captcha["challenge_id"] == "" || body.Captcha["question"] == "" {
		t.Errorf("captcha = %v, want the simple provider with challenge_id and question", body.Captcha)
	}
	if body.AbuseEmail != "abuse@example.com" || !strings.HasPrefix(body.SecurityURL, "http://wthr.example") || !strings.HasSuffix(body.SecurityURL, "/server/security") {
		t.Errorf("security_url=%q abuse_email=%q", body.SecurityURL, body.AbuseEmail)
	}
}

func TestHandleAuthCheck(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Token = "operator-secret-value"
	handler := HandleAuthCheck(nil, nil, cfg)

	t.Run("missing token is 401 without invalid_token", func(t *testing.T) {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(http.MethodGet, "/api/v1/server/auth/check", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
		challenge := w.Header().Get("WWW-Authenticate")
		if !strings.HasPrefix(challenge, "Bearer realm=") || strings.Contains(challenge, "invalid_token") {
			t.Errorf("WWW-Authenticate = %q", challenge)
		}
	})

	t.Run("operator token is kind operator and never echoed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/server/auth/check", nil)
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Authorization", "Bearer operator-secret-value")
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"operator"`) {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "operator-secret-value") {
			t.Error("the response must never echo the token")
		}
	})

	t.Run("wrong token is 401 invalid_token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/server/auth/check", nil)
		r.Header.Set("Authorization", "Bearer something-else")
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusUnauthorized || !strings.Contains(w.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
			t.Fatalf("status=%d challenge=%q", w.Code, w.Header().Get("WWW-Authenticate"))
		}
	})

	t.Run("non-bearer schemes are ignored", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/server/auth/check", nil)
		r.Header.Set("Authorization", "Basic b3BlcmF0b3Itc2VjcmV0LXZhbHVl")
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

func TestInvalidContactFieldNamesTheFirstOffender(t *testing.T) {
	cases := map[string]contactForm{
		"name":    {Name: "", Email: "a@example.com", Subject: "s", Message: "m"},
		"email":   {Name: "n", Email: "not-an-email", Subject: "s", Message: "m"},
		"subject": {Name: "n", Email: "a@example.com", Subject: strings.Repeat("s", contactSubjectMax+1), Message: "m"},
		"message": {Name: "n", Email: "a@example.com", Subject: "s", Message: strings.Repeat("m", contactMessageMax+1)},
	}
	for want, form := range cases {
		form := form
		if got := invalidContactField(&form); got != want {
			t.Errorf("invalidContactField() = %q, want %q", got, want)
		}
	}
	ok := contactForm{Name: " n ", Email: "a@example.com", Subject: "s", Message: "m"}
	if got := invalidContactField(&ok); got != "" || ok.Name != "n" {
		t.Errorf("valid form: field=%q trimmed name=%q", got, ok.Name)
	}
}
