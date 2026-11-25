package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// adminSecurityPath is the admin section that holds the disclosure feature (PART 11).
const adminSecurityPath = "/security"

// renderAdminSecurity renders one admin disclosure page.
func renderAdminSecurity(w http.ResponseWriter, r *http.Request, tmpl, page string, data map[string]interface{}) {
	data["page"] = page
	data["flash"] = TakeFlash(w, r)
	data["title"] = Translate(r, "admin.security_reports.nav_"+page)
	middleware.RenderHTML(w, r, http.StatusOK, tmpl, AdminTemplateData(r, data))
}

// ShowSecurityReports lists reports, filterable by status.
// GET /server/{admin_path}/security/reports
func ShowSecurityReports(w http.ResponseWriter, r *http.Request) {
	store := newSecurityStore()
	status := r.URL.Query().Get("status")
	if status != "" && !service.ValidStatus(status) {
		status = ""
	}
	var rows []*service.SecurityReportRow
	if store != nil {
		rows, _ = store.List(status)
	}
	renderAdminSecurity(w, r, "admin/admin_security_reports.tmpl", "reports", map[string]interface{}{
		"Reports": rows, "Filter": status, "Statuses": []string{"received", "triaged", "confirmed", "patching", "disclosed", "wont_fix"},
	})
}

// ShowSecurityReport shows one report with its decrypted body. Every view is audited, never logged with content.
// GET /server/{admin_path}/security/reports/{tracking_id}
func ShowSecurityReport(w http.ResponseWriter, r *http.Request) {
	store := newSecurityStore()
	if store == nil {
		http.NotFound(w, r)
		return
	}
	id := chi.URLParam(r, "tracking_id")
	row, err := store.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, contact, bodyErr := store.Body(id)
	logSecurityAudit(r, "security.report_viewed", id, "")
	renderAdminSecurity(w, r, "admin/admin_security_report.tmpl", "reports", map[string]interface{}{
		"Report": row, "Body": body, "Contact": contact, "BodyUnreadable": bodyErr != nil,
		"Statuses": []string{"received", "triaged", "confirmed", "patching", "disclosed", "wont_fix"},
	})
}

// UpdateSecurityReport changes status, researcher comment and disclosure date, then emails the
// researcher a content-free status notice when SMTP works.
// POST /server/{admin_path}/security/reports/{tracking_id}
func UpdateSecurityReport(w http.ResponseWriter, r *http.Request) {
	store := newSecurityStore()
	id := chi.URLParam(r, "tracking_id")
	back := adminPagePath(r, adminSecurityPath+"/reports/"+id)
	if store == nil || r.ParseForm() != nil {
		redirectAdminForm(w, r, back, "flash.security_report_updated", "flash.security_report_update_failed", errors.New("bad request"))
		return
	}
	old, err := store.Update(id, r.PostFormValue("status"), strings.TrimSpace(r.PostFormValue("researcher_comment")), strings.TrimSpace(r.PostFormValue("disclosure_date")))
	if err == nil {
		newStatus := r.PostFormValue("status")
		logSecurityEvent(r, "security.report_updated", fmt.Sprintf("tracking_id=%s old=%s new=%s", id, old, newStatus))
		if newStatus == "disclosed" && old != "disclosed" {
			_ = store.AddThanks(id)
		}
		if newStatus != old {
			notifyResearcherStatus(r, store, id, newStatus)
		}
	}
	redirectAdminForm(w, r, back, "flash.security_report_updated", "flash.security_report_update_failed", err)
}

// notifyResearcherStatus sends the status-change notice; it never carries report content.
func notifyResearcherStatus(r *http.Request, store *service.SecurityReportStore, id, status string) {
	smtp := GetSMTPService(r)
	if smtp == nil {
		return
	}
	_, contact, err := store.Body(id)
	if err != nil || contact == nil || contact.Email == "" {
		return
	}
	body := fmt.Sprintf("%s\n\n%s: %s\n%s: %s", Translate(r, "email.security_report.status_intro"),
		Translate(r, "email.security_report.label_tracking"), id,
		Translate(r, "email.security_report.label_status"), Translate(r, "security.report.state."+status))
	if contact.PGPKey != "" {
		if sealed, sealErr := service.EncryptToArmoredKey(contact.PGPKey, []byte(body)); sealErr == nil {
			body = string(sealed)
		}
	}
	_ = smtp.SendEmail(contact.Email, Translate(r, "email.security_report.status_subject"), body)
}

