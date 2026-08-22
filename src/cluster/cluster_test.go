// Tests for cluster.go per AI.md PART 23 (Cluster) / PART 29 (Testing)
package cluster

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newTestDB opens an isolated in-memory SQLite database per
// testing-rules.md ("file:name?mode=memory&cache=shared"). Each test gets a
// unique name so tests never share state.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newReadyManager returns a ClusterManager with cluster_nodes already
// created and this node registered, ready for election/heartbeat tests.
func newReadyManager(t *testing.T, nodeID, nodeAddress string) *ClusterManager {
	t.Helper()
	db := newTestDB(t)
	cm := NewClusterManager(db, nodeID, nodeAddress, true)
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("initializeClusterTables() error = %v", err)
	}
	if err := cm.registerNode(); err != nil {
		t.Fatalf("registerNode() error = %v", err)
	}
	return cm
}

// TestStart_Disabled verifies standalone mode: no DB tables touched, node
// immediately becomes primary, and Stop() is a safe no-op.
func TestStart_Disabled(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-1", "127.0.0.1:8080", false)

	if err := cm.Start(); err != nil {
		t.Fatalf("Start() error = %v, want nil in standalone mode", err)
	}
	if !cm.IsPrimary() {
		t.Error("IsPrimary() = false, want true immediately in standalone mode")
	}
	if cm.GetState() != NodeStatePrimary {
		t.Errorf("GetState() = %v, want %v", cm.GetState(), NodeStatePrimary)
	}

	// No cluster_nodes table should have been created in standalone mode.
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='cluster_nodes'`).Scan(&name)
	if err != sql.ErrNoRows {
		t.Errorf("expected cluster_nodes table to NOT exist in standalone mode, query err = %v", err)
	}

	// Stop() must not panic or block when never actually started (enabled=false).
	cm.Stop()
}

// TestInitializeClusterTables_Idempotent verifies calling it twice does not
// error (CREATE TABLE IF NOT EXISTS).
func TestInitializeClusterTables_Idempotent(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-1", "127.0.0.1:8080", true)

	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("first call error = %v", err)
	}
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("second call error = %v", err)
	}
}

// TestRegisterNode_InsertOrReplace verifies re-registering the same node
// (e.g. after a restart) updates rather than duplicates the row.
func TestRegisterNode_InsertOrReplace(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")

	// Re-register with a different address.
	cm.nodeAddress = "10.0.0.2:8080"
	if err := cm.registerNode(); err != nil {
		t.Fatalf("re-register error = %v", err)
	}

	var count int
	if err := cm.db.QueryRow(`SELECT COUNT(*) FROM cluster_nodes WHERE node_id = ?`, "node-1").Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 1 {
		t.Errorf("row count for node-1 = %d, want 1 (no duplicate)", count)
	}

	var address string
	if err := cm.db.QueryRow(`SELECT address FROM cluster_nodes WHERE node_id = ?`, "node-1").Scan(&address); err != nil {
		t.Fatalf("address query error = %v", err)
	}
	if address != "10.0.0.2:8080" {
		t.Errorf("address = %q, want updated address %q", address, "10.0.0.2:8080")
	}
}

// TestUnregisterNode marks the node unhealthy/unknown without deleting it.
func TestUnregisterNode(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")

	if err := cm.unregisterNode(); err != nil {
		t.Fatalf("unregisterNode() error = %v", err)
	}

	var state string
	var healthy bool
	if err := cm.db.QueryRow(`SELECT state, is_healthy FROM cluster_nodes WHERE node_id = ?`, "node-1").Scan(&state, &healthy); err != nil {
		t.Fatalf("query error = %v", err)
	}
	if state != "unknown" {
		t.Errorf("state = %q, want %q", state, "unknown")
	}
	if healthy {
		t.Error("is_healthy = true, want false after unregister")
	}
}

// TestElectPrimary_NoHealthyNodes verifies election fails explicitly rather
// than silently electing an empty/zero-value node when the cluster_nodes
// table has no healthy rows.
func TestElectPrimary_NoHealthyNodes(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-1", "127.0.0.1:8080", true)
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("setup error = %v", err)
	}
	// No nodes registered at all.

	if err := cm.electPrimary(); err == nil {
		t.Error("electPrimary() with zero nodes = nil, want error")
	}
}

// TestElectPrimary_Deterministic verifies the lowest node_id (ASC) among
// healthy nodes always wins, regardless of insertion order, and that the
// winner's own currentState reflects the outcome (primary vs secondary).
func TestElectPrimary_Deterministic(t *testing.T) {
	db := newTestDB(t)

	// Two managers sharing the same underlying cluster table, simulating
	// two nodes in the same cluster.
	cmA := NewClusterManager(db, "node-b", "10.0.0.2:8080", true)
	if err := cmA.initializeClusterTables(); err != nil {
		t.Fatalf("setup error = %v", err)
	}
	if err := cmA.registerNode(); err != nil {
		t.Fatalf("register node-b error = %v", err)
	}

	cmB := NewClusterManager(db, "node-a", "10.0.0.1:8080", true)
	if err := cmB.registerNode(); err != nil {
		t.Fatalf("register node-a error = %v", err)
	}

	if err := cmA.electPrimary(); err != nil {
		t.Fatalf("electPrimary() error = %v", err)
	}

	// "node-a" sorts before "node-b" — node-a must win regardless of which
	// manager triggered the election.
	if cmA.GetState() != NodeStateSecondary {
		t.Errorf("cmA (node-b) state = %v, want secondary (node-a should win)", cmA.GetState())
	}

	var primaryID string
	if err := db.QueryRow(`SELECT node_id FROM cluster_nodes WHERE state = 'primary'`).Scan(&primaryID); err != nil {
		t.Fatalf("query primary error = %v", err)
	}
	if primaryID != "node-a" {
		t.Errorf("elected primary = %q, want %q", primaryID, "node-a")
	}
}

// TestElectPrimary_IgnoresUnhealthyNodes verifies a lower node_id that is
// marked unhealthy is skipped in favor of the next healthy candidate.
func TestElectPrimary_IgnoresUnhealthyNodes(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-b", "10.0.0.2:8080", true)
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("setup error = %v", err)
	}

	// node-a has the lowest ID but is unhealthy.
	if _, err := db.Exec(`INSERT INTO cluster_nodes (node_id, address, state, is_healthy) VALUES ('node-a', 'x', 'secondary', 0)`); err != nil {
		t.Fatalf("insert node-a error = %v", err)
	}
	if err := cm.registerNode(); err != nil {
		t.Fatalf("register node-b error = %v", err)
	}

	if err := cm.electPrimary(); err != nil {
		t.Fatalf("electPrimary() error = %v", err)
	}

	var primaryID string
	if err := db.QueryRow(`SELECT node_id FROM cluster_nodes WHERE state = 'primary'`).Scan(&primaryID); err != nil {
		t.Fatalf("query primary error = %v", err)
	}
	if primaryID != "node-b" {
		t.Errorf("elected primary = %q, want %q (unhealthy node-a must be skipped)", primaryID, "node-b")
	}
	if cm.GetState() != NodeStatePrimary {
		t.Errorf("cm.GetState() = %v, want primary", cm.GetState())
	}
}

// TestCheckClusterHealth_StaleHeartbeatTriggersElection verifies a primary
// whose heartbeat is older than the 90-second threshold is marked
// unhealthy and a new election is triggered automatically.
func TestCheckClusterHealth_StaleHeartbeatTriggersElection(t *testing.T) {
	db := newTestDB(t)
	stale := time.Now().Add(-5 * time.Minute)
	fresh := time.Now()

	if _, err := db.Exec(`CREATE TABLE cluster_nodes (
		node_id TEXT PRIMARY KEY, address TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'secondary',
		last_heartbeat TIMESTAMP NOT NULL, is_healthy INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("setup error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cluster_nodes (node_id, address, state, last_heartbeat, is_healthy) VALUES ('node-old', 'x', 'primary', ?, 1)`, stale); err != nil {
		t.Fatalf("insert stale primary error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cluster_nodes (node_id, address, state, last_heartbeat, is_healthy) VALUES ('node-new', 'y', 'secondary', ?, 1)`, fresh); err != nil {
		t.Fatalf("insert fresh secondary error = %v", err)
	}

	cm := NewClusterManager(db, "node-new", "y", true)

	if err := cm.checkClusterHealth(); err != nil {
		t.Fatalf("checkClusterHealth() error = %v", err)
	}

	var oldHealthy bool
	if err := db.QueryRow(`SELECT is_healthy FROM cluster_nodes WHERE node_id = 'node-old'`).Scan(&oldHealthy); err != nil {
		t.Fatalf("query error = %v", err)
	}
	if oldHealthy {
		t.Error("stale node still marked healthy after checkClusterHealth()")
	}

	var newPrimary string
	if err := db.QueryRow(`SELECT node_id FROM cluster_nodes WHERE state = 'primary'`).Scan(&newPrimary); err != nil {
		t.Fatalf("query new primary error = %v", err)
	}
	if newPrimary != "node-new" {
		t.Errorf("new primary = %q, want %q (only remaining healthy node)", newPrimary, "node-new")
	}
}

// TestCheckClusterHealth_HealthyPrimaryNoElection verifies a healthy,
// recently-heartbeating primary is left untouched (no unnecessary
// re-election churn).
func TestCheckClusterHealth_HealthyPrimaryNoElection(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")
	if err := cm.electPrimary(); err != nil {
		t.Fatalf("initial election error = %v", err)
	}
	if cm.GetState() != NodeStatePrimary {
		t.Fatalf("sanity check failed: single node should be primary, got %v", cm.GetState())
	}

	if err := cm.checkClusterHealth(); err != nil {
		t.Fatalf("checkClusterHealth() error = %v", err)
	}

	var healthy bool
	if err := cm.db.QueryRow(`SELECT is_healthy FROM cluster_nodes WHERE node_id = 'node-1'`).Scan(&healthy); err != nil {
		t.Fatalf("query error = %v", err)
	}
	if !healthy {
		t.Error("healthy primary was incorrectly marked unhealthy")
	}
}

// TestSendHeartbeat verifies the timestamp and state columns are updated
// for this node only.
func TestSendHeartbeat(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")
	cm.currentState = NodeStatePrimary

	before := time.Now().Add(-1 * time.Hour)
	if _, err := cm.db.Exec(`UPDATE cluster_nodes SET last_heartbeat = ? WHERE node_id = 'node-1'`, before); err != nil {
		t.Fatalf("setup error = %v", err)
	}

	if err := cm.sendHeartbeat(); err != nil {
		t.Fatalf("sendHeartbeat() error = %v", err)
	}

	var state string
	var lastHeartbeat time.Time
	if err := cm.db.QueryRow(`SELECT state, last_heartbeat FROM cluster_nodes WHERE node_id = 'node-1'`).Scan(&state, &lastHeartbeat); err != nil {
		t.Fatalf("query error = %v", err)
	}
	if state != "primary" {
		t.Errorf("state = %q, want %q", state, "primary")
	}
	if !lastHeartbeat.After(before) {
		t.Errorf("last_heartbeat = %v, want updated to be after %v", lastHeartbeat, before)
	}
}

// clusterTestLocalLayout mirrors the layout time.Time.String() emits, which is
// what modernc.org/sqlite writes when a Go time.Time is bound directly.
const clusterTestLocalLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// farWestZone and farEastZone are fixed offsets chosen so a wall-clock text
// comparison and a real instant comparison always disagree, whatever zone the
// machine running the tests happens to be in. Their names are short and
// all-uppercase because the "MST" element of clusterTestLocalLayout reads back
// only three-letter names or four/five-letter names ending in T - a longer name
// such as "FARWEST" makes every fixture below unparseable, so the cases would
// silently test the fail-closed path instead of a real zone comparison.
var (
	farWestZone = time.FixedZone("WST", -11*60*60)
	farEastZone = time.FixedZone("EAST", 13*60*60)
)

// TestCheckClusterHealth_MixedZoneHeartbeats verifies node freshness is decided
// on the absolute instant, not on the text a heartbeat happens to be stored as.
// Heartbeats written by an older build carry the writer's local zone, so
// comparing them as text marked live nodes stale (and stale nodes live)
// depending on which side of UTC the writer sat on.
func TestCheckClusterHealth_MixedZoneHeartbeats(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		heartbeat   string
		wantHealthy bool
	}{
		{
			name:        "fresh canonical utc",
			heartbeat:   now.Format(sqlTimestampLayout),
			wantHealthy: true,
		},
		{
			name:        "fresh far west local layout",
			heartbeat:   now.In(farWestZone).Format(clusterTestLocalLayout),
			wantHealthy: true,
		},
		{
			name:        "fresh far west local layout with monotonic suffix",
			heartbeat:   now.In(farWestZone).Format(clusterTestLocalLayout) + " m=+0.000000001",
			wantHealthy: true,
		},
		{
			name:        "stale far east local layout",
			heartbeat:   now.Add(-5 * time.Minute).In(farEastZone).Format(clusterTestLocalLayout),
			wantHealthy: false,
		},
		{
			name:        "stale canonical utc",
			heartbeat:   now.Add(-5 * time.Minute).Format(sqlTimestampLayout),
			wantHealthy: false,
		},
		{
			name:        "just inside the degraded threshold",
			heartbeat:   now.Add(-heartbeatDegradedThreshold + 30*time.Second).Format(sqlTimestampLayout),
			wantHealthy: true,
		},
		{
			name:        "unparseable value is left alone",
			heartbeat:   "not-a-timestamp",
			wantHealthy: true,
		},
	}

	db := newTestDB(t)
	// This node is always fresh so the election checkClusterHealth triggers
	// always has a candidate.
	cm := NewClusterManager(db, "node-anchor", "10.0.0.1:8080", true)
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("initializeClusterTables() error = %v", err)
	}
	if err := cm.registerNode(); err != nil {
		t.Fatalf("registerNode() error = %v", err)
	}

	for i, tt := range tests {
		nodeID := fmt.Sprintf("node-%02d", i)
		if _, err := db.Exec(`INSERT INTO cluster_nodes (node_id, address, state, last_heartbeat, is_healthy) VALUES (?, ?, 'secondary', ?, 1)`, nodeID, nodeID, tt.heartbeat); err != nil {
			t.Fatalf("insert %s error = %v", nodeID, err)
		}
	}

	if err := cm.checkClusterHealth(); err != nil {
		t.Fatalf("checkClusterHealth() error = %v", err)
	}

	for i, tt := range tests {
		nodeID := fmt.Sprintf("node-%02d", i)
		t.Run(tt.name, func(t *testing.T) {
			var healthy bool
			if err := db.QueryRow(`SELECT is_healthy FROM cluster_nodes WHERE node_id = ?`, nodeID).Scan(&healthy); err != nil {
				t.Fatalf("query %s error = %v", nodeID, err)
			}
			if healthy != tt.wantHealthy {
				t.Errorf("is_healthy = %v, want %v (heartbeat stored as %q)", healthy, tt.wantHealthy, tt.heartbeat)
			}
		})
	}
}

