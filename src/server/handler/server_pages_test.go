package handler

import (
	"database/sql"
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

func newServerPagesTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := newTestDatabaseDB(t)
	setGlobalTestDualDB(t, db.DB, db.DB)
	return db
}

func newServerPagesTestConfig() *config.AppConfig {
	return &config.AppConfig{
		Server: config.ServerConfig{
			Mode: "production",
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

func TestShowAboutPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/about")
	ShowAboutPage(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowPrivacyPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/privacy")
	ShowPrivacyPage(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowContactPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/contact")
	ShowContactPage(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowHelpPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/help")
	ShowHelpPage(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestShowTermsPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/server/terms")
	ShowTermsPage(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetAboutAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/about")
	GetAboutAPI(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title":"Weather"`) {
		t.Errorf("body = %s, want it to contain the branding title", w.Body.String())
	}
}

func TestGetPrivacyAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/privacy")
	GetPrivacyAPI(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetHelpAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/help")
	GetHelpAPI(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetTermsAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	r, w := newJSONNegotiatedContext("/api/v1/server/terms")
	GetTermsAPI(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleContactFormSubmission(t *testing.T) {
	cfg := newServerPagesTestConfig()

	t.Run("invalid form data returns 400", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{"name": "Ada"})
		HandleContactFormSubmission(nil, cfg)(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
	})

	t.Run("no smtp and no db saves and returns 500", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{
			"name": "Ada", "email": "ada@example.com", "subject": "Hello", "message": "Test message",
		})
		HandleContactFormSubmission(nil, cfg)(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
		}
	})

	t.Run("no smtp with db saves successfully", func(t *testing.T) {
		db := newServerPagesTestDB(t)
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{
			"name": "Ada", "email": "ada@example.com", "subject": "Hello", "message": "Test message",
		})
		r = r.WithContext(reqctx.Set(r.Context(), "db", db))

		HandleContactFormSubmission(db, cfg)(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM contact_submissions WHERE email = ?`, "ada@example.com").Scan(&count); err != nil {
			t.Fatalf("query contact_submissions: %v", err)
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
	t.Cleanup(func() { raw.Close() })

	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", nil)
	r = r.WithContext(reqctx.Set(r.Context(), "db", &database.DB{DB: raw}))

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
		r = r.WithContext(reqctx.Set(r.Context(), "smtp", "not-a-service"))
		if got := GetSMTPService(r); got != nil {
			t.Errorf("GetSMTPService() = %v, want nil", got)
		}
	})
}
