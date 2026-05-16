package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// PIDFile represents a PID file for process management
type PIDFile struct {
	Path string
}

// NewPIDFile creates a new PID file manager
// Per TEMPLATE.md PART 6: PID file should be in /var/run/ (root) or user temp dir
func NewPIDFile(dataDir string) *PIDFile {
	var pidPath string

	if os.Geteuid() == 0 {
		// Running as root - use /var/run or /run
		if _, err := os.Stat("/run"); err == nil {
			pidPath = "/run/weather.pid"
		} else {
			pidPath = "/var/run/weather.pid"
		}
	} else {
		// Running as regular user - use data directory
		pidPath = filepath.Join(dataDir, "weather.pid")
	}

	return &PIDFile{Path: pidPath}
}

// Check verifies if a PID file exists and if the process is still running
// Returns: (isRunning bool, pid int, err error)
// Per TEMPLATE.md PART 6 lines 3474-3548: Implement stale PID detection
func (p *PIDFile) Check() (bool, int, error) {
	// Check if PID file exists
	data, err := os.ReadFile(p.Path)
	if os.IsNotExist(err) {
		// No PID file - service not running
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to read PID file %s: %w", p.Path, err)
	}

	// Parse PID from file
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// Invalid PID file - remove it
		os.Remove(p.Path)
		return false, 0, fmt.Errorf("invalid PID in file %s: %s (removed stale file)", p.Path, pidStr)
	}

	// Check if process is still running
	// Send signal 0 to check process existence without actually sending a signal
	process, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist - remove stale PID file
		os.Remove(p.Path)
		return false, 0, nil
	}

	// On Unix-like systems, sending signal 0 checks if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist - remove stale PID file
		os.Remove(p.Path)
		return false, pid, nil
	}

	// Process is running
	return true, pid, nil
}

// Create creates a PID file with the current process ID
func (p *PIDFile) Create() error {
	// Check if already running
	isRunning, existingPID, err := p.Check()
	if err != nil {
		return err
	}

	if isRunning {
		return fmt.Errorf("service already running with PID %d", existingPID)
	}

	// Create directory if it doesn't exist
	// AI.md PART 7: Permissions - root: 0755/0644, user: 0700/0600
	dirPerm := os.FileMode(0700)
	filePerm := os.FileMode(0600)
	if os.Geteuid() == 0 {
		dirPerm = 0755
		filePerm = 0644
	}
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("failed to create PID directory %s: %w", dir, err)
	}

	// Write current PID to file
	pid := os.Getpid()
	content := fmt.Sprintf("%d\n", pid)

	if err := os.WriteFile(p.Path, []byte(content), filePerm); err != nil {
		return fmt.Errorf("failed to write PID file %s: %w", p.Path, err)
	}

	return nil
}

// Remove removes the PID file
func (p *PIDFile) Remove() error {
	if err := os.Remove(p.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file %s: %w", p.Path, err)
	}
	return nil
}

// GetPID reads and returns the PID from the file
func (p *PIDFile) GetPID() (int, error) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %s", pidStr)
	}

	return pid, nil
}
