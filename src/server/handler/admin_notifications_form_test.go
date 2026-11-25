package handler

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/server/model"
)

func TestSaveAdminPreferencesFormResetRestoresDefaults(t *testing.T) {
	usersDB := newNotificationPreferencesTestDB(t)
	setGlobalTestDualDB(t, newTestServerDB(t), usersDB)
	handler := &AdminNotificationsHandler{UsersDB: storeForDB(usersDB)}
	form := url.Values{"reset": {"true"}}
	r, w := newTestContext(http.MethodPost, "/server/administration/admin/notifications/preferences")
	r = withAdminID(r, 42)
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.SaveAdminPreferencesForm(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", w.Code, w.Body.String())
	}
	prefs, err := (&model.NotificationPreferencesModel{UserStore: storeForDB(usersDB)}).GetAdminPreferences(42)
	if err != nil {
		t.Fatalf("load reset preferences: %v", err)
	}
	if !prefs.EnableToast || !prefs.EnableBanner || !prefs.EnableCenter || prefs.EnableSound || prefs.ToastDurationSuccess != 5 || prefs.ToastDurationInfo != 5 || prefs.ToastDurationWarning != 10 {
		t.Fatalf("reset preferences = %+v, want documented defaults", prefs)
	}
}
