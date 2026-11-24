package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/model"
)

func adminFormRequest(target string, values url.Values) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func assertAdminFormRedirect(t *testing.T, w *httptest.ResponseRecorder, target string) {
	t.Helper()
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != target {
		t.Fatalf("Location = %q, want %q", got, target)
	}
}

func TestSaveEmailFormValidatesAndPersists(t *testing.T) {
	newAdminTestHandler(t)
	settings := &model.SettingsModel{}
	invalid := adminFormRequest("/server/administration/email", url.Values{"smtp.port": {"70000"}})
	w := httptest.NewRecorder()
	SaveEmailForm(w, invalid)
	assertAdminFormRedirect(t, w, "/server/administration/email")

	valid := adminFormRequest("/server/administration/email", url.Values{
		"smtp.host": {"mail.example"}, "smtp.port": {"587"}, "smtp.username": {"mailer"},
		"smtp.tls": {"starttls"}, "smtp.from_address": {"weather@example.test"}, "smtp.from_name": {"Weather"}, "smtp.enabled": {"on"},
	})
	w = httptest.NewRecorder()
	SaveEmailForm(w, valid)
	assertAdminFormRedirect(t, w, "/server/administration/email")
	if got := settings.GetString("smtp.host", ""); got != "mail.example" {
		t.Fatalf("smtp.host = %q", got)
	}
	if got := settings.GetString("smtp.enabled", ""); got != "true" {
		t.Fatalf("smtp.enabled = %q", got)
	}
}

func TestSaveDatabaseAndMetricsForms(t *testing.T) {
	newAdminTestHandler(t)
	settings := &model.SettingsModel{}
	invalidDB := adminFormRequest("/server/administration/database", url.Values{"database.port": {"0"}})
	w := httptest.NewRecorder()
	SaveDatabaseForm(w, invalidDB)
	assertAdminFormRedirect(t, w, "/server/administration/database")

	validDB := adminFormRequest("/server/administration/database", url.Values{
		"database.driver": {"postgres"}, "database.host": {"db.example"}, "database.port": {"5432"}, "database.name": {"weather"},
		"database.username": {"wthr"}, "database.sslmode": {"require"}, "database.pool_size": {"10"}, "database.timeout": {"30s"},
	})
	w = httptest.NewRecorder()
	SaveDatabaseForm(w, validDB)
	assertAdminFormRedirect(t, w, "/server/administration/database")
	if got := settings.GetString("database.host", ""); got != "db.example" {
		t.Fatalf("database.host = %q", got)
	}

	previous := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	config.SetGlobalConfig(&config.AppConfig{Server: config.ServerConfig{Metrics: config.DefaultMetricsConfig()}})
	invalidMetrics := adminFormRequest("/server/administration/metrics", url.Values{"loki_max_entries": {"0"}})
	w = httptest.NewRecorder()
	SaveMetricsForm(w, invalidMetrics)
	assertAdminFormRedirect(t, w, "/server/administration/metrics")
	validMetrics := adminFormRequest("/server/administration/metrics", url.Values{"enabled": {"on"}, "loki_max_entries": {"1000"}, "loki_max_age": {"24h"}})
	w = httptest.NewRecorder()
	SaveMetricsForm(w, validMetrics)
	assertAdminFormRedirect(t, w, "/server/administration/metrics")
	if cfg := config.GetGlobalConfig(); !cfg.Server.Metrics.Enabled || cfg.Server.Metrics.Loki.MaxEntries != 1000 {
		t.Fatalf("metrics config was not updated: %#v", cfg.Server.Metrics)
	}
}

func TestAdminEmailDatabaseAndBackupFormsValidate(t *testing.T) {
	newAdminTestHandler(t)

	badRecipient := adminFormRequest("/server/administration/email/test", url.Values{"recipient": {"not an address"}})
	w := httptest.NewRecorder()
	TestSMTPEmailForm(w, badRecipient)
	assertAdminFormRedirect(t, w, "/server/administration/email")

	badDatabase := adminFormRequest("/server/administration/database/test", url.Values{"database.driver": {"postgres"}, "database.port": {"5432"}})
	w = httptest.NewRecorder()
	TestDatabaseConfigForm(w, badDatabase)
	assertAdminFormRedirect(t, w, "/server/administration/database")
	validSQLite := adminFormRequest("/server/administration/database/test", url.Values{"database.driver": {"sqlite"}})
	w = httptest.NewRecorder()
	TestDatabaseConfigForm(w, validSQLite)
	assertAdminFormRedirect(t, w, "/server/administration/database")
	validRemote := adminFormRequest("/server/administration/database/test", url.Values{
		"database.driver": {"postgres"}, "database.host": {"db.example"}, "database.port": {"5432"}, "database.name": {"weather"},
	})
	w = httptest.NewRecorder()
	TestDatabaseConfigForm(w, validRemote)
	assertAdminFormRedirect(t, w, "/server/administration/database")
	unknownOperation := adminFormRequest("/server/administration/database/action", url.Values{"operation": {"unknown"}})
	w = httptest.NewRecorder()
	DatabaseActionForm(w, unknownOperation)
	assertAdminFormRedirect(t, w, "/server/administration/database")

	badSchedule := adminFormRequest("/server/administration/backup/schedule", url.Values{"interval": {"0"}, "retention": {"5"}})
	w = httptest.NewRecorder()
	SaveBackupScheduleForm(w, badSchedule)
	assertAdminFormRedirect(t, w, "/server/administration/backup")
	validSchedule := adminFormRequest("/server/administration/backup/schedule", url.Values{"enabled": {"on"}, "interval": {"24"}, "retention": {"5"}})
	w = httptest.NewRecorder()
	SaveBackupScheduleForm(w, validSchedule)
	assertAdminFormRedirect(t, w, "/server/administration/backup")

	invalidRestore := httptest.NewRequest(http.MethodPost, "/server/administration/backup/restore", strings.NewReader("broken multipart"))
	invalidRestore.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
	w = httptest.NewRecorder()
	RestoreBackupForm(w, invalidRestore)
	assertAdminFormRedirect(t, w, "/server/administration/backup")
}
