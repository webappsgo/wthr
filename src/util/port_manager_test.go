package util

import (
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// newTestPortManager opens a fresh in-memory SQLite database with the
// server_config table PortManager depends on, wires it up as the package
// database.GetServerDB() singleton via SetGlobalDualDB (the same seam
// database's own tests use, see src/database/global_test.go), and returns
// a PortManager backed by it. This exercises the real query/exec paths
// instead of stubbing them out.
func newTestPortManager(t *testing.T) *PortManager {
	t.Helper()

	dsn := fmt.Sprintf("file:portmgr_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS server_config (
			key TEXT PRIMARY KEY,
			value TEXT,
			type TEXT DEFAULT 'string',
			description TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_by TEXT
		)`); err != nil {
		t.Fatalf("create server_config table: %v", err)
	}

	database.SetGlobalDualDB(&database.DualDB{Server: db})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	return NewPortManager(db)
}

// freeTCPPort asks the OS for an ephemeral port, closes the listener, and
// returns the port number. The port is not guaranteed to stay free but is
// extremely unlikely to collide within a single test run.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// TestNewPortManager verifies the constructor wires the *sql.DB through
// unchanged.
func TestNewPortManager(t *testing.T) {
	db, err := sql.Open("sqlite", "file:new_port_manager?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	pm := NewPortManager(db)
	if pm == nil {
		t.Fatal("NewPortManager returned nil")
	}
	if pm.db != db {
		t.Errorf("PortManager.db = %v, want the same *sql.DB passed in", pm.db)
	}
}

// TestPortManager_IntSetting covers the getIntSetting/setIntSetting round
// trip plus the default-value fallback for a missing key.
func TestPortManager_IntSetting(t *testing.T) {
	pm := newTestPortManager(t)

	if got := pm.getIntSetting("missing.key", 42); got != 42 {
		t.Errorf("getIntSetting(missing) = %d, want default 42", got)
	}

	if err := pm.setIntSetting("some.int", 100); err != nil {
		t.Fatalf("setIntSetting: %v", err)
	}
	if got := pm.getIntSetting("some.int", 0); got != 100 {
		t.Errorf("getIntSetting after set = %d, want 100", got)
	}

	// ON CONFLICT branch: setting the same key again must update, not
	// duplicate or error (key is a PRIMARY KEY).
	if err := pm.setIntSetting("some.int", 200); err != nil {
		t.Fatalf("setIntSetting (update): %v", err)
	}
	if got := pm.getIntSetting("some.int", 0); got != 200 {
		t.Errorf("getIntSetting after update = %d, want 200", got)
	}
}

// TestPortManager_BoolSetting verifies setBoolSetting writes the literal
// "true"/"false" strings the rest of the codebase expects for boolean
// server_config rows.
func TestPortManager_BoolSetting(t *testing.T) {
	pm := newTestPortManager(t)

	if err := pm.setBoolSetting("feature.enabled", true); err != nil {
		t.Fatalf("setBoolSetting(true): %v", err)
	}
	var value string
	if err := pm.db.QueryRow("SELECT value FROM server_config WHERE key = ?", "feature.enabled").Scan(&value); err != nil {
		t.Fatalf("query after setBoolSetting(true): %v", err)
	}
	if value != "true" {
		t.Errorf("stored value = %q, want %q", value, "true")
	}

	if err := pm.setBoolSetting("feature.enabled", false); err != nil {
		t.Fatalf("setBoolSetting(false): %v", err)
	}
	if err := pm.db.QueryRow("SELECT value FROM server_config WHERE key = ?", "feature.enabled").Scan(&value); err != nil {
		t.Fatalf("query after setBoolSetting(false): %v", err)
	}
	if value != "false" {
		t.Errorf("stored value = %q, want %q", value, "false")
	}
}

// TestPortManager_SaveAndGetSavedPort covers the save/read round trip and
// the not-found error path.
func TestPortManager_SaveAndGetSavedPort(t *testing.T) {
	pm := newTestPortManager(t)

	if _, err := pm.GetSavedPort("http"); err == nil {
		t.Error("GetSavedPort with nothing saved: err = nil, want error (sql.ErrNoRows)")
	}

	port := freeTCPPort(t)
	if err := pm.SavePort("http", port); err != nil {
		t.Fatalf("SavePort: %v", err)
	}

	got, err := pm.GetSavedPort("http")
	if err != nil {
		t.Fatalf("GetSavedPort: %v", err)
	}
	if got != port {
		t.Errorf("GetSavedPort() = %d, want %d", got, port)
	}

	// Saving again for the same portType must overwrite, not conflict.
	port2 := freeTCPPort(t)
	if err := pm.SavePort("http", port2); err != nil {
		t.Fatalf("SavePort (overwrite): %v", err)
	}
	got, err = pm.GetSavedPort("http")
	if err != nil {
		t.Fatalf("GetSavedPort after overwrite: %v", err)
	}
	if got != port2 {
		t.Errorf("GetSavedPort() after overwrite = %d, want %d", got, port2)
	}
}

// TestPortManager_GetOrAssignPort_AssignsWhenNoneSaved verifies a fresh
// PortManager with no saved port assigns and persists a random port in
// the documented range.
func TestPortManager_GetOrAssignPort_AssignsWhenNoneSaved(t *testing.T) {
	pm := newTestPortManager(t)

	port, err := pm.GetOrAssignPort("http")
	if err != nil {
		t.Fatalf("GetOrAssignPort: %v", err)
	}
	if port < MinPort || port > MaxPort {
		t.Errorf("assigned port %d outside [%d,%d]", port, MinPort, MaxPort)
	}

	// It must have been persisted, so a second call returns the same port
	// (still available, since we never bound it).
	again, err := pm.GetOrAssignPort("http")
	if err != nil {
		t.Fatalf("GetOrAssignPort (second call): %v", err)
	}
	if again != port {
		t.Errorf("GetOrAssignPort() second call = %d, want stable saved port %d", again, port)
	}
}

// TestPortManager_GetOrAssignPort_ReassignsWhenSavedPortBusy verifies the
// fallback path: a saved port that is no longer available (something else
// is bound to it) triggers assignment of a fresh port.
func TestPortManager_GetOrAssignPort_ReassignsWhenSavedPortBusy(t *testing.T) {
	pm := newTestPortManager(t)

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer l.Close()
	busyPort := l.Addr().(*net.TCPAddr).Port

	if err := pm.SavePort("http", busyPort); err != nil {
		t.Fatalf("SavePort: %v", err)
	}

	got, err := pm.GetOrAssignPort("http")
	if err != nil {
		t.Fatalf("GetOrAssignPort: %v", err)
	}
	if got == busyPort {
		t.Errorf("GetOrAssignPort() = %d, want a different port since %d is busy", got, busyPort)
	}
	if got < MinPort || got > MaxPort {
		t.Errorf("reassigned port %d outside [%d,%d]", got, MinPort, MaxPort)
	}
}

// TestPortManager_ParsePortConfig covers single-port, dual-port, standard
// Let's Encrypt ports (80,443), and the invalid-input error paths.
func TestPortManager_ParsePortConfig(t *testing.T) {
	t.Run("single_port", func(t *testing.T) {
		pm := newTestPortManager(t)
		port := freeTCPPort(t)
		httpPort, httpsPort, err := pm.ParsePortConfig(fmt.Sprintf("%d", port))
		if err != nil {
			t.Fatalf("ParsePortConfig: %v", err)
		}
		if httpPort != port {
			t.Errorf("httpPort = %d, want %d", httpPort, port)
		}
		if httpsPort != 0 {
			t.Errorf("httpsPort = %d, want 0 for single-port config", httpsPort)
		}
		if got := pm.getIntSetting("server.http_port", -1); got != port {
			t.Errorf("saved server.http_port = %d, want %d", got, port)
		}
	})

	t.Run("dual_port", func(t *testing.T) {
		pm := newTestPortManager(t)
		p1 := freeTCPPort(t)
		l2, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		p2 := l2.Addr().(*net.TCPAddr).Port
		l2.Close()

		httpPort, httpsPort, err := pm.ParsePortConfig(fmt.Sprintf(" %d , %d ", p1, p2))
		if err != nil {
			t.Fatalf("ParsePortConfig: %v", err)
		}
		if httpPort != p1 || httpsPort != p2 {
			t.Errorf("ParsePortConfig() = (%d,%d), want (%d,%d)", httpPort, httpsPort, p1, p2)
		}
		var enabled string
		if err := pm.db.QueryRow("SELECT value FROM server_config WHERE key = ?", "server.https_enabled").Scan(&enabled); err != nil {
			t.Fatalf("query https_enabled: %v", err)
		}
		if enabled != "true" {
			t.Errorf("server.https_enabled = %q, want true", enabled)
		}
	})

	t.Run("invalid_http_port", func(t *testing.T) {
		pm := newTestPortManager(t)
		if _, _, err := pm.ParsePortConfig("not-a-number"); err == nil {
			t.Error("ParsePortConfig(non-numeric) err = nil, want error")
		}
	})

	t.Run("invalid_https_port", func(t *testing.T) {
		pm := newTestPortManager(t)
		port := freeTCPPort(t)
		if _, _, err := pm.ParsePortConfig(fmt.Sprintf("%d,bogus", port)); err == nil {
			t.Error("ParsePortConfig(bad https port) err = nil, want error")
		}
	})

	t.Run("http_port_in_use", func(t *testing.T) {
		pm := newTestPortManager(t)
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		defer l.Close()
		busy := l.Addr().(*net.TCPAddr).Port
		if _, _, err := pm.ParsePortConfig(fmt.Sprintf("%d", busy)); err == nil {
			t.Error("ParsePortConfig(busy port) err = nil, want error")
		}
	})
}

// TestPortManager_GetServerPorts_UsesConfigPort verifies the priority
// chain falls through to the passed-in config port when nothing is saved
// in the database yet.
func TestPortManager_GetServerPorts_UsesConfigPort(t *testing.T) {
	pm := newTestPortManager(t)
	configPort := freeTCPPort(t)

	httpPort, httpsPort, err := pm.GetServerPortsWithConfig(configPort)
	if err != nil {
		t.Fatalf("GetServerPortsWithConfig: %v", err)
	}
	if httpPort != configPort {
		t.Errorf("httpPort = %d, want configPort %d", httpPort, configPort)
	}
	if httpsPort != 0 {
		t.Errorf("httpsPort = %d, want 0", httpsPort)
	}

	// Saved in DB now, so a later call (even with a different configPort)
	// must prefer the already-saved value.
	otherConfigPort := freeTCPPort(t)
	httpPort2, _, err := pm.GetServerPortsWithConfig(otherConfigPort)
	if err != nil {
		t.Fatalf("GetServerPortsWithConfig (second call): %v", err)
	}
	if httpPort2 != configPort {
		t.Errorf("httpPort on second call = %d, want previously saved %d", httpPort2, configPort)
	}
}

// TestPortManager_GetServerPorts_UsesPortEnv verifies the PORT env var is
// consulted (and parsed via ParsePortConfig) when no DB or config port is
// available.
func TestPortManager_GetServerPorts_UsesPortEnv(t *testing.T) {
	pm := newTestPortManager(t)
	envPort := freeTCPPort(t)
	t.Setenv("PORT", fmt.Sprintf("%d", envPort))

	httpPort, _, err := pm.GetServerPorts()
	if err != nil {
		t.Fatalf("GetServerPorts: %v", err)
	}
	if httpPort != envPort {
		t.Errorf("httpPort = %d, want PORT env value %d", httpPort, envPort)
	}
}

// TestPortManager_GetServerPorts_RandomFallback verifies a random port in
// range is assigned and persisted when nothing else is configured.
func TestPortManager_GetServerPorts_RandomFallback(t *testing.T) {
	pm := newTestPortManager(t)
	t.Setenv("PORT", "")

	httpPort, httpsPort, err := pm.GetServerPorts()
	if err != nil {
		t.Fatalf("GetServerPorts: %v", err)
	}
	if httpPort < MinPort || httpPort > MaxPort {
		t.Errorf("httpPort %d outside [%d,%d]", httpPort, MinPort, MaxPort)
	}
	if httpsPort != 0 {
		t.Errorf("httpsPort = %d, want 0", httpsPort)
	}
}

// TestPortManager_UpdatePort covers enabling HTTPS, disabling it again,
// and the port-unavailable error paths for both HTTP and HTTPS.
func TestPortManager_UpdatePort(t *testing.T) {
	t.Run("http_only", func(t *testing.T) {
		pm := newTestPortManager(t)
		port := freeTCPPort(t)
		if err := pm.UpdatePort(port, 0); err != nil {
			t.Fatalf("UpdatePort: %v", err)
		}
		if got := pm.getIntSetting("server.http_port", -1); got != port {
			t.Errorf("server.http_port = %d, want %d", got, port)
		}
		var enabled string
		if err := pm.db.QueryRow("SELECT value FROM server_config WHERE key = ?", "server.https_enabled").Scan(&enabled); err != nil {
			t.Fatalf("query https_enabled: %v", err)
		}
		if enabled != "false" {
			t.Errorf("server.https_enabled = %q, want false when httpsPort=0", enabled)
		}
	})

	t.Run("http_and_https", func(t *testing.T) {
		pm := newTestPortManager(t)
		httpPort := freeTCPPort(t)
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		httpsPort := l.Addr().(*net.TCPAddr).Port
		l.Close()

		if err := pm.UpdatePort(httpPort, httpsPort); err != nil {
			t.Fatalf("UpdatePort: %v", err)
		}
		if got := pm.getIntSetting("server.https_port", -1); got != httpsPort {
			t.Errorf("server.https_port = %d, want %d", got, httpsPort)
		}
	})

	t.Run("http_port_busy", func(t *testing.T) {
		pm := newTestPortManager(t)
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		defer l.Close()
		busy := l.Addr().(*net.TCPAddr).Port
		if err := pm.UpdatePort(busy, 0); err == nil {
			t.Error("UpdatePort(busy http) err = nil, want error")
		}
	})

	t.Run("https_port_busy", func(t *testing.T) {
		pm := newTestPortManager(t)
		httpPort := freeTCPPort(t)
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		defer l.Close()
		busy := l.Addr().(*net.TCPAddr).Port
		if err := pm.UpdatePort(httpPort, busy); err == nil {
			t.Error("UpdatePort(busy https) err = nil, want error")
		}
	})
}
