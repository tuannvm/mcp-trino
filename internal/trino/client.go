package trino

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	_ "github.com/trinodb/trino-go-client/trino"
	"github.com/tuannvm/mcp-trino/internal/config"
)

// Client is a wrapper around Trino client
type Client struct {
	db      *sql.DB
	config  *config.TrinoConfig
	timeout time.Duration
}

// NewClient creates a new Trino client
func NewClient(cfg *config.TrinoConfig) (*Client, error) {
	dsn := fmt.Sprintf("%s://%s:%s@%s:%d?catalog=%s&schema=%s&SSL=%t&SSLInsecure=%t",
		cfg.Scheme,
		url.QueryEscape(cfg.User),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		url.QueryEscape(cfg.Catalog),
		url.QueryEscape(cfg.Schema),
		cfg.SSL,
		cfg.SSLInsecure)

	// The Trino driver registers itself with database/sql on import
	// We can just use sql.Open directly with the trino driver

	// Open a connection
	db, err := sql.Open("trino", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Trino: %w", err)
	}

	// Set connection pool parameters
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			log.Printf("Error closing DB connection: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to ping Trino: %w", err)
	}

	return &Client{
		db:      db,
		config:  cfg,
		timeout: cfg.QueryTimeout,
	}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	return c.db.Close()
}

// isReadOnlyQuery checks if the SQL query is read-only (SELECT, SHOW, DESCRIBE, EXPLAIN)
// This helps prevent SQL injection attacks by restricting the types of queries allowed
func isReadOnlyQuery(query string) bool {
	// Convert to lowercase for case-insensitive comparison and normalize whitespace
	queryLower := strings.ToLower(strings.TrimSpace(query))

	// Replace any newline characters with spaces to normalize the query format
	queryLower = strings.ReplaceAll(queryLower, "\n", " ")
	queryLower = strings.ReplaceAll(queryLower, "\r", " ")

	// Ensure there's at least one space after keywords for proper prefix matching
	if strings.HasPrefix(queryLower, "select") && !strings.HasPrefix(queryLower, "select ") {
		queryLower = "select " + queryLower[6:]
	}
	if strings.HasPrefix(queryLower, "show") && !strings.HasPrefix(queryLower, "show ") {
		queryLower = "show " + queryLower[4:]
	}
	if strings.HasPrefix(queryLower, "describe") && !strings.HasPrefix(queryLower, "describe ") {
		queryLower = "describe " + queryLower[8:]
	}
	if strings.HasPrefix(queryLower, "explain") && !strings.HasPrefix(queryLower, "explain ") {
		queryLower = "explain " + queryLower[7:]
	}
	if strings.HasPrefix(queryLower, "with") && !strings.HasPrefix(queryLower, "with ") {
		queryLower = "with " + queryLower[4:]
	}

	// First check for SQL injection attempts with multiple statements
	if strings.Contains(queryLower, ";") {
		return false
	}

	// Check for write operations anywhere in the query
	writeOperations := []string{
		"insert ", "update ", "delete ", "drop ", "create ", "alter ", "truncate ",
	}

	for _, op := range writeOperations {
		if strings.Contains(queryLower, op) {
			return false
		}
	}

	// Check if query starts with SELECT, SHOW, DESCRIBE, EXPLAIN or WITH (for CTEs)
	// These are generally read-only operations
	readOnlyPrefixes := []string{
		"select ", "show ", "describe ", "explain ", "with ",
	}

	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(queryLower, prefix) {
			return true
		}
	}

	return false
}

