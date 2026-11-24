package util

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestProcessLoggerFallbackAndConfiguredLogger(t *testing.T) {
	previousLogger := currentProcessLogger()
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	t.Cleanup(func() {
		SetProcessLogger(previousLogger)
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	var fallback bytes.Buffer
	log.SetOutput(&fallback)
	log.SetFlags(0)
	SetProcessLogger(nil)
	LogInfo("fallback %s", "info")
	LogWarn("fallback %s", "warning")
	LogError("fallback %s", "error")
	for _, message := range []string{"[INFO] fallback info", "[WARN] fallback warning", "[ERROR] fallback error"} {
		if !strings.Contains(fallback.String(), message) {
			t.Errorf("fallback log missing %q: %s", message, fallback.String())
		}
	}

	var access, server, errors bytes.Buffer
	SetProcessLogger(&Logger{
		accessLog: log.New(&access, "", 0),
		serverLog: log.New(&server, "", 0),
		errorLog:  log.New(&errors, "", 0),
	})
	LogInfo("configured %s", "info")
	LogWarn("configured %s", "warning")
	LogError("configured %s", "error")
	if !strings.Contains(access.String(), "[INFO] configured info") {
		t.Errorf("configured info log = %q", access.String())
	}
	if !strings.Contains(server.String(), "[WARN] configured warning") {
		t.Errorf("configured warning log = %q", server.String())
	}
	if !strings.Contains(errors.String(), "[ERROR] configured error") {
		t.Errorf("configured error log = %q", errors.String())
	}
}
