package scheduler

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/webappsgo/wthr/src/backup"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// Task represents a scheduled task
// AI.md PART 19: Scheduler uses cron expressions, not intervals
type Task struct {
	Name string
	// Cron expression: "0 2 * * *", "@hourly", "@every 5m"
	Schedule string
	Fn       func() error
	schedule Schedule
	nextRun  time.Time
	// Can be toggled on/off
	enabled bool
	// Last execution time
	lastRun *time.Time
	// Consecutive failed attempts already retried, drives retryDelays
	retryCount int
	// configuredRetry is true when the task has its own retry_on_fail/retry_delay settings
	configuredRetry bool
	retryOnFail     bool
	retryDelay      time.Duration
	mu              sync.Mutex
}

// retryDelays is AI.md PART 19's retry policy: max_retries 3, retry_delay 5m,
// exponential backoff. A run that fails is re-attempted after 5m, then 10m,
// then 20m; after the third retry the task waits for its next scheduled
// occurrence like any other.
var retryDelays = []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute}

// defaultCatchUpWindow is AI.md PART 19's scheduler.catch_up_window default: an
// occurrence missed while the process was down is executed at startup only if
// it fell inside this window.
const defaultCatchUpWindow = 1 * time.Hour

// maxCatchUpScan bounds the walk used to find the most recent occurrence inside
// the catch-up window. A schedule that fires more often than the scan can cover
// (e.g. "@every 1s") is one that never needs catching up, so stopping early
// costs nothing.
const maxCatchUpScan = 512

// shutdownDrainTimeout is how long Stop waits for in-flight task executions
// before force-releasing this node's locks. AI.md PART 19: "Wait for running
// tasks to complete (max 30 seconds)".
const shutdownDrainTimeout = 30 * time.Second

// LockTimeout is how long a lock is valid before auto-release (5 minutes per AI.md)
const LockTimeout = 5 * time.Minute

// tickInterval is how often the scheduler loop wakes up to check for due
// tasks. AI.md PART 19: "Use Go's time/ticker - No external cron libraries
// required" - one second gives sub-minute schedules (e.g. "@every 30s") the
// precision they need while staying cheap to poll.
const tickInterval = 1 * time.Second

// Scheduler manages scheduled tasks using a built-in time.Ticker loop
// AI.md PART 19: Built-in scheduler with cron expression support
type Scheduler struct {
	ticker  *time.Ticker
	stopCh  chan struct{}
	stopped chan struct{}
	tasks   map[string]*Task
	db      *sql.DB
	nodeID  string
	mu      sync.RWMutex
	running bool
	// Tracks in-flight task executions so shutdown can drain them
	inFlight sync.WaitGroup
	// Names of tasks currently executing on this node, for interrupted-task marking
	active   map[string]bool
	activeMu sync.Mutex
	// AI.md PART 19 scheduler.catch_up_window
	catchUpWindow time.Duration
	// Timezone cron schedules are evaluated in (server.scheduler.timezone)
	loc *time.Location
	// Effective server.scheduler settings; nil until Configure is called
	settings *config.SchedulerConfig
}

// NewScheduler creates a new scheduler instance backed by a built-in
// time.Ticker loop instead of an external cron library.
func NewScheduler(db *sql.DB) *Scheduler {
	// Get node ID from hostname
	nodeID, err := getNodeID()
	if err != nil {
		nodeID = "default"
	}

	return &Scheduler{
		tasks:         make(map[string]*Task),
		active:        make(map[string]bool),
		db:            db,
		nodeID:        nodeID,
		catchUpWindow: defaultCatchUpWindow,
		loc:           time.Local,
	}
}

// Configure applies the server.scheduler settings (AI.md PART 19): the
// timezone cron schedules run in, the catch-up window, and each registered
// task's schedule and enabled state. It is safe to call again after every
// config change, which is how admin edits and file reloads reach the running
// scheduler without a restart.
func (s *Scheduler) Configure(cfg config.SchedulerConfig) {
	copied := cfg
	copied.Tasks = make(map[string]config.SchedulerTask, len(cfg.Tasks))
	for name, task := range cfg.Tasks {
		copied.Tasks[name] = task
	}

	s.mu.Lock()
	s.loc = copied.SchedulerLocation()
	s.catchUpWindow = copied.SchedulerCatchUp()
	s.settings = &copied
	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	s.mu.Unlock()

	for _, task := range tasks {
		s.applyTaskSettings(task)
	}
}

// location returns the timezone schedules are evaluated in.
func (s *Scheduler) location() *time.Location {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.loc == nil {
		return time.Local
	}
	return s.loc
}

// applyTaskSettings copies a task's configured schedule and enabled state onto
// the running task and into its scheduler_tasks row. A task with no entry in
// server.scheduler.tasks keeps the values it was registered with; an invalid
// schedule keeps the previous one and is logged.
func (s *Scheduler) applyTaskSettings(task *Task) {
	s.mu.RLock()
	cfg := s.settings
	loc := s.loc
	s.mu.RUnlock()
	if cfg == nil {
		return
	}
	if _, configured := cfg.Tasks[task.Name]; !configured {
		return
	}
	wanted := cfg.SchedulerTaskSettings(task.Name)

	parsed, err := parseSchedule(wanted.Schedule)
	if err != nil {
		log.Printf("WARN: Task '%s' keeps schedule '%s': configured schedule '%s' is invalid: %v", task.Name, task.Schedule, wanted.Schedule, err)
		parsed = nil
	}

	task.mu.Lock()
	if parsed != nil {
		task.Schedule = wanted.Schedule
		task.schedule = parsed
		task.nextRun = parsed.Next(time.Now().In(loc))
	}
	task.enabled = wanted.Enabled
	task.retryOnFail = wanted.RetryOnFail
	task.retryDelay = 0
	if wanted.RetryDelay != "" {
		if d, err := config.ParseDuration(wanted.RetryDelay); err == nil && d > 0 {
			task.retryDelay = d
		}
	}
	task.configuredRetry = wanted.RetryDelay != ""
	task.mu.Unlock()

	s.ensureTaskStateRow(task)
}

// SetCatchUpWindow overrides how far back startup looks for occurrences that
// were missed while the process was down. A window of zero disables catch-up.
func (s *Scheduler) SetCatchUpWindow(window time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if window < 0 {
		window = 0
	}
	s.catchUpWindow = window
}

// getNodeID returns a unique identifier for this node
func getNodeID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}

// AddTask adds a new task to the scheduler with a cron schedule
// AI.md PART 19: Schedule format - "0 2 * * *", "@hourly", "@daily", "@every 5m"
func (s *Scheduler) AddTask(name string, schedule string, fn func() error) error {
	parsed, err := parseSchedule(schedule)
	if err != nil {
		return fmt.Errorf("failed to add task '%s' with schedule '%s': %w", name, schedule, err)
	}

	s.mu.Lock()
	task := &Task{
		Name:     name,
		Schedule: schedule,
		Fn:       fn,
		schedule: parsed,
		nextRun:  parsed.Next(time.Now().In(s.locationLocked())),
		enabled:  true,
		lastRun:  nil,
	}

	s.tasks[name] = task
	configured := s.settings != nil
	s.mu.Unlock()

	s.ensureTaskStateRow(task)

	if configured {
		s.applyTaskSettings(task)
	}

	return nil
}

// locationLocked returns the schedule timezone; the caller must hold s.mu.
func (s *Scheduler) locationLocked() *time.Location {
	if s.loc == nil {
		return time.Local
	}
	return s.loc
}

