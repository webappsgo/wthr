package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/scheduler"
)

// SchedulerHandler handles scheduler-related requests
type SchedulerHandler struct {
	Scheduler *scheduler.Scheduler
}

// NewSchedulerHandler creates a new scheduler handler
func NewSchedulerHandler(s *scheduler.Scheduler) *SchedulerHandler {
	return &SchedulerHandler{Scheduler: s}
}

// GetAllTasks returns all tasks with their status and history
func (h *SchedulerHandler) GetAllTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.Scheduler.GetAllTaskInfo()
	if err != nil {
		log.Printf("scheduler: list tasks failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.scheduler.list_failed"))
		return
	}

	RespondPaginated(w, r, tasks, 1, len(tasks), len(tasks))
}

// schedulerLimit reads the ?limit query value, defaulting to 50 and capped at 100 per AI.md PART 19.
func schedulerLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// schedulerTaskView is one task as the admin API returns it: its live state
// and its effective settings from server.scheduler.tasks.
type schedulerTaskView struct {
	scheduler.TaskInfo
	Title    string               `json:"title"`
	Critical bool                 `json:"critical"`
	Global   bool                 `json:"global"`
	Settings config.SchedulerTask `json:"settings"`
}

// schedulerTaskDetail is a task plus its recent execution history.
type schedulerTaskDetail struct {
	schedulerTaskView
	History []scheduler.TaskRun `json:"history"`
}

// buildSchedulerTaskView joins the scheduler's live task info with the task's
// built-in definition and its configured settings.
func (h *SchedulerHandler) buildSchedulerTaskView(info scheduler.TaskInfo) schedulerTaskView {
	view := schedulerTaskView{TaskInfo: info, Title: info.Name}
	if def, ok := config.SchedulerTaskDefFor(info.Name); ok {
		view.Critical = def.Critical
		view.Global = def.Global
	}
	if cfg := config.GetGlobalConfig(); cfg != nil {
		view.Settings = cfg.Server.Scheduler.SchedulerTaskSettings(info.Name)
	}
	return view
}

// GetTask returns one task with its settings and recent history (GET .../scheduler/{id}).
func (h *SchedulerHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	infos, err := h.Scheduler.GetAllTaskInfo()
	if err != nil {
		log.Printf("scheduler: task detail failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.scheduler.list_failed"))
		return
	}
	for _, info := range infos {
		if info.Name != taskName {
			continue
		}
		history, err := h.Scheduler.GetTaskHistory(taskName, schedulerLimit(r))
		if err != nil {
			log.Printf("scheduler: task history failed: %v", err)
			RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.scheduler.history_failed"))
			return
		}
		RespondData(w, r, schedulerTaskDetail{schedulerTaskView: h.buildSchedulerTaskView(info), History: history})
		return
	}

	RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
}

// GetTaskHistory returns execution history for a specific task (GET .../scheduler/{id}/history)
func (h *SchedulerHandler) GetTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	if h.Scheduler.GetTask(taskName) == nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}

	history, err := h.Scheduler.GetTaskHistory(taskName, schedulerLimit(r))
	if err != nil {
		log.Printf("scheduler: task history failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.scheduler.history_failed"))
		return
	}

	RespondPaginated(w, r, history, 1, len(history), len(history))
}

// setTaskEnabled persists a task's enabled flag in server.yml and applies it to the running scheduler.
func (h *SchedulerHandler) setTaskEnabled(taskName string, enabled bool) []config.ValidationError {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return []config.ValidationError{{Field: "scheduler", Message: "errors.scheduler.config_unavailable"}}
	}
	task := cfg.Server.Scheduler.SchedulerTaskSettings(taskName)
	task.Enabled = enabled
	if errs := config.UpdateSchedulerTask(taskName, task, scheduler.ValidSchedule); len(errs) > 0 {
		return errs
	}
	if updated := config.GetGlobalConfig(); updated != nil {
		h.Scheduler.Configure(updated.Server.Scheduler)
	}
	return nil
}

