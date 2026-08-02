//go:build !windows
// +build !windows

package main

import (
	"log"
	"os"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

func handlePlatformSignal(sig os.Signal, db *database.DB, appLogger *utils.Logger, dirPaths *utils.DirectoryPaths) bool {
	switch sig {
	case syscall.SIGUSR1:
		log.Println("INFO: Received SIGUSR1, reopening log files...")
		if err := appLogger.RotateLogs(); err != nil {
			log.Printf("WARNING: Failed to rotate logs: %v", err)
		} else {
			log.Println("OK: Log files reopened")
		}
		return false

	case syscall.SIGUSR2:
		log.Println("INFO: Received SIGUSR2, toggling debug mode...")
		if gin.Mode() == gin.DebugMode {
			gin.SetMode(gin.ReleaseMode)
			log.Println("OK: Debug mode: OFF (release mode)")
		} else {
			gin.SetMode(gin.DebugMode)
			log.Println("OK: Debug mode: ON (debug mode)")
		}
		return false

	case sigRTMIN3:
		// Docker STOPSIGNAL (SIGRTMIN+3 = 37) per AI.md PART 27 line 6462
		// Treat as graceful shutdown signal (same as SIGTERM)
		log.Println("INFO: Received SIGRTMIN+3 (Docker STOPSIGNAL), initiating graceful shutdown...")
		return true
	}
	return false
}
