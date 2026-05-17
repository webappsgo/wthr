package handler

import (
	"net/http"
	"strconv"
	"github.com/casapps/wthr/src/scheduler"

	"github.com/gin-gonic/gin"
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
func (h *SchedulerHandler) GetAllTasks(c *gin.Context) {
	tasks, err := h.Scheduler.GetAllTaskInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get task information",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// GetTaskHistory returns execution history for a specific task
func (h *SchedulerHandler) GetTaskHistory(c *gin.Context) {
	taskName := c.Param("name")
	limitStr := c.DefaultQuery("limit", "50")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	history, err := h.Scheduler.GetTaskHistory(taskName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get task history",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_name": taskName,
		"history":   history,
		"count":     len(history),
	})
}

// EnableTask enables a specific task
func (h *SchedulerHandler) EnableTask(c *gin.Context) {
	taskName := c.Param("name")

	err := h.Scheduler.EnableTask(taskName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Task enabled successfully",
		"task_name": taskName,
	})
}

// DisableTask disables a specific task
func (h *SchedulerHandler) DisableTask(c *gin.Context) {
	taskName := c.Param("name")

	err := h.Scheduler.DisableTask(taskName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Task disabled successfully",
		"task_name": taskName,
	})
}

// UpdateTask updates task settings (enable/disable)
func (h *SchedulerHandler) UpdateTask(c *gin.Context) {
	taskName := c.Param("name")

	task := h.Scheduler.GetTask(taskName)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{"code": "TASK_NOT_FOUND", "message": "Task not found: " + taskName},
		})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "INVALID_REQUEST", "message": "Invalid request body"},
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
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{"code": "UPDATE_FAILED", "message": err.Error()},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"message":   "Task updated successfully",
		"task_name": taskName,
	})
}

// TriggerTask manually triggers a task to run immediately
func (h *SchedulerHandler) TriggerTask(c *gin.Context) {
	taskName := c.Param("name")

	err := h.Scheduler.TriggerTask(taskName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Task triggered successfully",
		"task_name": taskName,
	})
}
