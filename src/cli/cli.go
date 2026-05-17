package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	Privileged  bool
	Handler     func(args []string) error
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
		flags:    flag.NewFlagSet("wthr", flag.ExitOnError),
	}
}

// RegisterCommand registers a new CLI command
func (c *CLI) RegisterCommand(cmd *Command) {
	c.commands[cmd.Name] = cmd
}

// Parse parses command line arguments
func (c *CLI) Parse(args []string) error {
	// Preprocess args to handle short flags -h and -v (AI.md line 150)
	// Only -h (help) and -v (version) may have short flags
	preprocessedArgs := make([]string, len(args))
	copy(preprocessedArgs, args)
	for i, arg := range preprocessedArgs {
		if arg == "-h" {
			preprocessedArgs[i] = "--help"
		} else if arg == "-v" {
			preprocessedArgs[i] = "--version"
		}
	}

	// Define standard flags (AI.md PART 8)
	var (
		showHelp       = c.flags.Bool("help", false, "Show this help message")
		showVersion    = c.flags.Bool("version", false, "Show version information")
		showStatus     = c.flags.Bool("status", false, "Show server status and health")
		debug          = c.flags.Bool("debug", false, "Enable debug mode (verbose logging, debug endpoints)")
		colorMode      = c.flags.String("color", "", "Color output: always, never, auto (default: auto, respects NO_COLOR)")
		mode           = c.flags.String("mode", "", "Application mode: production or development")
		configDir      = c.flags.String("config", "", "Configuration directory")
		dataDir        = c.flags.String("data", "", "Data directory")
		cacheDir       = c.flags.String("cache", "", "Cache directory")
		logDir         = c.flags.String("log", "", "Log directory")
		backupDir      = c.flags.String("backup", "", "Backup directory")
		pidFile        = c.flags.String("pid", "", "PID file path")
		address        = c.flags.String("address", "", "Listen address")
		port           = c.flags.String("port", "", "Server port (deprecated, use --address)")
		baseURL        = c.flags.String("baseurl", "", "URL path prefix (default: /)")
		daemon         = c.flags.Bool("daemon", false, "Daemonize (detach from terminal, Unix only)")
		serviceCmd     = c.flags.String("service", "", "Service management: start, stop, restart, reload, --install, --uninstall")
		maintenanceCmd = c.flags.String("maintenance", "", "Maintenance: backup, restore, update, mode, setup")
		updateCmd      = c.flags.String("update", "", "Update: check, yes, branch {stable|beta|daily}")
		shellCmd       = c.flags.String("shell", "", "Shell integration: completions, init, --help")
	)

	if err := c.flags.Parse(preprocessedArgs); err != nil {
		return err
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

	// Handle service command
	if *serviceCmd != "" {
		if cmd, ok := c.commands["service"]; ok {
			return cmd.Handler([]string{*serviceCmd})
		}
		return fmt.Errorf("service command not registered")
	}

	// Handle maintenance command
	if *maintenanceCmd != "" {
		if cmd, ok := c.commands["maintenance"]; ok {
			return cmd.Handler(c.flags.Args())
		}
		return fmt.Errorf("maintenance command not registered")
	}

	// Handle update command
	if *updateCmd != "" {
		if cmd, ok := c.commands["update"]; ok {
			return cmd.Handler(c.flags.Args())
		}
		return fmt.Errorf("update command not registered")
	}

	// Handle shell command (AI.md PART 8: Shell integration)
	if *shellCmd != "" {
		return c.handleShellCommand(*shellCmd, c.flags.Args())
	}

	// Store flags for later access (AI.md PART 5: Environment Variables)
	if *mode != "" {
		os.Setenv("MODE", *mode)
	}
	if *debug {
		os.Setenv("DEBUG", "true")
	}
	if *port != "" {
		os.Setenv("PORT", *port)
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

	return nil
}

// ShowHelp displays help information per AI.md PART 8 spec
// AI.md PART 8: Must show actual binary name (if renamed)
func (c *CLI) ShowHelp() {
	binaryName := filepath.Base(os.Args[0])

	// AI.md PART 8 line 8558: First line format
	fmt.Printf("%s %s - Production-grade weather API server\n", binaryName, Version)
	fmt.Println()

	// Usage section
	fmt.Println("Usage:")
	fmt.Printf("  %s [flags]\n", binaryName)
	fmt.Println()

	// Information section
	fmt.Println("Information:")
	fmt.Println("  -h, --help                        Show help (--help for any command shows its help)")
	fmt.Println("  -v, --version                     Show version")
	fmt.Println("      --status                      Show server status and health")
	fmt.Println()

	// Shell Integration section
	fmt.Println("Shell Integration:")
	fmt.Println("      --shell completions [SHELL]   Print shell completions")
	fmt.Println("      --shell init [SHELL]          Print shell init command")
	fmt.Println("      --shell --help                Show shell help")
	fmt.Println()

	// Server Configuration section
	fmt.Println("Server Configuration:")
	fmt.Println("      --mode {production|development}  Application mode (default: production)")
	fmt.Println("      --config DIR                  Config directory")
	fmt.Println("      --data DIR                    Data directory")
	fmt.Println("      --cache DIR                   Cache directory")
	fmt.Println("      --log DIR                     Log directory")
	fmt.Println("      --backup DIR                  Backup directory")
	fmt.Println("      --pid FILE                    PID file path")
	fmt.Println("      --address ADDR                Listen address (default: 0.0.0.0)")
	fmt.Println("      --port PORT                   Listen port (default: random 64xxx, 80 in container)")
	fmt.Println("      --baseurl PATH                URL path prefix (default: /)")
	fmt.Println("      --daemon                      Run as daemon (detach from terminal)")
	fmt.Println("      --debug                       Enable debug mode")
	fmt.Println("      --color {always|never|auto}   Color output (default: auto)")
	fmt.Println()

	// Service Management section
	fmt.Println("Service Management:")
	fmt.Println("      --service CMD                 Service management (--service --help for details)")
	fmt.Println("      --maintenance CMD             Maintenance operations (--maintenance --help for details)")
	fmt.Println("      --update [CMD]                Check/perform updates (--update --help for details)")
	fmt.Println()

	// Final line per spec
	fmt.Printf("Run '%s <command> --help' for detailed help on any command.\n", binaryName)
}

// ShowVersion displays version information per AI.md PART 16 line 13778
// AI.md: Must show actual binary name (if renamed)
// Format per spec:
//
//	{binaryName} v{version}
//	Built: {timestamp}
//	Go: {go_version}
//	OS/Arch: {os}/{arch}
func (c *CLI) ShowVersion() {
	binaryName := filepath.Base(os.Args[0])
	fmt.Printf("%s v%s\n", binaryName, Version)
	fmt.Printf("Built: %s\n", BuildDate)
	fmt.Printf("Go: %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
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
