package util

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// newTestPIDFile builds a PIDFile pointed at a path inside t.TempDir(),
// bypassing NewPIDFile()'s root-vs-user branching (which targets a global
// system path like /run/casapps/wthr.pid when running as root inside the
// Docker verification container) so tests never touch real system paths.
func newTestPIDFile(t *testing.T) *PIDFile {
	t.Helper()
	return &PIDFile{Path: filepath.Join(t.TempDir(), "sub", "test.pid")}
}

// TestPIDFile_CheckNoFile verifies Check() reports "not running" with no
// error when the PID file does not exist.
func TestPIDFile_CheckNoFile(t *testing.T) {
	pf := newTestPIDFile(t)
	running, pid, err := pf.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if running {
		t.Error("running = true, want false for nonexistent PID file")
	}
	if pid != 0 {
		t.Errorf("pid = %d, want 0", pid)
	}
}

// TestPIDFile_CreateAndCheck verifies Create() writes the current PID and
// Check() reports it as running (since the test process itself is alive).
func TestPIDFile_CreateAndCheck(t *testing.T) {
	pf := newTestPIDFile(t)

	if err := pf.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}

	data, err := os.ReadFile(pf.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	gotPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("PID file content not an integer: %q", data)
	}
	if gotPID != os.Getpid() {
		t.Errorf("PID file contains %d, want %d", gotPID, os.Getpid())
	}

	running, pid, err := pf.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !running {
		t.Error("running = false, want true (current process is alive)")
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}
}

// TestPIDFile_CreateWhenAlreadyRunning verifies Create() refuses to
// overwrite a PID file for a still-running process.
func TestPIDFile_CreateWhenAlreadyRunning(t *testing.T) {
	pf := newTestPIDFile(t)
	if err := pf.Create(); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := pf.Create(); err == nil {
		t.Error("second Create() while already running: err = nil, want error")
	}
}

// TestPIDFile_Remove verifies Remove() deletes the file and is idempotent
// (no error when called again on a missing file).
func TestPIDFile_Remove(t *testing.T) {
	pf := newTestPIDFile(t)
	if err := pf.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := pf.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(pf.Path); !os.IsNotExist(err) {
		t.Errorf("PID file still exists after Remove: err = %v", err)
	}
	// Idempotent: removing an already-removed file is not an error.
	if err := pf.Remove(); err != nil {
		t.Errorf("second Remove() on missing file: %v, want nil", err)
	}
}

// TestPIDFile_GetPID covers the happy path and the missing-file error path.
func TestPIDFile_GetPID(t *testing.T) {
	t.Run("missing_file", func(t *testing.T) {
		pf := newTestPIDFile(t)
		if _, err := pf.GetPID(); err == nil {
			t.Error("GetPID on missing file: err = nil, want error")
		}
	})

	t.Run("valid_file", func(t *testing.T) {
		pf := newTestPIDFile(t)
		if err := pf.Create(); err != nil {
			t.Fatalf("Create: %v", err)
		}
		pid, err := pf.GetPID()
		if err != nil {
			t.Fatalf("GetPID: %v", err)
		}
		if pid != os.Getpid() {
			t.Errorf("GetPID() = %d, want %d", pid, os.Getpid())
		}
	})

	t.Run("invalid_pid_content", func(t *testing.T) {
		pf := newTestPIDFile(t)
		if err := os.MkdirAll(filepath.Dir(pf.Path), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(pf.Path, []byte("not-a-pid"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := pf.GetPID(); err == nil {
			t.Error("GetPID with non-numeric content: err = nil, want error")
		}
	})
}

// TestPIDFile_CheckStaleFile verifies a PID file referencing a definitely
// nonexistent process is treated as not-running and cleaned up.
func TestPIDFile_CheckStaleFile(t *testing.T) {
	pf := newTestPIDFile(t)
	if err := os.MkdirAll(filepath.Dir(pf.Path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// PID 1 exists on Linux (init) but is not our test process, so
	// use an implausibly large PID unlikely to be assigned.
	if err := os.WriteFile(pf.Path, []byte("999999"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	running, _, err := pf.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if running {
		t.Error("running = true for a PID that should not exist, want false")
	}
}

// TestNewPIDFile_Structural verifies NewPIDFile builds a path under the
// given data directory when not running as root, without invoking any
// file-writing methods (root-path branch is environment-dependent and
// intentionally not exercised here to avoid touching real system paths).
func TestNewPIDFile_Structural(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: NewPIDFile targets a global system path, not the data dir")
	}
	dataDir := t.TempDir()
	pf := NewPIDFile(dataDir)
	want := filepath.Join(dataDir, "wthr.pid")
	if pf.Path != want {
		t.Errorf("NewPIDFile(%q).Path = %q, want %q", dataDir, pf.Path, want)
	}
}
