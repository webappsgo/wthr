package service

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/mode"
)

// ConfigWatcher watches server.yml for changes and triggers reload
type ConfigWatcher struct {
	watcher    *fsnotify.Watcher
	configPath string
	reloadFunc func(*config.AppConfig) error
	stopChan   chan bool
}

// NewConfigWatcher creates a new config file watcher
func NewConfigWatcher(configPath string, reloadFunc func(*config.AppConfig) error) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &ConfigWatcher{
		watcher:    watcher,
		configPath: configPath,
		reloadFunc: reloadFunc,
		stopChan:   make(chan bool),
	}, nil
}

// StartConfigWatcher begins watching the config file for changes
func (cw *ConfigWatcher) StartConfigWatcher() error {
	// Add config file to watcher
	configDir := filepath.Dir(cw.configPath)
	if err := cw.watcher.Add(configDir); err != nil {
		return err
	}

	// AI.md PART 5: the yaml.d drop-in directory is watched as well; when it
	// does not exist yet, the Create event on the config directory adds it
	dropinDir := config.DropinDir(cw.configPath)
	if info, err := os.Stat(dropinDir); err == nil && info.IsDir() {
		if err := cw.watcher.Add(dropinDir); err != nil {
			log.Printf("WARNING: cannot watch %s: %v", dropinDir, err)
		}
	}

	// Capture the settings the process actually bound so a later reload can
	// tell live-applicable changes from restart-required ones
	RecordBootSettings(config.GetGlobalConfig())

	log.Printf("INFO: Watching for config file changes: %s", cw.configPath)

	// Start watching in goroutine
	go func() {
		// Debounce timer to avoid multiple reloads for rapid changes
		var debounceTimer *time.Timer
		debounceDuration := 500 * time.Millisecond

		for {
			select {
			case event, ok := <-cw.watcher.Events:
				if !ok {
					return
				}

				eventPath := filepath.Clean(event.Name)
				isServerYML := eventPath == filepath.Clean(cw.configPath)
				isDropinDir := eventPath == filepath.Clean(dropinDir)
				isDropinEntry := filepath.Dir(eventPath) == filepath.Clean(dropinDir)
				if !isServerYML && !isDropinDir && !isDropinEntry {
					continue
				}

				if isDropinDir && event.Op&fsnotify.Create == fsnotify.Create {
					if err := cw.watcher.Add(dropinDir); err != nil {
						log.Printf("WARNING: cannot watch %s: %v", dropinDir, err)
					}
				}

				// Handle write and create events (editors often write temp files);
				// a drop-in that is added, changed, renamed or removed re-merges everything
				changed := event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create
				if !isServerYML {
					changed = changed || event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename
				}
				if changed {
					// Reset or start debounce timer
					if debounceTimer != nil {
						debounceTimer.Stop()
					}

					debounceTimer = time.AfterFunc(debounceDuration, func() {
						log.Println("INFO: Config file changed, reloading...")

						// Load new config; an invalid server.yml or drop-in keeps the
						// last good configuration active
						newCfg, err := config.LoadConfigStrict()
						if err != nil {
							log.Printf("ERROR: Config reload rejected, keeping the last good configuration: %v", err)
							return
						}
						LogConfigLayers()

						// Trigger reload callback
						if err := cw.reloadFunc(newCfg); err != nil {
							log.Printf("ERROR: Failed to apply new config: %v", err)
							return
						}

						EvaluateRestartRequired(newCfg)
						if pending, reasons := GetRestartRequired(); pending {
							log.Printf("WARNING: Configuration reloaded, but a restart is required to apply: %v", reasons)
							return
						}

						log.Println("OK: Configuration reloaded successfully (live reload - no restart needed)")
					})
				}

			case err, ok := <-cw.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("WARNING: Config watcher error: %v", err)

			case <-cw.stopChan:
				log.Println("INFO: Stopping config file watcher")
				return
			}
		}
	}()

	return nil
}

// StopConfigWatcher stops the config file watcher
func (cw *ConfigWatcher) StopConfigWatcher() error {
	close(cw.stopChan)
	return cw.watcher.Close()
}

// LogConfigLayers logs the loaded yaml.d drop-ins in merge order at info level
// and the file that won for every overridden key at debug level. Secret values
// are never logged: only key names and file names appear.
func LogConfigLayers() {
	report := config.GetLayerReport()
	for _, file := range report.Files {
		log.Printf("INFO: Loaded config drop-in: %s", file)
	}
	for _, warning := range report.Warnings {
		log.Printf("WARNING: Config drop-in: %s", warning)
	}
	if mode.IsDebugEnabled() {
		for _, override := range report.Overrides {
			log.Printf("DEBUG: Config key %s is set by %s", override.Key, override.File)
		}
	}
}
