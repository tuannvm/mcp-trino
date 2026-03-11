package trino

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/trinodb/trino-go-client/trino"
	"github.com/tuannvm/mcp-trino/internal/config"
	oauth "github.com/tuannvm/oauth-mcp-proxy"
)

// Pre-compiled regexes for read-only query detection
var (
	readOnlyPrefixPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*select\b`),
		regexp.MustCompile(`^\s*show\b`),
		regexp.MustCompile(`^\s*describe\b`),
		regexp.MustCompile(`^\s*explain\b`),
		regexp.MustCompile(`^\s*with\b`),
	}

	showCreatePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*show\s+create\s+table\b`),
		regexp.MustCompile(`^\s*show\s+create\s+view\b`),
		regexp.MustCompile(`^\s*show\s+create\s+schema\b`),
		regexp.MustCompile(`^\s*show\s+create\s+materialized\s+view\b`),
	}

	showPrefixPattern = regexp.MustCompile(`^\s*show\b`)

	safeStartPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*select\b`),
		regexp.MustCompile(`^\s*describe\b`),
		regexp.MustCompile(`^\s*explain\b`),
		regexp.MustCompile(`^\s*with\b`),
	}

	// Pre-compiled write operation patterns
	writeOpPatterns     []*regexp.Regexp
	writeOpsExceptCreate []*regexp.Regexp

	// Pre-compiled sanitization patterns
	singleQuoteLiteral = regexp.MustCompile(`'(?:[^']|'')*'`)
	doubleQuoteIdent   = regexp.MustCompile(`"(?:[^"]|"")*"`)
	backtickIdent      = regexp.MustCompile("`[^`]*`")
	singleLineComment  = regexp.MustCompile(`--[^\r\n]*`)
	multiLineComment   = regexp.MustCompile(`/\*[^*]*\*+(?:[^/*][^*]*\*+)*/`)
)

func init() {
	writeOps := []string{
		"insert", "update", "delete", "drop", "create", "alter", "truncate",
		"merge", "copy", "grant", "revoke", "commit", "rollback",
		"call", "execute", "refresh", "set", "reset",
	}
	writeOpPatterns = make([]*regexp.Regexp, len(writeOps))
	for i, op := range writeOps {
		writeOpPatterns[i] = regexp.MustCompile(fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(op)))
	}

	writeOpsNoCreate := []string{
		"insert", "update", "delete", "drop", "alter", "truncate",
		"merge", "copy", "grant", "revoke", "commit", "rollback",
		"call", "execute", "refresh", "set", "reset",
	}
	writeOpsExceptCreate = make([]*regexp.Regexp, len(writeOpsNoCreate))
	for i, op := range writeOpsNoCreate {
		writeOpsExceptCreate[i] = regexp.MustCompile(fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(op)))
	}
}

// Context key for impersonated user
type contextKey string

const (
	impersonatedUserKey contextKey = "impersonated_user"
)

// headerRoundTripper adds X-Trino-Source and X-Trino-User headers to requests
type headerRoundTripper struct {
	base   http.RoundTripper
	config *config.TrinoConfig
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())

	// Set X-Trino-Source header for query attribution
	if t.config.TrinoSource != "" {
		req.Header.Set("X-Trino-Source", t.config.TrinoSource)
	}

	// Set X-Trino-User header if impersonation is enabled
	if t.config.EnableImpersonation {
		if user, ok := req.Context().Value(impersonatedUserKey).(string); ok && user != "" {
			req.Header.Set("X-Trino-User", user)
		}
	}

	return t.base.RoundTrip(req)
}

// Client is a wrapper around Trino client
type Client struct {
	db      *sql.DB
	config  *config.TrinoConfig
	timeout time.Duration
}

