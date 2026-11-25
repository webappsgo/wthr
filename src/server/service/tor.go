package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cretz/bine/control"
	"github.com/cretz/bine/tor"
	"github.com/webappsgo/wthr/src/common/display"
	"github.com/webappsgo/wthr/src/database"
	paths "github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/model"
)

const (
	torLifecycleStarting = "starting"
	torLifecycleRunning  = "running"
	torLifecycleFailed   = "failed"
	torLifecycleDisabled = "disabled"
)

// ErrTorBinaryMissing is returned by StartTorService when no tor binary is on
// PATH. It is informational: a missing Tor is a normal state, never a failure.
var ErrTorBinaryMissing = errors.New("tor binary not found")

// torBinaryFound reports whether a tor binary is on PATH; a missing binary is a
// normal state (disabled), never an error.
func torBinaryFound() bool {
	_, err := exec.LookPath("tor")
	return err == nil
}

// TorService manages the Tor hidden service
type TorService struct {
	tor               *tor.Tor
	onionServiceID    string
	dataDir           string
	db                *database.Store
	ctx               context.Context
	cancel            context.CancelFunc
	onionAddress      string
	isRunning         bool
	mu                sync.RWMutex
	startTime         time.Time
	lastHealthCheck   time.Time
	healthStatus      string
	restartCount      int
	monitorEnabled    bool
	monitorStop       chan struct{}
	monitorStopClosed bool
	stopMu            sync.Mutex
	// lifecycle is idle (never launched), starting, running or failed; it feeds Status()
	lifecycle string
	// httpPort is the local HTTP port the hidden service forwards to, kept for Restart
	httpPort int
}

// NewTorService creates a new Tor service instance
func NewTorService(db *database.Store, dataDir string) *TorService {
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
		lifecycle:      torLifecycleStarting,
		monitorEnabled: true,
		monitorStop:    make(chan struct{}),
	}
}

// StartTorService initializes and starts the Tor hidden service
func (ts *TorService) StartTorService(httpPort int) error {
	ts.mu.Lock()
	if ts.monitorStopClosed && ts.monitorEnabled {
		ts.monitorStop = make(chan struct{})
		ts.monitorStopClosed = false
	}
	ts.httpPort = httpPort
	ts.mu.Unlock()
	if !torBinaryFound() {
		ts.setLifecycle(torLifecycleDisabled)
		return ErrTorBinaryMissing
	}
	ts.setLifecycle(torLifecycleStarting)
	if err := ts.startTorService(httpPort); err != nil {
		ts.setLifecycle(torLifecycleFailed)
		return err
	}
	return nil
}

// Restart stops the hidden service and starts it again on the same local port.
func (ts *TorService) Restart() error {
	ts.mu.RLock()
	httpPort := ts.httpPort
	ts.mu.RUnlock()
	if err := ts.StopTorService(); err != nil {
		return fmt.Errorf("failed to stop Tor service: %w", err)
	}
	return ts.StartTorService(httpPort)
}

// SetDisabled records an admin switching the hidden service off.
func (ts *TorService) SetDisabled() {
	ts.setLifecycle(torLifecycleDisabled)
}

// setLifecycle records the launch state used by Status().
func (ts *TorService) setLifecycle(state string) {
	ts.mu.Lock()
	ts.lifecycle = state
	ts.mu.Unlock()
}

// startTorService launches the dedicated Tor process and the hidden service.
func (ts *TorService) startTorService(httpPort int) error {
	settingsModel := &model.SettingsModel{Store: database.GetServerStore()}

	// Check if Tor is enabled
	enabled := settingsModel.GetBool("tor.enabled", true)
	if !enabled {
		ts.setLifecycle(torLifecycleDisabled)
		log.Printf("%s Tor hidden service is disabled in settings", display.Emoji("ℹ️", "INFO:"))
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
		TempDataDirBase: filepath.Join(os.TempDir(), "webappsgo", "wthr-tor"),
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
		_ = ts.StopTorService()
		return fmt.Errorf("failed to enable Tor network: %w", err)
	}

	log.Println("OK: Tor is ready, creating hidden service...")

	// Create or load the v3 hidden service: ADD_ONION forwards .onion:80 to the server's own
	// port, so Tor never binds a second listener on that port (AI.md PART 32.1)
	keyPath := filepath.Join(ts.dataDir, "site", "private_key")
	onionKey, err := loadOrCreateOnionKey(keyPath)
	if err != nil {
		_ = ts.StopTorService()
		return fmt.Errorf("failed to prepare the hidden service key: %w", err)
	}

	onionResponse, err := t.Control.AddOnion(&control.AddOnionRequest{
		Key:   onionKey,
		Ports: []*control.KeyVal{control.NewKeyVal("80", fmt.Sprintf("127.0.0.1:%d", httpPort))},
	})
	if err != nil {
		_ = ts.StopTorService()
		return fmt.Errorf("failed to create hidden service: %w", err)
	}
	ts.onionServiceID = onionResponse.ServiceID

	// The config-side Tor directory is private like every private path (AI.md PART 32.1)
	if err := os.MkdirAll(filepath.Join(paths.GetConfigDir(), "tor"), 0o700); err != nil {
		log.Printf("WARNING: Failed to create the Tor config directory: %v", err)
	}

	// A generated key is saved so the .onion address survives restarts
	if onionResponse.Key != nil {
		if err := saveOnionKey(keyPath, onionResponse.Key.Blob()); err != nil {
			log.Printf("WARNING: Failed to save the hidden service key: %v", err)
		}
	}

	// Get the .onion address
	onionAddr := onionResponse.ServiceID + ".onion"

	// The hostname file next to the key records the address for the operator
	if err := os.WriteFile(filepath.Join(ts.dataDir, "site", "hostname"), []byte(onionAddr+"\n"), 0o600); err != nil {
		log.Printf("WARNING: Failed to write the onion hostname file: %v", err)
	}

	ts.mu.Lock()
	ts.onionAddress = onionAddr
	ts.isRunning = true
	ts.startTime = time.Now()
	ts.healthStatus = "healthy"
	ts.lifecycle = torLifecycleRunning
	ts.lastHealthCheck = time.Now()
	ts.mu.Unlock()

	// Save .onion address to database
	if err := settingsModel.SetSetting("tor.onion_address", onionAddr, "string"); err != nil {
		log.Printf("WARNING: Failed to save .onion address to database: %v", err)
	}

	// Save Tor data directory path
	if err := settingsModel.SetSetting("tor.data_dir", ts.dataDir, "string"); err != nil {
		log.Printf("WARNING: Failed to save Tor data directory: %v", err)
	}

	log.Printf("INFO: Tor hidden service active: http://%s", onionAddr)

	// Start monitoring in background
	if ts.monitorEnabled {
		go ts.monitorHealth()
	}

	return nil
}

