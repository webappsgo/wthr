package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/scheduler"
)

// TestNewSchedulerHandler verifies the constructor wires the Scheduler
// field as passed.
func TestNewSchedulerHandler(t *testing.T) {
	s := scheduler.NewScheduler(nil)
	h := NewSchedulerHandler(s)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.Scheduler != s {
		t.Error("expected Scheduler field to be the passed *scheduler.Scheduler")
	}
}

// TestSchedulerHandler_GetAllTasks_Empty verifies a scheduler with no
// registered tasks returns 200 with an empty task list, without touching
// any database (no tasks means GetAllTaskInfo's loop never runs).
func TestSchedulerHandler_GetAllTasks_Empty(t *testing.T) {
	s := scheduler.NewScheduler(nil)
	h := NewSchedulerHandler(s)

	c, w := newAPITestContext("/server/admin/config/scheduler/tasks")
	h.GetAllTasks(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"count":0`) {
		t.Errorf("expected count:0 in body, got: %s", body)
	}
}
