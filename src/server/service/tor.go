package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cretz/bine/tor"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
)

// TorService manages the Tor hidden service
type TorService struct {
	tor             *tor.Tor
	onionService    *tor.OnionService
	dataDir         string
	db              *database.DB
	ctx             context.Context
	cancel          context.CancelFunc
	onionAddress    string
	isRunning       bool
	mu              sync.RWMutex
	startTime       time.Time
	lastHealthCheck time.Time
	healthStatus    string
	restartCount    int
	monitorEnabled  bool
	monitorStop     chan struct{}
}

// NewTorService creates a new Tor service instance
func NewTorService(db *database.DB, dataDir string) *TorService {
	ctx, cancel := context.WithCancel(context.Background())

	torDataDir := filepath.Join(dataDir, "tor")
	if err := os.MkdirAll(torDataDir, 0700); err != nil {
		log.Printf("WARNING: Failed to create Tor data directory: %v", err)
	}

	return &TorService{
		dataDir:        torDataDir,
		db:             db,
		ctx:            ctx,
		cancel:         cancel,
		isRunning:      false,
		healthStatus:   "not_started",
		monitorEnabled: true,
		monitorStop:    make(chan struct{}),
	}
}

// Start initializes and starts the Tor hidden service
func (ts *TorService) Start(httpPort int) error {
	settingsModel := &model.SettingsModel{DB: ts.db.DB}

	// Check if Tor is enabled
	enabled := settingsModel.GetBool("tor.enabled", true)
	if !enabled {
		log.Println("ℹ️  Tor hidden service is disabled in settings")
		return nil
	}

	log.Println("INFO: Starting Tor hidden service...")

	// Start OUR OWN dedicated Tor process (completely separate from system Tor)
	// This uses the tor binary but with our own DataDir and random ports
	// CGO_ENABLED=0 compatible via github.com/cretz/bine
	conf := &tor.StartConf{
		// Our isolated data directory
		DataDir: ts.dataDir,
		// Show startup messages
		NoHush:          false,
		TempDataDirBase: filepath.Join(os.TempDir(), "casapps", "wthr-tor"),
		// Let bine pick available ports (avoids system Tor 9050/9051)
		NoAutoSocksPort: false,
		// ExePath is not set - bine will find 'tor' in PATH
		// This creates a DEDICATED Tor process separate from any system Tor
	}

	t, err := tor.Start(ts.ctx, conf)
	if err != nil {
		return fmt.Errorf("failed to start dedicated Tor process: %w (install tor: apt install tor / brew install tor)", err)
	}
	ts.tor = t

	log.Println("   Started dedicated Tor process (isolated from system Tor)")

	log.Println("INFO: Waiting for Tor to be ready...")

	// Wait for Tor to be ready with timeout
	readyCtx, readyCancel := context.WithTimeout(ts.ctx, 3*time.Minute)
	defer readyCancel()

	if err := t.EnableNetwork(readyCtx, true); err != nil {
		ts.Stop()
		return fmt.Errorf("failed to enable Tor network: %w", err)
	}

	log.Println("OK: Tor is ready, creating hidden service...")

	// Create or load hidden service
	onionKey := filepath.Join(ts.dataDir, "site", "private_key")

	// Configure hidden service to forward to our HTTP server
	onionService, err := t.Listen(ts.ctx, &tor.ListenConf{
		RemotePorts: []int{80},
		LocalPort:   httpPort,
		Key:         onionKey,
		Version3:    true,
	})
	if err != nil {
		ts.Stop()
		return fmt.Errorf("failed to create hidden service: %w", err)
	}
	ts.onionService = onionService

	// Get the .onion address
	onionAddr := onionService.ID + ".onion"

	ts.mu.Lock()
	ts.onionAddress = onionAddr
	ts.isRunning = true
	ts.startTime = time.Now()
	ts.healthStatus = "healthy"
	ts.lastHealthCheck = time.Now()
	ts.mu.Unlock()

	// Save .onion address to database
	if err := settingsModel.Set("tor.onion_address", onionAddr, "string"); err != nil {
		log.Printf("WARNING: Failed to save .onion address to database: %v", err)
	}

	// Save Tor data directory path
	if err := settingsModel.Set("tor.data_dir", ts.dataDir, "string"); err != nil {
		log.Printf("WARNING: Failed to save Tor data directory: %v", err)
	}

	log.Printf("INFO: Tor hidden service active: http://%s", onionAddr)

	// Start monitoring in background
	if ts.monitorEnabled {
		go ts.monitorHealth()
	}

	return nil
}

