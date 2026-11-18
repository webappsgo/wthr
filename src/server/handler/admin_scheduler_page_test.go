package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/scheduler"
)

func TestSchedulerPageStatus(t *testing.T) {
	success := &scheduler.TaskRun{Status: "success"}
	failed := &scheduler.TaskRun{Status: "failed"}
	cases := []struct {
		name string
		info scheduler.TaskInfo
		want string
	}{
		{"running wins over everything", scheduler.TaskInfo{Running: true, Enabled: false}, "running"},
		{"disabled", scheduler.TaskInfo{Enabled: false, LastRun: success}, "disabled"},
		{"pending before the first run", scheduler.TaskInfo{Enabled: true}, "pending"},
		{"success", scheduler.TaskInfo{Enabled: true, LastRun: success}, "success"},
		{"failed", scheduler.TaskInfo{Enabled: true, LastRun: failed}, "failed"},
	}
	for _, c := range cases {
		if got := schedulerPageStatus(c.info); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSchedulerLocationFallsBackToUTC(t *testing.T) {
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(nil)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	if got := schedulerLocation(); got != time.UTC {
		t.Errorf("without a config the zone must be UTC, got %s", got)
	}

	cfg := &config.AppConfig{}
	cfg.Server.Scheduler = config.DefaultSchedulerConfig()
	cfg.Server.Scheduler.Timezone = "Asia/Tokyo"
	config.SetGlobalConfig(cfg)
	if got := schedulerLocation(); got.String() != "Asia/Tokyo" {
		t.Errorf("configured zone not used, got %s", got)
	}
}

func TestSchedulerPageRowsEmpty(t *testing.T) {
	h := NewSchedulerHandler(scheduler.NewScheduler(nil))
	rows, err := h.schedulerPageRows(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no rows, got %d", len(rows))
	}
}

// schedulerFormRequest builds a POST to the scheduler page with form values.
func schedulerFormRequest(values url.Values) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/server/administration/scheduler", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, httptest.NewRecorder()
}

func withSchedulerConfig(t *testing.T) {
	t.Helper()
	t.Setenv("CONFIG_DIR", t.TempDir())
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Scheduler = config.DefaultSchedulerConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
}

func flashCookie(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == FlashCookieName {
			return c.Value
		}
	}
	return ""
}

func TestSchedulerSubmitPageRedirectsBack(t *testing.T) {
	withSchedulerConfig(t)
	h := NewSchedulerHandler(scheduler.NewScheduler(nil))

	cases := []struct {
		name   string
		values url.Values
		flash  string
	}{
		{"unknown action", url.Values{"action": {"explode"}}, "scheduler_invalid"},
		{"run unknown task", url.Values{"action": {"run"}, "task": {"nope"}}, "scheduler_unknown_task"},
		{"enable unknown task", url.Values{"action": {"enable"}, "task": {"nope"}}, "scheduler_unknown_task"},
		{"disable unknown task", url.Values{"action": {"disable"}, "task": {"nope"}}, "scheduler_unknown_task"},
		{"save unknown task", url.Values{"action": {"task"}, "task": {"nope"}}, "scheduler_unknown_task"},
		{"invalid settings", url.Values{"action": {"settings"}, "timezone": {"Not/AZone"}, "catch_up_window": {"1h"}}, "scheduler_invalid"},
		{"valid settings", url.Values{"action": {"settings"}, "timezone": {"UTC"}, "catch_up_window": {"2h"}}, "scheduler_settings_saved"},
	}
	for _, c := range cases {
		req, w := schedulerFormRequest(c.values)
		h.SubmitPage(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("%s: status %d, want 303", c.name, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "/server/administration/scheduler" {
			t.Errorf("%s: redirected to %q", c.name, loc)
		}
		if !strings.Contains(flashCookie(w), c.flash) {
			t.Errorf("%s: flash cookie %q does not name %q", c.name, flashCookie(w), c.flash)
		}
	}

	if got := config.GetGlobalConfig().Server.Scheduler.Timezone; got != "UTC" {
		t.Errorf("valid settings must be applied, timezone = %q", got)
	}
}

func TestSchedulerSubmitPageReturnsToTaskPage(t *testing.T) {
	withSchedulerConfig(t)
	h := NewSchedulerHandler(scheduler.NewScheduler(nil))

	req, w := schedulerFormRequest(url.Values{"action": {"run"}, "task": {"nope"}, "return": {"task"}})
	h.SubmitPage(w, req)
	if loc := w.Header().Get("Location"); loc != "/server/administration/scheduler/nope" {
		t.Errorf("redirected to %q", loc)
	}
}
