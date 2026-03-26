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

// RunCLIMode executes the CLI mode
func RunCLIMode() error {
	// Strip mode selection flags (--cli, --mcp) from args before parsing
	args := cleanArgs(os.Args[1:])

	// Define CLI flags
	flagSet := flag.NewFlagSet("mcp-trino", flag.ExitOnError)
	configFile := flagSet.String("config", "", "Path to config file")
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
		fmt.Println()
		fmt.Println("Flags:")
		flagSet.PrintDefaults()
		fmt.Println()
		fmt.Println("Environment Variables:")
		fmt.Println("  TRINO_HOST, TRINO_PORT, TRINO_USER, TRINO_PASSWORD")
		fmt.Println("  TRINO_CATALOG, TRINO_SCHEMA")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  mcp-trino query 'SELECT 1'")
		fmt.Println("  mcp-trino catalogs")
		fmt.Println("  mcp-trino schemas tpch")
		fmt.Println("  mcp-trino tables tpch tiny")
		fmt.Println("  mcp-trino describe tpch.tiny.orders")
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
		cliConfig, parseErr = cli.ParseCLIConfig(data)
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
	cliConfig.ApplyToEnv()

	// Apply CLI flags to environment (flags take precedence over config file)
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
		catalog := ""
		if len(commandArgs) > 0 {
			catalog = commandArgs[0]
		}
		return commands.Schemas(ctx, catalog)

	case "tables":
		catalog, schema := "", ""
		if len(commandArgs) > 0 {
			catalog = commandArgs[0]
		}
		if len(commandArgs) > 1 {
			schema = commandArgs[1]
		}
		return commands.Tables(ctx, catalog, schema)

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
