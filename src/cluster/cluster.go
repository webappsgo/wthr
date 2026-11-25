package cluster

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// The timestamp helpers below are thin aliases over src/common/dbtime, the
// project's single source of truth for SQL timestamp formatting, parsing and
// comparison. dbtime imports only the standard library, so this package shares
// that code with model and scheduler without any import cycle.

// sqlTimestampLayout is the canonical "YYYY-MM-DD HH:MM:SS" layout SQLite's
// CURRENT_TIMESTAMP emits and that PostgreSQL/MySQL accept as a timestamp
// literal. Heartbeats are computed in Go, converted to UTC and formatted with
// this layout so every producer of cluster_nodes.last_heartbeat agrees on one
// zone and one layout.
const sqlTimestampLayout = dbtime.SQLTimestampLayout

// heartbeatDegradedThreshold is AI.md PART 10's 90-second (3 missed heartbeats)
// window after which a node stops counting as healthy.
const heartbeatDegradedThreshold = 90 * time.Second

// updateChunkSize caps how many node IDs one UPDATE statement binds, keeping the
// statement well inside every driver's bind-parameter limit.
const updateChunkSize = 500
const deletedConfigSentinel = `"__wthr_config_deleted__"`
const clusterAssetPrefix = "__wthr_asset_v1__:"

// parseStoredTimestamp converts a value scanned from a TIMESTAMP column into a
// UTC time.Time, reporting false for NULL and for layouts the project never
// writes so an unrecognised value can never mark a live node unhealthy.
func parseStoredTimestamp(value interface{}) (time.Time, bool) {
	return dbtime.ParseStoredTimestamp(value)
}

// NodeState represents the state of a cluster node
type NodeState string

const (
	NodeStatePrimary   NodeState = "primary"
	NodeStateSecondary NodeState = "secondary"
	NodeStateUnknown   NodeState = "unknown"
)

// Node represents a cluster node
type Node struct {
	ID            string
	Address       string
	LastHeartbeat time.Time
	State         NodeState
	IsHealthy     bool
}

// ClusterManager manages cluster operations
// TEMPLATE.md PART 23: Cluster support with heartbeat, primary election, config sync
type ClusterManager struct {
	mu     sync.RWMutex
	db     *database.Store
	nodeID string
	// Runtime-detected node address (host:port)
	nodeAddress       string
	currentState      NodeState
	nodes             map[string]*Node
	heartbeatTick     *time.Ticker
	stopChan          chan struct{}
	enabled           bool
	lastConfigVersion int64
	pendingFileSync   bool
	newNode           bool
}

// NewClusterManager creates a new cluster manager
// nodeAddress should be the runtime-detected address (e.g., "192.168.1.100:8080")
func NewClusterManager(db *database.Store, nodeID, nodeAddress string, enabled bool) *ClusterManager {
	return &ClusterManager{
		db:           db,
		nodeID:       nodeID,
		nodeAddress:  nodeAddress,
		currentState: NodeStateUnknown,
		nodes:        make(map[string]*Node),
		stopChan:     make(chan struct{}),
		enabled:      enabled,
	}
}

// StartClusterManager initializes and starts the cluster manager
// TEMPLATE.md PART 23: Starts heartbeat, election, and config sync processes
func (cm *ClusterManager) StartClusterManager() error {
	if !cm.enabled {
		util.LogInfo("Cluster mode disabled - running in standalone mode")
		cm.currentState = NodeStatePrimary
		return nil
	}

	util.LogInfo("Starting cluster manager...")

	// Initialize cluster tables
	if err := cm.initializeClusterTables(); err != nil {
		return fmt.Errorf("failed to initialize cluster tables: %w", err)
	}
	if err := cm.initializeConfigSyncTables(); err != nil {
		return fmt.Errorf("failed to initialize cluster config tables: %w", err)
	}
	config.SetSharedConfigSyncHooks(cm.writeSharedSetting, cm.deleteSharedSetting, cm.writeConfigToSharedDatabase)
	var registered int
	if err := database.QueryRowContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `SELECT COUNT(*) FROM cluster_nodes WHERE node_id = ?`, cm.nodeID).Scan(&registered); err != nil {
		return fmt.Errorf("check prior cluster registration: %w", err)
	}
	cm.newNode = registered == 0

	// Register this node
	if err := cm.registerNode(); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// Start heartbeat (30 second interval per TEMPLATE.md)
	cm.heartbeatTick = time.NewTicker(30 * time.Second)
	go cm.heartbeatLoop()

	// Perform initial election
	if err := cm.electPrimary(); err != nil {
		util.LogWarn("Initial election failed: %v", err)
	}
	if err := cm.SyncConfig(); err != nil {
		util.LogWarn("Initial cluster config sync failed: %v", err)
	}
	go cm.configSyncLoop()

	util.LogInfo("Cluster manager started (Node ID: %s, State: %s)", cm.nodeID, cm.currentState)
	return nil
}

