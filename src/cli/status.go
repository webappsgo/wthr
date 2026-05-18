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
	fmt.Println("Weather - Server Status")
	fmt.Println()

	// Server Status
	fmt.Println("Server Status:")
	if s.ServerRunning {
		fmt.Println("  Status:   Running")
		fmt.Printf("  Port:     %d\n", s.Port)
		fmt.Printf("  Mode:     %s\n", s.Mode)
		if !s.StartTime.IsZero() {
			uptime := time.Since(s.StartTime)
			fmt.Printf("  Uptime:   %s\n", formatDuration(uptime))
		}
	} else {
		fmt.Println("  Status:   Stopped")
	}
	fmt.Println()

	// Node Information
	fmt.Println("Node Information:")
	if s.NodeID == "" || s.NodeID == "standalone" {
		fmt.Println("  Node:     standalone")
	} else {
		fmt.Printf("  Node:     %s\n", s.NodeID)
		hostname, _ := os.Hostname()
		if hostname != "" {
			fmt.Printf("  Hostname: %s\n", hostname)
		}
	}
	fmt.Println()

	// Cluster Information (per TEMPLATE.md PART 6)
	fmt.Println("Cluster:")
	if s.ClusterMode {
		fmt.Println("  Mode:     cluster")
		fmt.Printf("  Status:   %s\n", s.ClusterStatus)
		fmt.Printf("  Nodes:    %d\n", s.ClusterNodes)
		if s.DatabaseInfo != "" {
			fmt.Printf("  Database: %s\n", s.DatabaseInfo)
		}
	} else {
		fmt.Println("  Mode:     disabled")
	}
	fmt.Println()

	// Tor Hidden Service (per TEMPLATE.md PART 6)
	fmt.Println("Tor Hidden Service:")
	if s.TorEnabled {
		if s.TorConnected {
			fmt.Println("  Status:   Connected")
			if s.TorAddress != "" {
				fmt.Printf("  Address:  %s\n", s.TorAddress)
			}
		} else {
			fmt.Println("  Status:   Disconnected")
		}
	} else {
		fmt.Println("  Status:   Disabled")
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
	fmt.Println("Weather - Server Status")
	fmt.Println()
	fmt.Println("Server Status:  Stopped")
	fmt.Println("Node:           standalone")
	fmt.Println("Cluster:        disabled")
	fmt.Println("Tor:            Unknown (server not running)")
	fmt.Println()
	fmt.Println("Use 'wthr --service start' to start the server")
	fmt.Println()
}