// ensureTaskStateRow creates the task's scheduler_tasks row if it does
// not exist yet, so every later state write has a target. Existing rows keep
// their counters and only have their schedule/next_run refreshed.
func (s *Scheduler) ensureTaskStateRow(task *Task) {
	if s.db == nil {
		return
	}

	nextRun := ""
	if !task.nextRun.IsZero() {
		nextRun = dbtime.FormatSQLTimestamp(task.nextRun)
	}

	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO scheduler_tasks (task_id, task_name, schedule, next_run)
		 VALUES (?, ?, ?, ?)`,
		task.Name, task.Name, task.Schedule, nextRun,
	); err != nil {
		log.Printf("WARN: Failed to create scheduler state row for task '%s': %v", task.Name, err)
		return
	}

	task.mu.Lock()
	enabled := task.enabled
	task.mu.Unlock()

	if _, err := s.db.Exec(
		"UPDATE scheduler_tasks SET task_name = ?, schedule = ?, next_run = ?, enabled = ? WHERE task_id = ?",
		task.Name, task.Schedule, nextRun, enabled, task.Name,
	); err != nil {
		log.Printf("WARN: Failed to refresh scheduler state row for task '%s': %v", task.Name, err)
	}
}

// AddTaskInterval adds a task with a time.Duration interval (convenience method)
// Converts to @every format for the built-in scheduler
func (s *Scheduler) AddTaskInterval(name string, interval time.Duration, fn func() error) error {
	schedule := fmt.Sprintf("@every %s", interval.String())
	return s.AddTask(name, schedule, fn)
}

// StartScheduler starts the scheduler's ticker loop
// AI.md PART 19: "Use Go's time/ticker - No external cron libraries required"
func (s *Scheduler) StartScheduler() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.ticker = time.NewTicker(tickInterval)
	s.stopCh = make(chan struct{})
	s.stopped = make(chan struct{})
	s.running = true
	taskCount := len(s.tasks)
	s.mu.Unlock()

	// AI.md PART 19 Startup Behavior: load state, replay occurrences missed
	// while the process was down, then start the loop.
	s.runMissedTasks()

	go s.run()

	log.Printf("INFO: Task manager has started (%d scheduled tasks)", taskCount)
}

// runMissedTasks replays every occurrence that fell due while the process was
// down, provided it lies inside catchUpWindow. AI.md PART 19 requires the
// replay to happen "in order of original scheduled time", so the queue is
// executed sequentially on one goroutine rather than fanned out.
func (s *Scheduler) runMissedTasks() {
	s.mu.RLock()
	window := s.catchUpWindow
	candidates := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		candidates = append(candidates, task)
	}
	s.mu.RUnlock()

	if window <= 0 {
		return
	}

	type missedRun struct {
		task *Task
		due  time.Time
	}

	now := time.Now().In(s.location())
	missed := make([]missedRun, 0)

	for _, task := range candidates {
		task.mu.Lock()
		enabled := task.enabled
		sched := task.schedule
		task.mu.Unlock()

		if !enabled || sched == nil {
			continue
		}

		due, ok := latestOccurrenceBefore(sched, now, window)
		if !ok {
			continue
		}

		// A missing history row means the task has never run on this database,
		// so there is nothing to catch up on - a first-ever run belongs on the
		// task's own schedule, not at startup.
		lastRun, err := s.GetLastTaskRun(task.Name)
		if err != nil || lastRun == nil {
			continue
		}
		if !lastRun.StartTime.Before(due) {
			continue
		}

		missed = append(missed, missedRun{task: task, due: due})
	}

	if len(missed) == 0 {
		return
	}

	sort.SliceStable(missed, func(i, j int) bool {
		return missed[i].due.Before(missed[j].due)
	})

	s.inFlight.Add(1)
	go func() {
		defer s.inFlight.Done()
		for _, run := range missed {
			log.Printf("INFO: Catch-up: running task '%s' missed at %s", run.task.Name, run.due.UTC().Format(time.RFC3339))
			s.executeTask(run.task)
		}
	}()
}

// latestOccurrenceBefore returns the most recent time sched fired at or before
// now, searching back at most window. Schedules are only queryable forwards, so
// the walk starts at the far edge of the window and steps forward; a schedule
// that fails to advance ends the walk rather than looping forever.
func latestOccurrenceBefore(sched Schedule, now time.Time, window time.Duration) (time.Time, bool) {
	cursor := now.Add(-window)
	var latest time.Time
	found := false

	for i := 0; i < maxCatchUpScan; i++ {
		next := sched.Next(cursor)
		if !next.After(cursor) || next.After(now) {
			break
		}
		latest = next
		found = true
		cursor = next
	}

	return latest, found
}

// run is the scheduler's main loop, driven by a time.Ticker.
func (s *Scheduler) run() {
	defer close(s.stopped)

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-s.ticker.C:
			s.runDueTasks(now)
		}
	}
}

// runDueTasks executes (asynchronously) every task whose nextRun has passed,
// and advances each executed task's nextRun using its Schedule.
func (s *Scheduler) runDueTasks(now time.Time) {
	s.mu.RLock()
	now = now.In(s.locationLocked())
	due := make([]*Task, 0)
	for _, task := range s.tasks {
		task.mu.Lock()
		if task.enabled && !task.nextRun.IsZero() && !now.Before(task.nextRun) {
			task.nextRun = task.schedule.Next(now)
			due = append(due, task)
		}
		task.mu.Unlock()
	}
	s.mu.RUnlock()

	for _, task := range due {
		s.inFlight.Add(1)
		go func(t *Task) {
			defer s.inFlight.Done()
			s.executeTask(t)
		}(task)
	}
}

// StopScheduler stops the scheduler's ticker loop and drains in-flight task
// executions per AI.md PART 19's Shutdown Behavior: stop dispatching, wait up
// to 30 seconds for running tasks, then release this node's locks - force-
// releasing and marking still-running tasks for retry if the wait timed out.
func (s *Scheduler) StopScheduler() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	log.Println("INFO: Stopping scheduler...")
	s.ticker.Stop()
	close(s.stopCh)
	stopped := s.stopped
	s.running = false
	s.mu.Unlock()

	// Wait for the loop goroutine to exit
	<-stopped

	drained := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		s.releaseNodeLocks()
	case <-time.After(shutdownDrainTimeout):
		interrupted := s.activeTaskNames()
		log.Printf("WARN: Scheduler shutdown timed out after %v with %d task(s) still running", shutdownDrainTimeout, len(interrupted))
		s.markTasksInterrupted(interrupted)
		s.releaseNodeLocks()
	}

	log.Println("OK: Scheduler stopped")
}

// activeTaskNames returns the names of the tasks currently executing on this node.
func (s *Scheduler) activeTaskNames() []string {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	names := make([]string, 0, len(s.active))
	for name := range s.active {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// releaseNodeLocks clears every scheduler lock still held by this node so a
// restart (or another cluster node) can pick the tasks up immediately.
func (s *Scheduler) releaseNodeLocks() {
	if s.db == nil {
		return
	}

	if _, err := s.db.Exec(
		"UPDATE scheduler_tasks SET locked_by = NULL, locked_at = NULL WHERE locked_by = ?",
		s.nodeID,
	); err != nil {
		log.Printf("WARN: Failed to release scheduler locks for node %s: %v", s.nodeID, err)
	}
}

// markTasksInterrupted records tasks that were still running when the shutdown
// drain timed out, so their state reflects the interruption and the next start
// retries them instead of reporting a stale success.
func (s *Scheduler) markTasksInterrupted(names []string) {
	if s.db == nil || len(names) == 0 {
		return
	}

	for _, name := range names {
		if _, err := s.db.Exec(
			"UPDATE scheduler_tasks SET last_status = ?, last_error = ?, fail_count = fail_count + 1 WHERE task_id = ?",
			"interrupted", "interrupted by server shutdown", name,
		); err != nil {
			log.Printf("WARN: Failed to mark task '%s' interrupted: %v", name, err)
		}
	}
}

// isGlobalTask returns true if this task should only run on one node
func isGlobalTask(taskName string) bool {
	return config.IsGlobalSchedulerTask(taskName)
}

// acquireTaskLock attempts to acquire a distributed lock for a task.
// AI.md PART 19: Cluster-aware task locking.
// schedule is the task's own cron expression: scheduler_tasks.schedule
// is NOT NULL, and this insert is the row's first writer on a fresh database,
// so the real expression has to travel with the lock request rather than be
// backfilled later by whoever happens to update the row next.
func (s *Scheduler) acquireTaskLock(taskName, schedule string) bool {
	// For non-global tasks, always allow (run on every node)
	if !isGlobalTask(taskName) {
		return true
	}

	db := database.GetServerDB()
	ctx := context.Background()
	nowText := time.Now().UTC().Format(sqlTimestampLayout)
	lockExpiry := time.Now().UTC().Add(-LockTimeout)

	// Read the current holder first. The staleness test used to live in SQL
	// ("locked_at < ?" against a bound time.Time), which compared two different
	// text encodings whenever the row had been written by a different producer
	// and therefore expired locks early, late or never. Every comparison now
	// happens in Go against parsed instants; only the canonical text written
	// here ever lands in locked_at.
	var lockedBy sql.NullString
	var lockedAt interface{}
	err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect,
		"SELECT locked_by, locked_at FROM scheduler_tasks WHERE task_id = ?",
		taskName,
	).Scan(&lockedBy, &lockedAt)

	// No row yet: this node is the row's first writer. ON CONFLICT DO NOTHING
	// keeps the insert atomic, so a node that loses the race to create the row
	// simply does not get the lock this tick.
	if err == sql.ErrNoRows {
		result, insertErr := database.ExecContext(ctx, db, database.TimeoutWrite, `
			INSERT INTO scheduler_tasks (task_id, task_name, schedule, locked_by, locked_at, enabled)
			VALUES (?, ?, ?, ?, ?, true)
			ON CONFLICT(task_id) DO NOTHING
		`, taskName, taskName, schedule, s.nodeID, nowText)
		if insertErr != nil {
			log.Printf("WARNING: Failed to acquire lock for task '%s': %v", taskName, insertErr)
			return false
		}

		inserted, _ := result.RowsAffected()
		return inserted > 0
	}

	if err != nil {
		log.Printf("WARNING: Failed to read lock state for task '%s': %v", taskName, err)
		return false
	}

	// Decide acquirability from parsed instants:
	// 1. Nobody holds it, take it
	// 2. We hold it, refresh it
	// 3. Somebody else holds it, take it only when their lock is provably older
	//    than LockTimeout. A NULL or unparseable locked_at is treated as HELD,
	//    never as stale: guessing "stale" would let two nodes run the same
	//    global task at once, while guessing "held" only skips a tick.
	if lockedBy.Valid && lockedBy.String != s.nodeID {
		heldSince, ok := parseStoredTimestamp(lockedAt)
		if !ok || !heldSince.Before(lockExpiry) {
			return false
		}
	}

	// Compare-and-swap on the holder observed above so a concurrent stealer
	// loses: whichever node's UPDATE runs first flips locked_by, and the other
	// node's WHERE clause no longer matches.
	var result sql.Result
	if lockedBy.Valid {
		result, err = database.ExecContext(ctx, db, database.TimeoutWrite, `
			UPDATE scheduler_tasks
			SET schedule = ?, locked_by = ?, locked_at = ?
			WHERE task_id = ? AND locked_by = ?
		`, schedule, s.nodeID, nowText, taskName, lockedBy.String)
	} else {
		result, err = database.ExecContext(ctx, db, database.TimeoutWrite, `
			UPDATE scheduler_tasks
			SET schedule = ?, locked_by = ?, locked_at = ?
			WHERE task_id = ? AND locked_by IS NULL
		`, schedule, s.nodeID, nowText, taskName)
	}

	if err != nil {
		log.Printf("WARNING: Failed to acquire lock for task '%s': %v", taskName, err)
		return false
	}

	if rowsAffected, affectedErr := result.RowsAffected(); affectedErr == nil && rowsAffected > 0 {
		return true
	}

	// Zero affected rows means either a competitor won the row or the driver
	// reports "no change" for an update that rewrote identical values (MySQL
	// does this when we refresh our own lock inside the same second). Re-read
	// the holder to tell the two apart.
	var confirmed sql.NullString
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect,
		"SELECT locked_by FROM scheduler_tasks WHERE task_id = ?",
		taskName,
	).Scan(&confirmed); err != nil {
		return false
	}

	return confirmed.Valid && confirmed.String == s.nodeID
}

// releaseTaskLock releases the distributed lock for a task
func (s *Scheduler) releaseTaskLock(taskName string) {
	if !isGlobalTask(taskName) {
		return
	}

	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		UPDATE scheduler_tasks
		SET locked_by = NULL, locked_at = NULL
		WHERE task_id = ? AND locked_by = ?
	`, taskName, s.nodeID)

	if err != nil {
		log.Printf("WARNING: Failed to release lock for task '%s': %v", taskName, err)
	}
}

