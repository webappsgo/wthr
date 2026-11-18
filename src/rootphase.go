package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/privilege"
	"github.com/webappsgo/wthr/src/util"
)

// serviceDirectories lists every directory the service account owns after the drop:
// config, data (which holds the database and the backups), cache, log and the
// per-service PID directory (AI.md PART 8 step 8b).
func serviceDirectories(dirPaths *util.DirectoryPaths) []string {
	return []string{
		dirPaths.Config,
		dirPaths.Data,
		dirPaths.Cache,
		dirPaths.Log,
		util.GetDatabaseDir(dirPaths),
		util.GetBackupPath(dirPaths),
		filepath.Dir(util.SystemPIDFilePath()),
	}
}

// startupListenHost returns the host the server binds, resolved the same way the HTTP
// server resolves it later so a pre-bound privileged socket and the late bind agree.
func startupListenHost() string {
	listen := os.Getenv("LISTEN")
	if listen == "" {
		listen = os.Getenv("SERVER_ADDRESS")
	}
	if listen != "" {
		if host, _, err := net.SplitHostPort(listen); err == nil {
			return host
		}
		return listen
	}
	if config.IsTruthy(os.Getenv("REVERSE_PROXY")) {
		return "127.0.0.1"
	}
	return getDefaultListenAddress()
}

// needsPortBinding is false for --status: it only reads the database and must work while
// the server it reports on holds the ports.
func needsPortBinding() bool {
	return os.Getenv("CLI_STATUS_FLAG") != "1"
}

// runRootPhase performs AI.md PART 8 steps 8a-8i when the server starts as root: it
// creates the service account, makes the service directories and everything in them
// owned by that account, binds every privileged port while still root and then drops
// to the account and proves the drop. It does nothing for a regular user, on Windows
// (the service runs as a Virtual Service Account) or when the port needs no root.
func runRootPhase(dirPaths *util.DirectoryPaths, ports util.StartupPorts) {
	if runtime.GOOS == "windows" || !privilege.IsRoot() {
		return
	}

	account, err := privilege.EnsureAccount(privilege.InternalName, dirPaths.Config)
	if err != nil {
		// General startup failure (AI.md PART 8: exit code 1).
		log.Printf("Failed to create the service account: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to create the service account: %v\n", err)
		os.Exit(1)
	}

	pidDir := filepath.Dir(util.SystemPIDFilePath())
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		// Config-directed path unusable (AI.md PART 8: exit code 2).
		log.Printf("Failed to create the PID directory %s: %v", pidDir, err)
		os.Exit(2)
	}

	for _, dir := range serviceDirectories(dirPaths) {
		if err := privilege.ChownTree(dir, account); err != nil {
			log.Printf("Failed to give the service account ownership of %s: %v", dir, err)
			os.Exit(2)
		}
	}

	// Every listen port is offered to the binder: it keeps only those below 1024, and the
	// server wraps the socket in TLS later when that port serves HTTPS
	listenPorts := []int{ports.HTTP}
	if ports.HTTPS > 0 {
		listenPorts = append(listenPorts, ports.HTTPS)
	}
	if needsPortBinding() {
		if err := privilege.BindPrivilegedPorts(startupListenHost(), listenPorts); err != nil {
			// Listener bind failure (AI.md PART 8: exit code 3).
			log.Printf("Failed to bind a privileged port: %v", err)
			fmt.Fprintf(os.Stderr, "Failed to bind a privileged port: %v\n", err)
			os.Exit(3)
		}
	}

	if err := privilege.DropPrivileges(account); err != nil {
		// A server that could not drop root must not keep running as root.
		log.Printf("Failed to drop privileges to %s: %v", account.Name, err)
		fmt.Fprintf(os.Stderr, "Failed to drop privileges to %s: %v\n", account.Name, err)
		os.Exit(1)
	}
}