// NewClient creates a new Trino client
func NewClient(cfg *config.TrinoConfig) (*Client, error) {
	dsnURL := url.URL{
		Scheme: cfg.Scheme,
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}

	params := url.Values{}
	params.Add("catalog", cfg.Catalog)
	params.Add("schema", cfg.Schema)
	params.Add("SSL", fmt.Sprintf("%t", cfg.SSL))
	params.Add("SSLInsecure", fmt.Sprintf("%t", cfg.SSLInsecure))
	params.Add("custom_client", "mcp-trino")

	dsnURL.RawQuery = params.Encode()
	dsn := dsnURL.String()

	httpClient := &http.Client{
		Transport: &headerRoundTripper{
			base:   http.DefaultTransport,
			config: cfg,
		},
	}
	if err := trino.RegisterCustomClient("mcp-trino", httpClient); err != nil {
		// Ignore "already registered" errors - this can happen in tests or when client is recreated
		if !strings.Contains(err.Error(), "already registered") {
			return nil, fmt.Errorf("failed to register custom HTTP client: %w", err)
		}
	}

	db, err := sql.Open("trino", dsn)
	if err != nil {
		// Sanitize error to prevent password exposure
		sanitizedErr := sanitizeConnectionError(err, cfg.Password)
		return nil, fmt.Errorf("failed to connect to Trino: %w", sanitizedErr)
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
		// Sanitize error to prevent password exposure
		sanitizedErr := sanitizeConnectionError(err, cfg.Password)
		return nil, fmt.Errorf("failed to ping Trino: %w", sanitizedErr)
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

// WithImpersonatedUser adds impersonated user to context
func WithImpersonatedUser(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, impersonatedUserKey, username)
}

// GetImpersonatedUser retrieves impersonated user from context
func GetImpersonatedUser(ctx context.Context) (string, bool) {
	user, ok := ctx.Value(impersonatedUserKey).(string)
	return user, ok
}

// isReadOnlyQuery checks if the SQL query is read-only (SELECT, SHOW, DESCRIBE, EXPLAIN)
// This helps prevent SQL injection attacks by restricting the types of queries allowed
func isReadOnlyQuery(query string) bool {
	// Convert to lowercase for case-insensitive comparison and normalize whitespace
	queryLower := strings.ToLower(strings.TrimSpace(query))

	// Replace any newline characters with spaces to normalize the query format
	queryLower = strings.ReplaceAll(queryLower, "\n", " ")
	queryLower = strings.ReplaceAll(queryLower, "\r", " ")

	// Remove string literals and comments to avoid false positives
	queryLower = sanitizeQueryForKeywordDetection(queryLower)

	// First check for SQL injection attempts with multiple statements
	if strings.Contains(queryLower, ";") {
		return false
	}

	// Check if query starts with SELECT, SHOW, DESCRIBE, EXPLAIN or WITH (for CTEs)
	// These are generally read-only operations. Use word boundaries for robustness.
	// IMPORTANT: This check must come BEFORE write operation detection to avoid false positives
	// (e.g., "SHOW CREATE TABLE" contains "create" but is read-only)
	for _, re := range readOnlyPrefixPatterns {
		if re.MatchString(queryLower) {
			if isAllowedReadOnlyPattern(queryLower) {
				return true
			}
		}
	}

	// Check for write operations anywhere in the query using word boundaries
	//  - https://trino.io/docs/current/sql.html - Main SQL reference
	for _, re := range writeOpPatterns {
		if re.MatchString(queryLower) {
			return false
		}
	}

	return false
}

// isAllowedReadOnlyPattern checks if a query matches known safe read-only patterns
// even if it contains keywords that might look like write operations
func isAllowedReadOnlyPattern(queryLower string) bool {
	// SHOW CREATE statements are read-only (they just display DDL)
	for _, re := range showCreatePatterns {
		if re.MatchString(queryLower) {
			return true
		}
	}

	// Other SHOW statements without CREATE are safe
	if showPrefixPattern.MatchString(queryLower) {
		for _, re := range writeOpsExceptCreate {
			if re.MatchString(queryLower) {
				return false
			}
		}
		return true
	}

	// SELECT, DESCRIBE, EXPLAIN, WITH without write operations are safe
	for _, re := range safeStartPatterns {
		if re.MatchString(queryLower) {
			for _, wre := range writeOpPatterns {
				if wre.MatchString(queryLower) {
					return false
				}
			}
			return true
		}
	}

	return false
}

// sanitizeQueryForKeywordDetection removes string literals, quoted identifiers, and comments
// to prevent false positives when detecting write operations
func sanitizeQueryForKeywordDetection(query string) string {
	query = singleQuoteLiteral.ReplaceAllString(query, "'LITERAL'")
	query = doubleQuoteIdent.ReplaceAllString(query, "\"IDENTIFIER\"")
	query = backtickIdent.ReplaceAllString(query, "`IDENTIFIER`")
	query = singleLineComment.ReplaceAllString(query, "")
	query = multiLineComment.ReplaceAllString(query, "")
	return strings.TrimSpace(query)
}

// getQueryUsername returns the username of the user executing the query if present in OAuth context
// This is used for query attribution (X-Trino-Client-Tags/Info) independent of impersonation
func getQueryUsername(ctx context.Context) string {
	user, exists := oauth.GetUserFromContext(ctx)
	if !exists || user == nil {
		return ""
	}
	if user.Username != "" {
		return user.Username
	}
	if user.Email != "" {
		return user.Email
	}
	if user.Subject != "" {
		return user.Subject
	}
	return ""
}

// QueryResult holds query results along with metadata about truncation.
type QueryResult struct {
	Rows      []map[string]interface{}
	Truncated bool // true if results were truncated by MaxRows limit
	MaxRows   int  // the MaxRows limit that was applied (0 = unlimited)
}

// ExecuteQuery executes a SQL query and returns the results
func (c *Client) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	result, err := c.ExecuteQueryWithContext(context.Background(), query)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// ExecuteQueryWithContext executes a SQL query and returns the results
// It supports both:
// - User impersonation via X-Trino-User header (when EnableImpersonation is true)
// - Query attribution via X-Trino-Client-Tags/Info/Source (from OAuth user context)
func (c *Client) ExecuteQueryWithContext(ctx context.Context, query string) (*QueryResult, error) {
	// Strip trailing semicolon that Trino doesn't allow
	query = strings.TrimSuffix(strings.TrimSpace(query), ";")

	// SQL injection protection: only allow read-only queries unless explicitly allowed in config
	if !c.config.AllowWriteQueries && !isReadOnlyQuery(query) {
		return nil, fmt.Errorf("security restriction: only SELECT, SHOW, DESCRIBE, and EXPLAIN queries are allowed. " +
			"Set TRINO_ALLOW_WRITE_QUERIES=true to enable write operations (at your own risk)")
	}

	// Create context with timeout, preserving any impersonation data
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Build query arguments for attribution headers
	// These are complementary to the X-Trino-User header set by RoundTripper
	var queryArgs []interface{}
	if userName := getQueryUsername(ctx); userName != "" {
		queryArgs = append(queryArgs,
			sql.Named("X-Trino-Client-Tags", userName),
			sql.Named("X-Trino-Client-Info", userName),
		)
		// Only set X-Trino-Source if not already configured globally
		if c.config.TrinoSource == "" {
			queryArgs = append(queryArgs, sql.Named("X-Trino-Source", userName))
		}
	}

	// Execute the query with optional attribution headers
	rows, err := c.db.QueryContext(queryCtx, query, queryArgs...)
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
	maxRows := c.config.MaxRows
	initialCap := 64
	if maxRows > 0 && maxRows < initialCap {
		initialCap = maxRows
	}
	results := make([]map[string]interface{}, 0, initialCap)
	truncated := false

	// Iterate through rows
	for rows.Next() {
		if maxRows > 0 && len(results) >= maxRows {
			truncated = true
			break
		}

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

	// When truncated, close rows immediately to stop server-side streaming
	// before checking rows.Err() which could surface spurious cancel errors
	if truncated {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows after truncation: %v", err)
		}
		log.Printf("WARNING: Result truncated to %d rows (TRINO_MAX_ROWS limit). Add LIMIT to your query or increase TRINO_MAX_ROWS.", maxRows)
	} else {
		// Only check rows.Err() when we consumed the full result set
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating rows: %w", err)
		}
	}

	return &QueryResult{
		Rows:      results,
		Truncated: truncated,
		MaxRows:   maxRows,
	}, nil
}