// executeTask executes a task and logs results
func (s *Scheduler) executeTask(task *Task) {
	// Check if task is enabled
	task.mu.Lock()
	if !task.enabled {
		task.mu.Unlock()
		return
	}
	task.mu.Unlock()

	// AI.md PART 19: Cluster-aware locking for global tasks
	if !s.acquireTaskLock(task.Name, task.Schedule) {
		// Another node is running this task, skip
		return
	}
	defer s.releaseTaskLock(task.Name)

	s.markActive(task.Name, true)
	defer s.markActive(task.Name, false)

	start := time.Now()
	err := runTaskFn(task)
	end := time.Now()
	elapsed := end.Sub(start)

	// Update last run time
	task.mu.Lock()
	task.lastRun = &end
	task.mu.Unlock()

	if err != nil {
		log.Printf("ERROR: Task '%s' failed after %v: %v", task.Name, elapsed, err)
	} else {
		log.Printf("OK: Task '%s' completed in %v", task.Name, elapsed)
	}

	// AI.md PART 19 retry policy - a failed run is re-attempted before its next
	// scheduled occurrence, up to max_retries times.
	s.applyRetryPolicy(task, end, err)

	// Log to audit if enabled
	s.logTaskExecution(task.Name, elapsed, err)

	// AI.md PART 19 Implementation Requirement 2: all scheduler state lives in
	// server.db and survives restarts.
	s.persistTaskState(task, end, err)

	// Record in database last, so that once this row becomes visible to a
	// caller polling GetLastTaskRun, every side effect of the run has
	// already completed - avoids a caller tearing down the DB (e.g. test
	// cleanup) while logTaskExecution is still in flight.
	if recordErr := s.RecordTaskRun(task.Name, start, end, err); recordErr != nil {
		log.Printf("ERROR: Failed to record scheduler task run for %q: %v", task.Name, recordErr)
	}
}

// runTaskFn invokes a task's function inside its own panic/recover boundary.
// AI.md PART 19: a panicking task MUST be logged and marked failed for that run
// while the scheduler loop keeps running and still fires the next occurrence.
func runTaskFn(task *Task) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: Task '%s' panicked: %v\n%s", task.Name, r, debug.Stack())
			err = fmt.Errorf("task panicked: %v", r)
		}
	}()

	return task.Fn()
}

// isActive reports whether a task is executing on this node right now.
func (s *Scheduler) isActive(taskName string) bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return s.active[taskName]
}

