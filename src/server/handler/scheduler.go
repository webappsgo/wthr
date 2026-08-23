package handler

import (
	"encoding/json"
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
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":   "Failed to get task information",
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
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
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":   "Failed to get task history",
			"details": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"task_name": taskName,
		"history":   history,
		"count":     len(history),
	})
}

// EnableTask enables a specific task
func (h *SchedulerHandler) EnableTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	err := h.Scheduler.EnableTask(taskName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Task enabled successfully",
		"task_name": taskName,
	})
}

// DisableTask disables a specific task
func (h *SchedulerHandler) DisableTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	err := h.Scheduler.DisableTask(taskName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Task disabled successfully",
		"task_name": taskName,
	})
}

// UpdateTask updates task settings (enable/disable)
func (h *SchedulerHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	task := h.Scheduler.GetTask(taskName)
	if task == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": map[string]interface{}{"code": "TASK_NOT_FOUND", "message": "Task not found: " + taskName},
		})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]interface{}{"code": "INVALID_REQUEST", "message": "Invalid request body"},
		})
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
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": map[string]interface{}{"code": "UPDATE_FAILED", "message": err.Error()},
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"message":   "Task updated successfully",
		"task_name": taskName,
	})
}

// TriggerTask manually triggers a task to run immediately
func (h *SchedulerHandler) TriggerTask(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "name")

	err := h.Scheduler.TriggerTask(taskName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Task triggered successfully",
		"task_name": taskName,
	})
}