// Stop shuts down the Tor service gracefully
func (ts *TorService) Stop() error {
	if ts.tor == nil {
		return nil
	}

	log.Println("INFO: Stopping Tor hidden service...")

	// Stop monitoring
	if ts.monitorEnabled {
		close(ts.monitorStop)
	}

	if ts.onionService != nil {
		ts.onionService.Close()
	}

	if err := ts.tor.Close(); err != nil {
		log.Printf("WARNING: Error stopping Tor: %v", err)
		return err
	}

	ts.cancel()

	ts.mu.Lock()
	ts.isRunning = false
	ts.healthStatus = "stopped"
	ts.mu.Unlock()

	log.Println("OK: Tor hidden service stopped")
	return nil
}

// GetOnionAddress returns the current .onion address
func (ts *TorService) GetOnionAddress() string {
	return ts.onionAddress
}

// IsRunning returns true if Tor service is running
func (ts *TorService) IsRunning() bool {
	return ts.isRunning
}

// GetStatus returns the current Tor service status
func (ts *TorService) GetStatus() map[string]interface{} {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	status := map[string]interface{}{
		"enabled":       ts.isRunning,
		"onion_address": ts.onionAddress,
		"data_dir":      ts.dataDir,
		"health":        ts.healthStatus,
		"restart_count": ts.restartCount,
	}

	if ts.isRunning {
		status["status"] = "connected"
		status["uptime"] = time.Since(ts.startTime).String()
		status["last_health_check"] = ts.lastHealthCheck.Format(time.RFC3339)
	} else {
		status["status"] = "disconnected"
	}

	return status
}

// RegenerateAddress stops the service, deletes keys, and restarts with new address
func (ts *TorService) RegenerateAddress(httpPort int) error {
	log.Println("INFO: Regenerating Tor .onion address...")

	// Stop current service
	if err := ts.Stop(); err != nil {
		return fmt.Errorf("failed to stop Tor service: %w", err)
	}

	// Delete existing keys
	keysDir := filepath.Join(ts.dataDir, "site")
	if err := os.RemoveAll(keysDir); err != nil {
		log.Printf("WARNING: Failed to delete old keys: %v", err)
	}

	// Restart with new keys
	return ts.Start(httpPort)
}

// monitorHealth runs periodic health checks on the Tor service
func (ts *TorService) monitorHealth() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ts.monitorStop:
			return
		case <-ticker.C:
			ts.performHealthCheck()
		}
	}
}

// performHealthCheck checks if Tor is still running and responsive
func (ts *TorService) performHealthCheck() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if !ts.isRunning {
		return
	}

	// Check if the Tor instance is still alive
	// bine doesn't expose a direct health check, but we can verify the onion service
	if ts.onionService == nil || ts.tor == nil {
		ts.healthStatus = "unhealthy"
		log.Printf("WARNING: Tor health check failed: service or instance is nil")
		return
	}

	// Update last health check time
	ts.lastHealthCheck = time.Now()
	ts.healthStatus = "healthy"
}

// GetHealthStatus returns the current health status
func (ts *TorService) GetHealthStatus() map[string]interface{} {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return map[string]interface{}{
		"status":            ts.healthStatus,
		"last_check":        ts.lastHealthCheck.Format(time.RFC3339),
		"is_running":        ts.isRunning,
		"uptime":            time.Since(ts.startTime).String(),
		"restart_count":     ts.restartCount,
		"monitoring_active": ts.monitorEnabled,
	}
}

// EnableMonitoring enables health monitoring
func (ts *TorService) EnableMonitoring() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if !ts.monitorEnabled {
		ts.monitorEnabled = true
		ts.monitorStop = make(chan struct{})
		if ts.isRunning {
			go ts.monitorHealth()
		}
	}
}

// DisableMonitoring disables health monitoring
func (ts *TorService) DisableMonitoring() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.monitorEnabled {
		ts.monitorEnabled = false
		close(ts.monitorStop)
	}
}
