// Package cli provides command-line interface per TEMPLATE.md PART 6
package cli

import (
	"fmt"
	"os"
	"time"
)

// StatusCommand shows server status, node info, cluster info, Tor status
// Per TEMPLATE.md PART 6: Works without root privileges
type StatusCommand struct {
	// Will be populated by main.go when server is running
	ServerRunning bool
	Port          int
	Mode          string
	StartTime     time.Time
	NodeID        string
	ClusterMode   bool
	ClusterStatus string
	ClusterNodes  int
	DatabaseInfo  string
	TorEnabled    bool
	TorConnected  bool
	TorAddress    string
}

// Execute runs the status command
func (s *StatusCommand) Execute() error {
	fmt.Println(T("cli.status.title"))
	fmt.Println()

	// Server Status
	fmt.Println(T("cli.status.server_status_heading"))
	if s.ServerRunning {
		fmt.Println(T("cli.status.running"))
		fmt.Printf(T("cli.status.port")+"\n", s.Port)
		fmt.Printf(T("cli.status.mode")+"\n", s.Mode)
		if !s.StartTime.IsZero() {
			uptime := time.Since(s.StartTime)
			fmt.Printf(T("cli.status.uptime")+"\n", formatDuration(uptime))
		}
	} else {
		fmt.Println(T("cli.status.stopped"))
	}
	fmt.Println()

	// Node Information
	fmt.Println(T("cli.status.node_information_heading"))
	if s.NodeID == "" || s.NodeID == "standalone" {
		fmt.Println(T("cli.status.node_standalone"))
	} else {
		fmt.Printf(T("cli.status.node")+"\n", s.NodeID)
		hostname, _ := os.Hostname()
		if hostname != "" {
			fmt.Printf(T("cli.status.hostname")+"\n", hostname)
		}
	}
	fmt.Println()

	// Cluster Information (per TEMPLATE.md PART 6)
	fmt.Println(T("cli.status.cluster_heading"))
	if s.ClusterMode {
		fmt.Println(T("cli.status.cluster_mode_cluster"))
		fmt.Printf(T("cli.status.cluster_status")+"\n", s.ClusterStatus)
		fmt.Printf(T("cli.status.cluster_nodes")+"\n", s.ClusterNodes)
		if s.DatabaseInfo != "" {
			fmt.Printf(T("cli.status.database")+"\n", s.DatabaseInfo)
		}
	} else {
		fmt.Println(T("cli.status.cluster_mode_disabled"))
	}
	fmt.Println()

	// Tor Hidden Service (per TEMPLATE.md PART 6)
	fmt.Println(T("cli.status.tor_heading"))
	if s.TorEnabled {
		if s.TorConnected {
			fmt.Println(T("cli.status.tor_connected"))
			if s.TorAddress != "" {
				fmt.Printf(T("cli.status.tor_address")+"\n", s.TorAddress)
			}
		} else {
			fmt.Println(T("cli.status.tor_disconnected"))
		}
	} else {
		fmt.Println(T("cli.status.tor_disabled"))
	}
	fmt.Println()

	return nil
}

// formatDuration formats a duration in human-readable format
// Example: "2d 5h 30m" instead of "53h30m0s"
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// ShowStatusNotRunning shows status when server is not running
// This allows --status to work even when server is down
func ShowStatusNotRunning() {
	fmt.Println(T("cli.status.title"))
	fmt.Println()
	fmt.Println(T("cli.status.stopped_summary"))
	fmt.Println(T("cli.status.node_standalone_summary"))
	fmt.Println(T("cli.status.cluster_disabled_summary"))
	fmt.Println(T("cli.status.tor_unknown_summary"))
	fmt.Println()
	fmt.Println(T("cli.status.start_hint"))
	fmt.Println()
}
