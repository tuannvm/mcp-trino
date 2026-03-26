package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tuannvm/mcp-trino/internal/trino"
)

// TrinoClient interface defines the methods we need from trino.Client
// This allows us to use mock clients in tests
type TrinoClient interface {
	ExecuteQueryWithContext(ctx context.Context, query string) (*trino.QueryResult, error)
	ListCatalogsWithContext(ctx context.Context) ([]string, error)
	ListSchemasWithContext(ctx context.Context, catalog string) ([]string, error)
	ListTablesWithContext(ctx context.Context, catalog, schema string) ([]string, error)
	GetTableSchemaWithContext(ctx context.Context, catalog, schema, table string) (*trino.QueryResult, error)
	ExplainQueryWithContext(ctx context.Context, query string, format string) (*trino.QueryResult, error)
	Close() error
}

// Commands holds the Trino client for executing CLI commands
type Commands struct {
	client TrinoClient
	format string // output format: table, json, csv
}

// NewCommands creates a new CLI commands handler
func NewCommands(client TrinoClient, format string) *Commands {
	if format == "" {
		format = "table"
	}
	return &Commands{
		client: client,
		format: format,
	}
}

// Query executes a SQL query and displays results
func (c *Commands) Query(ctx context.Context, query string) error {
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	results, err := c.client.ExecuteQueryWithContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	// Format and display results
	return c.formatOutput(results)
}

// Catalogs lists all available catalogs
func (c *Commands) Catalogs(ctx context.Context) error {
	catalogs, err := c.client.ListCatalogsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to list catalogs: %w", err)
	}

	if c.format == "json" {
		return c.outputJSON(map[string]interface{}{
			"catalogs": catalogs,
		})
	}

	// Simple table output
	fmt.Println("Catalogs:")
	for _, catalog := range catalogs {
		fmt.Printf("  - %s\n", catalog)
	}
	return nil
}

// Schemas lists schemas in a catalog
func (c *Commands) Schemas(ctx context.Context, catalog string) error {
	// Use default catalog from config if not specified
	if catalog == "" {
		// Get catalog from environment or default
		catalog = os.Getenv("TRINO_CATALOG")
		if catalog == "" {
			catalog = "memory" // default Trino catalog
		}
	}

	schemas, err := c.client.ListSchemasWithContext(ctx, catalog)
	if err != nil {
		return fmt.Errorf("failed to list schemas: %w", err)
	}

	if c.format == "json" {
		return c.outputJSON(map[string]interface{}{
			"schemas": schemas,
			"catalog": catalog,
		})
	}

	fmt.Printf("Schemas in catalog '%s':\n", catalog)
	for _, schema := range schemas {
		fmt.Printf("  - %s\n", schema)
	}
	return nil
}

// Tables lists tables in a schema
func (c *Commands) Tables(ctx context.Context, catalog, schema string) error {
	// Use defaults from config if not specified
	if catalog == "" {
		catalog = os.Getenv("TRINO_CATALOG")
		if catalog == "" {
			catalog = "memory" // default Trino catalog
		}
	}
	if schema == "" {
		schema = os.Getenv("TRINO_SCHEMA")
		if schema == "" {
			schema = "default" // default Trino schema
		}
	}

	tables, err := c.client.ListTablesWithContext(ctx, catalog, schema)
	if err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}

	if c.format == "json" {
		return c.outputJSON(map[string]interface{}{
			"tables":  tables,
			"catalog": catalog,
			"schema":  schema,
		})
	}

	fmt.Printf("Tables in %s.%s:\n", catalog, schema)
	for _, table := range tables {
		fmt.Printf("  - %s\n", table)
	}
	return nil
}

// Describe shows the schema of a table
func (c *Commands) Describe(ctx context.Context, table string) error {
	if table == "" {
		return fmt.Errorf("table name is required (format: table, schema.table, or catalog.schema.table)")
	}

	schemaInfo, err := c.client.GetTableSchemaWithContext(ctx, "", "", table)
	if err != nil {
		return fmt.Errorf("failed to get table schema: %w", err)
	}

	if c.format == "json" {
		return c.outputJSON(schemaInfo)
	}

	// Use the requested table name for display
	fmt.Printf("Table: %s\n", table)
	fmt.Println("\nColumns:")
	for _, row := range schemaInfo.Rows {
		colName := fmt.Sprintf("%v", row["Column"])
		colType := fmt.Sprintf("%v", row["Type"])
		extra := ""
		if nullable, ok := row["Extra"].(string); ok && nullable != "" {
			extra = fmt.Sprintf(" (%s)", nullable)
		}
		if comment, ok := row["Comment"].(string); ok && comment != "" {
			if extra != "" {
				extra += " "
			}
			extra += fmt.Sprintf("# %s", comment)
		}
		fmt.Printf("  - %-30s %-20s%s\n", colName, colType, extra)
	}
	fmt.Printf("\n%d column(s)\n", len(schemaInfo.Rows))
	return nil
}