// markActive records whether a task is currently executing on this node, so
// shutdown can name the tasks it interrupted.
func (s *Scheduler) markActive(taskName string, active bool) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	if active {
		s.active[taskName] = true
		return
	}
	delete(s.active, taskName)
}

// applyRetryPolicy advances a failed task's next run to the retry delay for its
// current attempt (5m, then 10m, then 20m). Once max_retries is exhausted, or
// when the run succeeded, the task falls back to its normal schedule.
func (s *Scheduler) applyRetryPolicy(task *Task, end time.Time, runErr error) {
	task.mu.Lock()
	defer task.mu.Unlock()

	if runErr == nil {
		task.retryCount = 0
		return
	}

	delays := retryDelays
	if task.configuredRetry {
		if !task.retryOnFail || task.retryDelay <= 0 {
			task.retryCount = 0
			return
		}
		delays = []time.Duration{task.retryDelay, task.retryDelay * 2, task.retryDelay * 4}
	}

	if task.retryCount >= len(delays) {
		task.retryCount = 0
		return
	}

	retryAt := end.Add(delays[task.retryCount])
	task.retryCount++

	// Never push a run later than its own schedule would have placed it.
	if task.nextRun.IsZero() || retryAt.Before(task.nextRun) {
		task.nextRun = retryAt
		log.Printf("INFO: Task '%s' retry %d/%d scheduled for %s", task.Name, task.retryCount, len(delays), retryAt.UTC().Format(time.RFC3339))
	}
}

// persistTaskState writes the outcome of a run into scheduler_tasks so
// the admin panel, health endpoint, and a restarted process all see real
// last_run/last_status/next_run/run_count/fail_count values.
func (s *Scheduler) persistTaskState(task *Task, end time.Time, runErr error) {
	if s.db == nil {
		return
	}

	task.mu.Lock()
	nextRun := task.nextRun
	task.mu.Unlock()

	status := "success"
	errText := ""
	failIncrement := 0
	if runErr != nil {
		status = "failed"
		errText = runErr.Error()
		failIncrement = 1
	}

	nextRunText := ""
	if !nextRun.IsZero() {
		nextRunText = dbtime.FormatSQLTimestamp(nextRun)
	}

	if _, err := s.db.Exec(
		`UPDATE scheduler_tasks
		 SET last_run = ?, last_status = ?, last_error = ?, next_run = ?,
		     run_count = run_count + 1, fail_count = fail_count + ?
		 WHERE task_id = ?`,
		dbtime.FormatSQLTimestamp(end), status, errText, nextRunText, failIncrement, task.Name,
	); err != nil {
		log.Printf("WARN: Failed to persist scheduler state for task '%s': %v", task.Name, err)
	}
}

// logTaskExecution logs task execution to audit log
func serverSettingEnabled(key string) bool {
	settings := &model.SettingsModel{DB: database.GetServerDB()}
	return settings.GetBool(key, false)
}

func (s *Scheduler) logTaskExecution(taskName string, duration time.Duration, err error) {
	status := "success"
	details := fmt.Sprintf("Completed in %v", duration)
	if err != nil {
		status = "error"
		details = fmt.Sprintf("Failed: %v", err)
	}

	// audit_log has no user_id column: the actor is described by the
	// actor_type/actor_id pair, and ulid is UNIQUE NOT NULL. This mirrors the
	// canonical writer in src/server/middleware/audit.go, including its
	// crypto/rand-seeded ULID, so both writers produce identical row shapes.
	now := time.Now()
	auditID := ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()

	_, insertErr := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, ?, 'scheduler', ?, ?, 'scheduler', ?, ?, 'system', 'scheduler', ?)
	`, auditID, now.UTC().Format(sqlTimestampLayout), s.nodeID, taskName, taskName, details, status)

	if insertErr != nil {
		log.Printf("WARNING: Failed to log scheduler task: %v", insertErr)
	}
}

// GetTaskStatus returns status of all tasks
func (s *Scheduler) GetTaskStatus() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(s.tasks))

	for _, task := range s.tasks {
		task.mu.Lock()
		nextRun := task.nextRun
		status = append(status, map[string]interface{}{
			"name":     task.Name,
			"schedule": task.Schedule,
			"enabled":  task.enabled,
			"lastRun":  task.lastRun,
			"nextRun":  nextRun,
		})
		task.mu.Unlock()
	}

	return status
}

// CleanupOldSessions removes expired sessions
// user_sessions.expires_at has more than one producer (a bound Go time.Time in
// the local zone, and canonical UTC text), so the expiry test runs in Go
// against a UTC cutoff instead of SQLite's datetime('now') - comparing those
// mixed layouts as text deleted sessions that had not actually expired.
func CleanupOldSessions() error {
	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetUsersDB(), "user_sessions", "id", "expires_at", time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d expired sessions", rowsAffected)
	}

	return nil
}

// CleanupOldAuditLogs removes audit logs older than retention period
func CleanupOldAuditLogs() error {
	// Get retention days from settings. Key matches the one the admin panel
	// reads/writes (handler/admin.go "scheduler_cleanup_audit_logs_days" ->
	// "scheduler.cleanup_audit_logs_days") - a prior mismatched key here
	// ("audit.retention_days") meant the configured value was silently
	// ignored and the 90-day default always applied.
	retentionDays := 90
	if value, exists, err := config.ReadSetting("scheduler.cleanup_audit_logs_days"); err == nil && exists {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value))); parseErr == nil && parsed > 0 {
			retentionDays = parsed
		}
	}

	// The retained window is measured against audit_log.timestamp (the
	// table has no created_at column). That column has two producers - SQLite's
	// CURRENT_TIMESTAMP default and the explicit writes from this package and
	// src/server/middleware/audit.go - so the age test runs in Go against a UTC
	// cutoff instead of as an SQL text comparison, and rows whose timestamp is
	// NULL or unparseable are left alone rather than deleted.
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetServerDB(), "audit_log", "id", "timestamp", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup audit logs: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d old audit logs (retention: %d days)", rowsAffected, retentionDays)
	}

	return nil
}

// CheckWeatherAlerts checks for weather alerts on saved locations
func CheckWeatherAlerts() error {
	// Get all locations with alerts enabled
	rows, err := database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutComplexSelect, `
		SELECT l.id, l.name, l.latitude, l.longitude, l.user_id
		FROM user_saved_locations l
		JOIN users u ON l.user_id = u.id
		WHERE l.alerts_enabled = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to fetch locations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("WARNING: failed to close saved location alert rows: %v", err)
		}
	}()

	alertCount := 0

	// Bound each Open-Meteo fetch so a stalled upstream can't hang the task.
	client := &http.Client{Timeout: 30 * time.Second}

	for rows.Next() {
		var locationID int
		var name string
		var latitude, longitude float64
		var userID int

		if err := rows.Scan(&locationID, &name, &latitude, &longitude, &userID); err != nil {
			log.Printf("WARNING: Failed to scan location: %v", err)
			continue
		}

		// Fetch weather data from Open-Meteo API
		url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,wind_speed_10m,precipitation,weather_code&temperature_unit=fahrenheit&wind_speed_unit=mph&precipitation_unit=inch",
			latitude, longitude)

		var weatherData struct {
			Current struct {
				Temperature   float64 `json:"temperature_2m"`
				WindSpeed     float64 `json:"wind_speed_10m"`
				Precipitation float64 `json:"precipitation"`
				WeatherCode   int     `json:"weather_code"`
			} `json:"current"`
		}

		// Fetch and decode in a closure so the response body is closed on every
		// iteration rather than deferred until the whole loop returns.
		fetchErr := func() error {
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Open-Meteo returned HTTP %d", resp.StatusCode)
			}
			if resp.ContentLength > 8<<20 {
				return fmt.Errorf("Open-Meteo response exceeds 8 MiB")
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
			if err != nil {
				return fmt.Errorf("read Open-Meteo response: %w", err)
			}
			if len(body) > 8<<20 {
				return fmt.Errorf("Open-Meteo response exceeds 8 MiB")
			}
			if err := json.Unmarshal(body, &weatherData); err != nil {
				return fmt.Errorf("decode Open-Meteo response: %w", err)
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				return fmt.Errorf("decode Open-Meteo response fields: %w", err)
			}
			var currentFields map[string]json.RawMessage
			if len(envelope["current"]) == 0 || json.Unmarshal(envelope["current"], &currentFields) != nil {
				return fmt.Errorf("Open-Meteo response is missing current weather")
			}
			for _, key := range []string{"temperature_2m", "wind_speed_10m", "precipitation", "weather_code"} {
				if len(currentFields[key]) == 0 {
					return fmt.Errorf("Open-Meteo response is missing current.%s", key)
				}
			}
			return nil
		}()
		if fetchErr != nil {
			log.Printf("WARNING: Failed to fetch weather for %s: %v", name, fetchErr)
			continue
		}

		// Check for alert conditions and create notifications
		created := checkAndCreateAlerts(userID, locationID, name, weatherData)
		alertCount += created
	}

	if alertCount > 0 {
		log.Printf("INFO: Created %d weather alerts", alertCount)
	}

	return nil
}