// ListCatalogs returns a list of available catalogs
func (c *Client) ListCatalogs() ([]string, error) {
	return c.ListCatalogsWithContext(context.Background())
}

// ListCatalogsWithContext returns a list of available catalogs with context
func (c *Client) ListCatalogsWithContext(ctx context.Context) ([]string, error) {
	result, err := c.ExecuteQueryWithContext(ctx, "SHOW CATALOGS")
	if err != nil {
		return nil, err
	}

	catalogs := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if catalog, ok := row["Catalog"].(string); ok {
			catalogs = append(catalogs, catalog)
		}
	}

	// Apply catalog filtering if allowlist is configured
	if len(c.config.AllowedCatalogs) > 0 {
		catalogs = c.filterCatalogs(catalogs)
	}

	return catalogs, nil
}

// ListSchemas returns a list of schemas in the specified catalog
func (c *Client) ListSchemas(catalog string) ([]string, error) {
	return c.ListSchemasWithContext(context.Background(), catalog)
}

// ListSchemasWithContext returns a list of schemas in the specified catalog with context
func (c *Client) ListSchemasWithContext(ctx context.Context, catalog string) ([]string, error) {
	if catalog == "" {
		catalog = c.config.Catalog
	}

	query := fmt.Sprintf("SHOW SCHEMAS FROM %s", catalog)
	result, err := c.ExecuteQueryWithContext(ctx, query)
	if err != nil {
		return nil, err
	}

	schemas := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if schema, ok := row["Schema"].(string); ok {
			schemas = append(schemas, schema)
		}
	}

	// Apply schema filtering if allowlist is configured
	if len(c.config.AllowedSchemas) > 0 {
		schemas = c.filterSchemas(schemas, catalog)
	}

	return schemas, nil
}

