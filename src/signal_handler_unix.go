//go:build !windows
// +build !windows

package main

import (
	"log"
	"os"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/util"
)

func handlePlatformSignal(sig os.Signal, db *database.DB, appLogger *utils.Logger, dirPaths *utils.DirectoryPaths) bool {
	switch sig {
	case syscall.SIGUSR1:
		log.Println("📝 Received SIGUSR1, reopening log files...")
		if err := appLogger.RotateLogs(); err != nil {
			log.Printf("⚠️  Failed to rotate logs: %v", err)
		} else {
			log.Println("✅ Log files reopened")
		}
		return false

	case syscall.SIGUSR2:
		log.Println("🔧 Received SIGUSR2, toggling debug mode...")
		if gin.Mode() == gin.DebugMode {
			gin.SetMode(gin.ReleaseMode)
			log.Println("✅ Debug mode: OFF (release mode)")
		} else {
			gin.SetMode(gin.DebugMode)
			log.Println("✅ Debug mode: ON (debug mode)")
		}
		return false

	case sigRTMIN3:
		// Docker STOPSIGNAL (SIGRTMIN+3 = 37) per AI.md PART 27 line 6462
		// Treat as graceful shutdown signal (same as SIGTERM)
		log.Println("🐳 Received SIGRTMIN+3 (Docker STOPSIGNAL), initiating graceful shutdown...")
		return true
	}
	return false
}
