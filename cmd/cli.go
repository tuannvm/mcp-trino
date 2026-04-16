package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tuannvm/mcp-trino/internal/cli"
	"github.com/tuannvm/mcp-trino/internal/config"
	"github.com/tuannvm/mcp-trino/internal/trino"
)

// Exit codes following Unix conventions
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// cleanArgs removes mode selection flags from the argument list (before subcommand)
func cleanArgs(args []string) []string {
	cleaned := make([]string, 0, len(args))
	sawSubcommand := false
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && arg != "" {
			sawSubcommand = true
		}
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

// RunCLIMode executes the CLI mode and returns an exit code
func RunCLIMode() error {
	return runCLI(os.Stdout, os.Stderr, os.Args[1:])
}

func runCLI(stdout, stderr io.Writer, rawArgs []string) error {
	args := cleanArgs(rawArgs)

	flagSet := flag.NewFlagSet("mcp-trino", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	configFile := flagSet.String("config", "", "Path to config file (default: ~/.config/trino/config.json)")
	profileName := flagSet.String("profile", "", "Connection profile name")
	format := flagSet.String("format", "", "Output format: table, json, csv (default: table)")
	host := flagSet.String("host", "", "Trino host")
	port := flagSet.Int("port", 0, "Trino port (default: 8080)")
	user := flagSet.String("user", "", "Trino user")
	password := flagSet.String("password", "", "Trino password")
	catalog := flagSet.String("catalog", "", "Default catalog")
	schema := flagSet.String("schema", "", "Default schema")
	interactive := flagSet.Bool("interactive", false, "Start interactive REPL mode")
	showVersion := flagSet.Bool("version", false, "Show version information")

	// Override default usage to provide structured help
	flagSet.Usage = func() {
		printMainHelp(stderr)
	}

	if err := flagSet.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return &usageError{err: err}
	}

	if *showVersion {
		_, _ = fmt.Fprintf(stdout, "mcp-trino %s\n", Version)
		return nil
	}

	args = flagSet.Args()

	// Validate subcommand first, then check for --help
	validCommands := map[string]bool{
		"query": true, "catalogs": true, "schemas": true,
		"tables": true, "describe": true, "explain": true,
		"interactive": true, "config": true,
	}
	if len(args) > 0 && !*interactive {
		if !validCommands[args[0]] {
			return &usageError{err: fmt.Errorf("unknown command: %s", args[0])}
		}
		if isHelpRequest(args) {
			printSubcommandHelp(stderr, args[0])
			return nil
		}
	}

	if len(args) == 0 && !*interactive {
		printMainHelp(stderr)
		return nil
	}

	// Load configuration
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
			_, _ = fmt.Fprintf(stderr, "Warning: failed to load CLI config: %v\n", loadErr)
			cliConfig = cli.DefaultCLIConfig()
		}
	}

	// Resolve profile
	activeProfile := *profileName
	if activeProfile == "" {
		activeProfile = os.Getenv("TRINO_PROFILE")
	}

	// Handle config command early (doesn't need Trino connection)
	if len(args) > 0 && args[0] == "config" {
		return runConfigCommand(stdout, args, cliConfig)
	}

	// Validate profile
	profileToUse := activeProfile
	if profileToUse == "" {
		profileToUse = cliConfig.Current
	}
	if profileToUse == "" {
		profileToUse = "default"
	}
	if _, err := cliConfig.GetActiveProfile(profileToUse); err != nil {
		return fmt.Errorf("profile '%s' not found: %w", profileToUse, err)
	}

	// Apply profile to environment
	if err := cliConfig.ApplyToEnv(activeProfile); err != nil {
		_, _ = fmt.Fprintf(stderr, "Warning: failed to apply CLI config: %v\n", err)
	}

	// Apply CLI flags to environment (highest precedence)
	applyFlagToEnv("TRINO_HOST", *host)
	if *port != 0 {
		applyFlagToEnv("TRINO_PORT", fmt.Sprintf("%d", *port))
	}
	applyFlagToEnv("TRINO_USER", *user)
	applyFlagToEnv("TRINO_PASSWORD", *password)
	applyFlagToEnv("TRINO_CATALOG", *catalog)
	applyFlagToEnv("TRINO_SCHEMA", *schema)

	// Validate required fields
	if os.Getenv("TRINO_HOST") == "" {
		return fmt.Errorf("missing required configuration: host not set (provide via --host flag, profile, or TRINO_HOST env var)")
	}
	if os.Getenv("TRINO_USER") == "" {
		return fmt.Errorf("missing required configuration: user not set (provide via --user flag, profile, or TRINO_USER env var)")
	}

	// Determine and validate output format
	outputFormat := *format
	if outputFormat == "" {
		outputFormat = cliConfig.GetOutputFormat()
	}
	validFormats := map[string]bool{"table": true, "json": true, "csv": true}
	if outputFormat != "" && !validFormats[outputFormat] {
		return &usageError{err: fmt.Errorf("invalid output format '%s': must be one of table, json, csv", outputFormat)}
	}
	if outputFormat == "" {
		outputFormat = "table"
	}

	// Initialize Trino
	trinoConfig, err := config.NewTrinoConfigWithVersion(Version)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	trinoClient, err := trino.NewClient(trinoConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize Trino client: %w", err)
	}
	defer func() {
		if closeErr := trinoClient.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(stderr, "Error closing Trino client: %v\n", closeErr)
		}
	}()

	commands := cli.NewCommandsWithWriters(trinoClient, outputFormat, stdout, stderr)

	// Set up context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Handle interactive mode
	if *interactive || (len(args) > 0 && args[0] == "interactive") {
		repl := cli.NewREPL(commands, *catalog, *schema)
		return repl.Run(ctx)
	}

	// Dispatch subcommand
	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "query":
		if len(commandArgs) == 0 {
			return &usageError{err: fmt.Errorf("query command requires a SQL argument")}
		}
		return commands.Query(ctx, strings.Join(commandArgs, " "))

	case "catalogs":
		return commands.Catalogs(ctx)

	case "schemas":
		schemasFlagSet := flag.NewFlagSet("schemas", flag.ContinueOnError)
		schemasFlagSet.SetOutput(stderr)
		schemasCatalog := schemasFlagSet.String("catalog", "", "Catalog name")
		if err := schemasFlagSet.Parse(commandArgs); err != nil {
			if !hasFlags(commandArgs) && len(commandArgs) > 0 {
				return commands.Schemas(ctx, commandArgs[0])
			}
			return &usageError{err: fmt.Errorf("schemas command error: %w", err)}
		}
		if *schemasCatalog != "" {
			return commands.Schemas(ctx, *schemasCatalog)
		}
		remainingArgs := schemasFlagSet.Args()
		if len(remainingArgs) > 0 {
			return commands.Schemas(ctx, remainingArgs[0])
		}
		return commands.Schemas(ctx, "")

	case "tables":
		tablesFlagSet := flag.NewFlagSet("tables", flag.ContinueOnError)
		tablesFlagSet.SetOutput(stderr)
		tablesCatalog := tablesFlagSet.String("catalog", "", "Catalog name")
		tablesSchema := tablesFlagSet.String("schema", "", "Schema name")
		if err := tablesFlagSet.Parse(commandArgs); err != nil {
			if !hasFlags(commandArgs) {
				if len(commandArgs) >= 2 {
					return commands.Tables(ctx, commandArgs[0], commandArgs[1])
				}
				if len(commandArgs) == 1 {
					return commands.Tables(ctx, commandArgs[0], "")
				}
				return commands.Tables(ctx, "", "")
			}
			return &usageError{err: fmt.Errorf("tables command error: %w", err)}
		}
		remainingArgs := tablesFlagSet.Args()
		finalCatalog, finalSchema := "", ""
		if *tablesCatalog != "" {
			finalCatalog = *tablesCatalog
		}
		if *tablesSchema != "" {
			finalSchema = *tablesSchema
		}
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
			return &usageError{err: fmt.Errorf("describe command requires a table argument (format: catalog.schema.table)")}
		}
		return commands.Describe(ctx, commandArgs[0])

	case "explain":
		if len(commandArgs) == 0 {
			return &usageError{err: fmt.Errorf("explain command requires a SQL argument")}
		}
		return commands.Explain(ctx, strings.Join(commandArgs, " "), "")

	default:
		return &usageError{err: fmt.Errorf("unknown command: %s", command)}
	}
}

