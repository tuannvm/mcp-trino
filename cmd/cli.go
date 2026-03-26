package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tuannvm/mcp-trino/internal/cli"
	"github.com/tuannvm/mcp-trino/internal/config"
	"github.com/tuannvm/mcp-trino/internal/trino"
)

// cleanArgs removes mode selection flags from the argument list (before subcommand)
func cleanArgs(args []string) []string {
	cleaned := make([]string, 0, len(args))
	sawSubcommand := false
	for _, arg := range args {
		// Stop processing once we see a non-flag argument (subcommand)
		if !strings.HasPrefix(arg, "-") && arg != "" {
			sawSubcommand = true
		}

		// Only strip --cli/--mcp before the subcommand
		if !sawSubcommand && (arg == "--cli" || arg == "--mcp") {
			continue
		}
		cleaned = append(cleaned, arg)
	}
	return cleaned
}

// hasFlags checks if any argument appears to be a flag (starts with -)
func hasFlags(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// RunCLIMode executes the CLI mode
func RunCLIMode() error {
	// Strip mode selection flags (--cli, --mcp) from args before parsing
	args := cleanArgs(os.Args[1:])

	// Define CLI flags
	flagSet := flag.NewFlagSet("mcp-trino", flag.ExitOnError)
	configFile := flagSet.String("config", "", "Path to config file")
	profileName := flagSet.String("profile", "", "Profile name to use")
	format := flagSet.String("format", "", "Output format (table, json, csv)")
	host := flagSet.String("host", "", "Trino host")
	port := flagSet.Int("port", 0, "Trino port")
	user := flagSet.String("user", "", "Trino user")
	password := flagSet.String("password", "", "Trino password")
	catalog := flagSet.String("catalog", "", "Default catalog")
	schema := flagSet.String("schema", "", "Default schema")
	interactive := flagSet.Bool("interactive", false, "Interactive REPL mode")
	showVersion := flagSet.Bool("version", false, "Show version information")

	if err := flagSet.Parse(args); err != nil {
		return err
	}

	// Handle version flag
	if *showVersion {
		fmt.Printf("mcp-trino CLI version %s\n", Version)
		return nil
	}

	// Get the subcommand
	args = flagSet.Args()

	// Validate subcommand before connecting to Trino
	if len(args) > 0 && !*interactive {
		validCommands := map[string]bool{
			"query":       true,
			"catalogs":    true,
			"schemas":     true,
			"tables":      true,
			"describe":    true,
			"explain":     true,
			"interactive": true,
			"config":      true, // config profile management
		}
		if !validCommands[args[0]] {
			return fmt.Errorf("unknown command: %s (run 'mcp-trino' for usage)", args[0])
		}
	}

	if len(args) == 0 && !*interactive {
		fmt.Println("mcp-trino CLI - Trino query tool")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  mcp-trino [flags] <command> [arguments]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  query <sql>       Execute a SQL query")
		fmt.Println("  catalogs          List all catalogs")
		fmt.Println("  schemas <catalog> List schemas in a catalog")
		fmt.Println("  tables <cat> <sch> List tables in schema")
		fmt.Println("  describe <table>  Describe table schema")
		fmt.Println("  explain <sql>     Explain query plan")
		fmt.Println("  interactive       Start interactive REPL mode")
		fmt.Println("  config profile    Manage connection profiles")
		fmt.Println()
		fmt.Println("Flags:")
		flagSet.PrintDefaults()
		fmt.Println()
		fmt.Println("Environment Variables:")
		fmt.Println("  TRINO_HOST, TRINO_PORT, TRINO_USER, TRINO_PASSWORD")
		fmt.Println("  TRINO_CATALOG, TRINO_SCHEMA, TRINO_PROFILE")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  mcp-trino query 'SELECT 1'")
		fmt.Println("  mcp-trino --profile staging catalogs")
		fmt.Println("  mcp-trino config profile list")
		fmt.Println("  mcp-trino config profile use prod")
		fmt.Println("  mcp-trino --interactive")
		return nil
	}

	// Load configuration from file if specified, otherwise load default
	var cliConfig *cli.CLIConfig
	if *configFile != "" {
		data, readErr := os.ReadFile(*configFile)
		if readErr != nil {
			return fmt.Errorf("failed to read config file: %w", readErr)
		}
		var parseErr error
		cliConfig, parseErr = cli.ParseCLIConfigWithPath(data, *configFile)
		if parseErr != nil {
			return fmt.Errorf("failed to parse config file: %w", parseErr)
		}
	} else {
		var loadErr error
		cliConfig, loadErr = cli.LoadCLIConfig()
		if loadErr != nil {
			log.Printf("Warning: failed to load CLI config: %v", loadErr)
			cliConfig = cli.DefaultCLIConfig()
		}
	}

	// Apply CLI config to environment (config file values, flags will override)
	// Resolve profile name: --profile flag > TRINO_PROFILE env > current in config > default
	activeProfile := *profileName
	if activeProfile == "" {
		activeProfile = os.Getenv("TRINO_PROFILE")
	}

	// Handle config command early (doesn't need Trino connection or profile validation)
	if len(args) > 0 && args[0] == "config" {
		// Config commands don't need profile validation - allow users to fix stale profiles
		return runConfigCommand(args, cliConfig)
	}

	// Validate the profile that will be used (whether explicit or from current field)
	// This prevents silent misconfiguration when current points to a missing profile
	profileToUse := activeProfile
	if profileToUse == "" {
		profileToUse = cliConfig.Current
	}
	if profileToUse == "" {
		profileToUse = "default"
	}
	_, err := cliConfig.GetActiveProfile(profileToUse)
	if err != nil {
		// Fail hard if the resolved profile doesn't exist
		return fmt.Errorf("profile '%s' not found: %w", profileToUse, err)
	}

	// Apply profile to environment (profiles override pre-existing env vars)
	if err := cliConfig.ApplyToEnv(activeProfile); err != nil {
		log.Printf("Warning: failed to apply CLI config: %v", err)
	}

	// Apply CLI flags to environment (flags take precedence over everything)
	if *host != "" {
		_ = os.Setenv("TRINO_HOST", *host)
	}
	if *port != 0 {
		_ = os.Setenv("TRINO_PORT", fmt.Sprintf("%d", *port))
	}
	if *user != "" {
		_ = os.Setenv("TRINO_USER", *user)
	}
	if *password != "" {
		_ = os.Setenv("TRINO_PASSWORD", *password)
	}
	if *catalog != "" {
		_ = os.Setenv("TRINO_CATALOG", *catalog)
	}
	if *schema != "" {
		_ = os.Setenv("TRINO_SCHEMA", *schema)
	}

	// Validate required fields after precedence is applied (profile + CLI flags)
	// This ensures fail-fast behavior for incomplete configuration
	if os.Getenv("TRINO_HOST") == "" {
		return fmt.Errorf("missing required configuration: host not set (provide via --host flag, profile, or TRINO_HOST env var)")
	}
	if os.Getenv("TRINO_USER") == "" {
		return fmt.Errorf("missing required configuration: user not set (provide via --user flag, profile, or TRINO_USER env var)")
	}
	// Port is optional in config but TrinoConfig will use default 8080 if not set
	// We allow this to pass since it's a reasonable default

	// Determine output format
	outputFormat := *format
	if outputFormat == "" {
		outputFormat = cliConfig.GetOutputFormat()
	}

	// Validate output format
	validFormats := map[string]bool{"table": true, "json": true, "csv": true}
	if outputFormat != "" && !validFormats[outputFormat] {
		return fmt.Errorf("invalid output format '%s': must be one of table, json, csv", outputFormat)
	}

	// Default to table if empty
	if outputFormat == "" {
		outputFormat = "table"
	}

	// Initialize Trino configuration
	trinoConfig, err := config.NewTrinoConfigWithVersion(Version)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize Trino client
	trinoClient, err := trino.NewClient(trinoConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize Trino client: %w", err)
	}
	defer func() {
		if err := trinoClient.Close(); err != nil {
			log.Printf("Error closing Trino client: %v", err)
		}
	}()

	// Create CLI commands handler
	commands := cli.NewCommands(trinoClient, outputFormat)
	ctx := context.Background()

	// Handle interactive mode
	if *interactive || (len(args) > 0 && args[0] == "interactive") {
		repl := cli.NewREPL(commands, *catalog, *schema)
		return repl.Run(ctx)
	}

	// Handle subcommands
	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "query":
		if len(commandArgs) == 0 {
			return fmt.Errorf("query command requires a SQL argument")
		}
		query := strings.Join(commandArgs, " ")
		return commands.Query(ctx, query)

	case "catalogs":
		return commands.Catalogs(ctx)

	case "schemas":
		// Create a subcommand flag set for schemas-specific flags
		schemasFlagSet := flag.NewFlagSet("schemas", flag.ContinueOnError)
		schemasCatalog := schemasFlagSet.String("catalog", "", "Catalog name")
		// Parse the commandArgs as flags
		if err := schemasFlagSet.Parse(commandArgs); err != nil {
			// If flag parsing failed and no flags were present, treat as positional
			if !hasFlags(commandArgs) && len(commandArgs) > 0 {
				return commands.Schemas(ctx, commandArgs[0])
			}
			return fmt.Errorf("schemas command error: %w", err)
		}
		// Use flag value if set, otherwise use remaining positional arg
		if *schemasCatalog != "" {
			return commands.Schemas(ctx, *schemasCatalog)
		}
		// Use positional argument if provided
		remainingArgs := schemasFlagSet.Args()
		if len(remainingArgs) > 0 {
			return commands.Schemas(ctx, remainingArgs[0])
		}
		return commands.Schemas(ctx, "")

	case "tables":
		// Create a subcommand flag set for tables-specific flags
		tablesFlagSet := flag.NewFlagSet("tables", flag.ContinueOnError)
		tablesCatalog := tablesFlagSet.String("catalog", "", "Catalog name")
		tablesSchema := tablesFlagSet.String("schema", "", "Schema name")
		// Parse the commandArgs as flags
		if err := tablesFlagSet.Parse(commandArgs); err != nil {
			// If flag parsing failed and no flags were present, treat as positional
			if !hasFlags(commandArgs) {
				if len(commandArgs) >= 2 {
					return commands.Tables(ctx, commandArgs[0], commandArgs[1])
				}
				if len(commandArgs) == 1 {
					return commands.Tables(ctx, commandArgs[0], "")
				}
				return commands.Tables(ctx, "", "")
			}
			return fmt.Errorf("tables command error: %w", err)
		}
		// Use flag values if set, otherwise use remaining positional args
		remainingArgs := tablesFlagSet.Args()
		finalCatalog, finalSchema := "", ""
		if *tablesCatalog != "" {
			finalCatalog = *tablesCatalog
		}
		if *tablesSchema != "" {
			finalSchema = *tablesSchema
		}
		// Positional args fill in missing values in order
		posIndex := 0
		if finalCatalog == "" && len(remainingArgs) > posIndex {
			finalCatalog = remainingArgs[posIndex]
			posIndex++
		}
		if finalSchema == "" && len(remainingArgs) > posIndex {
			finalSchema = remainingArgs[posIndex]
		}
		return commands.Tables(ctx, finalCatalog, finalSchema)

	case "describe":
		if len(commandArgs) == 0 {
			return fmt.Errorf("describe command requires a table argument (format: catalog.schema.table)")
		}
		table := commandArgs[0]
		return commands.Describe(ctx, table)

	case "explain":
		if len(commandArgs) == 0 {
			return fmt.Errorf("explain command requires a SQL argument")
		}
		query := strings.Join(commandArgs, " ")
		return commands.Explain(ctx, query, "")

	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// runConfigCommand handles config profile management commands
func runConfigCommand(args []string, cliConfig *cli.CLIConfig) error {
	if len(args) < 2 {
		return fmt.Errorf("config command requires a subcommand: profile")
	}

	switch args[1] {
	case "profile":
		return runConfigProfileCommand(args, cliConfig)
	default:
		return fmt.Errorf("unknown config subcommand: %s (available: profile)", args[1])
	}
}

// runConfigProfileCommand handles profile management commands
func runConfigProfileCommand(args []string, cliConfig *cli.CLIConfig) error {
	if len(args) < 3 {
		fmt.Println("config profile - Manage Trino connection profiles")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  mcp-trino config profile list           List all profiles")
		fmt.Println("  mcp-trino config profile use <name>      Set current profile")
		fmt.Println("  mcp-trino config profile show <name>     Show profile details")
		fmt.Println()
		fmt.Printf("Current profile: %s\n", cliConfig.Current)
		return nil
	}

	switch args[2] {
	case "list":
		return runProfileList(cliConfig)
	case "use":
		if len(args) < 4 {
			return fmt.Errorf("config profile use requires a profile name")
		}
		return runProfileUse(cliConfig, args[3])
	case "show":
		if len(args) < 4 {
			return fmt.Errorf("config profile show requires a profile name")
		}
		return runProfileShow(cliConfig, args[3])
	default:
		return fmt.Errorf("unknown profile subcommand: %s (available: list, use, show)", args[2])
	}
}

// runProfileList lists all available profiles
func runProfileList(cliConfig *cli.CLIConfig) error {
	fmt.Printf("Available profiles (current: %s):\n", cliConfig.Current)
	fmt.Println()

	// Use sorted profile names for deterministic output
	for _, name := range cliConfig.GetProfileNames() {
		profile := cliConfig.Profiles[name]
		currentMarker := ""
		if name == cliConfig.Current {
			currentMarker = " *"
		}
		fmt.Printf("  %s%s: %s@%s:%d\n", name, currentMarker, profile.User, profile.Host, profile.Port)
	}

	// List profile count
	fmt.Printf("\nTotal: %d profile(s)\n", len(cliConfig.Profiles))
	return nil
}

// runProfileUse sets the current profile
func runProfileUse(cliConfig *cli.CLIConfig, name string) error {
	if err := cliConfig.SetCurrent(name); err != nil {
		return err
	}
	fmt.Printf("Current profile set to: %s\n", name)
	return nil
}

// runProfileShow shows detailed information about a profile
func runProfileShow(cliConfig *cli.CLIConfig, name string) error {
	profile, exists := cliConfig.Profiles[name]
	if !exists {
		return fmt.Errorf("profile '%s' not found. Available profiles: %v",
			name, cliConfig.GetProfileNames())
	}

	currentMarker := ""
	if name == cliConfig.Current {
		currentMarker = " (current)"
	}

	fmt.Printf("Profile: %s%s\n", name, currentMarker)
	fmt.Printf("  Host: %s\n", profile.Host)
	fmt.Printf("  Port: %d\n", profile.Port)
	fmt.Printf("  User: %s\n", profile.User)
	if profile.Password != "" {
		fmt.Printf("  Password: ********\n")
	}
	if profile.Catalog != "" {
		fmt.Printf("  Catalog: %s\n", profile.Catalog)
	}
	if profile.Schema != "" {
		fmt.Printf("  Schema: %s\n", profile.Schema)
	}
	if profile.SSL.Enabled != nil {
		fmt.Printf("  SSL: %v\n", *profile.SSL.Enabled)
	}
	if profile.SSL.Insecure {
		fmt.Printf("  SSL Insecure: true\n")
	}
	return nil
}