// TestSendHeartbeat_WritesCanonicalUTC verifies the heartbeat lands in the one
// layout every reader parses, in UTC, so heartbeats written on machines in
// different zones stay directly comparable.
func TestSendHeartbeat_WritesCanonicalUTC(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")

	if err := cm.sendHeartbeat(); err != nil {
		t.Fatalf("sendHeartbeat() error = %v", err)
	}

	// CAST(... AS TEXT) keeps the driver from converting the column to a
	// time.Time on the way out, so the assertion sees the bytes actually on
	// disk rather than a re-rendered value.
	var stored string
	if err := cm.db.QueryRow(`SELECT CAST(last_heartbeat AS TEXT) FROM cluster_nodes WHERE node_id = 'node-1'`).Scan(&stored); err != nil {
		t.Fatalf("query error = %v", err)
	}

	parsed, err := time.Parse(sqlTimestampLayout, stored)
	if err != nil {
		t.Fatalf("last_heartbeat %q is not in the canonical layout: %v", stored, err)
	}
	if drift := time.Since(parsed); drift < -time.Minute || drift > time.Minute {
		t.Errorf("last_heartbeat is %s away from now, want within a minute", drift)
	}
}

// TestRegisterNode_WritesCanonicalUTC pins the created_at/updated_at values
// registerNode writes.
//
// Both columns used to be left to the schema's CURRENT_TIMESTAMP default, which
// is canonical UTC text on SQLite but a native timestamp in the server's own
// zone on PostgreSQL, MySQL and SQL Server. Every reader in this package parses
// the column as canonical UTC text, so the values are now bound explicitly.
func TestRegisterNode_WritesCanonicalUTC(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")

	// CAST(... AS TEXT) keeps the driver from converting the columns to a
	// time.Time on the way out, so the assertion sees the bytes on disk.
	var createdAt, updatedAt string
	err := cm.db.QueryRow(`
		SELECT CAST(created_at AS TEXT), CAST(updated_at AS TEXT)
		FROM cluster_nodes
		WHERE node_id = 'node-1'
	`).Scan(&createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("query error = %v", err)
	}

	for _, column := range []struct {
		name  string
		value string
	}{{"created_at", createdAt}, {"updated_at", updatedAt}} {
		parsed, err := time.Parse(sqlTimestampLayout, column.value)
		if err != nil {
			t.Fatalf("%s %q is not in the canonical layout: %v", column.name, column.value, err)
		}
		if drift := time.Since(parsed); drift < -time.Minute || drift > time.Minute {
			t.Errorf("%s is %s away from now, want within a minute", column.name, drift)
		}
	}
}