// StopClusterManager stops the cluster manager
func (cm *ClusterManager) StopClusterManager() {
	if !cm.enabled {
		return
	}

	util.LogInfo("Stopping cluster manager...")
	close(cm.stopChan)

	if cm.heartbeatTick != nil {
		cm.heartbeatTick.Stop()
	}
	config.SetSharedConfigSyncHooks(nil, nil)

	// Mark node as offline
	_ = cm.unregisterNode()
}

// IsPrimary returns true if this node is the primary
func (cm *ClusterManager) IsPrimary() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.currentState == NodeStatePrimary
}

// GetState returns the current node state
func (cm *ClusterManager) GetState() NodeState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.currentState
}

// heartbeatLoop sends periodic heartbeats
// TEMPLATE.md PART 23: Heartbeat every 30 seconds
func (cm *ClusterManager) heartbeatLoop() {
	for {
		select {
		case <-cm.heartbeatTick.C:
			if err := cm.sendHeartbeat(); err != nil {
				util.LogWarn("Heartbeat failed: %v", err)
			}

			// Check cluster health and trigger election if needed
			if err := cm.checkClusterHealth(); err != nil {
				util.LogWarn("Cluster health check failed: %v", err)
			}

		case <-cm.stopChan:
			return
		}
	}
}

// sendHeartbeat updates this node's heartbeat timestamp
// The instant is formatted as canonical UTC text rather than bound as a raw
// time.Time: modernc.org/sqlite writes a bound time.Time as its local-zone
// String() form, which no longer compares correctly against timestamps written
// by CURRENT_TIMESTAMP or by another node in a different zone.
func (cm *ClusterManager) sendHeartbeat() error {
	now := dbtime.FormatSQLTimestamp(time.Now())
	_, err := database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `
		UPDATE cluster_nodes
		SET last_heartbeat = ?, state = ?
		WHERE node_id = ?
	`, now, cm.currentState, cm.nodeID)
	if err == nil {
		_, err = database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `UPDATE server_nodes SET last_heartbeat = ?, status = 'online' WHERE node_id = ?`, now, cm.nodeID)
	}

	return err
}

