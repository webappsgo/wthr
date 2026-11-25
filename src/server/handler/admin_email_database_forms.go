package handler

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
)

// TestSMTPEmailForm sends the configured SMTP test message to the signed-in
// administrator's email address using a normal session-authenticated form.
func TestSMTPEmailForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.test_email_sent", "flash.test_email_failed", err)
		return
	}
	recipient := strings.TrimSpace(r.FormValue("recipient"))
	parsed, err := mail.ParseAddress(recipient)
	if err != nil || parsed.Address != recipient {
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.test_email_sent", "flash.test_email_failed", fmt.Errorf("invalid recipient address"))
		return
	}
	smtp := service.SharedSMTPService(database.GetServerStore())
	if err := smtp.VerifyConfiguredConnection(); err != nil {
		log.Printf("WARNING: SMTP test email connection verification failed: %v", err)
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.test_email_sent", "flash.test_email_failed", err)
		return
	}
	if err := smtp.SendTestEmail(recipient); err != nil {
		log.Printf("WARNING: SMTP test email delivery failed: %v", err)
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.test_email_sent", "flash.test_email_failed", err)
		return
	}
	redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.test_email_sent", "flash.test_email_failed", nil)
}

// AutoDetectSMTPForm runs the configured SMTP discovery and returns to the
// email page with a one-shot status message.
func AutoDetectSMTPForm(w http.ResponseWriter, r *http.Request) {
	smtp := service.SharedSMTPService(database.GetServerStore())
	found, err := smtp.AutoDetect()
	if err == nil && !found {
		err = fmt.Errorf("no SMTP server detected")
	}
	if err == nil {
		err = smtp.VerifyConfiguredConnection()
	}
	redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.settings_saved", "flash.settings_save_failed", err)
}

// DatabaseActionForm dispatches the existing, bounded database/cache
// operations from a session-authenticated HTML form and uses POST/redirect/GET.
func DatabaseActionForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	var operation func(http.ResponseWriter, *http.Request)
	switch r.FormValue("operation") {
	case "test":
		operation = TestDatabaseConnection
	case "optimize":
		operation = OptimizeDatabase
	case "vacuum":
		operation = VacuumDatabase
	case "clear-cache":
		operation = ClearCache
	default:
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("unknown database operation"))
		return
	}
	result := httptest.NewRecorder()
	operation(result, r)
	if result.Code < http.StatusOK || result.Code >= http.StatusMultipleChoices {
		log.Printf("WARNING: admin database %s operation failed: HTTP %d", r.FormValue("operation"), result.Code)
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("database operation failed"))
		return
	}
	successKey := "flash.settings_saved"
	if r.FormValue("operation") == "vacuum" {
		successKey = "admin.database.vacuum_queued"
	}
	redirectAdminForm(w, r, adminPagePath(r, "/database"), successKey, "flash.settings_save_failed", nil)
}

// TestDatabaseConfigForm validates connection settings entered in the database
// settings form without saving them. It matches the existing API's validation
// contract and never includes credentials in the response or redirect.
func TestDatabaseConfigForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	driver := strings.ToLower(strings.TrimSpace(r.FormValue("database.driver")))
	if driver == "file" || driver == "sqlite" {
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", nil)
		return
	}
	port, err := strconv.Atoi(r.FormValue("database.port"))
	if err != nil || r.FormValue("database.host") == "" || port < 1 || port > 65535 || r.FormValue("database.name") == "" {
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("incomplete database connection settings"))
		return
	}
	redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", nil)
}
