package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// OfficialSite is the default server URL (set at build time or default)
// Per AI.md PART 33: Official site is https://wthr.top
var OfficialSite = "https://wthr.top"

// Execute is the main entry point for the CLI
// Per AI.md PART 33: Automatic mode detection, NEVER implement --tui flag
func Execute() error {
	// Get actual binary name for display
	// Per AI.md PART 33 line 9551: Display uses filepath.Base(os.Args[0])
	binaryName := filepath.Base(os.Args[0])

	// Check for exit-immediately flags first (before any parsing)
	// Per AI.md PART 33: -h, --help, -v, --version exit immediately (never TUI)
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "--help":
			printUsage(binaryName)
			return nil
		case "-v", "--version":
			return printVersion(binaryName)
		}
	}

	// Handle --shell flag for completions
	// Per AI.md PART 33 line 45761-45762
	for i, arg := range os.Args[1:] {
		if arg == "--shell" {
			if i+1 < len(os.Args)-1 {
				subArg := os.Args[i+2]
				switch subArg {
				case "completions":
					shell := "bash"
					if i+2 < len(os.Args)-1 {
						shell = os.Args[i+3]
					}
					return printShellCompletions(binaryName, shell)
				case "init":
					shell := "bash"
					if i+2 < len(os.Args)-1 {
						shell = os.Args[i+3]
					}
					return printShellInit(binaryName, shell)
				}
			}
			return NewUsageError("--shell requires 'completions' or 'init' subcommand")
		}
	}

	// Global flags
	flagSet := flag.NewFlagSet(binaryName, flag.ContinueOnError)
	flagSet.Usage = func() {
		printUsage(binaryName)
	}

	// Define global flags per AI.md PART 33 line 45519-45523
	// Standard Config Flags: --server, --token, --config, --output, --debug
	serverFlag := flagSet.String("server", "", "Server URL (default: "+OfficialSite+")")
	tokenFlag := flagSet.String("token", "", "API token for authentication")
	tokenFileFlag := flagSet.String("token-file", "", "Read token from file")
	userFlag := flagSet.String("user", "", "Target user or org (@user, +org, or auto-detect)")
	configFlag := flagSet.String("config", "", "Config profile name")
	outputFlag := flagSet.String("output", "", "Output format: json, table, plain, yaml, csv")
	debugFlag := flagSet.Bool("debug", false, "Enable debug mode")
	colorFlag := flagSet.String("color", "auto", "Color output: auto, yes, no")

	// Parse flags - supports both --flag=value and --flag value per line 45527-45540
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return NewUsageError(err.Error())
	}

	// Ensure directories exist before any file operations
	// Per AI.md PART 33 line 45006-45059
	if err := EnsureDirs(); err != nil {
		return err
	}

	// Load config from profile per AI.md line 45181-45195
	// --config NAME resolves to {config_dir}/{name}.yml
	config, err := LoadConfigFromProfile(*configFlag)
	if err != nil {
		return err
	}

	// Get token using priority order per AI.md line 43367-43372
	// 1. --token flag, 2. --token-file, 3. WEATHER_TOKEN env, 4. config, 5. token file
	config.Auth.Token = GetToken(*tokenFlag, *tokenFileFlag, config)

	// Override config with flags (CLI has highest priority per line 11712-11719)
	if *serverFlag != "" {
		config.Server.Primary = *serverFlag
	}

	// Apply server resolution priority per AI.md PART 33 line 45376-45395
	// 1. --server flag > 2. config server.primary > 3. OfficialSite
	if config.Server.Primary == "" {
		config.Server.Primary = OfficialSite
	}

	if *userFlag != "" {
		config.User = *userFlag
	}

	if *outputFlag != "" {
		config.Output.Format = *outputFlag
	}

	if *debugFlag {
		config.Debug = true
	}

	// Handle --color flag per AI.md line 45482, 9584
	// Respects NO_COLOR environment variable
	switch *colorFlag {
	case "no":
		config.Output.Color = "no"
	case "yes":
		config.Output.Color = "yes"
	case "auto":
		// Check NO_COLOR env var (standard: https://no-color.org/)
		if os.Getenv("NO_COLOR") != "" {
			config.Output.Color = "no"
		} else if !term.IsTerminal(int(os.Stdout.Fd())) {
			config.Output.Color = "no"
		} else {
			config.Output.Color = "auto"
		}
	}

	// Detect mode automatically
	// Per AI.md PART 33: Mode is auto-detected, NEVER use --tui/--cli/--gui flags
	mode := detectMode(flagSet.Args())

	if config.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Mode: %s\n", mode)
		fmt.Fprintf(os.Stderr, "[DEBUG] Server: %s\n", config.Server.Primary)
		fmt.Fprintf(os.Stderr, "[DEBUG] Token: %s\n", maskToken(config.Auth.Token))
	}

	// Handle based on mode
	switch mode {
	case "tui":
		// Per AI.md PART 33: Check for first-run setup
		if config.Server.Primary == "" {
			return runSetupWizard(config)
		}
		return runTUI(config)
	case "plain":
		// Force plain output for non-TTY
		config.Output.Color = "no"
		fallthrough
	case "cli":
		return handleCommand(config, flagSet.Args(), binaryName)
	default:
		return NewUsageError("failed to detect mode")
	}
}