// checkAndCreateAlerts checks weather conditions and creates notifications
func checkAndCreateAlerts(userID, locationID int, locationName string, weather struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		Precipitation float64 `json:"precipitation"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
}) int {
	alertCount := 0
	lang := resolveUserLang(userID)
	actionLabel := translateAlert(lang, "scheduler.weather_alerts.view_forecast")

	// Check for extreme cold (below 32°F / 0°C)
	if weather.Current.Temperature < 32 {
		createNotification(userID, model.NotificationTypeWarning,
			translateAlert(lang, "scheduler.weather_alerts.freezing_title"),
			fmt.Sprintf(translateAlert(lang, "scheduler.weather_alerts.freezing_message"), locationName, weather.Current.Temperature),
			actionLabel, fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for extreme heat (above 95°F / 35°C)
	if weather.Current.Temperature > 95 {
		createNotification(userID, model.NotificationTypeWarning,
			translateAlert(lang, "scheduler.weather_alerts.heat_title"),
			fmt.Sprintf(translateAlert(lang, "scheduler.weather_alerts.heat_message"), locationName, weather.Current.Temperature),
			actionLabel, fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for high winds (above 40 mph)
	if weather.Current.WindSpeed > 40 {
		createNotification(userID, model.NotificationTypeWarning,
			translateAlert(lang, "scheduler.weather_alerts.wind_title"),
			fmt.Sprintf(translateAlert(lang, "scheduler.weather_alerts.wind_message"), locationName, weather.Current.WindSpeed),
			actionLabel, fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for heavy precipitation (above 0.5 inches)
	if weather.Current.Precipitation > 0.5 {
		createNotification(userID, model.NotificationTypeInfo,
			translateAlert(lang, "scheduler.weather_alerts.rain_title"),
			fmt.Sprintf(translateAlert(lang, "scheduler.weather_alerts.rain_message"), locationName, weather.Current.Precipitation),
			actionLabel, fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for severe weather codes (thunderstorms, snow, etc.)
	if weather.Current.WeatherCode >= 95 {
		createNotification(userID, model.NotificationTypeWarning,
			translateAlert(lang, "scheduler.weather_alerts.severe_title"),
			fmt.Sprintf(translateAlert(lang, "scheduler.weather_alerts.severe_message"), locationName),
			actionLabel, fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	return alertCount
}

// resolveUserLang looks up the recipient's stored language preference for a
// scheduler-generated notification. Scheduler tasks run with no live
// http.Request, so they cannot use handler.Lang(r) - the recipient's saved
// preference (AI.md PART 31 fallback chain step 4: default en) is the
// closest equivalent available in a background context.
func resolveUserLang(userID int) string {
	instance := i18n.GetGlobalI18n()
	if instance == nil {
		return "en"
	}
	users := &model.UserModel{DB: database.GetUsersDB()}
	user, err := users.GetByID(int64(userID))
	if err != nil || user == nil || user.Language == "" || !instance.IsSupported(user.Language) {
		return instance.GetDefaultLanguage()
	}
	return user.Language
}

// translateAlert resolves a translation key for a background scheduler task.
// A missing global i18n instance falls back to the key itself so an alert
// never fails to send because translations are unavailable (AI.md PART 31).
func translateAlert(lang, key string) string {
	if instance := i18n.GetGlobalI18n(); instance != nil {
		return instance.T(lang, key)
	}
	return key
}

// createNotification creates a user notification for a weather alert.
//
// user_notifications has no link column and constrains type with a CHECK, so
// the row is written through model.UserNotificationModel.Create - the project's
// single writer for this table. That gives the row its ULID primary key, its
// display value and its 30-day expiry, and carries the deep link in action_json
// as a NotificationAction, which is where the schema actually keeps a link.
func createNotification(userID int, notifType model.NotificationType, title, message, actionLabel, link string) {
	notifications := &model.UserNotificationModel{DB: database.GetUsersDB()}

	// src/server/service/notification_service.go renders every non-security
	// severity as a toast; weather alerts follow the same mapping.
	action := &model.NotificationAction{Label: actionLabel, URL: link}

	if _, err := notifications.CreateUserNotification(userID, notifType, model.NotificationDisplayToast, title, message, action); err != nil {
		log.Printf("WARNING: Failed to create notification: %v", err)
	}
}

// CreateSystemBackup creates a backup of the database
// AI.md PART 19/25: backup_daily task - creates verified backups
func CreateSystemBackup() error {
	// Get backup settings
	if !serverSettingEnabled("backup.enabled") {
		// Backups disabled, skip silently
		return nil
	}

	// Get paths per AI.md PART 4
	p := path.GetDefaultPaths("wthr")
	if p == nil {
		return fmt.Errorf("failed to get default paths for backup")
	}

	// Create backup service per AI.md PART 22 (Backup Command)
	svc := backup.New(p.ConfigDir, p.DataDir)

	// AI.md PART 22 Backup Creation Flow step 2: abort (log
	// backup.skipped_disk_full, level=error) without creating a backup when
	// free space is under 2x the most recent backup's size, or disk usage is
	// above disk_threshold (default 90%).
	backupDir := backup.ResolveDir(p.DataDir)
	if skip, reason := backupDiskSpaceExceeded(backupDir); skip {
		logBackupSkippedDiskFullAudit(database.GetServerDB(), reason)
		log.Printf("ERROR: %s", reason)
		return nil
	}

	encryptionPassword, err := scheduledBackupPassword()
	if err != nil {
		return err
	}

	retention := systemBackupRetention()

	// AI.md PART 22 Backup Creation Flow steps 3-4: create and verify the
	// full backup ({project_name}_backup_YYYY-MM-DD.tar.gz[.enc]).
	fullOpts := backup.BackupOptions{
		ConfigDir:   p.ConfigDir,
		DataDir:     p.DataDir,
		OutputPath:  "", // Auto-generated from Kind
		Kind:        backup.KindDailyFull,
		Password:    encryptionPassword,
		IncludeSSL:  false, // Don't include SSL in automated backups
		IncludeData: false, // Don't include data files in automated backups
		CreatedBy:   "scheduler",
		AppVersion:  appVersion,
		Retention:   retention,
	}

	log.Println("INFO: Starting automated backup...")
	backupPath, deleted, err := svc.CreateBackupArchive(fullOpts)
	if err != nil {
		log.Printf("ERROR: Automated backup failed: %v", err)
		return fmt.Errorf("backup failed: %w", err)
	}

	logBackupRetentionAudit(database.GetServerDB(), "scheduler", "daily", backupPath, deleted)
	log.Printf("OK: Automated backup completed: %s", backupPath)

	// AI.md PART 22 Backup Creation Flow steps 5-6: create and verify the
	// daily incremental ({project_name}-daily.tar.gz[.enc]), replaced in
	// place each run and always exactly 1 file.
	dailyOpts := fullOpts
	dailyOpts.OutputPath = ""
	dailyOpts.Kind = backup.KindDailyIncremental

	dailyPath, dailyDeleted, dailyErr := svc.CreateBackupArchive(dailyOpts)
	if dailyErr != nil {
		log.Printf("ERROR: Daily incremental backup failed: %v", dailyErr)
		return fmt.Errorf("daily incremental backup failed: %w", dailyErr)
	}

	logBackupRetentionAudit(database.GetServerDB(), "scheduler", "daily", dailyPath, dailyDeleted)
	logBackupDailyUpdatedAudit(database.GetServerDB(), dailyPath)
	log.Printf("OK: Daily incremental backup completed: %s", dailyPath)

	return nil
}

// backupDiskSpaceExceeded implements AI.md PART 22 Backup Creation Flow step
// 2's disk-space guard: skip (abort, do NOT create the backup) when free
// space is under 2x the most recent backup's size, or disk usage is above
// disk_threshold (default 90%, not currently admin-configurable). Returns
// false with no reason when the backup dir doesn't exist yet (first run) or
// space cannot be determined - a guard that can't evaluate must never block
// the first backup. The actual free/total/prior-backup lookups happen here;
// the pass/fail decision itself lives in diskSpaceGuardDecision so it can be
// unit tested with synthetic numbers instead of real (and potentially huge)
// files.
func backupDiskSpaceExceeded(backupDir string) (bool, string) {
	free, freeErr := backup.VolumeFreeBytes(backupDir)
	total, totalErr := backup.VolumeTotalBytes(backupDir)
	if freeErr != nil || totalErr != nil || total <= 0 {
		return false, ""
	}

	backups, listErr := backup.ListDatedBackups(backupDir)
	var mostRecentSize int64
	hasPriorBackup := listErr == nil && len(backups) > 0
	if hasPriorBackup {
		mostRecentSize = backups[len(backups)-1].Size
	}

	return diskSpaceGuardDecision(free, total, mostRecentSize, hasPriorBackup)
}

// diskSpaceGuardDecision is the pure decision logic behind
// backupDiskSpaceExceeded: given already-resolved free/total disk bytes and
// the size of the most recent prior backup (if any), decide whether AI.md
// PART 22's disk-space guard should abort the scheduled backup run.
func diskSpaceGuardDecision(free, total, mostRecentBackupSize int64, hasPriorBackup bool) (bool, string) {
	const diskThresholdPercent = 90

	usedPercent := float64(total-free) / float64(total) * 100
	if usedPercent > diskThresholdPercent {
		return true, fmt.Sprintf("backup.skipped_disk_full: disk usage %.1f%% exceeds threshold %d%%", usedPercent, diskThresholdPercent)
	}

	if !hasPriorBackup {
		return false, ""
	}

	if free < 2*mostRecentBackupSize {
		return true, fmt.Sprintf("backup.skipped_disk_full: free space %d bytes is under 2x most recent backup size %d bytes", free, mostRecentBackupSize)
	}

	return false, ""
}

// logBackupSkippedDiskFullAudit records AI.md PART 22/36321's
// "backup.skipped_disk_full" audit event ("Backup skipped - insufficient
// free space or disk above threshold") when backupDiskSpaceExceeded aborts a
// scheduled run before any backup file is created.
func logBackupSkippedDiskFullAudit(db *sql.DB, reason string) {
	if db == nil {
		return
	}

	details, marshalErr := json.Marshal(map[string]interface{}{
		"reason": reason,
	})
	if marshalErr != nil {
		log.Printf("WARNING: failed to marshal backup.skipped_disk_full audit details: %v", marshalErr)
		return
	}

	now := time.Now()
	_, insertErr := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, ?, 'scheduler', 'daily', 'backup.skipped_disk_full', 'backup', '', ?, 'system', 'scheduler', 'error')
	`, ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(), now.UTC().Format(sqlTimestampLayout), string(details))

	if insertErr != nil {
		log.Printf("WARNING: failed to log backup.skipped_disk_full: %v", insertErr)
	}
}