// StopTorService shuts down the Tor service gracefully
func (ts *TorService) StopTorService() error {
	ts.stopMu.Lock()
	defer ts.stopMu.Unlock()

	ts.mu.Lock()
	torInstance := ts.tor
	if torInstance == nil {
		cancel := ts.cancel
		ts.mu.Unlock()
		// Nothing runs yet; cancelling also stops a start that is still launching the process
		if cancel != nil {
			cancel()
		}
		return nil
	}
	if ts.monitorEnabled {
		ts.stopMonitorLocked()
	}
	onionServiceID := ts.onionServiceID
	ts.onionServiceID = ""
	ts.mu.Unlock()

	log.Println("INFO: Stopping Tor hidden service...")

	if onionServiceID != "" {
		_ = torInstance.Control.DelOnion(onionServiceID)
	}

	err := torInstance.Close()
	if err != nil {
		log.Printf("WARNING: Error stopping Tor: %v", err)
	}

	ts.mu.Lock()
	if ts.tor == torInstance {
		ts.tor = nil
	}
	ts.isRunning = false
	ts.healthStatus = "stopped"
	if ts.lifecycle == torLifecycleRunning {
		ts.lifecycle = torLifecycleFailed
	}
	cancel := ts.cancel
	ts.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	log.Println("OK: Tor hidden service stopped")
	return err
}

// stopMonitorLocked closes the monitor signal at most once. The caller must
// hold ts.mu; startup failure and server shutdown can stop Tor concurrently.
func (ts *TorService) stopMonitorLocked() {
	if ts.monitorStop != nil && !ts.monitorStopClosed {
		close(ts.monitorStop)
		ts.monitorStopClosed = true
	}
}

// GetOnionAddress returns the current .onion address
func (ts *TorService) GetOnionAddress() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.onionAddress
}

// IsRunning returns true if Tor service is running
func (ts *TorService) IsRunning() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.isRunning
}

// Enabled reports whether the hidden service is active on this instance: a tor
// binary was found and the service was not switched off by an admin.
func (ts *TorService) Enabled() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.lifecycle != torLifecycleDisabled && torBinaryFound()
}

// Status returns one of the four health-contract words: disabled, starting,
// healthy or unavailable. Failure detail goes to the log, never to this value.
func (ts *TorService) Status() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if ts.lifecycle == torLifecycleDisabled || !torBinaryFound() {
		return "disabled"
	}
	switch {
	case ts.isRunning && ts.healthStatus != "unhealthy":
		return "healthy"
	case ts.lifecycle == torLifecycleFailed || ts.isRunning:
		return "unavailable"
	default:
		return "starting"
	}
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
	if err := ts.StopTorService(); err != nil {
		return fmt.Errorf("failed to stop Tor service: %w", err)
	}

	// Delete existing keys
	keysDir := filepath.Join(ts.dataDir, "site")
	if err := os.RemoveAll(keysDir); err != nil {
		log.Printf("WARNING: Failed to delete old keys: %v", err)
	}

	// Restart with new keys
	return ts.StartTorService(httpPort)
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
	if ts.onionServiceID == "" || ts.tor == nil {
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
		ts.monitorStopClosed = false
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
		ts.stopMonitorLocked()
	}
}

// loadOrCreateOnionKey returns the saved v3 key, or a request for Tor to generate a new one.
func loadOrCreateOnionKey(keyPath string) (control.Key, error) {
	blob, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return control.GenKey(control.KeyAlgoED25519V3), nil
	}
	if err != nil {
		return nil, err
	}

	key, err := control.ED25519KeyFromBlob(strings.TrimSpace(string(blob)))
	if err != nil {
		return nil, fmt.Errorf("the saved hidden service key is unreadable: %w", err)
	}

	return key, nil
}

// saveOnionKey writes the hidden service key into its private directory (0700 directory,
// 0600 file) so the address is stable across restarts.
func saveOnionKey(keyPath, blob string) error {
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return err
	}

	return os.WriteFile(keyPath, []byte(blob), 0o600)
}