// detectMode determines if we should use TUI or CLI mode
// Per AI.md PART 33: Mode is auto-detected, NEVER implement --tui flag
func detectMode(args []string) string {
	// Not a terminal = plain output
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "plain"
	}

	// Config-only flags don't prevent TUI
	// Per AI.md PART 33 line 43809-43825
	configFlags := map[string]bool{
		"--config": true, "--server": true, "--token": true, "--debug": true,
		"--token-file": true, "--user": true, "--color": true, "--output": true,
	}

	// Check if any non-config args provided
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// Command or argument provided = CLI mode
			return "cli"
		}
		f := strings.Split(arg, "=")[0]
		if !configFlags[f] {
			// Unknown flag = CLI mode
			return "cli"
		}
	}

	// No args or config-only args = TUI mode
	// Per AI.md PART 33 line 43765: Interactive terminal + no command = TUI
	return "tui"
}

// handleCommand routes to the appropriate command handler
func handleCommand(config *CLIConfig, args []string, binaryName string) error {
	if len(args) == 0 {
		printUsage(binaryName)
		return nil
	}

	command := args[0]
	commandArgs := args[1:]

	// Route to appropriate command handler
	// Per AI.md PART 33: No "tui" subcommand - TUI is auto-launched when no args
	switch command {
	case "config":
		return handleConfigCommand(commandArgs, binaryName)
	case "version":
		return printVersion(binaryName)
	case "login":
		return handleLoginCommand(config, commandArgs)
	case "logout":
		return handleLogoutCommand()
	case "current":
		return handleCurrentCommand(config, commandArgs)
	case "forecast":
		return handleForecastCommand(config, commandArgs)
	case "alerts":
		return handleAlertsCommand(config, commandArgs)
	case "moon":
		return handleMoonCommand(config, commandArgs)
	case "history":
		return handleHistoryCommand(config, commandArgs)
	case "earthquakes":
		return handleEarthquakesCommand(config, commandArgs)
	case "hurricanes":
		return handleHurricanesCommand(config, commandArgs)
	default:
		return NewUsageError(fmt.Sprintf("unknown command: %s", command))
	}
}