// usageError represents a usage/argument error (exit code 2)
type usageError struct {
	err error
}

func (e *usageError) Error() string {
	return e.err.Error()
}

// applyFlagToEnv sets an environment variable if value is non-empty
func applyFlagToEnv(key, value string) {
	if value != "" {
		_ = os.Setenv(key, value)
	}
}

// isHelpRequest checks if the args contain a help flag after a subcommand
func isHelpRequest(args []string) bool {
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// printMainHelp prints structured, LLM-friendly help to w
func printMainHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `NAME
    mcp-trino - Trino SQL client and MCP server

SYNOPSIS
    mcp-trino [flags] <command> [arguments]
    mcp-trino --interactive
    mcp-trino --version

DESCRIPTION
    A dual-purpose tool that works as both an interactive Trino SQL client
    and an MCP (Model Context Protocol) server for AI assistants.

    When invoked with a subcommand or --interactive, it runs as a CLI.
    Otherwise, it starts an MCP server (stdio or HTTP transport).

COMMANDS
    query <sql>              Execute a SQL query
    catalogs                 List all available catalogs
    schemas [catalog]        List schemas in a catalog
    tables [catalog] [schema]  List tables in a schema
    describe <table>         Describe table columns (catalog.schema.table)
    explain <sql>            Show query execution plan
    interactive              Start interactive REPL mode
    config profile <action>  Manage connection profiles

FLAGS
    --config <path>     Path to config file (default: ~/.config/trino/config.json)
    --profile <name>    Connection profile name
    --format <fmt>      Output format: table, json, csv (default: table)
    --host <host>       Trino host (overrides profile and env)
    --port <port>       Trino port (default: 8080)
    --user <user>       Trino user (overrides profile and env)
    --password <pass>   Trino password
    --catalog <name>    Default catalog
    --schema <name>     Default schema
    --interactive       Start interactive REPL mode
    --version           Show version information
    -h, --help          Show this help

EXAMPLES
    # Execute a query
    mcp-trino query 'SELECT 1'

    # List catalogs in JSON format
    mcp-trino --format json catalogs

    # Use a specific profile
    mcp-trino --profile staging catalogs

    # Start interactive REPL
    mcp-trino --interactive

    # Describe a table
    mcp-trino describe memory.default.my_table

    # Pipe-friendly: query with CSV output
    mcp-trino --format csv query 'SELECT * FROM t' > results.csv

    # Manage profiles
    mcp-trino config profile list
    mcp-trino config profile use prod

ENVIRONMENT
    TRINO_HOST              Trino server hostname
    TRINO_PORT              Trino server port (default: 8080)
    TRINO_USER              Trino username
    TRINO_PASSWORD          Trino password
    TRINO_CATALOG           Default catalog
    TRINO_SCHEMA            Default schema
    TRINO_PROFILE           Profile name (same as --profile flag)
    TRINO_SSL               Enable SSL (true/false)
    TRINO_SSL_INSECURE      Skip SSL certificate verification (true/false)
    TRINO_ALLOW_WRITE_QUERIES  Allow write queries (default: false)
    TRINO_QUERY_TIMEOUT     Query timeout in seconds (default: 30)
    NO_COLOR                Disable colored output when set

CONFIGURATION
    Config file: ~/.config/trino/config.json

    Precedence (highest to lowest):
      1. CLI flags (--host, --user, etc.)
      2. Profile from config file (--profile > TRINO_PROFILE > current)
      3. Environment variables (TRINO_HOST, etc.)
      4. Config file defaults

    Note: Configuration uses JSON format. If migrating from an older YAML
    config, convert ~/.config/trino/config.yaml to config.json manually.

SEE ALSO
    Use '<command> --help' for more information about a specific command.
`)
}