// EnableTask enables a specific task
func (h *SchedulerHandler) EnableTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	if h.Scheduler.GetTask(taskName) == nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}
	if errs := h.setTaskEnabled(taskName, true); len(errs) > 0 {
		log.Printf("scheduler: enabling task failed: %v", errs[0])
		RespondError(w, r, http.StatusInternalServerError, "UPDATE_FAILED", Translate(r, "errors.scheduler.update_failed"))
		return
	}

	RespondSuccess(w, r, Translate(r, "success.scheduler.task_enabled"), map[string]interface{}{"task_name": taskName})
}

// DisableTask disables a specific task
func (h *SchedulerHandler) DisableTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	if h.Scheduler.GetTask(taskName) == nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}
	if errs := h.setTaskEnabled(taskName, false); len(errs) > 0 {
		RespondError(w, r, http.StatusBadRequest, "VALIDATION_FAILED", Translate(r, "errors.scheduler.update_failed"))
		return
	}

	RespondSuccess(w, r, Translate(r, "success.scheduler.task_disabled"), map[string]interface{}{"task_name": taskName})
}

// UpdateTask updates a task's settings (PATCH .../scheduler/{id}). Only the
// fields present in the body change; the result is written to server.yml and
// applied to the running scheduler without a restart.
func (h *SchedulerHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	if h.Scheduler.GetTask(taskName) == nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}

	var req struct {
		Enabled       *bool   `json:"enabled"`
		Schedule      *string `json:"schedule"`
		RetryOnFail   *bool   `json:"retry_on_fail"`
		RetryDelay    *string `json:"retry_delay"`
		MaxAge        *string `json:"max_age"`
		MaxSize       *string `json:"max_size"`
		Compress      *bool   `json:"compress"`
		Verify        *bool   `json:"verify"`
		RestartOnFail *bool   `json:"restart_on_fail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.common.invalid_request_body"))
		return
	}

	cfg := config.GetGlobalConfig()
	if cfg == nil {
		RespondError(w, r, http.StatusInternalServerError, "UPDATE_FAILED", Translate(r, "errors.scheduler.update_failed"))
		return
	}
	task := cfg.Server.Scheduler.SchedulerTaskSettings(taskName)
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.Schedule != nil {
		task.Schedule = *req.Schedule
	}
	if req.RetryOnFail != nil {
		task.RetryOnFail = *req.RetryOnFail
	}
	if req.RetryDelay != nil {
		task.RetryDelay = *req.RetryDelay
	}
	if req.MaxAge != nil {
		task.MaxAge = *req.MaxAge
	}
	if req.MaxSize != nil {
		task.MaxSize = *req.MaxSize
	}
	if req.Compress != nil {
		task.Compress = *req.Compress
	}
	if req.Verify != nil {
		task.Verify = *req.Verify
	}
	if req.RestartOnFail != nil {
		task.RestartOnFail = *req.RestartOnFail
	}

	if errs := config.UpdateSchedulerTask(taskName, task, scheduler.ValidSchedule); len(errs) > 0 {
		details := make(map[string]string, len(errs))
		for _, e := range errs {
			details[e.Field] = Translate(r, e.Message)
		}
		RespondError(w, r, http.StatusBadRequest, "VALIDATION_FAILED", Translate(r, "errors.scheduler.update_failed"), map[string]interface{}{"fields": details})
		return
	}
	if updated := config.GetGlobalConfig(); updated != nil {
		h.Scheduler.Configure(updated.Server.Scheduler)
	}

	RespondSuccess(w, r, Translate(r, "success.scheduler.task_updated"), map[string]interface{}{"task_name": taskName})
}

// TriggerTask manually triggers a task to run immediately
func (h *SchedulerHandler) TriggerTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	err := h.Scheduler.TriggerTask(taskName)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}

	RespondSuccess(w, r, Translate(r, "success.scheduler.task_triggered"), map[string]interface{}{"task_name": taskName})
}