// printUsage prints the usage information
// Per AI.md PART 33 line 9551: Shows actual binary name
func printUsage(binaryName string) {
	fmt.Printf("%s - Weather CLI for %s\n", binaryName, OfficialSite)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s [flags] <command> [args]\n", binaryName)
	fmt.Printf("  %s                          # Launch interactive TUI\n", binaryName)
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  current      Get current weather")
	fmt.Println("  forecast     Get weather forecast")
	fmt.Println("  alerts       Get weather alerts")
	fmt.Println("  moon         Get moon phase information")
	fmt.Println("  history      Get historical weather data")
	fmt.Println("  earthquakes  Get earthquake data")
	fmt.Println("  hurricanes   Get hurricane data")
	fmt.Println("  config       Manage configuration (init, show, get, set)")
	fmt.Println("  login        Authenticate and save token")
	fmt.Println("  logout       Remove saved token")
	fmt.Println("  version      Show version information")
	fmt.Println()
	fmt.Println("Global Flags:")
	fmt.Println("  --server URL          Server URL (default: " + OfficialSite + ")")
	fmt.Println("  --token TOKEN         API token for authentication")
	fmt.Println("  --token-file FILE     Read token from file")
	fmt.Println("  --user NAME           Target user or org (@user, +org, or auto-detect)")
	fmt.Println("  --config NAME         Config profile name")
	fmt.Println("  --output FORMAT       Output format: json, table, plain, yaml, csv")
	fmt.Println("  --debug               Enable debug mode")
	fmt.Println("  --color MODE          Color output: auto, yes, no (default: auto)")
	fmt.Println("  --shell completions   Print shell completions")
	fmt.Println("  --shell init          Print shell init command")
	fmt.Println("  -v, --version         Show version information")
	fmt.Println("  -h, --help            Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s                                    # Launch TUI\n", binaryName)
	fmt.Printf("  %s current --location \"New York\"      # Current weather\n", binaryName)
	fmt.Printf("  %s forecast --zip 10001               # 7-day forecast\n", binaryName)
	fmt.Printf("  %s moon                               # Moon phase today\n", binaryName)
	fmt.Printf("  %s --output json current              # JSON output\n", binaryName)
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  WTHR_TOKEN              API token")
	fmt.Println("  WTHR_SERVER_PRIMARY     Server URL")
	fmt.Println("  WTHR_OUTPUT_FORMAT      Output format")
	fmt.Println("  MYLOCATION_NAME         Default location name")
	fmt.Println("  MYLOCATION_ZIP          Default ZIP code")
	fmt.Println("  NO_COLOR                Disable colors when set")
	fmt.Println()
}