// logBackupDailyUpdatedAudit records AI.md PART 22/36320's
// "backup.daily_updated" audit event ("Daily incremental updated") after the
// fixed-path daily incremental file is (re)created.
func logBackupDailyUpdatedAudit(db *sql.DB, dailyPath string) {
	if db == nil {
		return
	}

	details, marshalErr := json.Marshal(map[string]interface{}{
		"filename": filepath.Base(dailyPath),
	})
	if marshalErr != nil {
		log.Printf("WARNING: failed to marshal backup.daily_updated audit details: %v", marshalErr)
		return
	}

	now := time.Now()
	_, insertErr := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, ?, 'scheduler', 'daily', 'backup.daily_updated', 'backup', ?, ?, 'system', 'scheduler', 'success')
	`, ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(), now.UTC().Format(sqlTimestampLayout), filepath.Base(dailyPath), string(details))

	if insertErr != nil {
		log.Printf("WARNING: failed to log backup.daily_updated: %v", insertErr)
	}
}

// logBackupRetentionAudit records AI.md PART 22's "backup.retention_cleanup"
// audit event (lines 17009, 36318: "Old backups deleted" / "Deleted files,
// reason, remaining count") after a tiered retention sweep removes at least
// one backup. A no-op sweep writes nothing - there is no cleanup to audit.
// db may be nil (e.g. BackupHourlyTask running before startup wiring
// completes) - that case is treated as "audit unavailable", not an error.
func logBackupRetentionAudit(db *sql.DB, actorType, actorID, backupPath string, deleted []string) {
	if db == nil || len(deleted) == 0 {
		return
	}

	remaining, err := backup.CountBackups(filepath.Dir(backupPath))
	if err != nil {
		log.Printf("WARNING: failed to count remaining backups for audit log: %v", err)
	}

	details, marshalErr := json.Marshal(map[string]interface{}{
		"deleted_files":   deleted,
		"reason":          "tiered retention policy",
		"remaining_count": remaining,
	})
	if marshalErr != nil {
		log.Printf("WARNING: failed to marshal backup retention audit details: %v", marshalErr)
		return
	}

	now := time.Now()
	_, insertErr := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, ?, ?, ?, 'backup.retention_cleanup', 'backup', ?, ?, 'system', 'scheduler', 'success')
	`, ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(), now.UTC().Format(sqlTimestampLayout), actorType, actorID, filepath.Base(backupPath), string(details))

	if insertErr != nil {
		log.Printf("WARNING: failed to log backup retention cleanup: %v", insertErr)
	}
}

// systemBackupRetention reads the admin-configured tiered retention policy
// (AI.md PART 22, "backup.retention.*" keys) for the scheduled system backup.
// Missing or invalid values fall through to backup.DefaultRetention() and do
// not block the scheduled run.
func systemBackupRetention() *backup.RetentionConfig {
	retention := backup.DefaultRetention()

	get := func(key string) string {
		return (&model.SettingsModel{DB: database.GetServerDB()}).GetString(key, "")
	}

	if v := get("backup.retention.max_backups"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retention.MaxBackups = n
		}
	}
	if v := get("backup.retention.keep_weekly"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retention.KeepWeekly = n
		}
	}
	if v := get("backup.retention.keep_monthly"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retention.KeepMonthly = n
		}
	}
	if v := get("backup.retention.keep_yearly"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retention.KeepYearly = n
		}
	}
	if v := get("backup.retention.max_total_size"); v != "" {
		retention.MaxTotalSize = v
	}
	return &retention
}