// Explain analyzes a query execution plan
func (c *Commands) Explain(ctx context.Context, query string, formatOpt string) error {
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	result, err := c.client.ExplainQueryWithContext(ctx, query, formatOpt)
	if err != nil {
		return fmt.Errorf("failed to explain query: %w", err)
	}

	if c.format == "json" {
		return c.outputJSON(map[string]interface{}{
			"query": query,
			"plan":  result,
		})
	}

	// Print the query plan from the result rows
	fmt.Printf("Query Plan for: %s\n\n", query)
	for _, row := range result.Rows {
		// EXPLAIN results typically have a single column with the plan
		for _, val := range row {
			fmt.Printf("%v\n", val)
		}
	}
	return nil
}

// Helper functions

func (c *Commands) formatOutput(results interface{}) error {
	switch c.format {
	case "json":
		return c.outputJSON(results)
	case "csv":
		return c.outputCSV(results)
	default:
		// Default table formatting
		return c.outputTable(results)
	}
}

func (c *Commands) outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (c *Commands) outputCSV(results interface{}) error {
	// Type assertion for query results
	queryResults, ok := results.(*trino.QueryResult)
	if !ok {
		return fmt.Errorf("invalid result type")
	}

	if len(queryResults.Rows) == 0 {
		fmt.Println("No results")
		return nil
	}

	// Extract column names from the first row and sort for deterministic output
	columns := make([]string, 0, len(queryResults.Rows[0]))
	for col := range queryResults.Rows[0] {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Write CSV header
	for i, col := range columns {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf("%q", col)
	}
	fmt.Println()

	// Write data rows
	for _, row := range queryResults.Rows {
		for i, col := range columns {
			if i > 0 {
				fmt.Print(",")
			}
			// Convert value to string and quote it
			val := fmt.Sprintf("%v", row[col])
			fmt.Printf("%q", val)
		}
		fmt.Println()
	}

	if queryResults.Truncated {
		fmt.Printf("# %d row(s) (truncated, max %d)\n", len(queryResults.Rows), queryResults.MaxRows)
	}
	return nil
}

func (c *Commands) outputTable(results interface{}) error {
	// Type assertion for query results
	queryResults, ok := results.(*trino.QueryResult)
	if !ok {
		return fmt.Errorf("invalid result type")
	}

	if len(queryResults.Rows) == 0 {
		fmt.Println("No results")
		return nil
	}

	// Extract column names from the first row and sort for deterministic output
	columns := make([]string, 0, len(queryResults.Rows[0]))
	for col := range queryResults.Rows[0] {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Calculate column widths
	colWidths := make([]int, len(columns))
	for i, col := range columns {
		colWidths[i] = len(col)
	}
	for _, row := range queryResults.Rows {
		for i, col := range columns {
			strVal := fmt.Sprintf("%v", row[col])
			if len(strVal) > colWidths[i] {
				colWidths[i] = len(strVal)
			}
		}
	}

	// Print header
	for i, col := range columns {
		fmt.Printf("%-*s", colWidths[i]+2, col)
	}
	fmt.Println()

	// Print separator
	for _, width := range colWidths {
		fmt.Printf("%-*s", width+2, strings.Repeat("-", width))
	}
	fmt.Println()

	// Print data rows
	for _, row := range queryResults.Rows {
		for i, col := range columns {
			fmt.Printf("%-*v", colWidths[i]+2, row[col])
		}
		fmt.Println()
	}

	if queryResults.Truncated {
		fmt.Printf("\n%d row(s) (truncated, max %d)\n", len(queryResults.Rows), queryResults.MaxRows)
	} else {
		fmt.Printf("\n%d row(s)\n", len(queryResults.Rows))
	}
	return nil
}