// printVersion prints version information per AI.md PART 16
func printVersion(binaryName string) error {
	fmt.Printf("%s v%s\n", binaryName, Version)
	fmt.Printf("Built: %s\n", BuildDate)
	fmt.Printf("Go: %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return nil
}

// printShellCompletions prints shell completion script
func printShellCompletions(binaryName, shell string) error {
	switch shell {
	case "bash":
		fmt.Printf(`# Bash completion for %s
_%s_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands="current forecast alerts moon history earthquakes hurricanes config login logout version"
    COMPREPLY=($(compgen -W "$commands" -- "$cur"))
}
complete -F _%s_completions %s
`, binaryName, binaryName, binaryName, binaryName)
	case "zsh":
		fmt.Printf(`#compdef %s
_arguments \
    '1:command:(current forecast alerts moon history earthquakes hurricanes config login logout version)' \
    '*::arg:->args'
`, binaryName)
	case "fish":
		fmt.Printf(`# Fish completion for %s
complete -c %s -f -n "__fish_use_subcommand" -a "current" -d "Get current weather"
complete -c %s -f -n "__fish_use_subcommand" -a "forecast" -d "Get weather forecast"
complete -c %s -f -n "__fish_use_subcommand" -a "alerts" -d "Get weather alerts"
complete -c %s -f -n "__fish_use_subcommand" -a "moon" -d "Get moon phase"
complete -c %s -f -n "__fish_use_subcommand" -a "history" -d "Get historical weather"
complete -c %s -f -n "__fish_use_subcommand" -a "earthquakes" -d "Get earthquake data"
complete -c %s -f -n "__fish_use_subcommand" -a "hurricanes" -d "Get hurricane data"
complete -c %s -f -n "__fish_use_subcommand" -a "config" -d "Manage configuration"
complete -c %s -f -n "__fish_use_subcommand" -a "login" -d "Authenticate"
complete -c %s -f -n "__fish_use_subcommand" -a "logout" -d "Remove token"
complete -c %s -f -n "__fish_use_subcommand" -a "version" -d "Show version"
`, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName)
	default:
		return NewUsageError(fmt.Sprintf("unsupported shell: %s (use bash, zsh, or fish)", shell))
	}
	return nil
}

// printShellInit prints shell init command
func printShellInit(binaryName, shell string) error {
	switch shell {
	case "bash":
		fmt.Printf("eval \"$(%s --shell completions bash)\"\n", binaryName)
	case "zsh":
		fmt.Printf("eval \"$(%s --shell completions zsh)\"\n", binaryName)
	case "fish":
		fmt.Printf("%s --shell completions fish | source\n", binaryName)
	default:
		return NewUsageError(fmt.Sprintf("unsupported shell: %s (use bash, zsh, or fish)", shell))
	}
	return nil
}

// handleConfigCommand handles config subcommands
func handleConfigCommand(args []string, binaryName string) error {
	if len(args) == 0 {
		return NewUsageError("config command requires a subcommand (init, show, get, set)")
	}

	subcommand := args[0]

	switch subcommand {
	case "init":
		return InitConfig()

	case "show":
		configPath, err := ConfigPath()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(configPath)
		if err == nil {
			fmt.Println(string(data))
		} else {
			fmt.Println("# No config file found, showing defaults")
			fmt.Println("server:")
			fmt.Printf("  primary: %s\n", OfficialSite)
			fmt.Println("output:")
			fmt.Println("  format: table")
			fmt.Println("  color: auto")
		}
		return nil

	case "get":
		if len(args) < 2 {
			return NewUsageError("config get requires a key (e.g., server.primary, output.format)")
		}
		value, err := GetConfigValue(args[1])
		if err != nil {
			return err
		}
		fmt.Println(value)
		return nil

	case "set":
		if len(args) < 3 {
			return NewUsageError("config set requires a key and value")
		}
		if err := SetConfigValue(args[1], strings.Join(args[2:], " ")); err != nil {
			return err
		}
		fmt.Printf("Configuration updated: %s = %s\n", args[1], strings.Join(args[2:], " "))
		return nil

	case "path":
		configPath, err := ConfigPath()
		if err != nil {
			return err
		}
		fmt.Println(configPath)
		return nil

	default:
		return NewUsageError(fmt.Sprintf("unknown config subcommand: %s", subcommand))
	}
}

// handleLoginCommand handles the login command
func handleLoginCommand(config *CLIConfig, args []string) error {
	fmt.Printf("Server: %s\n", config.Server.Primary)
	fmt.Println()

	// Get username/email
	fmt.Print("Username or email: ")
	reader := bufio.NewReader(os.Stdin)
	identifier, err := reader.ReadString('\n')
	if err != nil {
		return NewAPIError(fmt.Sprintf("failed to read input: %v", err))
	}
	identifier = strings.TrimSpace(identifier)

	// Get password (hidden)
	fmt.Print("Password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return NewAPIError(fmt.Sprintf("failed to read password: %v", err))
	}
	fmt.Println()

	// Make login request
	client := NewHTTPClient(config)
	loginReq := map[string]string{
		"identifier": identifier,
		"password":   string(password),
	}

	var result map[string]interface{}
	if err := client.PostJSON(config.GetAPIPath()+"/server/auth/login", loginReq, &result); err != nil {
		return err
	}

	// Extract token from response (handle unified JSON response per line 12750-12770)
	var token string
	if data, ok := result["data"].(map[string]interface{}); ok {
		token, _ = data["token"].(string)
	} else {
		token, _ = result["token"].(string)
	}
	if token == "" {
		return NewAPIError("login successful but no token in response")
	}

	// Save token
	if err := SaveToken(token); err != nil {
		return err
	}

	tokenPath, _ := TokenPath()
	fmt.Printf("\nLogin successful! Token saved to %s\n", tokenPath)
	return nil
}

// handleLogoutCommand handles the logout command
func handleLogoutCommand() error {
	tokenPath, err := TokenPath()
	if err != nil {
		return err
	}

	// Remove token file
	if err := os.Remove(tokenPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No token file found. Already logged out.")
			return nil
		}
		return NewConfigError(fmt.Sprintf("failed to remove token: %v", err))
	}

	fmt.Println("Logged out. Token removed.")
	return nil
}

// handleEarthquakesCommand handles the earthquakes command
func handleEarthquakesCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("earthquakes", flag.ContinueOnError)
	minMagnitude := flagSet.Float64("min-mag", 0, "Minimum magnitude")
	limit := flagSet.Int("limit", 10, "Number of results")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	path := fmt.Sprintf("%s/earthquakes?min_magnitude=%f&limit=%d", config.GetAPIPath(), *minMagnitude, *limit)

	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatJSON(result))

	return nil
}

// handleHurricanesCommand handles the hurricanes command
func handleHurricanesCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("hurricanes", flag.ContinueOnError)
	active := flagSet.Bool("active", true, "Show only active storms")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	path := config.GetAPIPath() + "/hurricanes"
	if *active {
		path += "?active=true"
	}

	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatJSON(result))

	return nil
}

// maskToken masks a token for debug output
func maskToken(token string) string {
	if token == "" {
		return "(empty)"
	}
	if len(token) < 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