// checkClusterHealth checks if nodes are healthy and triggers election if primary is dead
func (cm *ClusterManager) checkClusterHealth() error {
	// AI.md PART 10: a node whose last heartbeat is older than 90 seconds (3x the
	// heartbeat interval) stops counting as healthy. The freshness test runs in Go
	// against a UTC cutoff rather than as an SQL text comparison, because
	// last_heartbeat has more than one producer and a lexicographic comparison
	// across mixed zones and layouts marked live nodes dead.
	threshold := time.Now().UTC().Add(-heartbeatDegradedThreshold)

	staleIDs, err := cm.staleNodeIDs(threshold)
	if err != nil {
		return err
	}

	if err := cm.markNodesUnhealthy(staleIDs); err != nil {
		return err
	}

	// Check if primary is unhealthy
	var primaryHealthy bool
	err = database.QueryRowContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT is_healthy
		FROM cluster_nodes
		WHERE state = 'primary'
		LIMIT 1
	`).Scan(&primaryHealthy)

	if err == sql.ErrNoRows || !primaryHealthy {
		// No primary or primary is dead - trigger election
		util.LogInfo("Primary node unavailable - triggering election")
		return cm.electPrimary()
	}

	return err
}

// staleNodeIDs returns the IDs of currently-healthy nodes whose last heartbeat
// is older than cutoff. Nodes whose last_heartbeat is NULL or stored in a layout
// this package does not recognise are left alone rather than assumed dead.
func (cm *ClusterManager) staleNodeIDs(cutoff time.Time) ([]string, error) {
	rows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT node_id, last_heartbeat
		FROM cluster_nodes
		WHERE is_healthy = 1 AND last_heartbeat IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var staleIDs []string
	for rows.Next() {
		var nodeID string
		var stored interface{}
		if scanErr := rows.Scan(&nodeID, &stored); scanErr != nil {
			return nil, scanErr
		}

		parsed, ok := parseStoredTimestamp(stored)
		if !ok {
			continue
		}

		if parsed.Before(cutoff.UTC()) {
			staleIDs = append(staleIDs, nodeID)
		}
	}

	return staleIDs, rows.Err()
}

// markNodesUnhealthy clears the healthy flag for the given node IDs in chunks.
// Only the node IDs are bound values, so no untrusted data reaches the
// statement text.
func (cm *ClusterManager) markNodesUnhealthy(nodeIDs []string) error {
	for start := 0; start < len(nodeIDs); start += updateChunkSize {
		end := start + updateChunkSize
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}

		chunk := nodeIDs[start:end]
		args := make([]interface{}, len(chunk))
		for i, nodeID := range chunk {
			args[i] = nodeID
		}

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := "UPDATE cluster_nodes SET is_healthy = 0 WHERE node_id IN (" + placeholders + ")"

		if _, err := database.ExecContext(context.Background(), cm.db, database.TimeoutBulk, query, args...); err != nil {
			return err
		}
	}

	return nil
}

// electPrimary performs primary node election
// TEMPLATE.md PART 23: Simple primary election (first healthy node wins)
func (cm *ClusterManager) electPrimary() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// NOTE: This is a simplified election algorithm
	// Production would use Raft or similar consensus algorithm

	// Get all healthy nodes ordered by node_id
	rows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT node_id
		FROM cluster_nodes
		WHERE is_healthy = 1
		ORDER BY node_id ASC
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var candidates []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			continue
		}
		candidates = append(candidates, nodeID)
	}

	if len(candidates) == 0 {
		return fmt.Errorf("no healthy nodes available for election")
	}

	// Elect first node (deterministic)
	newPrimary := candidates[0]

	// Update all nodes to secondary
	_, err = database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `UPDATE cluster_nodes SET state = 'secondary'`)
	if err != nil {
		return err
	}

	// Set the elected node as primary
	_, err = database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `
		UPDATE cluster_nodes
		SET state = 'primary'
		WHERE node_id = ?
	`, newPrimary)

	if err != nil {
		return err
	}

	// Update local state
	if newPrimary == cm.nodeID {
		cm.currentState = NodeStatePrimary
		util.LogInfo("Elected as PRIMARY node")
	} else {
		cm.currentState = NodeStateSecondary
		util.LogInfo("Running as SECONDARY node (Primary: %s)", newPrimary)
	}

	return nil
}

// SyncConfig synchronizes configuration from primary to secondaries
// TEMPLATE.md PART 23: Config sync across cluster nodes
func (cm *ClusterManager) SyncConfig() error {
	if !cm.enabled {
		return nil
	}

	if !cm.IsPrimary() {
		if !cm.newNode {
			if err := cm.writeConfigToSharedDatabase(); err != nil {
				return err
			}
		}
		return cm.pullConfigFromSharedDatabase()
	}
	return cm.writeConfigToSharedDatabase()
}

func (cm *ClusterManager) initializeConfigSyncTables() error {
	_, release, err := cm.acquireConfigSyncLock()
	if err != nil {
		return err
	}
	defer release()
	configValueType := "TEXT"
	if cm.db.Dialect() == "mysql" || cm.db.Dialect() == "mariadb" {
		configValueType = "MEDIUMTEXT"
	}
	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS srv_config (key VARCHAR(255) PRIMARY KEY, value %s NOT NULL, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_by VARCHAR(255) NOT NULL)`, configValueType),
		`CREATE TABLE IF NOT EXISTS srv_config_meta (id INTEGER PRIMARY KEY, version BIGINT NOT NULL DEFAULT 0, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, cluster_id VARCHAR(64) NOT NULL DEFAULT '')`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(context.Background(), cm.db, database.TimeoutMigration, statement); err != nil {
			return err
		}
	}
	if _, err := database.ExecContext(context.Background(), cm.db, database.TimeoutMigration, `ALTER TABLE srv_config_meta ADD COLUMN cluster_id VARCHAR(64) NOT NULL DEFAULT ''`); err != nil {
		message := strings.ToLower(err.Error())
		if !strings.Contains(message, "duplicate column") && !strings.Contains(message, "already exists") && !strings.Contains(message, "duplicate column name") {
			return err
		}
	}
	if configValueType == "MEDIUMTEXT" {
		if _, err := database.ExecContext(context.Background(), cm.db, database.TimeoutMigration, `ALTER TABLE srv_config MODIFY value MEDIUMTEXT NOT NULL`); err != nil {
			return err
		}
	}
	var exists int
	if err := database.QueryRowContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `SELECT COUNT(*) FROM srv_config_meta WHERE id = 1`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		if _, err := database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `INSERT INTO srv_config_meta (id, version) VALUES (1, 0)`); err != nil {
			return err
		}
	}
	return nil
}

