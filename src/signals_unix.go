//go:build !windows
// +build !windows

package main

import (
	"os"
	"syscall"
)

// SIGRTMIN+3 = signal 37 (Docker STOPSIGNAL per AI.md PART 27)
// On Linux: SIGRTMIN = 34, so SIGRTMIN+3 = 37
const sigRTMIN3 = syscall.Signal(37)

// restartProcess replaces the process image with a fresh copy of the same binary,
// arguments and environment. It runs after the graceful shutdown and the deferred
// cleanup (database close, PID file removal), so the new process starts clean.
func restartProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}

var platformSignals = []syscall.Signal{
	// Reopen log files
	syscall.SIGUSR1,
	// Toggle debug mode
	syscall.SIGUSR2,
	// Docker STOPSIGNAL (SIGRTMIN+3 = 37) per AI.md PART 27 line 6462
	sigRTMIN3,
}
