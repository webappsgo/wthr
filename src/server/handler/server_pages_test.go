package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
)

// newServerPagesTestDB wraps newTestDatabaseDB and also wires the global
// dual-DB, since model.SettingsModel.Get ignores its injected DB field and
// reads from database.GetServerDB() (the global accessor) instead — without
// this, GetBool/GetString silently fall back to their zero-value defaults
// via the "not found" error path rather than actually panicking, EXCEPT
// when the global dual-DB has never been set at all, in which case
// database.GetServerDB() returns a nil *sql.DB and the very first query
// panics. See the flagged-not-fixed production issue in the final report.
func newServerPagesTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := newTestDatabaseDB(t)
	setGlobalTestDualDB(t, db.DB, db.DB)
	return db
}

// newServerPagesTestConfig returns a minimal AppConfig sufficient for the
// server_pages.go handlers, which only read Server.Branding, Server.Mode,
// and Server.Admin.Email.
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
// RespondNegotiatedData never fall through to the c.HTML template branch
// (templates aren't loaded in this unit-test environment).
func newJSONNegotiatedContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	c.Request.Header.Set("Accept", "application/json")
	return c, w
}

func TestShowAboutPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newJSONNegotiatedContext("/server/about")

	ShowAboutPage(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestShowPrivacyPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newJSONNegotiatedContext("/server/privacy")

	ShowPrivacyPage(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestShowContactPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newJSONNegotiatedContext("/server/contact")

	ShowContactPage(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestShowHelpPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newJSONNegotiatedContext("/server/help")

	ShowHelpPage(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestShowTermsPage(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newJSONNegotiatedContext("/server/terms")

	ShowTermsPage(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestGetAboutAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newAPITestContext("/api/v1/server/about")

	GetAboutAPI(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title":"Weather"`) {
		t.Errorf("expected title in response, got: %s", w.Body.String())
	}
}

func TestGetPrivacyAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newAPITestContext("/api/v1/server/privacy")

	GetPrivacyAPI(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title":"Privacy Policy"`) {
		t.Errorf("expected title in response, got: %s", w.Body.String())
	}
}

func TestGetHelpAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newAPITestContext("/api/v1/server/help")

	GetHelpAPI(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title":"Help"`) {
		t.Errorf("expected title in response, got: %s", w.Body.String())
	}
}

func TestGetTermsAPI(t *testing.T) {
	db := newServerPagesTestDB(t)
	cfg := newServerPagesTestConfig()
	c, w := newAPITestContext("/api/v1/server/terms")

	GetTermsAPI(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title":"Terms of Service"`) {
		t.Errorf("expected title in response, got: %s", w.Body.String())
	}
}

func TestHandleContactFormSubmission(t *testing.T) {
	cfg := newServerPagesTestConfig()

	t.Run("invalid form data returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{
			"name": "Ada",
		})

		HandleContactFormSubmission(nil, cfg)(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no smtp and no db saves and returns 500", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{
			"name":    "Ada",
			"email":   "ada@example.com",
			"subject": "Hello",
			"message": "Test message",
		})

		HandleContactFormSubmission(nil, cfg)(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no smtp with db saves successfully", func(t *testing.T) {
		db := newServerPagesTestDB(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", map[string]interface{}{
			"name":    "Ada",
			"email":   "ada@example.com",
			"subject": "Hello",
			"message": "Test message",
		})
		c.Set("db", db)

		HandleContactFormSubmission(db, cfg)(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}

		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM contact_submissions WHERE email = ?`, "ada@example.com").Scan(&count); err != nil {
			t.Fatalf("query contact_submissions: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 saved submission, got %d", count)
		}
	})
}

// TestSaveContactToDBMissingSchema pins the behaviour that replaced the
// request-time CREATE TABLE: saveContactToDB no longer creates
// contact_submissions itself, so a database that never had ServerSchema
// applied must fail with an error that names the missing table rather than
// silently creating a second, divergent definition of it.
func TestSaveContactToDBMissingSchema(t *testing.T) {
	dsn := fmt.Sprintf("file:handler_contact_noschema_%d?mode=memory&cache=shared", time.Now().UnixNano())
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open schemaless db: %v", err)
	}
	t.Cleanup(func() { raw.Close() })

	c, _ := newAPITestContext("/api/v1/server/contact")
	c.Set("db", &database.DB{DB: raw})

	err = saveContactToDB(c, "Ada", "ada@example.com", "Hello", "Test message")
	if err == nil {
		t.Fatal("expected an error when contact_submissions is missing, got nil")
	}
	if !strings.Contains(err.Error(), "contact_submissions") {
		t.Errorf("error should name the missing table, got: %v", err)
	}

	// The handler must not have created the table as a side effect.
	var name string
	scanErr := raw.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'contact_submissions'`).Scan(&name)
	if scanErr == nil {
		t.Error("saveContactToDB created contact_submissions; schema is owned by database.ServerSchema only")
	} else if scanErr != sql.ErrNoRows {
		t.Fatalf("inspect sqlite_master: %v", scanErr)
	}
}

func TestGetSMTPService(t *testing.T) {
	t.Run("missing from context returns nil", func(t *testing.T) {
		c, _ := newAPITestContext("/x")

		if got := GetSMTPService(c); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("wrong type in context returns nil", func(t *testing.T) {
		c, _ := newAPITestContext("/x")
		c.Set("smtp", "not-a-service")

		if got := GetSMTPService(c); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}