// TestParseStoredTimestamp_Cluster covers the package-local timestamp parser
// against every layout a cluster_nodes row can actually hold.
func TestParseStoredTimestamp_Cluster(t *testing.T) {
	want := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		stored interface{}
		wantOK bool
	}{
		{name: "canonical utc text", stored: "2025-01-01 12:00:00", wantOK: true},
		{name: "canonical utc bytes", stored: []byte("2025-01-01 12:00:00"), wantOK: true},
		{name: "local zone layout", stored: want.In(farWestZone).Format(clusterTestLocalLayout), wantOK: true},
		{name: "local zone layout with monotonic suffix", stored: want.In(farEastZone).Format(clusterTestLocalLayout) + " m=+0.000000001", wantOK: true},
		{name: "rfc3339", stored: "2025-01-01T12:00:00Z", wantOK: true},
		{name: "driver returned time", stored: want.In(farEastZone), wantOK: true},
		{name: "null", stored: nil, wantOK: false},
		{name: "unparseable", stored: "not-a-timestamp", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStoredTimestamp(tt.stored)
			if ok != tt.wantOK {
				t.Fatalf("parseStoredTimestamp(%v) ok = %v, want %v", tt.stored, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if !got.Equal(want) {
				t.Fatalf("parseStoredTimestamp(%v) = %s, want %s", tt.stored, got, want)
			}
		})
	}
}