// printSubcommandHelp prints help for a specific subcommand
func printSubcommandHelp(w io.Writer, command string) {
	switch command {
	case "query":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino query - Execute a SQL query

SYNOPSIS
    mcp-trino [flags] query <sql>

DESCRIPTION
    Execute a SQL query against the configured Trino server and display
    the results in the specified output format.

    The query is passed as a single argument. Use quotes to prevent
    shell interpretation of SQL operators.

EXAMPLES
    mcp-trino query 'SELECT 1'
    mcp-trino query 'SELECT * FROM memory.default.my_table LIMIT 10'
    mcp-trino --format json query 'SELECT count(*) FROM t'
    mcp-trino --format csv query 'SELECT * FROM t' > out.csv
`)
	case "catalogs":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino catalogs - List all available catalogs

SYNOPSIS
    mcp-trino [flags] catalogs

DESCRIPTION
    List all catalogs available on the connected Trino server.

EXAMPLES
    mcp-trino catalogs
    mcp-trino --format json catalogs
`)
	case "schemas":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino schemas - List schemas in a catalog

SYNOPSIS
    mcp-trino [flags] schemas [catalog]
    mcp-trino [flags] schemas --catalog <name>

DESCRIPTION
    List all schemas within a catalog. If no catalog is specified,
    uses TRINO_CATALOG env var or defaults to "memory".

EXAMPLES
    mcp-trino schemas memory
    mcp-trino schemas --catalog hive
    mcp-trino --format json schemas
`)
	case "tables":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino tables - List tables in a schema

SYNOPSIS
    mcp-trino [flags] tables [catalog] [schema]
    mcp-trino [flags] tables --catalog <name> --schema <name>

DESCRIPTION
    List all tables within a schema. If catalog or schema are not
    specified, uses TRINO_CATALOG/TRINO_SCHEMA env vars or defaults
    to "memory"/"default".

EXAMPLES
    mcp-trino tables memory default
    mcp-trino tables --catalog hive --schema public
    mcp-trino --format json tables
`)
	case "describe":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino describe - Describe table columns

