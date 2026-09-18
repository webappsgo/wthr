package service

import (
	"fmt"
	"sort"
	"sync"

	"github.com/webappsgo/wthr/src/config"
)

// runtimeSettings holds the values the running process bound at startup. Per
// AI.md PART 0 these are the only settings a live config reload cannot apply,
// so a change to any of them marks the process as needing a restart.
type runtimeSettings struct {
	address        string
	port           string
	databaseDriver string
}

var (
	restartStateMu sync.RWMutex
	bootSettings   *runtimeSettings
	restartReasons []string
)

func settingsFromConfig(cfg *config.AppConfig) runtimeSettings {
	return runtimeSettings{
		address:        cfg.Server.Address,
		port:           fmt.Sprintf("%v", cfg.Server.Port),
		databaseDriver: cfg.Server.Database.Driver,
	}
}

// RecordBootSettings captures the restart-required settings the process
// actually started with. Called once before the config watcher begins.
func RecordBootSettings(cfg *config.AppConfig) {
	if cfg == nil {
		return
	}
	settings := settingsFromConfig(cfg)

	restartStateMu.Lock()
	defer restartStateMu.Unlock()
	bootSettings = &settings
	restartReasons = nil
}

// EvaluateRestartRequired compares a newly loaded config against the settings
// the process booted with and records which restart-required settings drifted.
func EvaluateRestartRequired(cfg *config.AppConfig) {
	if cfg == nil {
		return
	}
	current := settingsFromConfig(cfg)

	restartStateMu.Lock()
	defer restartStateMu.Unlock()

	if bootSettings == nil {
		bootSettings = &current
		restartReasons = nil
		return
	}

	reasons := make(map[string]struct{})
	for _, reason := range restartReasons {
		reasons[reason] = struct{}{}
	}
	if current.address != bootSettings.address {
		reasons["server.address"] = struct{}{}
	}
	if current.port != bootSettings.port {
		reasons["server.port"] = struct{}{}
	}
	if current.databaseDriver != bootSettings.databaseDriver {
		reasons["database.driver"] = struct{}{}
	}

	restartReasons = restartReasons[:0]
	for reason := range reasons {
		restartReasons = append(restartReasons, reason)
	}
	sort.Strings(restartReasons)
}

// GetRestartRequired reports whether a restart is pending and which settings
// caused it, for AI.md PART 13 pending_restart/restart_reason.
func GetRestartRequired() (bool, []string) {
	restartStateMu.RLock()
	defer restartStateMu.RUnlock()

	if len(restartReasons) == 0 {
		return false, nil
	}
	reasons := make([]string, len(restartReasons))
	copy(reasons, restartReasons)
	return true, reasons
}
