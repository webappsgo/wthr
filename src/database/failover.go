package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// FailoverManager handles database failover and read-only mode
type FailoverManager struct {
	primaryDB   *sql.DB
	// SQLite fallback
	cacheDB     *sql.DB
	readOnly    bool
	writeQueue  []QueuedWrite
	mu          sync.RWMutex
	lastError   error
	lastErrorAt time.Time
	retryTicker *time.Ticker
	stopChan    chan struct{}
}

// QueuedWrite represents a write operation that needs to be executed when database recovers
type QueuedWrite struct {
	Query     string
	Args      []interface{}
	Timestamp time.Time
}

// NewFailoverManager creates a new failover manager
func NewFailoverManager(primaryDB *sql.DB, cacheDB *sql.DB) *FailoverManager {
	fm := &FailoverManager{
		primaryDB:  primaryDB,
		cacheDB:    cacheDB,
		readOnly:   false,
		writeQueue: make([]QueuedWrite, 0),
		stopChan:   make(chan struct{}),
	}

	// Start monitoring goroutine
	go fm.monitorPrimaryDB()

	return fm
}

// IsReadOnly returns whether the system is in read-only mode
func (fm *FailoverManager) IsReadOnly() bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.readOnly
}

// GetLastError returns the last database error and when it occurred
func (fm *FailoverManager) GetLastError() (time.Time, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.lastErrorAt, fm.lastError
}

// Query executes a SELECT query (reads from cache if in read-only mode).
// AI.md PART 10: callers pass arbitrary SELECTs (including JOINs), so the
// Complex Select timeout tier (15s) is used here.
func (fm *FailoverManager) Query(query string, args ...interface{}) (*sql.Rows, error) {
	fm.mu.RLock()
	readOnly := fm.readOnly
	fm.mu.RUnlock()

	ctx := context.Background()
	if readOnly {
		// Use cache DB for reads
		return QueryContext(ctx, fm.cacheDB, TimeoutComplexSelect, query, args...)
	}

	// Try primary DB
	rows, err := QueryContext(ctx, fm.primaryDB, TimeoutComplexSelect, query, args...)
	if err != nil {
		// Primary DB failed, switch to read-only mode
		fm.handlePrimaryFailure(err)
		// Fallback to cache
		return QueryContext(ctx, fm.cacheDB, TimeoutComplexSelect, query, args...)
	}

	return rows, nil
}

// QueryRow executes a single-row query (reads from cache if in read-only mode).
// AI.md PART 10: Simple Select timeout tier (5s).
func (fm *FailoverManager) QueryRow(query string, args ...interface{}) *sql.Row {
	fm.mu.RLock()
	readOnly := fm.readOnly
	fm.mu.RUnlock()

	ctx := context.Background()
	if readOnly {
		// Use cache DB for reads
		return QueryRowContext(ctx, fm.cacheDB, TimeoutSimpleSelect, query, args...)
	}

	// Use primary DB
	return QueryRowContext(ctx, fm.primaryDB, TimeoutSimpleSelect, query, args...)
}

// Exec executes a write query (queues if in read-only mode).
// AI.md PART 10: Write timeout tier (10s).
func (fm *FailoverManager) Exec(query string, args ...interface{}) (sql.Result, error) {
	fm.mu.RLock()
	readOnly := fm.readOnly
	fm.mu.RUnlock()

	if readOnly {
		// Queue write for later
		fm.queueWrite(query, args...)
		return nil, fmt.Errorf("database unavailable - write queued for retry")
	}

	// Try primary DB
	result, err := ExecContext(context.Background(), fm.primaryDB, TimeoutWrite, query, args...)
	if err != nil {
		// Primary DB failed, queue write and switch to read-only
		fm.handlePrimaryFailure(err)
		fm.queueWrite(query, args...)
		return nil, fmt.Errorf("database unavailable - write queued for retry: %w", err)
	}

	return result, nil
}

// queueWrite adds a write operation to the queue
func (fm *FailoverManager) queueWrite(query string, args ...interface{}) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.writeQueue = append(fm.writeQueue, QueuedWrite{
		Query:     query,
		Args:      args,
		Timestamp: time.Now(),
	})
}