// CleanupExpiredTokens removes expired API tokens
// AI.md PART 19: token cleanup every 15 minutes
// tokens (database.UsersSchema) is this project's only API-token table and
// it lives in the users database, so the cleanup runs against
// database.GetUsersDB(). The one-time admin setup token is a file at
// {config_dir}/setup_token.txt (src/util/firstrun.go), not a database row, so
// there is nothing for a scheduled task to prune for it.
// Expiry is evaluated in Go against a UTC cutoff (see CleanupOldSessions) so a
// token whose expires_at was stored in a different layout or timezone than
// SQLite's datetime('now') is never deleted early. Rows with a NULL expires_at
// never expire and are always left in place.
func CleanupExpiredTokens() error {
	now := time.Now().UTC()

	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetUsersDB(), "tokens", "id", "expires_at", now)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired API tokens: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d expired API tokens", rowsAffected)
	}

	return nil
}

// SelfHealthCheck performs internal health verification
// AI.md PART 19: healthcheck_self every 5 minutes
func SelfHealthCheck() error {
	// Check database connectivity
	err := database.PingWithTimeout(database.GetServerDB())
	if err != nil {
		log.Printf("WARNING: Self health check: database ping failed: %v", err)
		return fmt.Errorf("database health check failed: %w", err)
	}

	// Check disk space - AI.md: alert when disk usage > 85%
	diskPercent, err := getDiskUsagePercent("/")
	if err != nil {
		log.Printf("WARNING: Self health check: disk space check failed: %v", err)
		// Don't fail health check if disk check fails, just log it
	} else {
		if diskPercent > 95 {
			log.Printf("CRITICAL: Self health check: disk usage at %d%%", diskPercent)
			return fmt.Errorf("critical disk usage: %d%% (threshold: 95%%)", diskPercent)
		} else if diskPercent > 85 {
			log.Printf("WARNING: Self health check: disk usage at %d%%", diskPercent)
			// Log warning but don't fail (AI.md: alert at 85%, critical at 95%)
		}
	}

	// Check users database connectivity too
	usersDB := database.GetUsersDB()
	if usersDB != nil {
		if err := database.PingWithTimeout(usersDB); err != nil {
			log.Printf("WARNING: Self health check: users database ping failed: %v", err)
			return fmt.Errorf("users database health check failed: %w", err)
		}
	}

	return nil
}

// CleanupRateLimitCounters resets rate limit counters
// The 1-hour cutoff is computed in Go and each window_start is compared as a
// UTC instant (see CleanupOldSessions), so counters inside the current window
// are never dropped because of a layout or timezone mismatch.
func CleanupRateLimitCounters() error {
	// Reset hourly counters that are older than 1 hour
	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetServerDB(), "rate_limits", "id", "window_start", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup rate limits: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d old rate limit counters", rowsAffected)
	}

	return nil
}