// ExecuteQuery executes a SQL query and returns the results
func (c *Client) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	// SQL injection protection: only allow read-only queries unless explicitly allowed in config
	if !c.config.AllowWriteQueries && !isReadOnlyQuery(query) {
		return nil, fmt.Errorf("security restriction: only SELECT, SHOW, DESCRIBE, and EXPLAIN queries are allowed. " +
			"Set TRINO_ALLOW_WRITE_QUERIES=true to enable write operations (at your own risk)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Execute the query
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	// Prepare result container
	results := make([]map[string]interface{}, 0)

	// Iterate through rows
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))

		// Initialize the pointers
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the row into values
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		// Create a map for the current row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			rowMap[col] = val
		}

		results = append(results, rowMap)
	}

	// Check for errors after iterating
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// ListCatalogs returns a list of available catalogs
func (c *Client) ListCatalogs() ([]string, error) {
	results, err := c.ExecuteQuery("SHOW CATALOGS")
	if err != nil {
		return nil, err
	}

	catalogs := make([]string, 0, len(results))
	for _, row := range results {
		if catalog, ok := row["Catalog"].(string); ok {
			catalogs = append(catalogs, catalog)
		}
	}

	return catalogs, nil
}

// ListSchemas returns a list of schemas in the specified catalog
func (c *Client) ListSchemas(catalog string) ([]string, error) {
	if catalog == "" {
		catalog = c.config.Catalog
	}

	query := fmt.Sprintf("SHOW SCHEMAS FROM %s", catalog)
	results, err := c.ExecuteQuery(query)
	if err != nil {
		return nil, err
	}

	schemas := make([]string, 0, len(results))
	for _, row := range results {
		if schema, ok := row["Schema"].(string); ok {
			schemas = append(schemas, schema)
		}
	}

	return schemas, nil
}

// ListTables returns a list of tables in the specified catalog and schema
func (c *Client) ListTables(catalog, schema string) ([]string, error) {
	if catalog == "" {
		catalog = c.config.Catalog
	}
	if schema == "" {
		schema = c.config.Schema
	}

	query := fmt.Sprintf("SHOW TABLES FROM %s.%s", catalog, schema)
	results, err := c.ExecuteQuery(query)
	if err != nil {
		return nil, err
	}

	tables := make([]string, 0, len(results))
	for _, row := range results {
		if table, ok := row["Table"].(string); ok {
			tables = append(tables, table)
		}
	}

	return tables, nil
}

// GetTableSchema returns the schema of a table
func (c *Client) GetTableSchema(catalog, schema, table string) ([]map[string]interface{}, error) {
	// Check if table already contains a fully qualified name (catalog.schema.table)
	parts := strings.Split(table, ".")
	if len(parts) == 3 {
		// If table is already fully qualified, use it directly
		query := fmt.Sprintf("DESCRIBE %s", table)
		return c.ExecuteQuery(query)
	} else if len(parts) == 2 {
		// If table has schema.table format
		schema = parts[0]
		table = parts[1]
		if catalog == "" {
			catalog = c.config.Catalog
		}
	} else {
		// Use provided or default catalog and schema
		if catalog == "" {
			catalog = c.config.Catalog
		}
		if schema == "" {
			schema = c.config.Schema
		}
	}

	query := fmt.Sprintf("DESCRIBE %s.%s.%s", catalog, schema, table)
	return c.ExecuteQuery(query)
}

// ExplainQuery returns the query execution plan for a given SQL query
func (c *Client) ExplainQuery(query string, format string) ([]map[string]interface{}, error) {
	// Build EXPLAIN query with optional TYPE format (LOGICAL|DISTRIBUTED|VALIDATE|IO)
	explainQuery := "EXPLAIN"
	if f := strings.ToUpper(strings.TrimSpace(format)); f != "" {
		switch f {
		case "LOGICAL", "DISTRIBUTED", "VALIDATE", "IO":
			explainQuery = fmt.Sprintf("EXPLAIN (TYPE %s)", f)
		default:
			return nil, fmt.Errorf("invalid EXPLAIN format: %q (allowed: LOGICAL, DISTRIBUTED, VALIDATE, IO)", format)
		}
	}
	explainQuery = fmt.Sprintf("%s %s", explainQuery, query)

	return c.ExecuteQuery(explainQuery)
}