SYNOPSIS
    mcp-trino [flags] describe <table>

DESCRIPTION
    Show the column names, types, and metadata for a table.
    The table argument can be in one of these formats:
      - table
      - schema.table
      - catalog.schema.table

EXAMPLES
    mcp-trino describe my_table
    mcp-trino describe public.my_table
    mcp-trino describe memory.default.my_table
    mcp-trino --format json describe my_table
`)
	case "explain":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino explain - Show query execution plan

SYNOPSIS
    mcp-trino [flags] explain <sql>

DESCRIPTION
    Display the execution plan for a SQL query without running it.
    Useful for understanding query performance and optimization.

EXAMPLES
    mcp-trino explain 'SELECT * FROM t WHERE id = 1'
    mcp-trino --format json explain 'SELECT count(*) FROM t'
`)
	case "config":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino config - Manage CLI configuration

SYNOPSIS
    mcp-trino config profile list
    mcp-trino config profile use <name>
    mcp-trino config profile show <name>

DESCRIPTION
    Manage connection profiles stored in the config file.

SUBCOMMANDS
    profile list          List all available profiles
    profile use <name>    Set the current (default) profile
    profile show <name>   Show details of a specific profile

EXAMPLES
    mcp-trino config profile list
    mcp-trino config profile use prod
    mcp-trino config profile show staging
`)
	case "interactive":
		_, _ = fmt.Fprintf(w, `NAME
    mcp-trino interactive - Start interactive REPL mode

SYNOPSIS
    mcp-trino [flags] interactive
    mcp-trino --interactive

DESCRIPTION
    Start an interactive SQL REPL (Read-Eval-Print Loop) with support
    for multi-line queries, meta-commands, and command history.

    Meta-commands start with \ (e.g., \help, \catalogs, \quit).
    SQL queries are terminated with a semicolon (;).

EXAMPLES
    mcp-trino interactive
    mcp-trino --interactive
    mcp-trino --profile staging interactive
