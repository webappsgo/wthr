package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/scheduler"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/util"
)

// schedulerTimeLayout is how the scheduler page prints a run time: the date, the
// time and the zone abbreviation, in the configured scheduler timezone.
const schedulerTimeLayout = "2006-01-02 15:04:05 MST"

// schedulerPageRow is one line of the task list on the admin scheduler page.
type schedulerPageRow struct {
	Name         string
	Schedule     string
	Enabled      bool
	Critical     bool
	Global       bool
	Status       string
	LastRun      string
	LastDuration string
	NextRun      string
	RunCount     int
	FailCount    int
}

// schedulerPageTimezones are the zones offered as suggestions; any IANA zone is accepted.
var schedulerPageTimezones = []string{
	"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
	"UTC", "Europe/London", "Europe/Paris", "Europe/Berlin", "Asia/Tokyo",
	"Asia/Shanghai", "Australia/Sydney",
}

// schedulerPageStatus maps a task's live state onto the AI.md PART 19 legend:
// running, success, failed, pending or disabled.
func schedulerPageStatus(info scheduler.TaskInfo) string {
	switch {
	case info.Running:
		return "running"
	case !info.Enabled:
		return "disabled"
	case info.LastRun == nil:
		return "pending"
	case info.LastRun.Status == "success":
		return "success"
	default:
		return "failed"
	}
}

// schedulerPageRows builds the task list rows in task-name order.
func (h *SchedulerHandler) schedulerPageRows(loc *time.Location) ([]schedulerPageRow, error) {
	infos, err := h.Scheduler.GetAllTaskInfo()
	if err != nil {
		return nil, err
	}

	rows := make([]schedulerPageRow, 0, len(infos))
	for _, info := range infos {
		view := h.buildSchedulerTaskView(info)
		row := schedulerPageRow{
			Name:      info.Name,
			Schedule:  info.Schedule,
			Enabled:   info.Enabled,
			Critical:  view.Critical,
			Global:    view.Global,
			Status:    schedulerPageStatus(info),
			RunCount:  info.RunCount,
			FailCount: info.ErrorCount,
		}
		if info.LastRun != nil {
			row.LastRun = info.LastRun.StartTime.In(loc).Format(schedulerTimeLayout)
			row.LastDuration = (time.Duration(info.LastRun.Duration) * time.Millisecond).String()
		}
		if info.NextRun != nil {
			row.NextRun = info.NextRun.In(loc).Format(schedulerTimeLayout)
		}
		rows = append(rows, row)
	}

	// stable alphabetical order so the list does not reshuffle between loads
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Name < rows[j-1].Name; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows, nil
}

// schedulerLocation returns the configured scheduler timezone for display.
func schedulerLocation() *time.Location {
	if cfg := config.GetGlobalConfig(); cfg != nil {
		return cfg.Server.Scheduler.SchedulerLocation()
	}
	return time.UTC
}

// ShowPage renders the admin scheduler page (AI.md PART 19 "Admin Panel"): the
// task list with status, schedule, last and next run, and the scheduler settings.
func (h *SchedulerHandler) ShowPage(w http.ResponseWriter, r *http.Request) {
	loc := schedulerLocation()
	rows, err := h.schedulerPageRows(loc)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.scheduler.list_failed"))
		return
	}

	settings := config.DefaultSchedulerConfig()
	if cfg := config.GetGlobalConfig(); cfg != nil {
		settings = cfg.Server.Scheduler
	}
	serverCtx, _ := middleware.GetServerContext(r.Context())

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_scheduler.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":      Translate(r, "admin.scheduler.page_title"),
		"page":       "scheduler",
		"breadcrumb": Translate(r, "admin.scheduler.page_title"),
		"flash":      TakeFlash(w, r),
		"tasks":      rows,
		"timezone":   settings.Timezone,
		"catch_up":   settings.CatchUpWindow,
		"timezones":  schedulerPageTimezones,
		"server":     serverCtx,
	}))
}

