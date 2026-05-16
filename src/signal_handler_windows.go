//go:build windows
// +build windows

package main

import (
	"os"

	"github.com/apimgr/weather/src/database"
	"github.com/apimgr/weather/src/util"
)

// handlePlatformSignal handles platform-specific signals (Windows)
// Per AI.md PART 27 line 6472: Windows does NOT support SIGHUP, SIGUSR1, SIGUSR2, SIGQUIT
// Windows only supports os.Interrupt (Ctrl+C, Ctrl+Break) which is handled in main.go
func handlePlatformSignal(sig os.Signal, db *database.DB, appLogger *utils.Logger, dirPaths *utils.DirectoryPaths) bool {
	// Windows has no platform-specific signals to handle
	// All shutdown is handled via os.Interrupt in main signal loop
	return false
}