// ListTables returns a list of tables in the specified catalog and schema
func (c *Client) ListTables(catalog, schema string) ([]string, error) {
	return c.ListTablesWithContext(context.Background(), catalog, schema)
}

// ListTablesWithContext returns a list of tables in the specified catalog and schema with context
func (c *Client) ListTablesWithContext(ctx context.Context, catalog, schema string) ([]string, error) {
	if catalog == "" {
		catalog = c.config.Catalog
	}
	if schema == "" {
		schema = c.config.Schema
	}

	query := fmt.Sprintf("SHOW TABLES FROM %s.%s", catalog, schema)
	result, err := c.ExecuteQueryWithContext(ctx, query)
	if err != nil {
		return nil, err
	}

	tables := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if table, ok := row["Table"].(string); ok {
			tables = append(tables, table)
		}
	}

	// Apply table filtering if allowlist is configured
	if len(c.config.AllowedTables) > 0 {
		tables = c.filterTables(tables, catalog, schema)
	}

	return tables, nil
}

// GetTableSchema returns the schema of a table
func (c *Client) GetTableSchema(catalog, schema, table string) (*QueryResult, error) {
	return c.GetTableSchemaWithContext(context.Background(), catalog, schema, table)
}

// GetTableSchemaWithContext returns the schema of a table with context
func (c *Client) GetTableSchemaWithContext(ctx context.Context, catalog, schema, table string) (*QueryResult, error) {
	// Resolve catalog/schema/table parameters first
	parts := strings.Split(table, ".")
	if len(parts) == 3 {
		// If table is already fully qualified, extract components
		catalog = parts[0]
		schema = parts[1]
		table = parts[2]
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

	// Check if table access is allowed when table allowlist is configured (after resolution)
	if len(c.config.AllowedTables) > 0 {
		if !c.isTableAllowed(catalog, schema, table) {
			return nil, fmt.Errorf("table access denied: %s.%s.%s not in allowlist", catalog, schema, table)
		}
	}

	// Build and execute query with resolved parameters
	query := fmt.Sprintf("DESCRIBE %s.%s.%s", catalog, schema, table)
	return c.ExecuteQueryWithContext(ctx, query)
}

// ExplainQuery returns the query execution plan for a given SQL query
func (c *Client) ExplainQuery(query string, format string) (*QueryResult, error) {
	return c.ExplainQueryWithContext(context.Background(), query, format)
}

// ExplainQueryWithContext returns the query execution plan for a given SQL query with context
func (c *Client) ExplainQueryWithContext(ctx context.Context, query string, format string) (*QueryResult, error) {
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

	return c.ExecuteQueryWithContext(ctx, explainQuery)
}

// sanitizeConnectionError removes sensitive information from connection errors
func sanitizeConnectionError(err error, password string) error {
	if err == nil {
		return err
	}

	errStr := err.Error()

	// Replace password in error message if it exists
	if password != "" {
		// Replace URL-encoded password
		encodedPassword := url.QueryEscape(password)
		errStr = strings.ReplaceAll(errStr, encodedPassword, "[PASSWORD_REDACTED]")

		// Replace plain password
		errStr = strings.ReplaceAll(errStr, password, "[PASSWORD_REDACTED]")
	}

	return fmt.Errorf("%s", errStr)
}

// filterCatalogs filters a list of catalogs based on the allowlist configuration
func (c *Client) filterCatalogs(catalogs []string) []string {
	if len(c.config.AllowedCatalogs) == 0 {
		return catalogs
	}

	filtered := make([]string, 0, len(catalogs))
	for _, catalog := range catalogs {
		if c.isCatalogAllowed(catalog) {
			filtered = append(filtered, catalog)
		}
	}

	log.Printf("DEBUG: Catalog filtering: %d catalogs -> %d catalogs", len(catalogs), len(filtered))
	return filtered
}

// filterSchemas filters a list of schemas based on the allowlist configuration
func (c *Client) filterSchemas(schemas []string, catalog string) []string {
	if len(c.config.AllowedSchemas) == 0 {
		return schemas
	}

	filtered := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		if c.isSchemaAllowed(catalog, schema) {
			filtered = append(filtered, schema)
		}
	}

	log.Printf("DEBUG: Schema filtering: %d schemas -> %d schemas", len(schemas), len(filtered))
	return filtered
}

