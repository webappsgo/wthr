// Package health holds the dataset and dependency check registry required by
// AI.md PART 13: each data set registers itself once and the health response
// iterates the registry, so adding a data set never edits the health handler.
package health

import (
	"context"
	"os"
	"sync"
	"time"
)

// Status is the public-safe value of one check.
type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusError    Status = "error"
	// StatusOmit hides a check, used when the feature behind a data set is disabled.
	StatusOmit Status = ""
)

// RefreshInterval is how often the scheduler loop re-runs the checks.
const RefreshInterval = 60 * time.Second

// Result is the cached outcome of one registered check.
type Result struct {
	Key      string
	Status   Status
	Critical bool
}

type entry struct {
	key      string
	critical bool
	fn       func() Status
	status   Status
}

var (
	mu      sync.RWMutex
	entries []*entry
)

// Register adds a check. The key must be a stable snake_case identifier such
// as geoip_country, blocklist_ads or data_countries, never a path or URL. The
// check runs once immediately so a request never sees an unevaluated entry.
func Register(key string, critical bool, fn func() Status) {
	e := &entry{key: key, critical: critical, fn: fn, status: fn()}
	mu.Lock()
	defer mu.Unlock()
	for i, existing := range entries {
		if existing.key == key {
			entries[i] = e
			return
		}
	}
	entries = append(entries, e)
}

// Refresh re-runs every registered check and caches the result.
func Refresh() {
	mu.RLock()
	snapshot := append([]*entry(nil), entries...)
	mu.RUnlock()

	for _, e := range snapshot {
		status := e.fn()
		mu.Lock()
		e.status = status
		mu.Unlock()
	}
}

// Results returns the cached results in registration order, without running
// anything, with omitted checks left out.
func Results() []Result {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Result, 0, len(entries))
	for _, e := range entries {
		if e.status == StatusOmit {
			continue
		}
		out = append(out, Result{Key: e.key, Status: e.status, Critical: e.critical})
	}

	return out
}

// Worst reports the status contribution of the dataset checks: unhealthy when
// a critical check is degraded or in error, degraded when any other check is.
func Worst() (unhealthy, degraded bool) {
	for _, r := range Results() {
		if r.Status == StatusOK {
			continue
		}
		if r.Critical {
			unhealthy = true
		} else {
			degraded = true
		}
	}

	return unhealthy, degraded
}

// Run re-runs the checks every RefreshInterval until ctx is cancelled.
func Run(ctx context.Context) {
	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			Refresh()
		}
	}
}

// FileCheck builds a check for one data file: error when it is missing or
// empty, degraded when it is older than maxAge, ok otherwise. A nil enabled
// function means always enabled.
func FileCheck(path string, maxAge time.Duration, enabled func() bool) func() Status {
	return func() Status {
		if enabled != nil && !enabled() {
			return StatusOmit
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return StatusError
		}
		if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
			return StatusDegraded
		}

		return StatusOK
	}
}
