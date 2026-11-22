package handler

import (
	"net/http"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

// SaveBackupEncryptionPassword stores a password for unattended scheduled backups.
func SaveBackupEncryptionPassword(w http.ResponseWriter, r *http.Request) {
	back := adminPagePath(r, "/backup")
	if err := r.ParseForm(); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	password := r.PostForm.Get("password")
	if password == "" || password != r.PostForm.Get("password_confirm") {
		SetFlash(w, r, "error", "flash.backup_password_mismatch")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil || cfg.Server.Security.EncryptionKey == "" {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	sealed, err := util.EncryptAtRest(cfg.Server.Security.EncryptionKey, password)
	if err == nil {
		err = config.WriteSecret("server.backup.encryption_password", sealed)
	}
	if err == nil {
		cfg.Server.Maintenance.Backup.Encryption.Enabled = true
		err = config.SaveConfig(cfg)
	}
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
	} else {
		logSecurityAudit(r, "backup.encryption_password_changed", "backup", "")
		SetFlash(w, r, "success", "flash.settings_saved")
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
