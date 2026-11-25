package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"

	"github.com/webappsgo/wthr/src/server/reqctx"
)

func newServerPagesTestDB(t *testing.T) *database.Store {
	t.Helper()
	db := newTestDatabaseDB(t)
	setGlobalTestDualDB(t, db.DB(), db.DB())
	return db
}

func newServerPagesTestConfig() *config.AppConfig {
	return &config.AppConfig{
		Server: config.ServerConfig{
			Mode:  "production",
			Pages: config.DefaultPagesConfig(),
			Branding: config.BrandingConfig{
				Title:       "Weather",
				Description: "Weather forecasts and alerts",
			},
			Admin: config.AdminConfig{Email: ""},
		},
	}
}

// newJSONNegotiatedContext builds a GET request that forces JSON content
// negotiation (Accept: application/json), so NegotiateResponse/
// RespondNegotiatedData never fall through to the HTML template branch
// (templates aren't loaded in this unit-test environment). Used for both the
// page handlers (ShowXxxPage) and the API handlers (GetXxxAPI) in this file,
// since both negotiate on the same Accept header.
func newJSONNegotiatedContext(target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Header.Set("Accept", "application/json")
	return r, w
}

func TestShowExamplesPage(t *testing.T) {
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/examples")
	ShowExamplesPage(cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowAboutPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/about")
	ShowAboutPage(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowPrivacyPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/privacy")
	ShowPrivacyPage(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowContactPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/contact")
	ShowContactPage(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowHelpPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/help")
	ShowHelpPage(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowTermsPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/terms")
	ShowTermsPage(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetAboutAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/about")
	GetAboutAPI(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title": "Weather"`) {
		t.Errorf("body = %s, want it to contain the branding title", w.Body.String())
	}
}

func TestGetPrivacyAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/privacy")
	GetPrivacyAPI(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetPrivacyAPIUsesConfiguredPolicy(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	cfg.Server.Privacy = config.DefaultPrivacyConfig()
	cfg.Server.Privacy.Data.Sold = true
	cfg.Server.Privacy.Content.DataUsageIfSold = "Configured sold-data policy"
	cfg.Server.Privacy.ThirdParty.Services = []config.PrivacyThirdPartyService{{Name: "Example", Purpose: "weather", Data: "city query", URL: "https://example.test/privacy"}}
	cfg.Server.Tracking.Type = "plausible"
	r, w := newJSONNegotiatedContext("/api/v1/server/privacy")
	r.AddCookie(&http.Cookie{Name: "ccpa_opt_out", Value: "true"})
	GetPrivacyAPI(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := w.Body.String()
	for _, expected := range []string{`"data_sold": true`, `"type": "plausible"`, `"data_usage": "Configured sold-data policy"`, `"name": "Example"`, `"user_opted_out": true`} {
		if !strings.Contains(body, expected) {
			t.Errorf("privacy response is missing %s: %s", expected, body)
		}
	}
}

func TestGetHelpAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/help")
	GetHelpAPI(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetTermsAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/terms")
	GetTermsAPI(storeForDB(db), cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

// validContactBody returns a complete contact submission carrying a freshly solved built-in captcha.
func validContactBody() map[string]interface{} {
	id, a, b := contactCaptcha.New()
	return map[string]interface{}{
		"type": "privacy", "request_kind": "access", "name": "Ada", "email": "ada@example.com", "subject": "Hello", "message": "Test message",
		"captcha_id": id, "captcha_answer": fmt.Sprintf("%d", a+b),
	}
}

func TestHandleContactFormSubmission(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	cfg.Server.Pages.Contact.Antispam.ReviewMinSeconds = 0

	t.Run("invalid form data returns 400", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{"name": "Ada"})
		HandleContactFormSubmission(storeForDB(db), cfg)(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
	})

	t.Run("first call reviews and confirm saves exactly once", func(t *testing.T) {
		manager, err := getContactAntispam(db, cfg)
		if err != nil {
			t.Fatal(err)
		}
		formToken, err := manager.NewContactFormToken(time.Now().Add(-5 * time.Second))
		if err != nil {
			t.Fatal(err)
		}
		first := validContactBody()
		first["form_token"] = formToken
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", first)
		HandleContactFormSubmission(storeForDB(db), cfg)(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		var firstResponse struct {
			Data struct {
				Status      string `json:"status"`
				FormToken   string `json:"form_token"`
				ReviewToken string `json:"review_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &firstResponse); err != nil {
			t.Fatal(err)
		}
		if firstResponse.Data.Status != "review" || firstResponse.Data.ReviewToken == "" || firstResponse.Data.FormToken == "" {
			t.Fatalf("first response = %s", w.Body.String())
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM contact_submissions WHERE email = ?`, "ada@example.com").Scan(&count); err != nil {
			t.Fatalf("query contact_submissions: %v", err)
		}
		if count != 0 {
			t.Fatalf("first step delivered %d messages", count)
		}
		second := validContactBody()
		second["step"], second["confirmed"] = "confirm", true
		second["form_token"], second["review_token"] = firstResponse.Data.FormToken, firstResponse.Data.ReviewToken
		r, w = newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", second)
		r = r.WithContext(reqctx.SetValue(r.Context(), "db", storeForDB(db)))
		HandleContactFormSubmission(storeForDB(db), cfg)(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body=%s", w.Code, w.Body.String())
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM contact_submissions WHERE email = ?`, "ada@example.com").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("contact_submissions count = %d, want 1", count)
		}
	})
}

func TestSaveContactToDBMissingSchema(t *testing.T) {
	dsn := fmt.Sprintf("file:handler_contact_noschema_%d?mode=memory&cache=shared", time.Now().UnixNano())
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	t.Cleanup(func() {
		if err := raw.Close(); err != nil {
			t.Errorf("close raw test database: %v", err)
		}
	})

	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", nil)
	r = r.WithContext(reqctx.SetValue(r.Context(), "db", storeForDB(raw)))

	err = saveContactToDB(r, "Ada", "ada@example.com", "Hello", "Test message")
	if err == nil {
		t.Fatal("saveContactToDB with a schema missing contact_submissions returned nil error, want an error")
	}
	if !strings.Contains(err.Error(), "contact_submissions") {
		t.Errorf("error = %v, want it to mention contact_submissions", err)
	}

	var name string
	scanErr := raw.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'contact_submissions'`).Scan(&name)
	if scanErr == nil {
		t.Error("contact_submissions table unexpectedly exists")
	} else if scanErr != sql.ErrNoRows {
		t.Fatalf("unexpected error checking for contact_submissions table: %v", scanErr)
	}
}

func TestGetSMTPService(t *testing.T) {
	t.Run("missing from context returns nil", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		if got := GetSMTPService(r); got != nil {
			t.Errorf("GetSMTPService() = %v, want nil", got)
		}
	})

	t.Run("wrong type in context returns nil", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r = r.WithContext(reqctx.SetValue(r.Context(), "smtp", "not-a-service"))
		if got := GetSMTPService(r); got != nil {
			t.Errorf("GetSMTPService() = %v, want nil", got)
		}
	})
}
