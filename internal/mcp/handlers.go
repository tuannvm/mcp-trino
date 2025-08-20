package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tuannvm/mcp-trino/internal/trino"
)

// TrinoHandlers contains all handlers for Trino-related tools
type TrinoHandlers struct {
	TrinoClient *trino.Client
}

// NewTrinoHandlers creates a new set of Trino handlers
func NewTrinoHandlers(client *trino.Client) *TrinoHandlers {
	return &TrinoHandlers{
		TrinoClient: client,
	}
}

// ExecuteQuery handles query execution
func (h *TrinoHandlers) ExecuteQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	// Type assert Arguments to map[string]interface{}
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		mcpErr := fmt.Errorf("invalid arguments format")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Extract the query parameter
	query, ok := args["query"].(string)
	if !ok {
		mcpErr := fmt.Errorf("query parameter must be a string")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Execute the query - SQL injection protection is handled within the client
	results, err := h.TrinoClient.ExecuteQuery(query)
	if err != nil {
		log.Printf("Error executing query: %v", err)
		mcpErr := fmt.Errorf("query execution failed: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Convert results to JSON string for display
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		mcpErr := fmt.Errorf("failed to marshal results to JSON: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Return the results as formatted JSON text
	return mcp.NewToolResultText(string(jsonData)), nil
}

// ListCatalogs handles catalog listing
func (h *TrinoHandlers) ListCatalogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	catalogs, err := h.TrinoClient.ListCatalogs()
	if err != nil {
		log.Printf("Error listing catalogs: %v", err)
		mcpErr := fmt.Errorf("failed to list catalogs: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Convert catalogs to JSON string for display
	jsonData, err := json.MarshalIndent(catalogs, "", "  ")
	if err != nil {
		mcpErr := fmt.Errorf("failed to marshal catalogs to JSON: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// ListSchemas handles schema listing
func (h *TrinoHandlers) ListSchemas(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	// Type assert Arguments to map[string]interface{}
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		mcpErr := fmt.Errorf("invalid arguments format")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Extract catalog parameter (optional)
	var catalog string
	if catalogParam, ok := args["catalog"].(string); ok {
		catalog = catalogParam
	}

	schemas, err := h.TrinoClient.ListSchemas(catalog)
	if err != nil {
		log.Printf("Error listing schemas: %v", err)
		mcpErr := fmt.Errorf("failed to list schemas: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Convert schemas to JSON string for display
	jsonData, err := json.MarshalIndent(schemas, "", "  ")
	if err != nil {
		mcpErr := fmt.Errorf("failed to marshal schemas to JSON: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// ListTables handles table listing
func (h *TrinoHandlers) ListTables(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	// Type assert Arguments to map[string]interface{}
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		mcpErr := fmt.Errorf("invalid arguments format")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Extract catalog and schema parameters (optional)
	var catalog, schema string
	if catalogParam, ok := args["catalog"].(string); ok {
		catalog = catalogParam
	}
	if schemaParam, ok := args["schema"].(string); ok {
		schema = schemaParam
	}

	tables, err := h.TrinoClient.ListTables(catalog, schema)
	if err != nil {
		log.Printf("Error listing tables: %v", err)
		mcpErr := fmt.Errorf("failed to list tables: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Convert tables to JSON string for display
	jsonData, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		mcpErr := fmt.Errorf("failed to marshal tables to JSON: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// GetTableSchema handles table schema retrieval
func (h *TrinoHandlers) GetTableSchema(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	// Type assert Arguments to map[string]interface{}
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		mcpErr := fmt.Errorf("invalid arguments format")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Extract parameters
	var catalog, schema string
	var table string

	if catalogParam, ok := args["catalog"].(string); ok {
		catalog = catalogParam
	}
	if schemaParam, ok := args["schema"].(string); ok {
		schema = schemaParam
	}

	// Table parameter is required
	tableParam, ok := args["table"].(string)
	if !ok {
		mcpErr := fmt.Errorf("table parameter is required")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}
	table = tableParam

	tableSchema, err := h.TrinoClient.GetTableSchema(catalog, schema, table)
	if err != nil {
		log.Printf("Error getting table schema: %v", err)
		mcpErr := fmt.Errorf("failed to get table schema: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Convert table schema to JSON string for display
	jsonData, err := json.MarshalIndent(tableSchema, "", "  ")
	if err != nil {
		mcpErr := fmt.Errorf("failed to marshal table schema to JSON: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// ExplainQuery handles query plan analysis
func (h *TrinoHandlers) ExplainQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	// Type assert Arguments to map[string]interface{}
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		mcpErr := fmt.Errorf("invalid arguments format")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Extract the query parameter
	query, ok := args["query"].(string)
	if !ok {
		mcpErr := fmt.Errorf("query parameter must be a string")
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Extract optional format parameter
	var format string
	if formatParam, ok := args["format"].(string); ok {
		format = formatParam
	}

	// Execute the explain query
	results, err := h.TrinoClient.ExplainQuery(query, format)
	if err != nil {
		log.Printf("Error explaining query: %v", err)
		mcpErr := fmt.Errorf("query explanation failed: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	// Convert results to JSON string for display
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		mcpErr := fmt.Errorf("failed to marshal explanation results to JSON: %w", err)
		return mcp.NewToolResultErrorFromErr(mcpErr.Error(), mcpErr), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// RegisterTrinoTools registers all Trino-related tools with the MCP server
func RegisterTrinoTools(m *server.MCPServer, h *TrinoHandlers) {
	// Get OAuth middleware if available
	var middleware func(server.ToolHandlerFunc) server.ToolHandlerFunc
	if oauthMiddleware := GetOAuthMiddleware(m); oauthMiddleware != nil {
		middleware = oauthMiddleware
	}

	// Helper function to apply middleware if available
	applyMiddleware := func(handler server.ToolHandlerFunc) server.ToolHandlerFunc {
		if middleware != nil {
			return middleware(handler)
		}
		return handler
	}

	m.AddTool(mcp.NewTool("execute_query",
		mcp.WithDescription("Execute SQL queries on Trino's fast distributed query engine for big data analytics. Run SELECT, SHOW, DESCRIBE, EXPLAIN statements across multiple data sources simultaneously. Perfect for complex analytics, aggregations, joins, and cross-system data exploration on large datasets."),
		mcp.WithString("query", mcp.Required(), mcp.Description("SQL query to execute on Trino cluster (SELECT, SHOW, DESCRIBE, EXPLAIN supported)")),
	), applyMiddleware(h.ExecuteQuery))

	m.AddTool(mcp.NewTool("list_catalogs", mcp.WithDescription("Discover available Trino catalogs - each catalog represents a connector to different data systems (PostgreSQL, MySQL, S3, HDFS, Kafka, etc.). Catalogs are your entry point to querying data across heterogeneous systems in a single SQL query.")),
		applyMiddleware(h.ListCatalogs))

	m.AddTool(mcp.NewTool("list_schemas",
		mcp.WithDescription("Browse schemas (databases/namespaces) within a Trino catalog. Each schema contains related tables and views. Use this to navigate the data hierarchy before querying specific datasets."),
		mcp.WithString("catalog", mcp.Description("Trino catalog name (optional; defaults to server configuration if omitted)"))),
		applyMiddleware(h.ListSchemas))

	m.AddTool(mcp.NewTool("list_tables",
		mcp.WithDescription("Discover tables and views available for querying in Trino schemas. Essential for finding datasets to analyze. Can scope to specific catalog/schema or browse all available data across the distributed system."),
		mcp.WithString("catalog", mcp.Description("Trino catalog name (optional)")),
		mcp.WithString("schema", mcp.Description("Schema name within catalog (optional)"))),
		applyMiddleware(h.ListTables))

	m.AddTool(mcp.NewTool("get_table_schema",
		mcp.WithDescription("Inspect table structure and column metadata from Trino's distributed data sources. Shows column names, data types, nullability, and constraints. Critical for understanding data before writing analytical queries."),
		mcp.WithString("catalog", mcp.Description("Trino catalog containing the table (optional)")),
		mcp.WithString("schema", mcp.Description("Schema containing the table (optional)")),
		mcp.WithString("table", mcp.Required(), mcp.Description("Table name to inspect"))),
		applyMiddleware(h.GetTableSchema))

	m.AddTool(mcp.NewTool("explain_query",
		mcp.WithDescription("Analyze Trino query execution plans without running expensive queries. Shows distributed execution stages, data movement between nodes, and resource estimates. Essential for query optimization and performance tuning."),
		mcp.WithString("query", mcp.Required(), mcp.Description("SQL query to analyze (SELECT, JOIN, aggregations, etc.)")),
		mcp.WithString("format", mcp.Description("Plan type: LOGICAL, DISTRIBUTED, VALIDATE, or IO (optional)"))),
		applyMiddleware(h.ExplainQuery))
}
