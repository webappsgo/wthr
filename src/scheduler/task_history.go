package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
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
	Interval     string     `json:"interval"`
	Enabled      bool       `json:"enabled"`
	Running      bool       `json:"running"`
	NextRun      *time.Time `json:"next_run,omitempty"`
	LastRun      *TaskRun   `json:"last_run,omitempty"`
	RunCount     int        `json:"run_count"`
	SuccessCount int        `json:"success_count"`
	ErrorCount   int        `json:"error_count"`
}

// InitTaskHistoryTable verifies that server_scheduler_history is present.
//
// The table and its indexes are declared once in database.ServerSchema, which
// database.InitDualDB executes on every startup - no DDL runs at scheduler
// start any more. This check stays because the caller treats a missing table as
// a fatal startup condition: it turns "the schema was never applied to this
// database" into a clear error instead of a stream of failed inserts later.
func (s *Scheduler) InitTaskHistoryTable() error {
	db := database.GetServerDB()
	if db == nil {
		return fmt.Errorf("server database unavailable")
	}

	var present int
	if err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM server_scheduler_history WHERE 1 = 0",
	).Scan(&present); err != nil {
		return fmt.Errorf("server_scheduler_history is missing from the server database schema: %w", err)
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

	// Both timestamps are bound as canonical UTC text. Binding the time.Time
	// values themselves would let the driver render them with
	// time.Time.String(), in the host's local zone, so two servers - or one
	// server either side of a timezone change - would fill the same column with
	// two encodings that no longer share an ordering.
	_, dbErr := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_scheduler_history (task_name, start_time, end_time, duration_ms, status, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, taskName, dbtime.FormatSQLTimestamp(startTime), dbtime.FormatSQLTimestamp(endTime), duration, status, errorMsg)

	return dbErr
}

// loadTaskRunsNewestFirst returns the recorded runs of taskName ordered by the
// absolute instant each run started, most recent first, keeping at most limit
// of them (limit <= 0 means all).
//
// Ordering decision: the ordering is done in Go, after parsing, rather than by
// the SQL "ORDER BY start_time DESC" this used to rely on. RecordTaskRun now
// writes canonical UTC text, but rows written by earlier builds are still on
// disk in the driver's local-zone time.Time.String() layout, and SQL compares
// those two encodings as text - by wall clock and by leading character, not by
// instant. A text ORDER BY therefore interleaves legacy and current rows
// wrongly, and an SQL LIMIT stacked on top of it discards rows off the wrong
// end of that ordering, which is how GetLastTaskRun could return a run that was
// not the most recent. Parsing every value to an absolute instant first makes
// mixed on-disk data give the same answer as uniform data. The scan stays
// bounded: it is filtered to a single task by an index, and
// CleanupOldTaskHistory prunes the table on a schedule.
//
// A start_time that is NULL or in a layout the project never writes keeps its
// zero value, so such a row sorts to the very end and can never be mistaken for
// the newest run. It is still listed, because the run genuinely happened and
// hiding it would misreport the task's history.
func (s *Scheduler) loadTaskRunsNewestFirst(taskName string, limit int) ([]TaskRun, error) {
	// id DESC only breaks ties between runs that share a start instant; the
	// real ordering is applied below.
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutComplexSelect, `
		SELECT id, task_name, start_time, end_time, duration_ms, status, COALESCE(error, '')
		FROM server_scheduler_history
		WHERE task_name = ?
		ORDER BY id DESC
	`, taskName)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []TaskRun
	for rows.Next() {
		var run TaskRun
		var storedStart, storedEnd interface{}
		err := rows.Scan(&run.ID, &run.TaskName, &storedStart, &storedEnd,
			&run.Duration, &run.Status, &run.Error)
		if err != nil {
			continue
		}

		if parsed, ok := dbtime.ParseStoredTimestamp(storedStart); ok {
			run.StartTime = parsed
		}
		if parsed, ok := dbtime.ParseStoredTimestamp(storedEnd); ok {
			run.EndTime = parsed
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
	err = database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as errors
		FROM server_scheduler_history
		WHERE task_name = ?
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
			Interval: task.Schedule, // Now uses cron schedule string
			Enabled:  task.enabled,
			Running:  false, // Cron handles running state internally
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

	// RecordTaskRun writes start_time as canonical UTC text, but rows left by
	// earlier builds hold the driver's local-zone rendering of a bound
	// time.Time - compare each value as a UTC instant in Go rather than against
	// SQLite's datetime('now'), which would read that local-zone text as if it
	// were UTC (see CleanupOldSessions).
	cutoff := time.Now().UTC().AddDate(0, 0, -days)

	rows, err := deleteRowsWithTimestampBefore(database.GetServerDB(), "server_scheduler_history", "id", "start_time", cutoff)
	if err != nil {
		return err
	}

	if rows > 0 {
		log.Printf("INFO: Cleaned up %d old task history records (older than %d days)", rows, days)
	}

	return nil
}
