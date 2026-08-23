//go:build !windows
// +build !windows

package main

import (
	"syscall"
	"testing"

	"github.com/webappsgo/wthr/src/mode"
)

// TestHandlePlatformSignal_SIGUSR2 exercises the debug-mode toggle branch.
// It does not touch db/appLogger/dirPaths, so nils are safe fixtures here.
func TestHandlePlatformSignal_SIGUSR2(t *testing.T) {
	origDebug := mode.IsDebugEnabled()
	defer mode.SetDebugEnabled(origDebug)

	tests := []struct {
		name       string
		startDebug bool
		wantDebug  bool
	}{
		{"debug toggles to release", true, false},
		{"release toggles to debug", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode.SetDebugEnabled(tt.startDebug)

			shouldShutdown := handlePlatformSignal(syscall.SIGUSR2, nil, nil, nil)

			if shouldShutdown {
				t.Errorf("handlePlatformSignal(SIGUSR2) = shutdown true, want false")
			}
			if got := mode.IsDebugEnabled(); got != tt.wantDebug {
				t.Errorf("mode.IsDebugEnabled() after SIGUSR2 = %v, want %v", got, tt.wantDebug)
			}
		})
	}
}

// TestHandlePlatformSignal_SIGRTMIN3 verifies the Docker STOPSIGNAL is
// treated as a graceful-shutdown request (return value true).
func TestHandlePlatformSignal_SIGRTMIN3(t *testing.T) {
	if !handlePlatformSignal(sigRTMIN3, nil, nil, nil) {
		t.Error("handlePlatformSignal(sigRTMIN3) = false, want true (graceful shutdown)")
	}
}

// TestHandlePlatformSignal_UnknownSignal is the default/boundary case: a
// signal not explicitly handled must return false and must not panic even
// with nil db/appLogger/dirPaths.
func TestHandlePlatformSignal_UnknownSignal(t *testing.T) {
	if handlePlatformSignal(syscall.SIGHUP, nil, nil, nil) {
		t.Error("handlePlatformSignal(SIGHUP) = true, want false (unhandled signal)")
	}
}

// TestPlatformSignals confirms the registered signal set matches what
// handlePlatformSignal actually understands (SIGUSR1, SIGUSR2, SIGRTMIN+3),
// catching drift if one list is updated without the other.
func TestPlatformSignals(t *testing.T) {
	want := map[syscall.Signal]bool{
		syscall.SIGUSR1: true,
		syscall.SIGUSR2: true,
		sigRTMIN3:       true,
	}
	if len(platformSignals) != len(want) {
		t.Fatalf("len(platformSignals) = %d, want %d", len(platformSignals), len(want))
	}
	for _, sig := range platformSignals {
		if !want[sig] {
			t.Errorf("platformSignals contains unexpected signal %v", sig)
		}
	}
}

// TestSigRTMIN3Value locks down the exact numeric value (37) that Docker's
// STOPSIGNAL relies on, per AI.md PART 27.
func TestSigRTMIN3Value(t *testing.T) {
	if sigRTMIN3 != syscall.Signal(37) {
		t.Errorf("sigRTMIN3 = %d, want 37", sigRTMIN3)
	}
}
