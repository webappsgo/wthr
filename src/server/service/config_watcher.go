package service

import (
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/webappsgo/wthr/src/config"
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

// Start begins watching the config file for changes
func (cw *ConfigWatcher) Start() error {
	// Add config file to watcher
	configDir := filepath.Dir(cw.configPath)
	if err := cw.watcher.Add(configDir); err != nil {
		return err
	}

	log.Printf("👁️  Watching for config file changes: %s", cw.configPath)

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

				// Check if this is our config file
				if filepath.Clean(event.Name) != filepath.Clean(cw.configPath) {
					continue
				}

				// Handle write and create events (editors often write temp files)
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					// Reset or start debounce timer
					if debounceTimer != nil {
						debounceTimer.Stop()
					}

					debounceTimer = time.AfterFunc(debounceDuration, func() {
						log.Println("🔄 Config file changed, reloading...")

						// Load new config
						newCfg, err := config.LoadConfig()
						if err != nil {
							log.Printf("❌ Failed to load new config: %v", err)
							return
						}

						// Trigger reload callback
						if err := cw.reloadFunc(newCfg); err != nil {
							log.Printf("❌ Failed to apply new config: %v", err)
							return
						}

						log.Println("✅ Configuration reloaded successfully (live reload - no restart needed)")
					})
				}

			case err, ok := <-cw.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("⚠️  Config watcher error: %v", err)

			case <-cw.stopChan:
				log.Println("👁️  Stopping config file watcher")
				return
			}
		}
	}()

	return nil
}

// Stop stops the config file watcher
func (cw *ConfigWatcher) Stop() error {
	close(cw.stopChan)
	return cw.watcher.Close()
}
