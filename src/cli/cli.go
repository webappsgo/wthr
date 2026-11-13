package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	wpath "github.com/webappsgo/wthr/src/path"
)

// Version information (set via ldflags during build per AI.md PART 6)
var (
	Version   = "dev"
	BuildDate = "unknown"
	CommitID  = "unknown"
	// CGOEnabled is set at build time to show CGO_ENABLED status
	// Default to 0 (static binary requirement)
	CGOEnabled = "0"
)

// Command represents a CLI command
type Command struct {
	Name        string
	Description string
	// Requires root/admin
	Privileged bool
	Handler    func(args []string) error
}

// CLI manages command-line interface
type CLI struct {
	commands map[string]*Command
	flags    *flag.FlagSet
}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	return &CLI{
		commands: make(map[string]*Command),
		// AI.md PART 8: usage/error output must carry the actual (possibly
		// renamed) binary name, never a hardcoded project name
		flags: flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ExitOnError),
	}
}

// RegisterCommand registers a new CLI command
func (c *CLI) RegisterCommand(cmd *Command) {
	c.commands[cmd.Name] = cmd
}

// Parse parses command line arguments
func (c *CLI) Parse(args []string) error {
	_ = os.Unsetenv("CLI_COMMAND_HANDLED")
	// Preprocess args to handle short flags -h and -v (AI.md line 150)
	// Only -h (help) and -v (version) may have short flags
	preprocessedArgs := make([]string, len(args))
	copy(preprocessedArgs, args)
	preprocessedArgs = normalizeUpdateFlagArgs(preprocessedArgs)
	for i, arg := range preprocessedArgs {
		if arg == "-h" {
			preprocessedArgs[i] = "--help"
		} else if arg == "-v" {
			preprocessedArgs[i] = "--version"
		}
	}

	// AI.md PART 8 "Command-Flag Help": help for a command flag is routed before the main parse
	if name, ok := commandHelp(preprocessedArgs, "shell", "service", "maintenance", "update"); ok {
		c.printCommandHelp(name)
		os.Exit(0)
	}

	// Define standard flags (AI.md PART 8)
	var (
		showHelp       = c.flags.Bool("help", false, "Show this help message")
		showVersion    = c.flags.Bool("version", false, "Show version information")
		showStatus     = c.flags.Bool("status", false, "Show server status and health")
		debug          = c.flags.Bool("debug", false, "Enable debug mode (verbose logging, debug endpoints)")
		i2p            = c.flags.Bool("i2p", false, "Enable the I2P eepsite (opt-in)")
		colorMode      = c.flags.String("color", "auto", "Color output: auto, yes, no (default: auto, respects NO_COLOR)")
		lang           = c.flags.String("lang", "", "Language for output: auto or a supported code such as en, es, fr, de, it, ar, zh, ja (default: auto)")
		mode           = c.flags.String("mode", "", "Application mode: production or development (aliases: prod, dev, devel, debug)")
		configDir      = c.flags.String("config", "", "Configuration directory")
		dataDir        = c.flags.String("data", "", "Data directory")
		cacheDir       = c.flags.String("cache", "", "Cache directory")
		logDir         = c.flags.String("log", "", "Log directory")
		backupDir      = c.flags.String("backup", "", "Backup directory")
		pidFile        = c.flags.String("pid", "", "PID file path")
		address        = c.flags.String("address", "", "Listen address")
		port           = c.flags.String("port", "", "Listen port (env: PORT)")
		baseURL        = c.flags.String("baseurl", "", "URL path prefix (default: /)")
		daemon         = c.flags.Bool("daemon", false, "Daemonize (detach from terminal, Unix only)")
		serviceCmd     = c.flags.String("service", "", "Service management: start, stop, restart, reload, --install, --uninstall")
		maintenanceCmd = c.flags.String("maintenance", "", "Maintenance: backup, restore, update, mode, setup")
		updateCmd      = c.flags.String("update", "", "Update: check, yes, branch {stable|beta|devel}")
		shellCmd       = c.flags.String("shell", "", "Shell integration: completions, init, --help")
	)

	if err := c.flags.Parse(preprocessedArgs); err != nil {
		return err
	}
	// AI.md PART 5: directory overrides are real filesystem paths, validated with SafeDirPath
	// (absolute paths preserved; control characters and ".." segments rejected)
	for _, dir := range []struct {
		flagName string
		value    *string
	}{{"config", configDir}, {"data", dataDir}, {"cache", cacheDir}, {"log", logDir}, {"backup", backupDir}} {
		// --help and --version always exit immediately and never fail on other flags
		if *dir.value == "" || *showHelp || *showVersion {
			continue
		}
		safe, err := wpath.SafeDirPath(*dir.value)
		if err != nil {
			return fmt.Errorf("invalid --%s value: %w", dir.flagName, err)
		}
		*dir.value = safe
	}

	if *configDir != "" {
		os.Setenv("CONFIG_DIR", *configDir)
	}

	// Handle help flag
	if *showHelp {
		c.ShowHelp()
		os.Exit(0)
	}

	// Handle version flag
	if *showVersion {
		c.ShowVersion()
		os.Exit(0)
	}

	// Handle status flag - store for main.go to check
	if *showStatus {
		os.Setenv("CLI_STATUS_FLAG", "1")
		return nil
	}

	// Handle shell command (AI.md PART 8: Shell integration)
	// Phase 1 immediate-exit flag - dispatched before the service, maintenance
	// and update subcommand phases
	if *shellCmd != "" {
		if err := c.handleShellCommand(*shellCmd, c.flags.Args()); err != nil {
			return err
		}
		_ = os.Setenv("CLI_COMMAND_HANDLED", "1")
		return nil
	}

	// Handle service command
	if *serviceCmd != "" {
		if cmd, ok := c.commands["service"]; ok {
			if err := cmd.Handler([]string{*serviceCmd}); err != nil {
				return err
			}
			_ = os.Setenv("CLI_COMMAND_HANDLED", "1")
			return nil
		}
		return fmt.Errorf("service command not registered")
	}

	// Handle maintenance command
	if *maintenanceCmd != "" {
		if cmd, ok := c.commands["maintenance"]; ok {
			if err := cmd.Handler(commandWithArgs(*maintenanceCmd, c.flags.Args())); err != nil {
				return err
			}
			_ = os.Setenv("CLI_COMMAND_HANDLED", "1")
			return nil
		}
		return fmt.Errorf("maintenance command not registered")
	}

	// Handle update command
	if *updateCmd != "" {
		if cmd, ok := c.commands["update"]; ok {
			if err := cmd.Handler(commandWithArgs(*updateCmd, c.flags.Args())); err != nil {
				return err
			}
			_ = os.Setenv("CLI_COMMAND_HANDLED", "1")
			return nil
		}
		return fmt.Errorf("update command not registered")
	}

	// Store flags for later access (AI.md PART 5: Environment Variables)
	if *mode != "" {
		os.Setenv("MODE", *mode)
	}
	if *debug {
		os.Setenv("DEBUG", "true")
	}
	// AI.md PART 8: --port falls back to the PORT env var, so precedence is flag > PORT
	if *port != "" {
		os.Setenv("PORT", *port)
	}
	if *i2p {
		os.Setenv("I2P_ENABLED", "true")
	}
	if *address != "" {
		os.Setenv("LISTEN", *address)
	}
	if *configDir != "" {
		os.Setenv("CONFIG_DIR", *configDir)
	}
	if *dataDir != "" {
		os.Setenv("DATA_DIR", *dataDir)
	}
	if *cacheDir != "" {
		os.Setenv("CACHE_DIR", *cacheDir)
	}
	if *logDir != "" {
		os.Setenv("LOG_DIR", *logDir)
	}
	if *backupDir != "" {
		os.Setenv("BACKUP_DIR", *backupDir)
	}
	if *pidFile != "" {
		os.Setenv("PID_FILE", *pidFile)
	}
	if *daemon {
		os.Setenv("DAEMON", "true")
	}
	if *baseURL != "" {
		os.Setenv("BASE_URL", *baseURL)
	}
	// Handle --color flag (AI.md PART 8: NO_COLOR support)
	if *colorMode != "" {
		os.Setenv("CLI_COLOR_MODE", *colorMode)
	}
	// Handle --lang flag (AI.md PART 31: highest priority language source)
	if *lang != "" {
		os.Setenv("CLI_LANG", *lang)
	}

	return nil
}