// UpdateBlocklist updates the IP blocklist database
// AI.md PART 19: blocklist_update daily at 04:00
func UpdateBlocklist() error {
	log.Println("INFO: Updating IP blocklist database...")

	// Check if blocklist is enabled
	if !serverSettingEnabled("security.blocklist.enabled") {
		// Blocklist not enabled, skip
		return nil
	}

	// Blocklist sources (Spamhaus DROP is free and reliable)
	sources := []struct {
		name string
		url  string
	}{
		{"spamhaus_drop", "https://www.spamhaus.org/drop/drop.txt"},
		{"spamhaus_edrop", "https://www.spamhaus.org/drop/edrop.txt"},
	}

	db := database.GetServerDB()

	// Create table if not exists
	if _, err := database.ExecContext(context.Background(), db, database.TimeoutMigration, `
		CREATE TABLE IF NOT EXISTS server_ip_blocklist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			ip_range TEXT NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(source, ip_range)
		)
	`); err != nil {
		return fmt.Errorf("failed to create blocklist table: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	totalAdded := 0

	for _, source := range sources {
		resp, err := client.Get(source.url)
		if err != nil {
			log.Printf("WARNING: Failed to fetch %s: %v", source.name, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			log.Printf("WARNING: Failed to fetch %s: HTTP %d", source.name, resp.StatusCode)
			continue
		}

		// Parse the blocklist (format: CIDR ; description)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ";") {
				continue
			}

			parts := strings.SplitN(line, ";", 2)
			ipRange := strings.TrimSpace(parts[0])
			description := ""
			if len(parts) > 1 {
				description = strings.TrimSpace(parts[1])
			}

			// Validate CIDR format
			_, _, err := net.ParseCIDR(ipRange)
			if err != nil {
				continue
			}

			// Insert or update. The timestamp is computed in Go and bound
			// twice rather than left to CURRENT_TIMESTAMP: on SQLite the
			// literal yields canonical UTC text, but PostgreSQL and MySQL
			// render it in the server's own zone and type, which would mix
			// layouts inside one column and break the cutoff comparison below.
			blocklistNow := dbtime.FormatSQLTimestamp(time.Now())
			_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
				INSERT INTO server_ip_blocklist (source, ip_range, description, updated_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(source, ip_range) DO UPDATE SET
					description = excluded.description,
					updated_at = ?
			`, source.name, ipRange, description, blocklistNow, blocklistNow)
			if err == nil {
				totalAdded++
			}
		}
		_ = resp.Body.Close()
	}

	// Clean up old entries (older than 7 days and not in latest update)
	// updated_at is only ever written as canonical UTC text above, so a bound
	// canonical UTC cutoff compares exactly and works on every driver.
	blocklistCutoff := time.Now().UTC().AddDate(0, 0, -7).Format(sqlTimestampLayout)
	if _, err := database.ExecContext(context.Background(), db, database.TimeoutBulk, `
		DELETE FROM server_ip_blocklist
		WHERE updated_at < ?
	`, blocklistCutoff); err != nil {
		log.Printf("WARNING: Failed to prune old blocklist entries: %v", err)
	}

	log.Printf("INFO: Blocklist update complete: %d entries processed", totalAdded)
	return nil
}

// nvdPublishedLayouts lists the shapes the NVD API 2.0 has been observed to
// render its "published" field in. The documented form carries milliseconds and
// no zone marker and is UTC by definition; the other two are accepted because a
// feed that starts emitting them should not silently produce NULL publication
// dates. Zone-less layouts are parsed with time.Parse, which yields UTC.
var nvdPublishedLayouts = []string{
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04:05",
	time.RFC3339,
}

// parseNVDPublished converts an NVD publication string to an instant. It never
// falls back to "now" or to the zero time on failure - the caller stores NULL
// instead, so a value this function cannot read stays visibly absent rather than
// becoming a plausible-looking wrong date.
func parseNVDPublished(published string) (time.Time, error) {
	trimmed := strings.TrimSpace(published)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty published timestamp")
	}

	for _, layout := range nvdPublishedLayouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized published timestamp %q", trimmed)
}

// UpdateCVEDatabase updates the CVE vulnerability database
// AI.md PART 19: cve_update daily at 05:00
func UpdateCVEDatabase() error {
	log.Println("INFO: Updating CVE database...")

	// Check if CVE monitoring is enabled
	if !serverSettingEnabled("security.cve.enabled") {
		// CVE monitoring not enabled, skip
		return nil
	}

	db := database.GetServerDB()

	// server_cve_alerts is declared in database.ServerSchema, which
	// database.InitDualDB executes on startup. The CREATE TABLE that used to run
	// here at task time named a column "references" - a reserved word no
	// supported driver accepts as a bare identifier - so this task aborted on its
	// very first statement every time CVE monitoring was enabled, and no CVE was
	// ever stored. The schema owns the definition now; this check only turns a
	// missing table into a named error.
	var cveTablePresent int
	if err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM server_cve_alerts WHERE 1 = 0",
	).Scan(&cveTablePresent); err != nil {
		return fmt.Errorf("server_cve_alerts is missing from the server database schema: %w", err)
	}

	// NVD API v2 - fetch recent CVEs (last 7 days)
	// Using the public API (no API key required, but rate limited)
	pubStartDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02T15:04:05.000")
	pubEndDate := time.Now().Format("2006-01-02T15:04:05.000")

	apiURL := fmt.Sprintf(
		"https://services.nvd.nist.gov/rest/json/cves/2.0?pubStartDate=%s&pubEndDate=%s&resultsPerPage=100",
		pubStartDate, pubEndDate,
	)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch CVE data: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NVD API returned status %d", resp.StatusCode)
	}
	if resp.ContentLength > 16<<20 {
		return fmt.Errorf("NVD API response exceeds 16 MiB")
	}

	var nvdResponse struct {
		Vulnerabilities []struct {
			CVE struct {
				ID string `json:"id"`
				// NVD API 2.0 returns cve.descriptions as an array of
				// {lang, value} objects directly. It was previously decoded as an
				// object wrapping a "description_data" array - the shape of the
				// retired 1.0 feed - so the array never bound and every stored CVE
				// had an empty description.
				Descriptions []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
				} `json:"descriptions"`
				Metrics struct {
					CvssMetricV31 []struct {
						CvssData struct {
							BaseScore    float64 `json:"baseScore"`
							BaseSeverity string  `json:"baseSeverity"`
						} `json:"cvssData"`
					} `json:"cvssMetricV31"`
				} `json:"metrics"`
				Published string `json:"published"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return fmt.Errorf("failed to read CVE data: %w", err)
	}
	if len(body) > 16<<20 {
		return fmt.Errorf("NVD API response exceeds 16 MiB")
	}
	if err := json.Unmarshal(body, &nvdResponse); err != nil {
		return fmt.Errorf("failed to parse CVE data: %w", err)
	}

	added := 0
	for _, vuln := range nvdResponse.Vulnerabilities {
		cve := vuln.CVE

		// Get English description
		description := ""
		for _, desc := range cve.Descriptions {
			if desc.Lang == "en" {
				description = desc.Value
				break
			}
		}

		// Get CVSS score and severity
		var cvssScore float64
		severity := "UNKNOWN"
		if len(cve.Metrics.CvssMetricV31) > 0 {
			cvssScore = cve.Metrics.CvssMetricV31[0].CvssData.BaseScore
			severity = cve.Metrics.CvssMetricV31[0].CvssData.BaseSeverity
		}

		// published_at was previously bound straight from the NVD API's own
		// string, putting a third layout into a column every reader parses as
		// canonical UTC text. It is converted here instead. A value the API
		// renders in some other shape is stored as NULL rather than guessed at:
		// the column is nullable, and a wrong instant is worse than a missing one.
		var publishedAt interface{}
		if parsed, perr := parseNVDPublished(cve.Published); perr == nil {
			publishedAt = dbtime.FormatSQLTimestamp(parsed)
		} else {
			log.Printf("WARN: CVE %s has an unrecognized published timestamp %q; storing NULL", cve.ID, cve.Published)
		}

		// updated_at is bound as canonical UTC text for the same cross-driver
		// reason as the blocklist upsert above.
		cveNow := dbtime.FormatSQLTimestamp(time.Now())
		_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
			INSERT INTO server_cve_alerts (cve_id, description, severity, cvss_score, published_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(cve_id) DO UPDATE SET
				description = excluded.description,
				severity = excluded.severity,
				cvss_score = excluded.cvss_score,
				published_at = excluded.published_at,
				updated_at = ?
		`, cve.ID, description, severity, cvssScore, publishedAt, cveNow, cveNow)
		if err == nil {
			added++
		}
	}

	log.Printf("INFO: CVE database update complete: %d CVEs processed", added)
	return nil
}

// ClusterHeartbeat sends a heartbeat to indicate this node is alive
// AI.md PART 19 (Built-in Tasks): cluster_heartbeat every 30 seconds (cluster mode only)
func ClusterHeartbeat(nodeID string) error {
	// Check if cluster mode is enabled
	if !serverSettingEnabled("server.cluster.enabled") {
		// Not in cluster mode, skip silently
		return nil
	}

	// Update node heartbeat in cluster nodes table
	// Per AI.md lines 22616-22620
	// The heartbeat instant is computed in Go, converted to UTC and bound as
	// canonical text rather than emitted by SQLite's datetime('now'): readers
	// compare last_heartbeat against a Go-side UTC cutoff, and datetime() is not
	// available on PostgreSQL/MySQL, both of which this project supports.
	heartbeat := time.Now().UTC().Format(sqlTimestampLayout)

	// server_nodes.hostname is NOT NULL, and this upsert is the only writer that
	// ever creates the row, so it has to supply the name. util.GetFQDN is the
	// project's single host-resolution helper (proxy/DOMAIN aware, falling back
	// through os.Hostname to a routable address), so a node identifies itself the
	// same way here as everywhere else the app names itself.
	if _, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_nodes (node_id, hostname, last_heartbeat, status)
		VALUES (?, ?, ?, 'online')
		ON CONFLICT(node_id) DO UPDATE SET
			hostname = excluded.hostname,
			last_heartbeat = excluded.last_heartbeat,
			status = 'online'
	`, nodeID, util.GetFQDN(), heartbeat); err != nil {
		return fmt.Errorf("failed to send cluster heartbeat: %w", err)
	}

	return nil
}

// EnableTask enables a task by name
func (s *Scheduler) EnableTask(taskName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	task.mu.Lock()
	task.enabled = true
	task.mu.Unlock()
	s.persistEnabled(taskName, true)
	log.Printf("OK: Task '%s' enabled", taskName)
	return nil
}

// persistEnabled stores a task's enabled flag in scheduler_tasks so the admin
// panel and a restarted process see it (AI.md PART 19 "Scheduler State").
func (s *Scheduler) persistEnabled(taskName string, enabled bool) {
	if s.db == nil {
		return
	}
	if _, err := s.db.Exec("UPDATE scheduler_tasks SET enabled = ? WHERE task_id = ?", enabled, taskName); err != nil {
		log.Printf("WARN: Failed to persist enabled state for task '%s': %v", taskName, err)
	}
}

// ValidSchedule reports whether spec is a cron expression, a descriptor such
// as @daily, or an @every interval the scheduler can run.
func ValidSchedule(spec string) bool {
	_, err := parseSchedule(spec)
	return err == nil
}

// DisableTask disables a task by name
func (s *Scheduler) DisableTask(taskName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	task.mu.Lock()
	task.enabled = false
	task.mu.Unlock()
	s.persistEnabled(taskName, false)
	log.Printf("INFO: Task '%s' disabled", taskName)
	return nil
}

// TriggerTask manually triggers a task to run immediately
func (s *Scheduler) TriggerTask(taskName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	log.Printf("INFO: Manually triggering task '%s'", taskName)
	go s.executeTask(task)
	return nil
}

// GetTask returns a task by name
func (s *Scheduler) GetTask(taskName string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.tasks[taskName]
}