// TestGetClusterInfo_Ordering verifies results follow the literal SQL
// "ORDER BY state DESC, node_id ASC": since "secondary" > "primary"
// alphabetically, secondary nodes sort BEFORE the primary (node_id ASC
// within each group), not the "primary first" ordering the column name
// might suggest. A UI or CLI relying on this order would silently
// misrender if it regressed.
func TestGetClusterInfo_Ordering(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-z", "z", true)
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("setup error = %v", err)
	}

	inserts := []struct{ id, state string }{
		{"node-c", "secondary"},
		{"node-a", "secondary"},
		{"node-b", "primary"},
	}
	for _, ins := range inserts {
		if _, err := db.Exec(`INSERT INTO cluster_nodes (node_id, address, state, last_heartbeat, is_healthy) VALUES (?, ?, ?, ?, 1)`, ins.id, ins.id, ins.state, time.Now()); err != nil {
			t.Fatalf("insert %s error = %v", ins.id, err)
		}
	}

	nodes, err := cm.GetClusterInfo()
	if err != nil {
		t.Fatalf("GetClusterInfo() error = %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}

	wantOrder := []string{"node-a", "node-c", "node-b"}
	for i, want := range wantOrder {
		if nodes[i].ID != want {
			t.Errorf("nodes[%d].ID = %q, want %q (full order: %v)", i, nodes[i].ID, want, nodes)
		}
	}
}

// TestGetClusterInfo_Empty verifies an empty cluster returns an empty,
// non-error result rather than nil-panicking downstream consumers.
func TestGetClusterInfo_Empty(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-1", "x", true)
	if err := cm.initializeClusterTables(); err != nil {
		t.Fatalf("setup error = %v", err)
	}

	nodes, err := cm.GetClusterInfo()
	if err != nil {
		t.Fatalf("GetClusterInfo() error = %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("got %d nodes, want 0 for empty cluster", len(nodes))
	}
}

// TestSyncConfig_DisabledIsNoop verifies SyncConfig short-circuits when the
// cluster manager is not enabled, never touching the database.
func TestSyncConfig_DisabledIsNoop(t *testing.T) {
	db := newTestDB(t)
	cm := NewClusterManager(db, "node-1", "x", false)

	if err := cm.SyncConfig(); err != nil {
		t.Fatalf("SyncConfig() error = %v, want nil when disabled", err)
	}
}

// TestPullConfigFromPrimary_NoPrimary verifies a secondary with no healthy
// primary in the table reports an explicit error rather than silently
// succeeding (which would mask a split-brain / no-primary condition).
func TestPullConfigFromPrimary_NoPrimary(t *testing.T) {
	cm := newReadyManager(t, "node-2", "10.0.0.2:8080")
	// registerNode() defaults this node to 'secondary' with no primary present.

	if err := cm.pullConfigFromPrimary(); err == nil {
		t.Error("pullConfigFromPrimary() with no primary = nil, want error")
	}
}

// TestPushConfigToSecondaries_NoSecondaries verifies a primary with zero
// secondaries returns nil (nothing to push) rather than erroring.
func TestPushConfigToSecondaries_NoSecondaries(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")
	if err := cm.electPrimary(); err != nil {
		t.Fatalf("electPrimary() error = %v", err)
	}

	if err := cm.pushConfigToSecondaries(); err != nil {
		t.Errorf("pushConfigToSecondaries() error = %v, want nil with no secondaries", err)
	}
}

// TestSyncConfig_PrimaryPushesSecondaryPulls exercises SyncConfig's
// branch selection end to end: the elected primary must take the push
// path, and a secondary must take the pull path (and succeed once a
// healthy primary exists).
func TestSyncConfig_PrimaryPushesSecondaryPulls(t *testing.T) {
	db := newTestDB(t)
	// pullConfigFromPrimary/pushConfigToSecondaries read from server_config,
	// which is owned by the server/database package, not cluster.go — create
	// it here so this in-memory DB matches the real schema those methods
	// expect at runtime.
	if _, err := db.Exec(`CREATE TABLE server_config (key TEXT PRIMARY KEY, value TEXT, type TEXT, description TEXT, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("setup server_config table error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO server_config (key, value, type, description) VALUES ('server.mode', 'production', 'string', 'app mode')`); err != nil {
		t.Fatalf("seed server_config error = %v", err)
	}

	cmPrimary := NewClusterManager(db, "node-a", "10.0.0.1:8080", true)
	if err := cmPrimary.initializeClusterTables(); err != nil {
		t.Fatalf("setup error = %v", err)
	}
	if err := cmPrimary.registerNode(); err != nil {
		t.Fatalf("register primary error = %v", err)
	}

	cmSecondary := NewClusterManager(db, "node-b", "10.0.0.2:8080", true)
	if err := cmSecondary.registerNode(); err != nil {
		t.Fatalf("register secondary error = %v", err)
	}

	if err := cmPrimary.electPrimary(); err != nil {
		t.Fatalf("electPrimary() error = %v", err)
	}
	if !cmPrimary.IsPrimary() {
		t.Fatal("sanity check failed: node-a should be primary (lowest node_id)")
	}

	if err := cmPrimary.SyncConfig(); err != nil {
		t.Errorf("SyncConfig() on primary error = %v, want nil", err)
	}

	cmSecondary.currentState = NodeStateSecondary
	if err := cmSecondary.SyncConfig(); err != nil {
		t.Errorf("SyncConfig() on secondary error = %v, want nil (healthy primary exists)", err)
	}
}

// TestIsPrimary_And_GetState_ConcurrentReads guards against a data race on
// currentState: readers use mu.RLock while a writer holds mu.Lock in
// electPrimary. This test exercises IsPrimary/GetState concurrently with
// an election under the race detector.
func TestIsPrimary_And_GetState_ConcurrentReads(t *testing.T) {
	cm := newReadyManager(t, "node-1", "10.0.0.1:8080")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = cm.IsPrimary()
			_ = cm.GetState()
		}
	}()

	for i := 0; i < 5; i++ {
		if err := cm.electPrimary(); err != nil {
			t.Fatalf("electPrimary() error = %v", err)
		}
	}
	<-done
}
