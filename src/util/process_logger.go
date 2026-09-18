package util

import (
	"log"
	"sync"
)

// processLogger holds the single *Logger the running process created at
// startup, so packages that are constructed without a logger reference (the
// cluster manager, background workers) can still write through the project's
// structured logger instead of the bare stdlib one.
var (
	processLoggerMu sync.RWMutex
	processLogger   *Logger
)

// SetProcessLogger registers the process-wide structured logger. It is called
// once, immediately after NewLogger succeeds during startup.
func SetProcessLogger(l *Logger) {
	processLoggerMu.Lock()
	processLogger = l
	processLoggerMu.Unlock()
}

// currentProcessLogger returns the registered logger, or nil when the process
// has not created one yet (unit tests, early startup).
func currentProcessLogger() *Logger {
	processLoggerMu.RLock()
	defer processLoggerMu.RUnlock()

	return processLogger
}

// LogInfo writes an informational line through the process logger, falling
// back to the stdlib logger when none is registered yet so a message is never
// silently dropped.
func LogInfo(format string, v ...interface{}) {
	if l := currentProcessLogger(); l != nil {
		l.Info(format, v...)
		return
	}

	log.Printf("[INFO] "+format, v...)
}

// LogWarn writes a warning line through the process logger, with the same
// stdlib fallback as LogInfo.
func LogWarn(format string, v ...interface{}) {
	if l := currentProcessLogger(); l != nil {
		l.Warn(format, v...)
		return
	}

	log.Printf("[WARN] "+format, v...)
}

// LogError writes an error line through the process logger, with the same
// stdlib fallback as LogInfo.
func LogError(format string, v ...interface{}) {
	if l := currentProcessLogger(); l != nil {
		l.Error(format, v...)
		return
	}

	log.Printf("[ERROR] "+format, v...)
}
