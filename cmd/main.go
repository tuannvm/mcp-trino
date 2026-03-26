package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/tuannvm/mcp-trino/internal/config"
	"github.com/tuannvm/mcp-trino/internal/mcp"
	"github.com/tuannvm/mcp-trino/internal/trino"
)

// These variables will be set during the build via ldflags
var (
	// Version is the server version, set by the build process
	Version = "dev"
)

// Context keys are now imported from auth package

func main() {
	// Detect mode: CLI or MCP
	// Priority order:
	// 1. MCP_PROTOCOL_VERSION env var → MCP mode (explicit handshake)
	// 2. --mcp flag → MCP mode (explicit user choice)
	// 3. Any CLI command/flag → CLI mode
	// 4. If stdin is a TTY (interactive terminal) → CLI help
	// 5. Otherwise → MCP mode (default for backward compatibility with hosts)

	args := os.Args[1:]

	// Check for version flag first (works for both modes)
	for _, arg := range args {
		if arg == "--version" || arg == "-v" {
			println("mcp-trino version", Version)
			return
		}
	}

	// Check for explicit mode selection
	explicitMCP := false
	explicitCLI := false
	for _, arg := range args {
		if arg == "--mcp" {
			explicitMCP = true
			break
		}
		if arg == "--cli" {
			explicitCLI = true
			break
		}
	}

	// Check for MCP protocol version (set by MCP clients)
	_, hasMCPProtocol := os.LookupEnv("MCP_PROTOCOL_VERSION")

	// Determine mode
	if explicitMCP || (hasMCPProtocol && !explicitCLI) {
		// MCP mode
		runMCPServer()
		return
	}

	if explicitCLI || shouldRunCLIMode(args) {
		// CLI mode
		if err := RunCLIMode(); err != nil {
			log.Fatalf("CLI error: %v", err)
		}
		return
	}

	// Default: if no args and no MCP context, check if we're on a TTY
	// If TTY, show CLI help; otherwise assume MCP mode for backward compatibility
	if len(args) == 0 {
		// Check for MCP_TRANSPORT environment variable - if set, always run MCP server
		if getEnv("MCP_TRANSPORT", "") != "" {
			runMCPServer()
			return
		}

		if isTTY() {
			// Interactive terminal - show CLI help
			if err := RunCLIMode(); err != nil {
				log.Fatalf("CLI error: %v", err)
			}
			return
		}
		// Non-interactive (piped, redirected, etc.) - assume MCP mode for backward compatibility
		// Most MCP hosts use stdio pipes without setting MCP_PROTOCOL_VERSION upfront
		runMCPServer()
		return
	}

	// Check if we have CLI-only flags (no subcommand but CLI flags present)
	// In this case, show CLI help (user likely wants CLI usage)
	if hasCLIOnlyFlags(args) {
		if err := RunCLIMode(); err != nil {
			log.Fatalf("CLI error: %v", err)
		}
		return
	}

	// Default to MCP mode for backward compatibility
	runMCPServer()
}

func runMCPServer() {
	log.Println("Starting Trino MCP Server...")

	// Initialize Trino configuration
	log.Println("Loading Trino configuration...")
	trinoConfig, err := config.NewTrinoConfigWithVersion(Version)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize Trino client
	log.Println("Connecting to Trino server...")
	trinoClient, err := trino.NewClient(trinoConfig)
	if err != nil {
		log.Fatalf("Failed to initialize Trino client: %v", err)
	}
	defer func() {
		if err := trinoClient.Close(); err != nil {
			log.Printf("Error closing Trino client: %v", err)
		}
	}()

	// Test connection by listing catalogs
	log.Println("Testing Trino connection...")
	catalogs, err := trinoClient.ListCatalogsWithContext(context.Background())
	if err != nil {
		log.Fatalf("Failed to connect to Trino: %v", err)
	}
	log.Printf("Connected to Trino server. Available catalogs: %s", strings.Join(catalogs, ", "))

	// Create MCP server
	log.Println("Initializing MCP server...")
	server := mcp.NewServer(trinoClient, trinoConfig, Version)

	// Choose server mode
	transport := getEnv("MCP_TRANSPORT", "stdio")

	log.Printf("Starting MCP server with %s transport...", transport)
	switch transport {
	case "stdio":
		if err := server.ServeStdio(); err != nil {
			log.Fatalf("STDIO server error: %v", err)
		}
	case "http":
		port := getEnv("MCP_PORT", "8080")
		if err := server.ServeHTTP(port); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	default:
		log.Fatalf("Unsupported transport: %s", transport)
	}

	log.Println("Server shutdown complete")
}

// shouldRunCLIMode determines if we should run in CLI mode based on arguments
func shouldRunCLIMode(args []string) bool {
	// Check for explicit CLI flags
	for _, arg := range args {
		if arg == "--cli" || arg == "--interactive" {
			return true
		}
	}

	// Check for CLI subcommands (first non-flag argument)
	cliCommands := map[string]bool{
		"query":       true,
		"catalogs":    true,
		"schemas":     true,
		"tables":      true,
		"describe":    true,
		"explain":     true,
		"interactive": true,
		"config":      true, // config profile management
	}

	for _, arg := range args {
		// Skip flags
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Only return true if it's a known CLI command
		// Unknown positional args should NOT trigger CLI mode (preserves MCP compatibility)
		if cliCommands[arg] {
			return true
		}
		// Unknown positional argument - don't assume CLI mode
		// This preserves backward compatibility for MCP integrations
	}

	return false
}

// isTTY checks if stdin is a terminal (interactive)
func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// hasCLIOnlyFlags checks if args contain CLI-only flags (no subcommand)
func hasCLIOnlyFlags(args []string) bool {
	cliFlags := map[string]bool{
		"--help":     true,
		"-h":         true,
		"--version":  true,
		"-v":         true,
		"--config":   true,
		"--format":   true,
		"--host":     true,
		"--port":     true,
		"--user":     true,
		"--password": true,
		"--catalog":  true,
		"--schema":   true,
		"--profile":  true, // profile selection is CLI-specific
		"--interactive": true,
	}

	// Check if we have any CLI flags
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			flagName := strings.Split(arg, "=")[0]
			if cliFlags[flagName] {
				return true
			}
		}
		if strings.HasPrefix(arg, "-") && len(arg) == 2 {
			if cliFlags[arg] {
				return true
			}
		}
	}
	return false
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
