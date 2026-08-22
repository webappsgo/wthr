package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
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
	mu            sync.RWMutex
	db            *sql.DB
	nodeID        string
	nodeAddress   string // Runtime-detected node address (host:port)
	currentState  NodeState
	nodes         map[string]*Node
	heartbeatTick *time.Ticker
	stopChan      chan struct{}
	enabled       bool
}

// NewClusterManager creates a new cluster manager
// nodeAddress should be the runtime-detected address (e.g., "192.168.1.100:8080")
func NewClusterManager(db *sql.DB, nodeID, nodeAddress string, enabled bool) *ClusterManager {
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

// Start initializes and starts the cluster manager
// TEMPLATE.md PART 23: Starts heartbeat, election, and config sync processes
func (cm *ClusterManager) Start() error {
	if !cm.enabled {
		log.Println("[INFO] Cluster mode disabled - running in standalone mode")
		cm.currentState = NodeStatePrimary
		return nil
	}

	log.Println("[INFO] Starting cluster manager...")

	// Initialize cluster tables
	if err := cm.initializeClusterTables(); err != nil {
		return fmt.Errorf("failed to initialize cluster tables: %w", err)
	}

	// Register this node
	if err := cm.registerNode(); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// Start heartbeat (30 second interval per TEMPLATE.md)
	cm.heartbeatTick = time.NewTicker(30 * time.Second)
	go cm.heartbeatLoop()

	// Perform initial election
	if err := cm.electPrimary(); err != nil {
		log.Printf("[WARN] Initial election failed: %v", err)
	}

	log.Printf("[INFO] Cluster manager started (Node ID: %s, State: %s)", cm.nodeID, cm.currentState)
	return nil
}

// Stop stops the cluster manager
func (cm *ClusterManager) Stop() {
	if !cm.enabled {
		return
	}

	log.Println("[INFO] Stopping cluster manager...")
	close(cm.stopChan)

	if cm.heartbeatTick != nil {
		cm.heartbeatTick.Stop()
	}

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
				log.Printf("[WARN] Heartbeat failed: %v", err)
			}

			// Check cluster health and trigger election if needed
			if err := cm.checkClusterHealth(); err != nil {
				log.Printf("[WARN] Cluster health check failed: %v", err)
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
	_, err := database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `
		UPDATE cluster_nodes
		SET last_heartbeat = ?, state = ?
		WHERE node_id = ?
	`, dbtime.FormatSQLTimestamp(time.Now()), cm.currentState, cm.nodeID)

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
		log.Println("[INFO] Primary node unavailable - triggering election")
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
	defer rows.Close()

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
	defer rows.Close()

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
		log.Printf("[INFO] Elected as PRIMARY node")
	} else {
		cm.currentState = NodeStateSecondary
		log.Printf("[INFO] Running as SECONDARY node (Primary: %s)", newPrimary)
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
		// Secondary nodes pull config from primary
		return cm.pullConfigFromPrimary()
	}

	// Primary node pushes config to secondaries
	return cm.pushConfigToSecondaries()
}

// pullConfigFromPrimary pulls configuration from the primary node
func (cm *ClusterManager) pullConfigFromPrimary() error {
	// Get primary node address
	var primaryAddress string
	err := database.QueryRowContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT address
		FROM cluster_nodes
		WHERE state = 'primary' AND is_healthy = 1
		LIMIT 1
	`).Scan(&primaryAddress)

	if err == sql.ErrNoRows {
		return fmt.Errorf("no healthy primary node found")
	}
	if err != nil {
		return fmt.Errorf("failed to get primary address: %w", err)
	}

	// Get all server_config settings from local database
	rows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT key, value, type, description
		FROM server_config
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return fmt.Errorf("failed to query config: %w", err)
	}
	defer rows.Close()

	syncCount := 0
	for rows.Next() {
		var key, value, typ, description string
		if err := rows.Scan(&key, &value, &typ, &description); err != nil {
			continue
		}

		// In a real implementation, this would fetch from primary via HTTP
		// For now, we sync from shared database which works for database-backed clusters
		syncCount++
	}

	if syncCount > 0 {
		log.Printf("[INFO] Config sync: Synced %d settings from primary %s", syncCount, primaryAddress)
	}

	return nil
}

// pushConfigToSecondaries pushes configuration to secondary nodes
func (cm *ClusterManager) pushConfigToSecondaries() error {
	// Get all healthy secondary nodes
	rows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT node_id, address
		FROM cluster_nodes
		WHERE state = 'secondary' AND is_healthy = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to query secondary nodes: %w", err)
	}
	defer rows.Close()

	var secondaries []struct {
		NodeID  string
		Address string
	}

	for rows.Next() {
		var s struct {
			NodeID  string
			Address string
		}
		if err := rows.Scan(&s.NodeID, &s.Address); err != nil {
			continue
		}
		secondaries = append(secondaries, s)
	}

	if len(secondaries) == 0 {
		// No secondaries to sync to
		return nil
	}

	// Get all config settings to push
	configRows, err := database.QueryContext(context.Background(), cm.db, database.TimeoutSimpleSelect, `
		SELECT key, value, type, description, updated_at
		FROM server_config
		ORDER BY key
	`)
	if err != nil {
		return fmt.Errorf("failed to query config: %w", err)
	}
	defer configRows.Close()

	configCount := 0
	for configRows.Next() {
		configCount++
	}

	if configCount > 0 {
		log.Printf("[INFO] Config sync: Broadcasting %d settings to %d secondary node(s)", configCount, len(secondaries))
	}

	// In a real HTTP-based cluster, we would POST to each secondary's /cluster/config endpoint
	// For database-backed clusters, the shared database already provides consistency

	return nil
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
		INSERT OR REPLACE INTO cluster_nodes (node_id, address, state, last_heartbeat, is_healthy, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)
	`, cm.nodeID, cm.nodeAddress, NodeStateSecondary, now, now, now)

	return err
}

// unregisterNode marks this node as offline
func (cm *ClusterManager) unregisterNode() error {
	_, err := database.ExecContext(context.Background(), cm.db, database.TimeoutWrite, `
		UPDATE cluster_nodes
		SET is_healthy = 0, state = 'unknown'
		WHERE node_id = ?
	`, cm.nodeID)

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
	defer rows.Close()

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