// ShowSecurityPolicyEditor edits the disclosure policy page.
// GET /server/{admin_path}/security/policy
func ShowSecurityPolicyEditor(w http.ResponseWriter, r *http.Request) {
	renderAdminSecurity(w, r, "admin/admin_security_policy.tmpl", "policy", map[string]interface{}{"Policy": securityPolicyText()})
}

// UpdateSecurityPolicy saves the disclosure policy (Markdown, at most 20000 characters).
// POST /server/{admin_path}/security/policy
func UpdateSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	back := adminPagePath(r, adminSecurityPath+"/policy")
	text := strings.TrimSpace(r.PostFormValue("policy"))
	var err error
	switch db := database.GetServerStore(); {
	case db == nil, len([]rune(text)) > 20000:
		err = errors.New("invalid policy")
	default:
		err = (&model.SettingsModel{Store: db}).SetString("security.policy_text", text)
	}
	redirectAdminForm(w, r, back, "flash.settings_saved", "flash.settings_save_failed", err)
}

// ShowSecurityThanksEditor lists every hall-of-fame entry for the admin.
// GET /server/{admin_path}/security/thanks
func ShowSecurityThanksEditor(w http.ResponseWriter, r *http.Request) {
	var entries []service.ThanksEntry
	if store := newSecurityStore(); store != nil {
		entries, _ = store.Thanks(false)
	}
	renderAdminSecurity(w, r, "admin/admin_security_thanks.tmpl", "thanks", map[string]interface{}{"Entries": entries})
}

// UpdateSecurityThanks edits one entry (credit, visibility, sort order).
// POST /server/{admin_path}/security/thanks
func UpdateSecurityThanks(w http.ResponseWriter, r *http.Request) {
	back := adminPagePath(r, adminSecurityPath+"/thanks")
	store := newSecurityStore()
	id, _ := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	order, _ := strconv.Atoi(r.PostFormValue("sort_order"))
	err := errors.New("unavailable")
	if store != nil && id > 0 {
		err = store.UpdateThanks(id, strings.TrimSpace(r.PostFormValue("credit")), config.IsTruthy(r.PostFormValue("visible")), order)
	}
	redirectAdminForm(w, r, back, "flash.settings_saved", "flash.settings_save_failed", err)
}

// ShowSecurityKeypair shows the keypair state and its actions.
// GET /server/{admin_path}/security/keypair
func ShowSecurityKeypair(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"HasKey": false}
	if store := newSecurityStore(); store != nil {
		if store.PGP.Exists() {
			if info, err := store.PGP.Info(); err == nil {
				data["HasKey"], data["Info"] = true, info
			}
		}
		data["Keyservers"] = keyserverList()
	}
	renderAdminSecurity(w, r, "admin/admin_security_keypair.tmpl", "keypair", data)
}

// keyserverList returns web.security.keyservers (comma separated), defaulting to keys.openpgp.org.
func keyserverList() []string {
	raw := "https://keys.openpgp.org"
	if db := database.GetServerStore(); db != nil {
		raw = (&model.SettingsModel{Store: db}).GetString("web.security.keyservers", raw)
	}
	var out []string
	for _, ks := range strings.Split(raw, ",") {
		if ks = strings.TrimSpace(ks); ks != "" {
			out = append(out, ks)
		}
	}
	return out
}

// verifyAdminPassword re-prompts the signed-in admin's password for a sensitive operation.
func verifyAdminPassword(r *http.Request, password string) bool {
	db := database.GetUsersStore()
	username := AdminUsername(r)
	if db == nil || username == "" || password == "" || passwordHasSurroundingWhitespace(password) {
		return false
	}
	var hash string
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT password_hash FROM admins WHERE username = ?", username).Scan(&hash)
	if err != nil {
		return false
	}
	ok, verr := model.VerifyPassword(password, hash)
	return verr == nil && ok
}

