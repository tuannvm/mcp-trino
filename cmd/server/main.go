package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tuannvm/mcp-trino/internal/config"
	"github.com/tuannvm/mcp-trino/internal/handlers"
	"github.com/tuannvm/mcp-trino/internal/trino"
)

// These variables will be set during the build via ldflags
var (
	// Version is the server version, set by the build process
	Version = "dev"
)

func main() {
	log.Println("Starting Trino MCP Server...")

	// Initialize Trino configuration
	log.Println("Loading Trino configuration...")
	trinoConfig := config.NewTrinoConfig()

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

	// Create and initialize MCP server
	log.Println("Initializing MCP server...")
	mcpServer := server.NewMCPServer("Trino MCP Server", Version)

	// Initialize tool handlers
	trinoHandlers := handlers.NewTrinoHandlers(trinoClient)

	// Register Trino tools
	registerTrinoTools(mcpServer, trinoHandlers)

	// Choose server mode based on environment
	transport := getEnv("MCP_TRANSPORT", "stdio")

	// Setup graceful shutdown
	done := make(chan bool, 1)
	go handleSignals(done)

	// Start the server
	log.Printf("Starting Trino MCP Server with %s transport...", transport)
	switch transport {
	case "stdio":
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Fatalf("STDIO server error: %v", err)
		}
	case "http":
		// HTTP server implementation
		port := getEnv("MCP_PORT", "9097")
		host := getEnv("MCP_HOST", "localhost")
		addr := fmt.Sprintf(":%s", port)

		// Create SSE server for MCP
		log.Println("Creating SSE server...")
		baseURL := fmt.Sprintf("http://%s:%s", host, port)
		sseServer := server.NewSSEServer(mcpServer,
			server.WithSSEEndpoint("/sse"),        // Set the SSE endpoint to /sse for Cursor
			server.WithMessageEndpoint("/api/v1"), // Set the message endpoint
			server.WithKeepAlive(true),
			server.WithBaseURL(baseURL),                   // Explicitly set the base URL with the correct port
			server.WithUseFullURLForMessageEndpoint(true), // Use full URLs for message endpoints
		)
		log.Printf("SSE server created with endpoint: %s", sseServer.CompleteSsePath())
		if endpoint, err := sseServer.CompleteSseEndpoint(); err != nil {
			log.Printf("Error getting SSE endpoint URL: %v", err)
		} else {
			log.Printf("Full SSE endpoint URL: %s", endpoint)
		}
		log.Printf("Message endpoint: %s", sseServer.CompleteMessagePath())
		if endpoint, err := sseServer.CompleteMessageEndpoint(); err != nil {
			log.Printf("Error getting message endpoint URL: %v", err)
		} else {
			log.Printf("Full Message endpoint URL: %s", endpoint)
		}

		// Create an HTTP server
		log.Printf("Starting HTTP server on %s", addr)
		httpServer := &http.Server{
			Addr: addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				log.Printf("Received request: %s %s", r.Method, r.URL.Path)

				// Set CORS headers
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")

				// Handle preflight OPTIONS requests
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusOK)
					return
				}

				// Handle SSE requests with proper content-type
				if r.URL.Path == "/sse" {
					log.Printf("Handling SSE request to /sse")
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					w.Header().Set("Connection", "keep-alive")
					sseServer.ServeHTTP(w, r)
					return
				}

				if r.Method == http.MethodPost && r.URL.Path == "/api/query" {
					handleTrinoQuery(w, r, trinoClient)
					return
				}

				// Add a GET handler for root path
				if r.Method == http.MethodGet && r.URL.Path == "/" {
					handleStatus(w, r)
					return
				}

				// Handle all other requests
				log.Printf("Delegating request to SSE server: %s %s", r.Method, r.URL.Path)
				sseServer.ServeHTTP(w, r)
			}),
		}

		// Start HTTP server in goroutine
		go func() {
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()

		// Wait for shutdown signal
		<-done
		log.Println("Shutting down HTTP server...")
		if err := httpServer.Close(); err != nil {
			log.Printf("Error closing HTTP server: %v", err)
		}
	default:
		log.Fatalf("Unsupported transport: %s", transport)
	}

	log.Println("Server shutdown complete")
}

// handleTrinoQuery handles HTTP requests for Trino queries
func handleTrinoQuery(w http.ResponseWriter, r *http.Request, client *trino.Client) {
	if client == nil {
		http.Error(w, "Trino client not available", http.StatusServiceUnavailable)
		return
	}

	// Parse request
	var request struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Execute query (SQL injection protection is handled in the client)
	results, err := client.ExecuteQuery(request.Query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Return results as JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
		return
	}
}

// registerTrinoTools registers all Trino-related tools with the MCP server
func registerTrinoTools(mcpServer *server.MCPServer, h *handlers.TrinoHandlers) {
	// Register ExecuteQuery tool
	executeQueryTool := mcp.NewTool("execute_query",
		mcp.WithDescription("Execute a SQL query against the Trino server"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SQL query to execute"),
		),
	)
	mcpServer.AddTool(executeQueryTool, h.ExecuteQuery)

	// Register ListCatalogs tool
	listCatalogsTool := mcp.NewTool("list_catalogs",
		mcp.WithDescription("List all available catalogs in the Trino server"),
	)
	mcpServer.AddTool(listCatalogsTool, h.ListCatalogs)

	// Register ListSchemas tool
	listSchemasTool := mcp.NewTool("list_schemas",
		mcp.WithDescription("List all schemas in a catalog"),
		mcp.WithString("catalog",
			mcp.Description("The catalog to list schemas from (optional)"),
		),
	)
	mcpServer.AddTool(listSchemasTool, h.ListSchemas)

	// Register ListTables tool
	listTablesTool := mcp.NewTool("list_tables",
		mcp.WithDescription("List all tables in a schema"),
		mcp.WithString("catalog",
			mcp.Description("The catalog containing the schema (optional)"),
		),
		mcp.WithString("schema",
			mcp.Description("The schema to list tables from (optional)"),
		),
	)
	mcpServer.AddTool(listTablesTool, h.ListTables)

	// Register GetTableSchema tool
	getTableSchemaTool := mcp.NewTool("get_table_schema",
		mcp.WithDescription("Get the schema of a table"),
		mcp.WithString("catalog",
			mcp.Description("The catalog containing the table (optional)"),
		),
		mcp.WithString("schema",
			mcp.Description("The schema containing the table (optional)"),
		),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("The table to get the schema for"),
		),
	)
	mcpServer.AddTool(getTableSchemaTool, h.GetTableSchema)
}

// handleSignals handles OS signals for graceful shutdown
func handleSignals(done chan<- bool) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	log.Println("Received shutdown signal, shutting down...")
	done <- true
}

// handleStatus responds to GET requests with server status
func handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "ok",
		"version": Version,
		"server":  "Trino MCP Server",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
		return
	}
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
