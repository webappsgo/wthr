package cli

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/webappsgo/wthr/src/common/display"
	"github.com/webappsgo/wthr/src/database"
	projectpath "github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/util"
	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
)

// BackupManifest represents metadata for a backup archive per AI.md PART 22
type BackupManifest struct {
	Version       string    `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
	AppVersion    string    `json:"app_version"`
	ServerDB      bool      `json:"server_db"`
	UsersDB       bool      `json:"users_db"`
	ConfigFiles   bool      `json:"config_files"`
	LogsIncluded  bool      `json:"logs_included"`
	GeoIPIncluded bool      `json:"geoip_included"`
	Encrypted     bool      `json:"encrypted"`
	// "AES-256-GCM"
	EncryptionMethod string `json:"encryption_method,omitempty"`
	// sha256:...
	Checksum string `json:"checksum"`
}

// MaintenanceCommand handles maintenance operations per AI.md PART 22
func MaintenanceCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no maintenance command specified. Use: backup, restore, verify, admin-recovery")
	}

	cmd := args[0]
	remainingArgs := args[1:]

	switch cmd {
	case "--help", "help":
		printMaintenanceHelp()
		return nil

	case "backup":
		// Per AI.md PART 22 (Backup & Restore)
		return MaintenanceBackupCommand(remainingArgs)

	case "restore":
		// Per AI.md PART 22 (Backup & Restore)
		return MaintenanceRestoreCommand(remainingArgs)

	case "verify":
		// AI.md PART 22: Verify system integrity
		return verifySystem()

	case "admin-recovery", "setup":
		// AI.md PART 8: Setup token authentication for authorized setup operations
		// AI.md PART 22: sole recovery path for a lost admin password/token
		token := ""
		for _, arg := range remainingArgs {
			if strings.HasPrefix(arg, "--token=") {
				token = strings.TrimPrefix(arg, "--token=")
				break
			}
		}
		return adminRecoverySetup(token)

	case "update":
		return UpdateCommand([]string{"yes"})

	case "mode":
		if len(remainingArgs) < 1 {
			return fmt.Errorf("mode requires a value: production or development")
		}
		return setMaintenanceMode(remainingArgs[0])

	default:
		return fmt.Errorf("unknown maintenance command: %s", cmd)
	}
}

func printMaintenanceHelp() {
	projectName := filepath.Base(os.Args[0])
	fmt.Printf("%s\n", "Maintenance commands:")
	fmt.Println("backup [file]                         - Create backup of all data")
	fmt.Printf("                                        Default: %s/%s-{timestamp}.tar.gz\n", projectpath.GetBackupDir(), projectName)
	fmt.Println("restore <file>                        - Restore from backup file")
	fmt.Println("                                        Stops server, restores data, restarts server")
	fmt.Println("update [cmd]                          - Manage updates")
	fmt.Println("                                        check         - Check for available updates")
	fmt.Println("                                        yes           - Download and install update")
	fmt.Println("                                        branch <name> - Switch update branch (stable|beta|devel)")
	fmt.Println("mode <mode>                           - Set application mode")
	fmt.Println("                                        production    - Normal operation (default)")
	fmt.Println("                                        development   - Relaxed dev defaults (does NOT enable debug; use --debug/DEBUG=true)")
	fmt.Println("setup                                 - Run interactive setup wizard")
	fmt.Println("                                        Creates primary Server Admin, configures server")
	fmt.Println("\nExamples:")
	fmt.Printf("  %s --maintenance backup\n", projectName)
	fmt.Printf("  %s --maintenance backup /path/to/backup.tar.gz\n", projectName)
	fmt.Printf("  %s --maintenance restore /path/to/backup.tar.gz\n", projectName)
	fmt.Printf("  %s --maintenance update check\n", projectName)
	fmt.Printf("  %s --maintenance update yes\n", projectName)
	fmt.Printf("  %s --maintenance mode development\n", projectName)
	fmt.Printf("  %s --maintenance setup\n", projectName)
}

// splitKey splits "section.sub.key" and returns ["section", "sub", "key"]
func splitKey(key string) []string {
	var parts []string
	start := 0
	for i, c := range key {
		if c == '.' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}

// yamlQuote wraps a value in double-quotes when it contains special YAML characters.
func yamlQuote(v string) string {
	for _, c := range v {
		if c == ':' || c == '#' || c == '"' || c == '\'' || c == '\n' || c == '{' || c == '}' || c == '[' || c == ']' {
			return fmt.Sprintf("%q", v)
		}
	}
	return v
}

// setMaintenanceMode sets the application mode by rewriting the MODE line in server.yml.
func setMaintenanceMode(mode string) error {
	switch mode {
	case "prod", "production":
		mode = "production"
	case "dev", "development":
		mode = "development"
	default:
		return fmt.Errorf("invalid mode: %s (use production or development)", mode)
	}

	fmt.Printf(T("cli.maintenance.setting_mode")+"\n", mode)

	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "/etc/webappsgo/wthr"
	}
	configPath := filepath.Join(configDir, "server.yml")

	var existing []byte
	if data, err := os.ReadFile(configPath); err == nil {
		existing = data
	}

	// Rewrite or append the server.mode line
	updated := updateYAMLKey(string(existing), "server.mode", mode)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(updated), 0640); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	fmt.Printf("\n"+T("cli.maintenance.mode_set")+"\n", display.Emoji("✓", "[OK]"), mode, configPath)
	fmt.Println(T("cli.maintenance.restart_hint"))
	return nil
}

// updateYAMLKey updates a dotted key (e.g. "server.mode") in a YAML string,
// replacing the existing line or appending under the correct section.
func updateYAMLKey(yaml, key, value string) string {
	parts := splitKey(key)
	if len(parts) < 2 {
		return yaml
	}
	section := parts[0]
	subKey := strings.Join(parts[1:], ".")
	newLine := fmt.Sprintf("  %s: %s", subKey, yamlQuote(value))

	lines := strings.Split(yaml, "\n")
	inSection := false
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, section+":") && !strings.HasPrefix(line, " ") {
			inSection = true
			continue
		}
		if inSection && !strings.HasPrefix(line, " ") && trimmed != "" {
			inSection = false
		}
		if inSection && strings.HasPrefix(strings.TrimSpace(line), subKey+":") {
			lines[i] = newLine
			replaced = true
		}
	}

	if replaced {
		return strings.Join(lines, "\n")
	}

	// Section or key not found — append
	var out strings.Builder
	out.WriteString(strings.TrimRight(yaml, "\n"))
	out.WriteString("\n\n" + section + ":\n" + newLine + "\n")
	return out.String()
}

// adminRecoverySetup allows recovery of admin access after restore or lockout
func adminRecoverySetup(setupToken string) (retErr error) {
	fmt.Printf(T("cli.maintenance.admin_recovery_title")+"\n", display.Emoji("🔧", "*"))
	fmt.Println(T("cli.maintenance.admin_recovery_description"))
	fmt.Println()

	// Get directory paths
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/webappsgo/wthr"
	}
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "/etc/webappsgo/wthr"
	}

	// Server Admin accounts live in users.db (AI.md PART 12)
	usersDBPath := filepath.Join(dataDir, "db", "users.db")

	// Check if database exists
	if _, err := os.Stat(usersDBPath); os.IsNotExist(err) {
		return fmt.Errorf("users database not found at %s", usersDBPath)
	}

	store, err := openDatabase(usersDBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, store.Close()) }()
	db := store

	// Check if database is empty (first-run)
	var adminCount int
	err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM admins").Scan(&adminCount)
	if err != nil {
		return fmt.Errorf("failed to check admin accounts: %w", err)
	}

	// AI.md PART 8: Setup authorization flow
	// Check authorization in order:
	// 1. Is database empty (no admins exist)?
	// 2. Is user root/admin?
	// 3. Is valid setup token provided (--token=XXX)?
	// 4. NO authorization → Reject with helpful error

	isFirstRun := adminCount == 0
	isRoot := os.Geteuid() == 0
	hasValidToken := false

	if setupToken != "" {
		// Validate the provided token
		valid, err := util.ValidateSetupToken(configDir, setupToken)
		if err != nil {
			fmt.Printf("\n"+T("cli.maintenance.invalid_token")+"\n", display.Emoji("❌", "[FAIL]"))
			fmt.Println(T("cli.maintenance.token_error"), err.Error())
			return fmt.Errorf("invalid setup token")
		}
		hasValidToken = valid
	}

	// Determine authorization status
	if !isFirstRun && !isRoot && !hasValidToken {
		// No authorization
		fmt.Printf("\n"+T("cli.maintenance.setup_denied")+"\n", display.Emoji("❌", "[FAIL]"))
		fmt.Println("\nSetup already completed. To reconfigure:")
		fmt.Println("1. Use existing admin credentials via WebUI")
		fmt.Println("2. Run as root: sudo wthr --maintenance setup")
		fmt.Println("3. Use setup token shown at first-run (if you saved it)")
		return fmt.Errorf("setup authorization required")
	}

	// Handle first-run: generate and display setup token
	if isFirstRun {
		token, err := util.GenerateSetupToken()
		if err != nil {
			return fmt.Errorf("failed to generate setup token: %w", err)
		}

		if err := util.SaveSetupToken(configDir, token); err != nil {
			return fmt.Errorf("failed to save setup token: %w", err)
		}

		fmt.Printf(T("cli.maintenance.first_run_detected")+"\n", display.Emoji("🔑", "[KEY]"))
		fmt.Println("\n" + T("cli.maintenance.setup_token_instructions"))
		fmt.Printf("\n  %s\n\n", token)
		fmt.Println(T("cli.maintenance.setup_token_warning"))
		fmt.Println("\n" + T("cli.maintenance.setup_token_note") + "\n")

		return nil
	}

	// Authorized recovery (root, or a valid setup token): confirm when running as
	// root without a token, then clear the admin credentials and issue a new token
	if isRoot && !hasValidToken {
		fmt.Printf(T("cli.maintenance.root_recovery_title")+"\n\n", display.Emoji("🔐", "[AUTH]"))
		fmt.Print(T("cli.maintenance.confirm_token_regen"))
		var confirm string
		_, _ = fmt.Scanln(&confirm)

		if !strings.EqualFold(strings.TrimSpace(confirm), "yes") && !strings.EqualFold(strings.TrimSpace(confirm), "y") {
			fmt.Println(T("cli.maintenance.operation_cancelled"))
			return nil
		}
	} else {
		fmt.Printf("\n"+T("cli.maintenance.setup_authorized")+"\n", display.Emoji("✓", "[OK]"))
		fmt.Println(T("cli.maintenance.token_accepted"))
	}

	// AI.md PART 22: clear the admin password, API token and passkeys; user data is untouched
	if err := clearAdminCredentials(db, filepath.Join(dataDir, "db", "server.db")); err != nil {
		return fmt.Errorf("failed to clear admin credentials: %w", err)
	}
	fmt.Printf("\n"+T("cli.maintenance.admin_credentials_cleared")+"\n", display.Emoji("🔑", "[KEY]"))

	token, err := util.GenerateSetupToken()
	if err != nil {
		return fmt.Errorf("failed to generate setup token: %w", err)
	}
	if err := util.SaveSetupToken(configDir, token); err != nil {
		return fmt.Errorf("failed to save setup token: %w", err)
	}

	fmt.Printf("\n"+T("cli.maintenance.new_setup_token")+"\n", display.Emoji("🔑", "[KEY]"))
	fmt.Printf("\n  %s\n\n", token)
	fmt.Println(T("cli.maintenance.setup_token_warning"))

	return nil
}

// clearAdminCredentials removes every Server Admin account together with its API
// tokens, passkeys, preferences, notifications and login sessions, which resets the
// installation to the setup wizard (AI.md PART 22 admin recovery). usersDB is the
// users.db handle; the admin sessions live in server.db at serverDBPath. Regular
// users, their data and the configuration are not touched. The action is written
// to the audit log when server.db is available.
func clearAdminCredentials(usersDB *database.Store, serverDBPath string) error {
	ctx := context.Background()
	if usersDB == nil {
		return errors.New("users database is unavailable")
	}
	if err := usersDB.Write(ctx, func(tx *sql.Tx) error {
		for _, statement := range []string{
			"DELETE FROM tokens WHERE owner_type = 'admin'",
			"DELETE FROM passkeys WHERE user_type = 'admin'",
			"DELETE FROM admins",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	serverStore, err := openDatabase(serverDBPath)
	if err != nil {
		return nil
	}
	defer func() { _ = serverStore.Close() }()
	serverDB := serverStore
	if _, err := database.ExecContext(ctx, serverDB, database.TimeoutWrite, "DELETE FROM admin_sessions"); err != nil {
		return err
	}
	// The audit entry is best effort: recovery must still succeed on a damaged log
	_, _ = database.ExecContext(ctx, serverDB, database.TimeoutWrite, `
		INSERT INTO audit_log (ulid, actor_type, actor_id, action, resource_type, source, status)
		VALUES (?, 'system', 'console', 'maintenance.setup', 'admins', 'cli', 'success')
	`, ulid.Make().String())
	return nil
}

// openDatabase opens a SQLite database connection
func openDatabase(dbPath string) (*database.Store, error) {
	// Driver name must be "sqlite" (modernc.org/sqlite, the mandated CGO-free
	// driver per AI.md PART 3) — "sqlite3" is only registered by the CGO
	// mattn/go-sqlite3 driver, which this project never imports.
	store, err := database.OpenSQLite(dbPath, 1)
	if err != nil {
		return nil, err
	}
	// Test connection with a timeout per AI.md PART 10 (Query Timeouts)
	if err := store.Probe(context.Background()); err != nil {
		return nil, errors.Join(err, store.Close())
	}

	return store, nil
}

// hashPasswordArgon2id hashes a password using Argon2id
// AI.md PART 3: Argon2id parameters (time=3, memory=64*1024, threads=4, keyLen=32)
func hashPasswordArgon2id(password string) (string, error) {
	// Generate random salt (16 bytes)
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Argon2id parameters from AI.md PART 3
	const (
		time = 3
		// 64 MB
		memory  = 64 * 1024
		threads = 4
		keyLen  = 32
	)

	// Generate hash
	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)

	// Encode as: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, time, threads, b64Salt, b64Hash), nil
}

// verifySystem verifies system integrity per AI.md PART 22
func verifySystem() error {
	fmt.Printf(T("cli.maintenance.verify_title")+"\n", display.Emoji("🔍", "[..]"))
	fmt.Println()

	// Get directory paths
	dataDir := os.Getenv("DATA_DIR")
	configDir := os.Getenv("CONFIG_DIR")
	logDir := os.Getenv("LOG_DIR")

	// Default paths if not set
	if dataDir == "" {
		dataDir = "/var/lib/webappsgo/wthr"
	}
	if configDir == "" {
		configDir = "/etc/webappsgo/wthr"
	}
	if logDir == "" {
		logDir = "/var/log/webappsgo/wthr"
	}

	errors := 0

	// 1. Verify server.db exists and is accessible
	fmt.Print(T("cli.maintenance.checking_server_db"))
	serverDBPath := filepath.Join(dataDir, "db", "server.db")
	if err := verifyDatabaseFile(serverDBPath); err != nil {
		fmt.Printf(T("cli.maintenance.check_fail")+"\n", display.Emoji("❌", "[FAIL]"), err)
		errors++
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// 2. Verify users.db exists and is accessible
	fmt.Print(T("cli.maintenance.checking_users_db"))
	usersDBPath := filepath.Join(dataDir, "db", "users.db")
	if err := verifyDatabaseFile(usersDBPath); err != nil {
		fmt.Printf(T("cli.maintenance.check_fail")+"\n", display.Emoji("❌", "[FAIL]"), err)
		errors++
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// 3. Verify server.yml exists
	fmt.Print(T("cli.maintenance.checking_server_yml"))
	configPath := filepath.Join(configDir, "server.yml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf(T("cli.maintenance.config_not_found")+"\n", display.Emoji("⚠️", "WARNING:"))
	} else if err != nil {
		fmt.Printf(T("cli.maintenance.check_fail")+"\n", display.Emoji("❌", "[FAIL]"), err)
		errors++
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// 4. Verify log directory is writable
	fmt.Print(T("cli.maintenance.checking_log_dir"))
	if err := verifyDirectoryWritable(logDir); err != nil {
		fmt.Printf(T("cli.maintenance.check_fail")+"\n", display.Emoji("❌", "[FAIL]"), err)
		errors++
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// 5. Verify data directory is writable
	fmt.Print(T("cli.maintenance.checking_data_dir"))
	if err := verifyDirectoryWritable(dataDir); err != nil {
		fmt.Printf(T("cli.maintenance.check_fail")+"\n", display.Emoji("❌", "[FAIL]"), err)
		errors++
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// 6. Verify at least one admin account exists
	fmt.Print(T("cli.maintenance.checking_admin_accounts"))
	if err := verifyAdminExists(filepath.Join(dataDir, "db", "users.db")); err != nil {
		fmt.Printf(T("cli.maintenance.admin_check_warning")+"\n", display.Emoji("⚠️", "[!]"), err)
		fmt.Println(T("cli.maintenance.admin_recovery_run_hint"))
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// 7. Check GeoIP databases (optional)
	fmt.Print(T("cli.maintenance.checking_geoip"))
	geoipDir := filepath.Join(dataDir, "security", "geoip")
	if _, err := os.Stat(geoipDir); os.IsNotExist(err) {
		fmt.Printf(T("cli.maintenance.geoip_not_found")+"\n", display.Emoji("⚠️", "WARNING:"))
	} else {
		fmt.Printf(T("cli.maintenance.check_ok")+"\n", display.Emoji("✓", "[OK]"))
	}

	// Summary
	fmt.Println()
	if errors == 0 {
		fmt.Printf(T("cli.maintenance.verify_success")+"\n", display.Emoji("✓", "[OK]"))
		return nil
	}

	fmt.Printf(T("cli.maintenance.verify_failed")+"\n", display.Emoji("❌", "[FAIL]"), errors)
	return fmt.Errorf("verification failed")
}

// verifyDatabaseFile verifies a database file exists and is accessible
func verifyDatabaseFile(dbPath string) (retErr error) {
	// Check if file exists
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("database not found")
	}
	if err != nil {
		return fmt.Errorf("cannot access: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}

	// Try to open it
	store, err := database.OpenSQLite(dbPath, 1)
	if err != nil {
		return fmt.Errorf("cannot open: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, store.Close()) }()
	// Try to ping, with a timeout per AI.md PART 10 (Query Timeouts)
	if err := store.Probe(context.Background()); err != nil {
		return fmt.Errorf("database corrupted: %w", err)
	}

	return nil
}

// verifyDirectoryWritable verifies a directory exists and is writable
func verifyDirectoryWritable(dirPath string) error {
	// Check if directory exists
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("cannot create: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot access: %w", err)
	}

	// Check if it's a directory
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}

	// Create a unique probe file so checking writability cannot truncate an
	// existing file in the target directory.
	testFile, err := os.CreateTemp(dirPath, ".write-test-*")
	if err != nil {
		return fmt.Errorf("not writable: %w", err)
	}
	if err := testFile.Close(); err != nil {
		_ = os.Remove(testFile.Name())
		return fmt.Errorf("failed to close write test file: %w", err)
	}
	if err := os.Remove(testFile.Name()); err != nil {
		return fmt.Errorf("failed to remove write test file: %w", err)
	}

	return nil
}

// verifyAdminExists checks if at least one admin account exists
func verifyAdminExists(usersDBPath string) (retErr error) {
	store, err := database.OpenSQLite(usersDBPath, 1)
	if err != nil {
		return fmt.Errorf("cannot open database: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, store.Close()) }()
	var count int
	err = database.QueryRowContext(context.Background(), store, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM admins").Scan(&count)
	if err != nil {
		return fmt.Errorf("cannot query: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("no admin accounts found")
	}

	return nil
}