// SecurityKeypairAction runs one keypair action. Destructive and key-exporting actions are
// sensitive operations: the admin password is re-prompted and the action is audited.
// POST /server/{admin_path}/security/keypair
func SecurityKeypairAction(w http.ResponseWriter, r *http.Request) {
	back := adminPagePath(r, adminSecurityPath+"/keypair")
	store := newSecurityStore()
	cfg := config.GetGlobalConfig()
	if store == nil || cfg == nil || r.ParseForm() != nil {
		redirectAdminForm(w, r, back, "flash.keypair_done", "flash.keypair_failed", errors.New("unavailable"))
		return
	}
	appName := cfg.Server.Branding.Title
	email := cfg.Server.Contact.ResolveContactEmail("security")
	action := r.PostFormValue("action")
	sensitive := action == "delete" || action == "export_private" || action == "import_private" || action == "rotate_installation_secret"
	if sensitive && !verifyAdminPassword(r, r.PostFormValue("password")) {
		logSecurityAudit(r, "security.keypair_"+action+"_denied", "pgp", "password re-prompt failed")
		redirectAdminForm(w, r, back, "flash.keypair_done", "flash.keypair_password_invalid", errors.New("password"))
		return
	}
	var err error
	switch action {
	case "generate", "rotate":
		if _, err = store.PGP.Generate(appName, email, action == "rotate"); err == nil {
			_, _ = store.PGP.PublishToKeyservers(r.Context(), keyserverList())
			logSecurityAudit(r, "security.keypair_"+action, "pgp", "")
		}
	case "publish":
		_, err = store.PGP.PublishToKeyservers(r.Context(), keyserverList())
	case "delete":
		if r.PostFormValue("confirm") != "DELETE" {
			err = errors.New("confirmation missing")
		} else if err = store.PGP.Delete(); err == nil {
			logSecurityAudit(r, "security.keypair_deleted", "pgp", "")
		}
	case "import_private":
		var warning string
		if warning, err = store.PGP.ImportPrivate(r.PostFormValue("private_key"), appName+" Security"); err == nil {
			logSecurityAudit(r, "security.keypair_imported", "pgp", warning)
		}
	case "rotate_installation_secret":
		// PART 11 "installation_secret": the PGP private key is re-encrypted under the new secret and
		// the previous secret stays valid for 7 days so in-flight {security_id} URLs keep working
		if err = service.RotateInstallationSecret(store.Store, store.PGP); err == nil {
			logSecurityAudit(r, "security.installation_secret_rotated", "installation_secret", "")
		}
	case "export_private":
		if exportPrivateKey(w, r, store) {
			return
		}
		err = errors.New("export refused")
	default:
		err = errors.New("unknown action")
	}
	redirectAdminForm(w, r, back, "flash.keypair_done", "flash.keypair_failed", err)
}

// exportPrivateKey streams the decrypted private key once per hour per admin, with a typed reason.
// It returns true when the download was written.
func exportPrivateKey(w http.ResponseWriter, r *http.Request, store *service.SecurityReportStore) bool {
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	db := database.GetServerStore()
	if reason == "" || len([]rune(reason)) > 500 || db == nil {
		return false
	}
	settings := &model.SettingsModel{Store: db}
	key := "security.private_key_export." + AdminUsername(r)
	if last, err := time.Parse(time.RFC3339, settings.GetString(key, "")); err == nil && time.Since(last) < time.Hour {
		return false
	}
	armored, err := store.PGP.PrivateKey()
	if err != nil {
		return false
	}
	_ = settings.SetString(key, time.Now().UTC().Format(time.RFC3339))
	logSecurityAudit(r, "security.private_key_exported", "pgp", "reason="+sanitizeLogToken(reason))
	w.Header().Set("Content-Type", "application/pgp-keys")
	w.Header().Set("Content-Disposition", `attachment; filename="pgp.priv.asc"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(armored))
	return true
}

// DownloadSecurityPublicKey serves pgp.pub.asc from the admin panel.
// GET /server/{admin_path}/security/keypair/public
func DownloadSecurityPublicKey(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Disposition", `attachment; filename="pgp.pub.asc"`)
	_, _ = w.Write([]byte(pub))
}

// PurgeSecurityReports is the daily cleanup task body: closed reports past retention are deleted.
func PurgeSecurityReports(db *database.Store, retentionDays int) (int64, error) {
	return (&service.SecurityReportStore{Store: db}).Purge(retentionDays)
}