func (cm *ClusterManager) configSyncLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	fileSyncTicker := time.NewTicker(5 * time.Minute)
	defer fileSyncTicker.Stop()
	for {
		select {
		case <-fileSyncTicker.C:
			if err := config.SyncSharedConfigFromFiles(); err != nil {
				util.LogWarn("Cluster file config sync failed: %v", err)
			}
			if err := cm.pullConfigFromSharedDatabase(); err != nil {
				util.LogWarn("Cluster config refresh failed: %v", err)
			}
		case <-ticker.C:
			cm.mu.RLock()
			pending := cm.pendingFileSync
			cm.mu.RUnlock()
			var err error
			if pending {
				err = cm.writeConfigToSharedDatabase()
				if err == nil {
					cm.mu.Lock()
					cm.pendingFileSync = false
					cm.mu.Unlock()
				}
			}
			if err == nil {
				err = cm.pullConfigFromSharedDatabase()
			}
			if err != nil {
				util.LogWarn("Cluster config sync failed: %v", err)
			}
		case <-cm.stopChan:
			return
		}
	}
}

func (cm *ClusterManager) writeConfigToSharedDatabase() (returnErr error) {
	defer func() {
		if returnErr != nil {
			cm.markPendingConfigSync()
		}
	}()
	settings, err := config.ReadAllSettings()
	if err != nil {
		return err
	}
	assets, err := config.ReadBrandingAssets()
	if err != nil {
		return fmt.Errorf("read branding assets for cluster sync: %w", err)
	}
	for name, data := range assets {
		settings["assets."+name] = clusterAssetPrefix + base64.StdEncoding.EncodeToString(data)
	}
	keys := sortedSharedConfigKeys(settings)
	conn, release, lockErr := cm.acquireConfigSyncLock()
	if lockErr != nil {
		return lockErr
	}
	defer release()
	tx, err := conn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	changed := false
	conflicted := false
	sharedKeys := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		sharedKeys[key] = struct{}{}
		value, err := json.Marshal(settings[key])
		if err != nil {
			return fmt.Errorf("encode cluster setting %s: %w", key, err)
		}
		var existing, updatedBy string
		var storedUpdatedAt any
		err = tx.QueryRowContext(context.Background(), `SELECT value, updated_at, updated_by FROM srv_config WHERE key = ?`, key).Scan(&existing, &storedUpdatedAt, &updatedBy)
		if err == nil && existing == string(value) {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		updatedAtTime := time.Now().UTC()
		if fileTime, ok := config.SettingFileModTime(key); ok {
			updatedAtTime = fileTime.UTC()
		}
		if err == nil && !configUpdateWins(storedUpdatedAt, updatedBy, updatedAtTime, cm.nodeID) {
			conflicted = true
			continue
		}
		updatedAt := dbtime.FormatSQLTimestamp(updatedAtTime)
		if err == sql.ErrNoRows {
			if _, err := tx.ExecContext(context.Background(), `INSERT INTO srv_config (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)`, key, string(value), updatedAt, "node:"+cm.nodeID); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(context.Background(), `UPDATE srv_config SET value = ?, updated_at = ?, updated_by = ? WHERE key = ?`, string(value), updatedAt, "node:"+cm.nodeID, key); err != nil {
				return err
			}
		}
		changed = true
	}
	rows, err := tx.QueryContext(context.Background(), `SELECT key, updated_at, updated_by, value FROM srv_config`)
	if err != nil {
		return err
	}
	var staleKeys []struct {
		key              string
		updatedAt        any
		updatedBy, value string
	}
	for rows.Next() {
		var item struct {
			key              string
			updatedAt        any
			updatedBy, value string
		}
		if err := rows.Scan(&item.key, &item.updatedAt, &item.updatedBy, &item.value); err != nil {
			_ = rows.Close()
			return err
		}
		if _, exists := sharedKeys[item.key]; !exists && item.value != deletedConfigSentinel {
			staleKeys = append(staleKeys, item)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range staleKeys {
		fileTime := time.Now().UTC()
		if modTime, ok := config.SettingFileModTime(item.key); ok {
			fileTime = modTime.UTC()
		}
		if !configUpdateWins(item.updatedAt, item.updatedBy, fileTime, cm.nodeID) {
			continue
		}
		if _, err := tx.ExecContext(context.Background(), `UPDATE srv_config SET value = ?, updated_at = ?, updated_by = ? WHERE key = ?`, deletedConfigSentinel, dbtime.FormatSQLTimestamp(fileTime), "node:"+cm.nodeID, item.key); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		if _, err := tx.ExecContext(context.Background(), `UPDATE srv_config_meta SET version = version + 1, updated_at = ? WHERE id = 1`, dbtime.FormatSQLTimestamp(time.Now())); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	var version int64
	if err := database.QueryRowContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `SELECT version FROM srv_config_meta WHERE id = 1`).Scan(&version); err != nil {
		return err
	}
	cm.mu.Lock()
	cm.lastConfigVersion = version
	cm.mu.Unlock()
	if conflicted {
		cm.mu.Lock()
		cm.lastConfigVersion = -1
		cm.mu.Unlock()
	}
	return nil
}

func (cm *ClusterManager) writeSharedSetting(key string, value any) (returnErr error) {
	defer func() {
		if returnErr != nil {
			cm.markPendingConfigSync()
		}
	}()
	if cm.db == nil || !cm.enabled {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	conn, release, lockErr := cm.acquireConfigSyncLock()
	if lockErr != nil {
		cm.markPendingConfigSync()
		return lockErr
	}
	defer release()
	tx, err := conn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		cm.markPendingConfigSync()
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updatedAtTime := time.Now().UTC()
	updatedAt := dbtime.FormatSQLTimestamp(updatedAtTime)
	var existing string
	var storedUpdatedAt any
	var updatedBy string
	queryErr := tx.QueryRowContext(context.Background(), `SELECT value, updated_at, updated_by FROM srv_config WHERE key = ?`, key).Scan(&existing, &storedUpdatedAt, &updatedBy)
	if queryErr == nil && existing == string(encoded) {
		return tx.Commit()
	}
	if queryErr != nil && queryErr != sql.ErrNoRows {
		cm.markPendingConfigSync()
		return queryErr
	}
	if queryErr == nil && !configUpdateWins(storedUpdatedAt, updatedBy, updatedAtTime, cm.nodeID) {
		cm.mu.Lock()
		cm.lastConfigVersion = -1
		cm.mu.Unlock()
		return tx.Commit()
	}
	result, err := tx.ExecContext(context.Background(), `UPDATE srv_config SET value = ?, updated_at = ?, updated_by = ? WHERE key = ?`, string(encoded), updatedAt, "node:"+cm.nodeID, key)
	if err != nil {
		cm.markPendingConfigSync()
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO srv_config (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)`, key, string(encoded), updatedAt, "node:"+cm.nodeID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(context.Background(), `UPDATE srv_config_meta SET version = version + 1, updated_at = ? WHERE id = 1`, updatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func configUpdateWins(storedValue any, storedBy string, candidate time.Time, candidateNode string) bool {
	storedTime, ok := dbtime.ParseStoredTimestamp(storedValue)
	if !ok {
		return true
	}
	candidate = candidate.UTC().Truncate(time.Second)
	storedTime = storedTime.UTC().Truncate(time.Second)
	if candidate.After(storedTime) {
		return true
	}
	if candidate.Before(storedTime) {
		return false
	}
	return candidateNode > strings.TrimPrefix(storedBy, "node:")
}

func (cm *ClusterManager) markPendingConfigSync() {
	cm.mu.Lock()
	cm.pendingFileSync = true
	cm.mu.Unlock()
}

func (cm *ClusterManager) acquireConfigSyncLock() (*sql.Conn, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	conn, err := cm.db.DB().Conn(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	dialect := cm.db.Dialect()
	switch dialect {
	case "postgres":
		for {
			var acquired bool
			if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(?)`, int64(731942817)).Scan(&acquired); err != nil {
				_ = conn.Close()
				cancel()
				return nil, nil, err
			}
			if acquired {
				release := func() {
					var released bool
					_ = conn.QueryRowContext(context.Background(), `SELECT pg_advisory_unlock(?)`, int64(731942817)).Scan(&released)
					_ = conn.Close()
					cancel()
				}
				return conn, release, nil
			}
			select {
			case <-ctx.Done():
				_ = conn.Close()
				cancel()
				return nil, nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	case "mysql":
		var acquired sql.NullInt64
		if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30)`, "wthr:srv_config:init").Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
			_ = conn.Close()
			cancel()
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("timed out waiting for cluster config lock")
		}
		release := func() {
			var released sql.NullInt64
			_ = conn.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK(?)`, "wthr:srv_config:init").Scan(&released)
			_ = conn.Close()
			cancel()
		}
		return conn, release, nil
	default:
		return conn, func() { _ = conn.Close(); cancel() }, nil
	}
}