// filterTables filters a list of tables based on the allowlist configuration
func (c *Client) filterTables(tables []string, catalog, schema string) []string {
	if len(c.config.AllowedTables) == 0 {
		return tables
	}

	filtered := make([]string, 0, len(tables))
	for _, table := range tables {
		if c.isTableAllowed(catalog, schema, table) {
			filtered = append(filtered, table)
		}
	}

	log.Printf("DEBUG: Table filtering: %d tables -> %d tables", len(tables), len(filtered))
	return filtered
}

// isCatalogAllowed checks if a catalog is in the allowed catalogs list
func (c *Client) isCatalogAllowed(catalog string) bool {
	for _, allowed := range c.config.AllowedCatalogs {
		if strings.EqualFold(catalog, allowed) {
			return true
		}
	}
	return false
}

// isSchemaAllowed checks if a schema is in the allowed schemas list
func (c *Client) isSchemaAllowed(catalog, schema string) bool {
	fullSchemaName := catalog + "." + schema
	for _, allowed := range c.config.AllowedSchemas {
		if strings.EqualFold(fullSchemaName, allowed) {
			return true
		}
	}
	return false
}

// isTableAllowed checks if a table is in the allowed tables list
func (c *Client) isTableAllowed(catalog, schema, table string) bool {
	fullTableName := catalog + "." + schema + "." + table
	for _, allowed := range c.config.AllowedTables {
		if strings.EqualFold(fullTableName, allowed) {
			return true
		}
	}
	return false
}
