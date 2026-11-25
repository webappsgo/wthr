package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// TaskRun represents a single execution of a task
type TaskRun struct {
	ID        int64     `json:"id"`
	TaskName  string    `json:"task_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	// milliseconds
	Duration int64 `json:"duration_ms"`
	// "success", "error"
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// TaskInfo provides detailed information about a task
type TaskInfo struct {
	Name         string     `json:"name"`
	Schedule     string     `json:"schedule"`
	Enabled      bool       `json:"enabled"`
	Running      bool       `json:"running"`
	NextRun      *time.Time `json:"next_run,omitempty"`
	LastRun      *TaskRun   `json:"last_run,omitempty"`
	RunCount     int        `json:"run_count"`
	SuccessCount int        `json:"success_count"`
	ErrorCount   int        `json:"error_count"`
}

// InitTaskHistoryTable verifies that scheduler_history is present.
//
// The table and its indexes are declared once in database.ServerSchema, which
// database.InitDualDB executes on every startup - no DDL runs at scheduler
// start any more. This check stays because the caller treats a missing table as
// a fatal startup condition: it turns "the schema was never applied to this
// database" into a clear error instead of a stream of failed inserts later.
func (s *Scheduler) InitTaskHistoryTable() error {
	db := database.GetServerStore()
	if db == nil {
		return fmt.Errorf("server database unavailable")
	}

	var present int
	if err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM scheduler_history WHERE 1 = 0",
	).Scan(&present); err != nil {
		return fmt.Errorf("scheduler_history is missing from the server database schema: %w", err)
	}

	return nil
}

// RecordTaskRun records a task execution in the database
func (s *Scheduler) RecordTaskRun(taskName string, startTime, endTime time.Time, err error) error {
	duration := endTime.Sub(startTime).Milliseconds()
	status := "success"
	errorMsg := ""

	if err != nil {
		status = "error"
		errorMsg = err.Error()
	}

	// Scheduler history timestamps are Unix seconds, so they sort identically
	// across hosts and time zones.
	_, dbErr := database.ExecContext(context.Background(), database.GetServerStore(), database.TimeoutWrite, `
		INSERT INTO scheduler_history (task_id, started_at, finished_at, duration_ms, status, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, taskName, startTime.Unix(), endTime.Unix(), duration, status, errorMsg)

	return dbErr
}

// loadTaskRunsNewestFirst returns the recorded runs of taskName ordered by the
// absolute instant each run started, most recent first, keeping at most limit
// of them (limit <= 0 means all).
//
// The canonical table stores Unix seconds, so ordering by the integer start
// time is consistent across hosts and time zones.
func (s *Scheduler) loadTaskRunsNewestFirst(taskName string, limit int) ([]TaskRun, error) {
	// id DESC only breaks ties between runs that share a start instant; the
	// real ordering is applied below.
	rows, err := database.QueryContext(context.Background(), database.GetServerStore(), database.TimeoutComplexSelect, `
		SELECT id, task_id, started_at, finished_at, duration_ms, status, COALESCE(error, '')
		FROM scheduler_history
		WHERE task_id = ?
		ORDER BY id DESC
	`, taskName)

	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("WARNING: failed to close scheduler history rows: %v", err)
		}
	}()

	var history []TaskRun
	for rows.Next() {
		var run TaskRun
		var storedStart, storedEnd sql.NullInt64
		var duration sql.NullInt64
		err := rows.Scan(&run.ID, &run.TaskName, &storedStart, &storedEnd,
			&duration, &run.Status, &run.Error)
		if err != nil {
			continue
		}
		if duration.Valid {
			run.Duration = duration.Int64
		}

		if storedStart.Valid {
			run.StartTime = time.Unix(storedStart.Int64, 0).UTC()
		}
		if storedEnd.Valid {
			run.EndTime = time.Unix(storedEnd.Int64, 0).UTC()
		}

		history = append(history, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(history, func(i, j int) bool {
		return history[i].StartTime.After(history[j].StartTime)
	})

	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}

	return history, nil
}

// GetTaskHistory returns execution history for a specific task
func (s *Scheduler) GetTaskHistory(taskName string, limit int) ([]TaskRun, error) {
	if limit <= 0 {
		limit = 50
	}

	return s.loadTaskRunsNewestFirst(taskName, limit)
}

// GetLastTaskRun returns the most recent run for a task
func (s *Scheduler) GetLastTaskRun(taskName string) (*TaskRun, error) {
	history, err := s.loadTaskRunsNewestFirst(taskName, 1)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, nil
	}

	return &history[0], nil
}

// GetTaskStats returns statistics for a task
func (s *Scheduler) GetTaskStats(taskName string) (total, success, errors int, err error) {
	err = database.QueryRowContext(context.Background(), database.GetServerStore(), database.TimeoutSimpleSelect, `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as errors
		FROM scheduler_history
		WHERE task_id = ?
	`, taskName).Scan(&total, &success, &errors)

	return
}

// GetAllTaskInfo returns detailed information about all tasks
func (s *Scheduler) GetAllTaskInfo() ([]TaskInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := make([]TaskInfo, 0, len(s.tasks))

	for _, task := range s.tasks {
		task.mu.Lock()

		taskInfo := TaskInfo{
			Name:     task.Name,
			Schedule: task.Schedule,
			Enabled:  task.enabled,
			Running:  s.isActive(task.Name),
		}

		// Get next run time from the task's own schedule
		if task.enabled && !task.nextRun.IsZero() {
			nextRun := task.nextRun
			taskInfo.NextRun = &nextRun
		}

		task.mu.Unlock()

		// Get last run from database
		lastRun, _ := s.GetLastTaskRun(task.Name)
		taskInfo.LastRun = lastRun

		// Get stats
		total, success, errors, _ := s.GetTaskStats(task.Name)
		taskInfo.RunCount = total
		taskInfo.SuccessCount = success
		taskInfo.ErrorCount = errors

		info = append(info, taskInfo)
	}

	return info, nil
}

// CleanupOldTaskHistory removes task history older than the specified days
func (s *Scheduler) CleanupOldTaskHistory(days int) error {
	if days <= 0 {
		// Default: keep 90 days
		days = 90
	}

	// Unix seconds allow cleanup comparisons to use the same UTC instant
	// independent of the server's local time zone.
	cutoff := time.Now().UTC().AddDate(0, 0, -days)

	result, err := database.ExecContext(context.Background(), database.GetServerStore(), database.TimeoutBulk,
		"DELETE FROM scheduler_history WHERE started_at < ?", cutoff.Unix())
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows > 0 {
		log.Printf("INFO: Cleaned up %d old task history records (older than %d days)", rows, days)
	}

	return nil
}
