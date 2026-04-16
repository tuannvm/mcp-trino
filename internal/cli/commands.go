package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

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
	out    io.Writer
	errOut io.Writer
}

// NewCommands creates a new CLI commands handler
func NewCommands(client TrinoClient, format string) *Commands {
	if format == "" {
		format = "table"
	}
	return &Commands{
		client: client,
		format: format,
		out:    os.Stdout,
		errOut: os.Stderr,
	}
}

// NewCommandsWithWriters creates a new CLI commands handler with custom writers
func NewCommandsWithWriters(client TrinoClient, format string, out, errOut io.Writer) *Commands {
	if format == "" {
		format = "table"
	}
	return &Commands{
		client: client,
		format: format,
		out:    out,
		errOut: errOut,
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

	fmt.Fprintln(c.out, "Catalogs:")
	for _, catalog := range catalogs {
		fmt.Fprintf(c.out, "  - %s\n", catalog)
	}
	return nil
}

// Schemas lists schemas in a catalog
func (c *Commands) Schemas(ctx context.Context, catalog string) error {
	if catalog == "" {
		catalog = os.Getenv("TRINO_CATALOG")
		if catalog == "" {
			catalog = "memory"
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

	fmt.Fprintf(c.out, "Schemas in catalog '%s':\n", catalog)
	for _, schema := range schemas {
		fmt.Fprintf(c.out, "  - %s\n", schema)
	}
	return nil
}

// Tables lists tables in a schema
func (c *Commands) Tables(ctx context.Context, catalog, schema string) error {
	if catalog == "" {
		catalog = os.Getenv("TRINO_CATALOG")
		if catalog == "" {
			catalog = "memory"
		}
	}
	if schema == "" {
		schema = os.Getenv("TRINO_SCHEMA")
		if schema == "" {
			schema = "default"
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

	fmt.Fprintf(c.out, "Tables in %s.%s:\n", catalog, schema)
	for _, table := range tables {
		fmt.Fprintf(c.out, "  - %s\n", table)
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

	fmt.Fprintf(c.out, "Table: %s\n", table)
	fmt.Fprintln(c.out, "\nColumns:")
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
		fmt.Fprintf(c.out, "  - %-30s %-20s%s\n", colName, colType, extra)
	}
	fmt.Fprintf(c.out, "\n%d column(s)\n", len(schemaInfo.Rows))
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

	fmt.Fprintf(c.out, "Query Plan for: %s\n\n", query)
	for _, row := range result.Rows {
		for _, val := range row {
			fmt.Fprintf(c.out, "%v\n", val)
		}
	}
	return nil
}

// formatOutput dispatches to the appropriate output formatter
func (c *Commands) formatOutput(results interface{}) error {
	switch c.format {
	case "json":
		return c.outputJSON(results)
	case "csv":
		return c.outputCSV(results)
	default:
		return c.outputTable(results)
	}
}

func (c *Commands) outputJSON(data interface{}) error {
	encoder := json.NewEncoder(c.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (c *Commands) outputCSV(results interface{}) error {
	queryResults, ok := results.(*trino.QueryResult)
	if !ok {
		return fmt.Errorf("invalid result type")
	}

	if len(queryResults.Rows) == 0 {
		fmt.Fprintln(c.out, "No results")
		return nil
	}

	columns := extractSortedColumns(queryResults.Rows[0])

	w := csv.NewWriter(c.out)

	// Write header
	if err := w.Write(columns); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	record := make([]string, len(columns))
	for _, row := range queryResults.Rows {
		for i, col := range columns {
			record[i] = fmt.Sprintf("%v", row[col])
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("CSV write error: %w", err)
	}

	if queryResults.Truncated {
		fmt.Fprintf(c.out, "# %d row(s) (truncated, max %d)\n", len(queryResults.Rows), queryResults.MaxRows)
	}
	return nil
}

func (c *Commands) outputTable(results interface{}) error {
	queryResults, ok := results.(*trino.QueryResult)
	if !ok {
		return fmt.Errorf("invalid result type")
	}

	if len(queryResults.Rows) == 0 {
		fmt.Fprintln(c.out, "No results")
		return nil
	}

	columns := extractSortedColumns(queryResults.Rows[0])

	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintln(tw, strings.Join(columns, "\t"))

	// Separator
	seps := make([]string, len(columns))
	for i, col := range columns {
		width := len(col)
		for _, row := range queryResults.Rows {
			strVal := fmt.Sprintf("%v", row[col])
			if len(strVal) > width {
				width = len(strVal)
			}
		}
		seps[i] = strings.Repeat("-", width)
	}
	fmt.Fprintln(tw, strings.Join(seps, "\t"))

	// Data rows
	vals := make([]string, len(columns))
	for _, row := range queryResults.Rows {
		for i, col := range columns {
			vals[i] = fmt.Sprintf("%v", row[col])
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("table write error: %w", err)
	}

	if queryResults.Truncated {
		fmt.Fprintf(c.out, "\n%d row(s) (truncated, max %d)\n", len(queryResults.Rows), queryResults.MaxRows)
	} else {
		fmt.Fprintf(c.out, "\n%d row(s)\n", len(queryResults.Rows))
	}
	return nil
}

// extractSortedColumns returns sorted column names from a result row
func extractSortedColumns(row map[string]interface{}) []string {
	columns := make([]string, 0, len(row))
	for col := range row {
		columns = append(columns, col)
	}
	sort.Strings(columns)
	return columns
}
