package utils

import (
	"net"
	"testing"
)

// TestIsPortAvailable_Available checks a port with nothing bound reports
// available, using a port picked by the OS to avoid flaky collisions.
func TestIsPortAvailable_Available(t *testing.T) {
	// Ask the OS for a free ephemeral port, close it, then verify
	// IsPortAvailable reports it as available immediately after.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	if !IsPortAvailable(port) {
		t.Errorf("IsPortAvailable(%d) = false immediately after closing, want true", port)
	}
}

// TestIsPortAvailable_InUse verifies a port with an active listener is
// reported as unavailable.
func TestIsPortAvailable_InUse(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	if IsPortAvailable(port) {
		t.Errorf("IsPortAvailable(%d) = true while port is in use, want false", port)
	}
}

// TestGetRandomAvailablePort verifies the returned port is within the
// documented range and is actually available to bind at return time.
func TestGetRandomAvailablePort(t *testing.T) {
	port, err := GetRandomAvailablePort()
	if err != nil {
		t.Fatalf("GetRandomAvailablePort: %v", err)
	}
	if port < MinPort || port > MaxPort {
		t.Errorf("GetRandomAvailablePort() = %d, want in [%d,%d]", port, MinPort, MaxPort)
	}
	if !IsPortAvailable(port) {
		t.Errorf("GetRandomAvailablePort() returned %d which is not actually available", port)
	}
}

// Coverage gap: PortManager methods (GetOrAssignPort, SavePort,
// GetSavedPort, GetServerPorts*, ParsePortConfig, UpdatePort) all depend
// on database.GetServerDB(), a package-level singleton SQLite connection
// with no dependency-injection seam in this package. Per testing-rules.md
// (no real databases in unit tests, no external services), and since
// GetServerDB() cannot be swapped for an in-memory DB from this package,
// these are left untested here. GetServerIP()/GetServerAddress() are also
// excluded: GetServerIP() shells out to `hostname -I` and dials
// 8.8.8.8:80, both of which are host/network-dependent and disallowed by
// the no-external-services rule for unit tests.