func normalizeUpdateFlagArgs(args []string) []string {
	normalized := make([]string, 0, len(args))
	for index, argument := range args {
		if argument == "--update" && (index+1 == len(args) || strings.HasPrefix(args[index+1], "-") && args[index+1] != "--help") {
			normalized = append(normalized, "--update=yes")
			continue
		}
		normalized = append(normalized, argument)
	}
	return normalized
}

func commandWithArgs(command string, args []string) []string {
	result := make([]string, 1, len(args)+1)
	result[0] = command
	return append(result, args...)
}

// commandHelp returns the command flag whose help was requested, matching both
// "--shell --help" and "--shell help"; it runs before the flag set parses so the
// command flag cannot consume the help word as its value.
func commandHelp(args []string, commands ...string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		next := args[i+1]
		if next != "--help" && next != "-h" && next != "help" {
			continue
		}
		for _, command := range commands {
			if args[i] == "--"+command {
				return command, true
			}
		}
	}
	return "", false
}

// printCommandHelp prints the help for one command flag
func (c *CLI) printCommandHelp(name string) {
	switch name {
	case "shell":
		c.showShellHelp(filepath.Base(os.Args[0]))
	case "service":
		showServiceHelp()
	case "maintenance":
		printMaintenanceHelp()
	case "update":
		printUpdateHelp()
	}
}