`)
	default:
		_, _ = fmt.Fprintf(w, "No help available for command: %s\n", command)
		_, _ = fmt.Fprintf(w, "Run 'mcp-trino --help' for a list of commands.\n")
	}
}

// runConfigCommand handles config profile management commands
func runConfigCommand(w io.Writer, args []string, cliConfig *cli.CLIConfig) error {
	if len(args) < 2 {
		return &usageError{err: fmt.Errorf("config command requires a subcommand: profile")}
	}

	switch args[1] {
	case "profile":
		return runConfigProfileCommand(w, args, cliConfig)
	default:
		return &usageError{err: fmt.Errorf("unknown config subcommand: %s (available: profile)", args[1])}
	}
}

// runConfigProfileCommand handles profile management commands
func runConfigProfileCommand(w io.Writer, args []string, cliConfig *cli.CLIConfig) error {
	if len(args) < 3 {
		_, _ = fmt.Fprintln(w, "config profile - Manage Trino connection profiles")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "Usage:")
		_, _ = fmt.Fprintln(w, "  mcp-trino config profile list           List all profiles")
		_, _ = fmt.Fprintln(w, "  mcp-trino config profile use <name>      Set current profile")
		_, _ = fmt.Fprintln(w, "  mcp-trino config profile show <name>     Show profile details")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Current profile: %s\n", cliConfig.Current)
		return &usageError{err: fmt.Errorf("config profile requires a subcommand: list, use, or show")}
	}

	switch args[2] {
	case "list":
		return runProfileList(w, cliConfig)
	case "use":
		if len(args) < 4 {
			return &usageError{err: fmt.Errorf("config profile use requires a profile name")}
		}
		return runProfileUse(w, cliConfig, args[3])
	case "show":
		if len(args) < 4 {
			return &usageError{err: fmt.Errorf("config profile show requires a profile name")}
		}
		return runProfileShow(w, cliConfig, args[3])
	default:
		return &usageError{err: fmt.Errorf("unknown profile subcommand: %s (available: list, use, show)", args[2])}
	}
}

func runProfileList(w io.Writer, cliConfig *cli.CLIConfig) error {
	_, _ = fmt.Fprintf(w, "Available profiles (current: %s):\n", cliConfig.Current)
	_, _ = fmt.Fprintln(w)

	for _, name := range cliConfig.GetProfileNames() {
		profile := cliConfig.Profiles[name]
		currentMarker := ""
		if name == cliConfig.Current {
			currentMarker = " *"
		}
		_, _ = fmt.Fprintf(w, "  %s%s: %s@%s:%d\n", name, currentMarker, profile.User, profile.Host, profile.Port)
	}

	_, _ = fmt.Fprintf(w, "\nTotal: %d profile(s)\n", len(cliConfig.Profiles))
	return nil
}

func runProfileUse(w io.Writer, cliConfig *cli.CLIConfig, name string) error {
	if err := cliConfig.SetCurrent(name); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Current profile set to: %s\n", name)
	return nil
}

func runProfileShow(w io.Writer, cliConfig *cli.CLIConfig, name string) error {
	profile, exists := cliConfig.Profiles[name]
	if !exists {
		return fmt.Errorf("profile '%s' not found. Available profiles: %v",
			name, cliConfig.GetProfileNames())
	}

	currentMarker := ""
	if name == cliConfig.Current {
		currentMarker = " (current)"
	}

	_, _ = fmt.Fprintf(w, "Profile: %s%s\n", name, currentMarker)
	_, _ = fmt.Fprintf(w, "  Host: %s\n", profile.Host)
	_, _ = fmt.Fprintf(w, "  Port: %d\n", profile.Port)
	_, _ = fmt.Fprintf(w, "  User: %s\n", profile.User)
	if profile.Password != "" {
		_, _ = fmt.Fprintln(w, "  Password: ********")
	}
	if profile.Catalog != "" {
		_, _ = fmt.Fprintf(w, "  Catalog: %s\n", profile.Catalog)
	}
	if profile.Schema != "" {
		_, _ = fmt.Fprintf(w, "  Schema: %s\n", profile.Schema)
	}
	if profile.SSL.Enabled != nil {
		_, _ = fmt.Fprintf(w, "  SSL: %v\n", *profile.SSL.Enabled)
	}
	if profile.SSL.Insecure {
		_, _ = fmt.Fprintln(w, "  SSL Insecure: true")
	}
	return nil
}
