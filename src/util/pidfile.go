package util

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/webappsgo/wthr/src/privilege"
)

// PIDFile represents a PID file for process management
type PIDFile struct {
	Path string
}

// pidFileOrg and pidFileName are the on-disk identity of this service (AI.md PART 4:
// {internal_org} and {internal_name}); they never change when the project is renamed.
const (
	pidFileOrg  = privilege.InternalOrg
	pidFileName = privilege.InternalName
)

// SystemPIDFilePath is the root-mode PID file: one directory per service so a service
// account can remove only its own PID file (AI.md PART 8 "PID directory (root)").
func SystemPIDFilePath() string {
	return filepath.Join("/var/run", pidFileOrg, pidFileName, pidFileName+".pid")
}

// NewPIDFile creates a new PID file manager.
// AI.md PART 8: root mode uses /var/run/{internal_org}/{internal_name}/{internal_name}.pid,
// user mode uses {data_dir}/{internal_name}.pid.
func NewPIDFile(dataDir string) *PIDFile {
	var pidPath string

	if custom := strings.TrimSpace(os.Getenv("PID_FILE")); custom != "" {
		pidPath = custom
	} else if os.Geteuid() == 0 {
		pidPath = SystemPIDFilePath()
	} else {
		pidPath = filepath.Join(dataDir, pidFileName+".pid")
	}

	return &PIDFile{Path: pidPath}
}

// CheckStalePID verifies if a PID file exists and if the process is still running
// Returns: (isRunning bool, pid int, err error)
// Per TEMPLATE.md PART 6 lines 3474-3548: Implement stale PID detection
func (p *PIDFile) CheckStalePID() (bool, int, error) {
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
	if err != nil && !errors.Is(err, syscall.EPERM) {
		// Process doesn't exist - remove stale PID file
		os.Remove(p.Path)
		return false, pid, nil
	}

	// The PID may have been reused by another program; only our own binary counts
	if !isOurProcess(pid) {
		os.Remove(p.Path)
		return false, pid, nil
	}

	// Process is running
	return true, pid, nil
}

// CreatePIDFile creates a PID file with the current process ID
func (p *PIDFile) CreatePIDFile() error {
	// Check if already running
	isRunning, existingPID, err := p.CheckStalePID()
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

// isOurProcess verifies pid runs this same binary. The comparison is an exact
// match on the executable name, never a substring, so the companion
// {project_name}-cli binary is not mistaken for the server (AI.md PART 8).
func isOurProcess(pid int) bool {
	own := executableBaseName()
	if own == "" {
		return true
	}

	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return baseName(strings.TrimSuffix(exe, " (deleted)")) == own
	}

	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}

	return baseName(strings.TrimSpace(string(output))) == own
}

// StartPIDFile creates the PID file for this process. Containers skip it
// entirely: PIDs are namespace-local and the runtime supervises the process
// (AI.md PART 8). The returned file is nil when none was created.
func StartPIDFile(dataDir string) (*PIDFile, error) {
	if IsContainer() {
		return nil, nil
	}

	pf := NewPIDFile(dataDir)
	if err := pf.CreatePIDFile(); err != nil {
		return nil, err
	}

	return pf, nil
}
