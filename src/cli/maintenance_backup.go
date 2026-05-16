// Package cli - maintenance command with backup/restore integration
// Per AI.md PART 25 lines 22351-22649
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/apimgr/weather/src/backup"
	"github.com/apimgr/weather/src/path"
	"golang.org/x/term"
)

// MaintenanceBackupCommand handles backup creation per AI.md PART 25 lines 22351-22467
func MaintenanceBackupCommand(args []string) error {
	// Parse flags
	var backupFile string
	var password string
	var includeSSL bool
	var includeData bool

	// Parse args for flags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--password":
			if i+1 < len(args) {
				password = args[i+1]
				i++
			}
		case "--include-ssl":
			includeSSL = true
		case "--include-data":
			includeData = true
		default:
			if backupFile == "" && !strings.HasPrefix(args[i], "--") {
				backupFile = args[i]
			}
		}
	}

	// Get paths per AI.md PART 4
	p := paths.GetDefaultPaths("weather")
	if p == nil {
		return fmt.Errorf("failed to get default paths")
	}

	// Override with environment variables if set
	if configDir := os.Getenv("CONFIG_DIR"); configDir != "" {
		p.ConfigDir = configDir
	}
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		p.DataDir = dataDir
	}

	// Create backup service
	svc := backup.New(p.ConfigDir, p.DataDir)

	// Prompt for password if encryption enabled and no password provided
	// Per AI.md PART 25 line 22457: "Prompts for password"
	if password == "" {
		// Check if encryption is enabled in config
		// For now, always prompt to allow encrypted backups
		fmt.Print("Backup encryption password (leave empty for unencrypted): ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = string(passwordBytes)
	}

	fmt.Println("🔄 Creating backup...")
	fmt.Println()

	// Create backup per AI.md PART 25
	opts := backup.BackupOptions{
		ConfigDir:   p.ConfigDir,
		DataDir:     p.DataDir,
		OutputPath:  backupFile,
		Password:    password,
		IncludeSSL:  includeSSL,
		IncludeData: includeData,
		CreatedBy:   "cli",
		AppVersion:  Version,
	}

	backupPath, err := svc.Create(opts)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Backup completed successfully!")
	fmt.Printf("📦 Backup file: %s\n", backupPath)

	// Show file size
	if info, err := os.Stat(backupPath); err == nil {
		size := float64(info.Size()) / 1024 / 1024
		fmt.Printf("📊 Size: %.2f MB\n", size)
	}

	if password != "" {
		fmt.Println("🔒 Backup is encrypted")
		fmt.Println()
		fmt.Println("⚠️  IMPORTANT: Save your encryption password securely!")
		fmt.Println("   Without the password, this backup cannot be restored.")
	}

	return nil
}

// MaintenanceRestoreCommand handles backup restoration per AI.md PART 25 lines 22588-22649
func MaintenanceRestoreCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("restore requires a backup file path")
	}

	backupFile := args[0]
	var password string

	// Parse additional flags
	for i := 1; i < len(args); i++ {
		if args[i] == "--password" && i+1 < len(args) {
			password = args[i+1]
			i++
		}
	}

	// Check if backup file exists
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupFile)
	}

	// Check if encrypted (has .enc extension)
	if filepath.Ext(backupFile) == ".enc" && password == "" {
		// Prompt for password per AI.md PART 25 line 22464
		fmt.Print("Backup encryption password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = string(passwordBytes)
	}

	// Get paths per AI.md PART 4
	p := paths.GetDefaultPaths("weather")
	if p == nil {
		return fmt.Errorf("failed to get default paths")
	}

	// Override with environment variables
	if configDir := os.Getenv("CONFIG_DIR"); configDir != "" {
		p.ConfigDir = configDir
	}
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		p.DataDir = dataDir
	}

	// Confirm restore operation
	fmt.Println("⚠️  WARNING: This will overwrite current configuration and databases!")
	fmt.Print("Are you sure you want to restore? (yes/no): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "yes" {
		fmt.Println("Restore cancelled.")
		return nil
	}

	fmt.Println()
	fmt.Println("🔄 Restoring backup...")
	fmt.Println()

	// Create backup service
	svc := backup.New(p.ConfigDir, p.DataDir)

	// Restore backup per AI.md PART 25
	opts := backup.RestoreOptions{
		BackupPath: backupFile,
		Password:   password,
		ConfigDir:  p.ConfigDir,
		DataDir:    p.DataDir,
		Force:      false,
	}

	if err := svc.Restore(opts); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Restore completed successfully!")
	fmt.Println()
	fmt.Println("Please restart the weather service for changes to take effect:")
	fmt.Println("  systemctl restart weather")
	fmt.Println("  or")
	fmt.Println("  weather --service restart")

	return nil
}
