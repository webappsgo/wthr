package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

const hotCounterFlushInterval = 30 * time.Second

type hotCounterKey struct {
	db  *sql.DB
	key string
}

type hotCounterUpdate struct {
	query  string
	args   []interface{}
	queued time.Time
}

var hotCounterQueue = struct {
	sync.Mutex
	updates map[hotCounterKey]hotCounterUpdate
}{updates: make(map[hotCounterKey]hotCounterUpdate)}

var hotCounterFlusher = struct {
	sync.Mutex
	started  bool
	stopped  bool
	stop     chan context.Context
	done     chan error
	stopOnce sync.Once
	stopErr  error
}{}

// QueueHotCounterUpdate coalesces a bookkeeping update in memory until the
// periodic writer flushes it. The key must identify one row within db and must
// not contain a secret or raw bearer token.
func QueueHotCounterUpdate(db *sql.DB, key, query string, args ...interface{}) error {
	if db == nil {
		return errors.New("hot-counter database is required")
	}
	if key == "" || query == "" {
		return errors.New("hot-counter key and query are required")
	}
	update := hotCounterUpdate{query: query, args: append([]interface{}(nil), args...), queued: time.Now()}
	queueKey := hotCounterKey{db: db, key: key}
	hotCounterQueue.Lock()
	hotCounterQueue.updates[queueKey] = update
	hotCounterQueue.Unlock()
	ensureHotCounterFlusher()
	return nil
}

// FlushHotCounterUpdates writes queued bookkeeping updates in one transaction
// per database. Failed batches are restored unless a newer update is already
// queued for the same row.
func FlushHotCounterUpdates(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	hotCounterQueue.Lock()
	updates := hotCounterQueue.updates
	hotCounterQueue.updates = make(map[hotCounterKey]hotCounterUpdate)
	hotCounterQueue.Unlock()
	if len(updates) == 0 {
		return nil
	}

	byDB := make(map[*sql.DB][]struct {
		key    hotCounterKey
		update hotCounterUpdate
	})
	for key, update := range updates {
		byDB[key.db] = append(byDB[key.db], struct {
			key    hotCounterKey
			update hotCounterUpdate
		}{key: key, update: update})
	}

	var flushErrors []error
	for db, batch := range byDB {
		sort.Slice(batch, func(i, j int) bool { return batch[i].key.key < batch[j].key.key })
		err := WithTransaction(ctx, db, func(tx *sql.Tx) error {
			for _, item := range batch {
				if _, err := tx.ExecContext(ctx, item.update.query, item.update.args...); err != nil {
					return fmt.Errorf("flush hot-counter update: %w", err)
				}
			}
			return nil
		})
		if err == nil {
			continue
		}
		flushErrors = append(flushErrors, err)
		restoreHotCounterUpdates(batch)
	}
	return errors.Join(flushErrors...)
}

func restoreHotCounterUpdates(batch []struct {
	key    hotCounterKey
	update hotCounterUpdate
}) {
	hotCounterQueue.Lock()
	defer hotCounterQueue.Unlock()
	for _, item := range batch {
		current, exists := hotCounterQueue.updates[item.key]
		if !exists || current.queued.Before(item.update.queued) {
			hotCounterQueue.updates[item.key] = item.update
		}
	}
}

func ensureHotCounterFlusher() {
	hotCounterFlusher.Lock()
	defer hotCounterFlusher.Unlock()
	if hotCounterFlusher.started || hotCounterFlusher.stopped {
		return
	}
	hotCounterFlusher.started = true
	hotCounterFlusher.stop = make(chan context.Context)
	hotCounterFlusher.done = make(chan error, 1)
	go func(stop <-chan context.Context, done chan<- error) {
		ticker := time.NewTicker(hotCounterFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := FlushHotCounterUpdates(context.Background()); err != nil {
					log.Printf("WARNING: failed to flush hot-counter updates: %v", err)
				}
			case ctx := <-stop:
				done <- FlushHotCounterUpdates(ctx)
				return
			}
		}
	}(hotCounterFlusher.stop, hotCounterFlusher.done)
}

// StopHotCounterFlusher stops the periodic writer and flushes pending updates
// with the supplied shutdown context. Calling it before the first queued update
// still performs a final flush.
func StopHotCounterFlusher(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	hotCounterFlusher.Lock()
	if !hotCounterFlusher.started {
		hotCounterFlusher.Unlock()
		return FlushHotCounterUpdates(ctx)
	}
	stop, done := hotCounterFlusher.stop, hotCounterFlusher.done
	hotCounterFlusher.Unlock()

	hotCounterFlusher.stopOnce.Do(func() {
		stop <- ctx
		hotCounterFlusher.stopErr = <-done
		hotCounterFlusher.Lock()
		hotCounterFlusher.stopped = true
		hotCounterFlusher.Unlock()
	})
	return hotCounterFlusher.stopErr
}
