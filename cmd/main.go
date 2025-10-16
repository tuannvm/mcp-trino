package main

import (
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
	catalogs, err := trinoClient.ListCatalogs()
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

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