func (cm *ClusterManager) deleteSharedSetting(key string) error {
	if cm.db == nil || !cm.enabled {
		return nil
	}
	conn, release, lockErr := cm.acquireConfigSyncLock()
	if lockErr != nil {
		cm.markPendingConfigSync()
		return lockErr
	}
	defer release()
	tx, err := conn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updatedAt := dbtime.FormatSQLTimestamp(time.Now())
	result, err := tx.ExecContext(context.Background(), `UPDATE srv_config SET value = ?, updated_at = ?, updated_by = ? WHERE key = ?`, deletedConfigSentinel, updatedAt, "node:"+cm.nodeID, key)
	if err != nil {
		cm.markPendingConfigSync()
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		cm.markPendingConfigSync()
		return err
	}
	if updated == 0 {
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO srv_config (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)`, key, deletedConfigSentinel, updatedAt, "node:"+cm.nodeID); err != nil {
			cm.markPendingConfigSync()
			return err
		}
	}
	if _, err := tx.ExecContext(context.Background(), `UPDATE srv_config_meta SET version = version + 1, updated_at = ? WHERE id = 1`, updatedAt); err != nil {
		cm.markPendingConfigSync()
		return err
	}
	return tx.Commit()
}

func (cm *ClusterManager) pullConfigFromSharedDatabase() (returnErr error) {
	var version int64
	if err := database.QueryRowContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `SELECT version FROM srv_config_meta WHERE id = 1`).Scan(&version); err != nil {
		cm.mu.Lock()
		cm.lastConfigVersion = -1
		cm.mu.Unlock()
		return err
	}
	cm.mu.RLock()
	lastVersion := cm.lastConfigVersion
	cm.mu.RUnlock()
	if version == lastVersion {
		return nil
	}
	rows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutComplexSelect, `SELECT key, value FROM srv_config ORDER BY key`)
	if err != nil {
		return err
	}
	type entry struct{ key, raw string }
	var entries []entry
	for rows.Next() {
		var item entry
		if err := rows.Scan(&item.key, &item.raw); err != nil {
			_ = rows.Close()
			return err
		}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range entries {
		if item.raw == deletedConfigSentinel {
			if strings.HasPrefix(item.key, "assets.") {
				if err := config.DeleteBrandingAsset(strings.TrimPrefix(item.key, "assets.")); err != nil {
					return fmt.Errorf("remove synced asset %s: %w", item.key, err)
				}
				continue
			}
			if err := config.DeleteSyncedSetting(item.key); err != nil {
				return fmt.Errorf("remove synced setting %s: %w", item.key, err)
			}
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(item.raw), &value); err != nil {
			return fmt.Errorf("decode cluster setting %s: %w", item.key, err)
		}
		if strings.HasPrefix(item.key, "assets.") {
			encoded, ok := value.(string)
			if !ok || !strings.HasPrefix(encoded, clusterAssetPrefix) {
				return fmt.Errorf("decode synced asset %s: invalid payload", item.key)
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, clusterAssetPrefix))
			if err != nil {
				return fmt.Errorf("decode synced asset %s: %w", item.key, err)
			}
			if err := config.SaveBrandingAsset(strings.TrimPrefix(item.key, "assets."), data); err != nil {
				return fmt.Errorf("write synced asset %s: %w", item.key, err)
			}
			continue
		}
		if err := config.WriteSyncedSetting(item.key, value); err != nil {
			return fmt.Errorf("write synced setting %s: %w", item.key, err)
		}
	}
	cm.mu.Lock()
	cm.lastConfigVersion = version
	cm.mu.Unlock()
	return nil
}

func sortedSharedConfigKeys(settings map[string]any) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		if strings.HasPrefix(key, "assets.") {
			keys = append(keys, key)
			continue
		}
		owner := config.CanonicalConfigOwner(key)
		if owner == "server.yml" || owner == "secrets.yml" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// initializeClusterTables creates cluster-related database tables
func (cm *ClusterManager) initializeClusterTables() error {
	_, err := database.ExecContext(context.Background(), cm.db, database.TimeoutMigration, `
		CREATE TABLE IF NOT EXISTS cluster_nodes (
			node_id TEXT PRIMARY KEY,
			address TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'secondary',
			last_heartbeat TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			is_healthy INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// registerNode registers this node in the cluster
// created_at and updated_at are supplied explicitly rather than left to the
// column's CURRENT_TIMESTAMP default, which produces a different type and zone
// on PostgreSQL, MySQL and SQL Server than the canonical UTC text every reader
// in this package parses.
func (cm *ClusterManager) registerNode() error {
	now := dbtime.FormatSQLTimestamp(time.Now())

	_, err := database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `
		INSERT INTO cluster_nodes (node_id, address, state, last_heartbeat, is_healthy, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET address = excluded.address, state = excluded.state,
		last_heartbeat = excluded.last_heartbeat, is_healthy = excluded.is_healthy, updated_at = excluded.updated_at
	`, cm.nodeID, cm.nodeAddress, NodeStateSecondary, now, now, now)

	if err != nil {
		return err
	}
	host, portText, splitErr := net.SplitHostPort(cm.nodeAddress)
	if splitErr != nil {
		host = cm.nodeID
		portText = ""
	}
	port, _ := strconv.Atoi(portText)
	if address := net.ParseIP(host); address != nil {
		host = canonicalIP(address)
	}
	_, err = database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `INSERT INTO server_nodes (node_id, hostname, ip_address, port, status, last_heartbeat, created_at, joined_at) VALUES (?, ?, ?, ?, 'online', ?, ?, ?) ON CONFLICT(node_id) DO UPDATE SET hostname = excluded.hostname, ip_address = excluded.ip_address, port = excluded.port, status = 'online', last_heartbeat = excluded.last_heartbeat`, cm.nodeID, cm.nodeID, host, port, now, now, now)
	return err
}

func canonicalIP(address net.IP) string {
	if ipv4 := address.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return address.String()
}

// unregisterNode marks this node as offline
func (cm *ClusterManager) unregisterNode() error {
	_, err := database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `
		UPDATE cluster_nodes
		SET is_healthy = 0, state = 'unknown'
		WHERE node_id = ?
	`, cm.nodeID)
	if err == nil {
		_, err = database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `UPDATE server_nodes SET status = 'offline' WHERE node_id = ?`, cm.nodeID)
	}

	return err
}

// GetClusterInfo returns information about all cluster nodes
func (cm *ClusterManager) GetClusterInfo() ([]Node, error) {
	rows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT node_id, address, state, last_heartbeat, is_healthy
		FROM cluster_nodes
		ORDER BY state DESC, node_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var nodes []Node
	for rows.Next() {
		var node Node
		var stored interface{}
		if err := rows.Scan(&node.ID, &node.Address, &node.State, &stored, &node.IsHealthy); err != nil {
			continue
		}

		// last_heartbeat is read through the same parser the freshness check
		// uses, so a row written in any historical layout still reports a usable
		// instant instead of silently dropping the node on a scan error.
		if parsed, ok := parseStoredTimestamp(stored); ok {
			node.LastHeartbeat = parsed
		}

		nodes = append(nodes, node)
	}

	return nodes, rows.Err()
}
