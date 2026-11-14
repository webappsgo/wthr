package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

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

// GetTaskHistory returns execution history for a specific task
func (h *SchedulerHandler) GetTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "50"
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	history, err := h.Scheduler.GetTaskHistory(taskName, limit)
	if err != nil {
		log.Printf("scheduler: task history failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", Translate(r, "errors.scheduler.history_failed"))
		return
	}

	RespondPaginated(w, r, history, 1, len(history), len(history))
}

// EnableTask enables a specific task
func (h *SchedulerHandler) EnableTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	err := h.Scheduler.EnableTask(taskName)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}

	RespondSuccess(w, r, Translate(r, "success.scheduler.task_enabled"), map[string]interface{}{"task_name": taskName})
}

// DisableTask disables a specific task
func (h *SchedulerHandler) DisableTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	err := h.Scheduler.DisableTask(taskName)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}

	RespondSuccess(w, r, Translate(r, "success.scheduler.task_disabled"), map[string]interface{}{"task_name": taskName})
}

// UpdateTask updates task settings (enable/disable)
func (h *SchedulerHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	task := h.Scheduler.GetTask(taskName)
	if task == nil {
		RespondError(w, r, http.StatusNotFound, "TASK_NOT_FOUND", TranslateFormat(r, "errors.scheduler.task_not_found", map[string]string{"task": taskName}))
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.common.invalid_request_body"))
		return
	}

	if req.Enabled != nil {
		var err error
		if *req.Enabled {
			err = h.Scheduler.EnableTask(taskName)
		} else {
			err = h.Scheduler.DisableTask(taskName)
		}
		if err != nil {
			log.Printf("scheduler: task update failed: %v", err)
			RespondError(w, r, http.StatusInternalServerError, "UPDATE_FAILED", Translate(r, "errors.scheduler.update_failed"))
			return
		}
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
