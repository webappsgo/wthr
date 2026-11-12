// Package lifecycle carries the process shutdown state that readiness checks
// and long-lived connections observe (AI.md PART 8 shutdown flow).
package lifecycle

import "sync"

var (
	mu       sync.Mutex
	done     = make(chan struct{})
	shutting bool
)

// BeginShutdown marks the process as shutting down. Readiness then reports
// not-ready and every long-lived connection watching Done is told to close.
// Calling it more than once is safe.
func BeginShutdown() {
	mu.Lock()
	defer mu.Unlock()
	if !shutting {
		shutting = true
		close(done)
	}
}

// IsShuttingDown reports whether BeginShutdown has been called.
func IsShuttingDown() bool {
	mu.Lock()
	defer mu.Unlock()

	return shutting
}

// Done is closed when shutdown begins.
func Done() <-chan struct{} {
	return done
}

// Reset clears the shutdown state; tests only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	shutting = false
	done = make(chan struct{})
}