// handlePrimaryFailure handles a primary database failure
func (fm *FailoverManager) handlePrimaryFailure(err error) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if !fm.readOnly {
		// First failure - switch to read-only mode
		fm.readOnly = true
		fm.lastError = err
		fm.lastErrorAt = time.Now()

		log.Printf("⚠️  DATABASE FAILURE: Switching to read-only mode: %v", err)
		log.Printf("📝 All writes will be queued and retried when database recovers")

		// Start retry ticker if not already running
		if fm.retryTicker == nil {
			fm.retryTicker = time.NewTicker(30 * time.Second)
		}
	}
}

// monitorPrimaryDB monitors the primary database and attempts recovery
func (fm *FailoverManager) monitorPrimaryDB() {
	for {
		select {
		case <-fm.stopChan:
			if fm.retryTicker != nil {
				fm.retryTicker.Stop()
			}
			return

		case <-func() <-chan time.Time {
			fm.mu.RLock()
			defer fm.mu.RUnlock()
			if fm.retryTicker != nil {
				return fm.retryTicker.C
			}
			// Return a channel that never receives if ticker is nil
			return make(<-chan time.Time)
		}():
			fm.attemptRecovery()
		}
	}
}

// attemptRecovery attempts to recover the primary database connection
func (fm *FailoverManager) attemptRecovery() {
	fm.mu.RLock()
	if !fm.readOnly {
		fm.mu.RUnlock()
		return
	}
	fm.mu.RUnlock()

	// Test connection with a simple query
	// AI.md PART 10: Simple Select timeout tier (5s)
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutSimpleSelect)
	defer cancel()

	err := fm.primaryDB.PingContext(ctx)
	if err != nil {
		log.Printf("⚠️  Database recovery failed: %v (will retry in 30 seconds)", err)
		return
	}

	// Verify with a test query
	var testResult int
	err = fm.primaryDB.QueryRowContext(ctx, "SELECT 1").Scan(&testResult)
	if err != nil {
		log.Printf("⚠️  Database recovery failed: %v (will retry in 30 seconds)", err)
		return
	}

	// Database is back online!
	log.Printf("✅ Database connection recovered!")

	// Replay queued writes
	fm.replayWrites()

	// Exit read-only mode
	fm.mu.Lock()
	fm.readOnly = false
	fm.lastError = nil
	if fm.retryTicker != nil {
		fm.retryTicker.Stop()
		fm.retryTicker = nil
	}
	fm.mu.Unlock()

	log.Printf("✅ System returned to normal operation")
}

// replayWrites replays all queued writes
func (fm *FailoverManager) replayWrites() {
	fm.mu.Lock()
	queue := make([]QueuedWrite, len(fm.writeQueue))
	copy(queue, fm.writeQueue)
	// Clear queue
	fm.writeQueue = fm.writeQueue[:0]
	fm.mu.Unlock()

	if len(queue) == 0 {
		return
	}

	log.Printf("📝 Replaying %d queued writes...", len(queue))

	successCount := 0
	failCount := 0

	for _, write := range queue {
		_, err := ExecContext(context.Background(), fm.primaryDB, TimeoutWrite, write.Query, write.Args...)
		if err != nil {
			log.Printf("⚠️  Failed to replay write from %s: %v", write.Timestamp.Format(time.RFC3339), err)
			failCount++
			// Re-queue failed writes
			fm.queueWrite(write.Query, write.Args...)
		} else {
			successCount++
		}
	}

	if failCount > 0 {
		log.Printf("⚠️  Replayed %d/%d writes (%d failed, re-queued)", successCount, len(queue), failCount)
	} else {
		log.Printf("✅ Successfully replayed all %d queued writes", successCount)
	}
}

// GetQueuedWriteCount returns the number of queued writes
func (fm *FailoverManager) GetQueuedWriteCount() int {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return len(fm.writeQueue)
}

// Close stops the failover manager
func (fm *FailoverManager) Close() {
	close(fm.stopChan)
}