// ShowHelp displays help information per AI.md PART 8 spec
// AI.md PART 8: Must show actual binary name (if renamed)
func (c *CLI) ShowHelp() {
	binaryName := filepath.Base(os.Args[0])

	// AI.md PART 8 line 8558: First line format
	fmt.Printf(T("cli.help.title")+"\n", binaryName, Version)
	fmt.Println()

	// Usage section
	fmt.Println(T("cli.help.usage_heading"))
	fmt.Printf(T("cli.help.usage_line")+"\n", binaryName)
	fmt.Println()

	// Information section
	fmt.Println(T("cli.help.information_heading"))
	fmt.Println(T("cli.help.flag_help"))
	fmt.Println(T("cli.help.flag_version"))
	fmt.Println(T("cli.help.flag_status"))
	fmt.Println()

	// Shell Integration section
	fmt.Println(T("cli.help.shell_integration_heading"))
	fmt.Println(T("cli.help.flag_shell_completions"))
	fmt.Println(T("cli.help.flag_shell_init"))
	fmt.Println(T("cli.help.flag_shell_help"))
	fmt.Println()

	// Server Configuration section
	fmt.Println(T("cli.help.server_configuration_heading"))
	fmt.Println(T("cli.help.flag_mode"))
	fmt.Println(T("cli.help.flag_config"))
	fmt.Println(T("cli.help.flag_data"))
	fmt.Println(T("cli.help.flag_cache"))
	fmt.Println(T("cli.help.flag_log"))
	fmt.Println(T("cli.help.flag_backup"))
	fmt.Println(T("cli.help.flag_pid"))
	fmt.Println(T("cli.help.flag_address"))
	fmt.Println(T("cli.help.flag_port"))
	fmt.Println(T("cli.help.flag_baseurl"))
	fmt.Println(T("cli.help.flag_daemon"))
	fmt.Println(T("cli.help.flag_debug"))
	fmt.Println(T("cli.help.flag_i2p"))
	fmt.Println(T("cli.help.flag_color"))
	fmt.Println(T("cli.help.flag_lang"))
	fmt.Println()

	// Service Management section
	fmt.Println(T("cli.help.service_management_heading"))
	fmt.Println(T("cli.help.flag_service"))
	fmt.Println(T("cli.help.flag_maintenance"))
	fmt.Println(T("cli.help.flag_update"))
	fmt.Println()

	// Final line per spec
	fmt.Printf(T("cli.help.footer")+"\n", binaryName)
}

// shortCommit returns the first 7 characters of a commit id (AI.md PART 8 "--version Output")
func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// ShowVersion displays version information per AI.md PART 8 "--version Output"
// AI.md: Must show actual binary name (if renamed)
// Format per spec:
//
//	{binaryName} {version} ({short_commit})
//	Built: {timestamp}
//	Go: {go_version}
//	OS/Arch: {os}/{arch}
func (c *CLI) ShowVersion() {
	binaryName := filepath.Base(os.Args[0])
	fmt.Printf(T("cli.version.name_version")+"\n", binaryName, Version, shortCommit(CommitID))
	fmt.Printf(T("cli.version.built")+"\n", BuildDate)
	fmt.Printf(T("cli.version.go")+"\n", runtime.Version())
	fmt.Printf(T("cli.version.os_arch")+"\n", runtime.GOOS, runtime.GOARCH)
}

// GetFlag returns a flag value
func (c *CLI) GetFlag(name string) interface{} {
	return c.flags.Lookup(name)
}

// IsFlagSet checks if a flag was set
func (c *CLI) IsFlagSet(name string) bool {
	found := false
	c.flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