// ShowTaskPage renders one task's settings and execution history (AI.md PART 19 "Task Detail").
func (h *SchedulerHandler) ShowTaskPage(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")
	loc := schedulerLocation()

	infos, err := h.Scheduler.GetAllTaskInfo()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.scheduler.list_failed"))
		return
	}
	for _, info := range infos {
		if info.Name != taskName {
			continue
		}
		history, err := h.Scheduler.GetTaskHistory(taskName, 100)
		if err != nil {
			InternalError(w, r, Translate(r, "errors.scheduler.history_failed"))
			return
		}

		type historyRow struct {
			Started  string
			Duration string
			Status   string
			Error    string
		}
		runs := make([]historyRow, 0, len(history))
		for _, run := range history {
			runs = append(runs, historyRow{
				Started:  run.StartTime.In(loc).Format(schedulerTimeLayout),
				Duration: (time.Duration(run.Duration) * time.Millisecond).String(),
				Status:   run.Status,
				Error:    run.Error,
			})
		}

		view := h.buildSchedulerTaskView(info)
		row := schedulerPageRow{
			Name: info.Name, Schedule: info.Schedule, Enabled: info.Enabled,
			Critical: view.Critical, Global: view.Global, Status: schedulerPageStatus(info),
			RunCount: info.RunCount, FailCount: info.ErrorCount,
		}
		if info.NextRun != nil {
			row.NextRun = info.NextRun.In(loc).Format(schedulerTimeLayout)
		}
		if info.LastRun != nil {
			row.LastRun = info.LastRun.StartTime.In(loc).Format(schedulerTimeLayout)
		}
		serverCtx, _ := middleware.GetServerContext(r.Context())

		middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_scheduler_task.tmpl", AdminTemplateData(r, map[string]interface{}{
			"title":      Translate(r, "admin.scheduler.detail_title"),
			"page":       "scheduler",
			"breadcrumb": Translate(r, "admin.scheduler.detail_title"),
			"flash":      TakeFlash(w, r),
			"task":       row,
			"settings":   view.Settings,
			"history":    runs,
			"server":     serverCtx,
		}))
		return
	}

	middleware.RenderHTML(w, r, http.StatusNotFound, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": Translate(r, "error.not_found"),
		"error": Translate(r, "error.not_found"),
	}))
}

// SubmitPage handles the scheduler page forms (POST /scheduler): saving the
// settings, saving or toggling one task, and running a task now. Every form is a
// plain POST that redirects back with a flash message, so it works without JavaScript.
func (h *SchedulerHandler) SubmitPage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.scheduler.invalid_request_body"))
		return
	}

	back := adminPagePath(r, "/scheduler")
	taskName := strings.TrimSpace(r.PostFormValue("task"))
	if taskName != "" && r.PostFormValue("return") == "task" {
		back = adminPagePath(r, "/scheduler/"+taskName)
	}

	switch r.PostFormValue("action") {
	case "settings":
		errs := config.UpdateSchedulerSettings(r.PostFormValue("timezone"), r.PostFormValue("catch_up_window"))
		h.finishSchedulerForm(w, r, back, errs, "flash.scheduler_settings_saved")

	case "run":
		if err := h.Scheduler.TriggerTask(taskName); err != nil {
			SetFlash(w, r, "error", "flash.scheduler_unknown_task")
		} else {
			SetFlash(w, r, "success", "flash.scheduler_run_started")
		}
		http.Redirect(w, r, back, http.StatusSeeOther)

	case "enable", "disable":
		if h.Scheduler.GetTask(taskName) == nil {
			SetFlash(w, r, "error", "flash.scheduler_unknown_task")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		enabled := r.PostFormValue("action") == "enable"
		errs := h.setTaskEnabled(taskName, enabled)
		key := "flash.scheduler_task_disabled"
		if enabled {
			key = "flash.scheduler_task_enabled"
		}
		h.finishSchedulerForm(w, r, back, errs, key)

	case "task":
		if h.Scheduler.GetTask(taskName) == nil {
			SetFlash(w, r, "error", "flash.scheduler_unknown_task")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		cfg := config.GetGlobalConfig()
		if cfg == nil {
			SetFlash(w, r, "error", "flash.scheduler_invalid")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		task := cfg.Server.Scheduler.SchedulerTaskSettings(taskName)
		task.Schedule = r.PostFormValue("schedule")
		task.Enabled = r.PostFormValue("enabled") == "on"
		task.RetryOnFail = r.PostFormValue("retry_on_fail") == "on"
		task.Compress = r.PostFormValue("compress") == "on"
		if v := r.PostFormValue("retry_delay"); v != "" {
			task.RetryDelay = v
		}
		if v := r.PostFormValue("max_age"); v != "" {
			task.MaxAge = v
		}
		if v := r.PostFormValue("max_size"); v != "" {
			task.MaxSize = v
		}
		errs := config.UpdateSchedulerTask(taskName, task, scheduler.ValidSchedule)
		if len(errs) == 0 {
			if updated := config.GetGlobalConfig(); updated != nil {
				h.Scheduler.Configure(updated.Server.Scheduler)
			}
		}
		h.finishSchedulerForm(w, r, back, nil, "flash.scheduler_task_saved", errs...)

	default:
		SetFlash(w, r, "error", "flash.scheduler_invalid")
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// finishSchedulerForm applies a saved config to the running scheduler and
// redirects back with a success or failure flash.
func (h *SchedulerHandler) finishSchedulerForm(w http.ResponseWriter, r *http.Request, back string, errs []config.ValidationError, successKey string, more ...config.ValidationError) {
	errs = append(errs, more...)
	if len(errs) > 0 {
		SetFlash(w, r, "error", "flash.scheduler_invalid")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if updated := config.GetGlobalConfig(); updated != nil {
		h.Scheduler.Configure(updated.Server.Scheduler)
	}
	SetFlash(w, r, "success", successKey)
	http.Redirect(w, r, back, http.StatusSeeOther)
}
