package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// SaveBackupScheduleForm saves the browser form without requiring JavaScript.
func SaveBackupScheduleForm(w http.ResponseWriter, r *http.Request) {
	back := adminPagePath(r, "/backup")
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, back, "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	interval, intervalErr := strconv.Atoi(r.PostForm.Get("interval"))
	retention, retentionErr := strconv.Atoi(r.PostForm.Get("retention"))
	if intervalErr != nil || interval < 1 || interval > 168 || retentionErr != nil || retention < 1 {
		redirectAdminForm(w, r, back, "flash.settings_saved", "flash.settings_save_failed", strconv.ErrSyntax)
		return
	}
	settings := &model.SettingsModel{DB: database.GetServerDB()}
	values := []struct {
		key string
		val string
	}{
		{"backup.enabled", strconv.FormatBool(r.PostForm.Get("enabled") != "")},
		{"backup.interval", strconv.Itoa(interval)},
		{"backup.retention.max_backups", strconv.Itoa(retention)},
	}
	for _, setting := range values {
		if err := settings.SetString(setting.key, setting.val); err != nil {
			redirectAdminForm(w, r, back, "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
	}
	redirectAdminForm(w, r, back, "flash.settings_saved", "flash.settings_save_failed", nil)
}

// RestoreBackupForm adapts the existing bounded restore handler to PRG browser responses.
func RestoreBackupForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		redirectAdminForm(w, r, adminPagePath(r, "/backup"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	ctx := reqctx.SetValue(r.Context(), "db", database.GetServerDB())
	r = r.WithContext(ctx)
	result := httptest.NewRecorder()
	RestoreBackup(result, r)
	if result.Code < 200 || result.Code >= 300 {
		redirectAdminForm(w, r, adminPagePath(r, "/backup"), "flash.settings_saved", "flash.settings_save_failed", errors.New("backup restore failed"))
		return
	}
	redirectAdminForm(w, r, adminPagePath(r, "/backup"), "flash.settings_saved", "flash.settings_save_failed", nil)
}
