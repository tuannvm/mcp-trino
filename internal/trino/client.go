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
	"sync"
	"time"

	"github.com/trinodb/trino-go-client/trino"
	"github.com/tuannvm/mcp-trino/internal/config"
)

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
	db            *sql.DB
	config        *config.TrinoConfig
	timeout       time.Duration
	authenticator *ExternalAuthenticator
	initialized   bool
	mu            sync.Mutex // Protects concurrent access to connection state
	httpClient    *http.Client
}

// NewClient creates a new Trino client
func NewClient(cfg *config.TrinoConfig) (*Client, error) {
	// Create HTTP client with custom headers (created once, reused for all connections)
	httpClient := &http.Client{
		Transport: &headerRoundTripper{
			base:   http.DefaultTransport,
			config: cfg,
		},
	}

	// Register custom client once globally
	if err := trino.RegisterCustomClient("mcp-trino", httpClient); err != nil {
		// Ignore "already registered" errors (process reuse, tests)
		if !strings.Contains(err.Error(), "already registered") {
			return nil, fmt.Errorf("failed to register custom HTTP client: %w", err)
		}
	}

	client := &Client{
		config:     cfg,
		timeout:    cfg.QueryTimeout,
		httpClient: httpClient,
	}

	// If external auth is enabled, defer connection until first query (lazy auth)
	if cfg.ExternalAuth {
		baseURL := fmt.Sprintf("%s://%s:%d", cfg.Scheme, cfg.Host, cfg.Port)
		client.authenticator = NewExternalAuthenticator(baseURL, cfg.User, cfg.ExternalAuthTimeout)
		log.Println("INFO: External authentication enabled - connection will be established on first query")
		return client, nil
	}

	// Standard connection flow
	if err := client.connect(""); err != nil {
		return nil, err
	}

	return client, nil
}

// connect establishes the database connection, optionally with an access token
func (c *Client) connect(accessToken string) error {
	dsnURL := url.URL{
		Scheme: c.config.Scheme,
		User:   url.UserPassword(c.config.User, c.config.Password),
		Host:   fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
	}

	params := url.Values{}
	params.Add("catalog", c.config.Catalog)
	params.Add("schema", c.config.Schema)
	params.Add("SSL", fmt.Sprintf("%t", c.config.SSL))
	params.Add("SSLInsecure", fmt.Sprintf("%t", c.config.SSLInsecure))
	params.Add("custom_client", "mcp-trino")

	// Add access token if provided (for external auth)
	if accessToken != "" {
		params.Add("accessToken", accessToken)
	}

	dsnURL.RawQuery = params.Encode()
	dsn := dsnURL.String()

	db, err := sql.Open("trino", dsn)
	if err != nil {
		// Sanitize error to prevent password exposure
		sanitizedErr := sanitizeConnectionError(err, c.config.Password)
		return fmt.Errorf("failed to connect to Trino: %w", sanitizedErr)
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
		sanitizedErr := sanitizeConnectionError(err, c.config.Password)
		return fmt.Errorf("failed to ping Trino: %w", sanitizedErr)
	}

	c.db = db
	c.initialized = true
	return nil
}

// ensureConnected establishes connection if needed and returns the db handle.
// Always acquires lock to prevent returning a connection being closed by another goroutine.
func (c *Client) ensureConnected(ctx context.Context) (*sql.DB, error) {
	c.mu.Lock()

	// Check if already connected
	if c.initialized && c.db != nil {
		db := c.db
		c.mu.Unlock()
		return db, nil
	}

	if c.authenticator == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("client not initialized and no authenticator available")
	}

	// Release lock during potentially long-running auth to allow other operations (e.g. Close)
	c.mu.Unlock()

	// Get token via external auth flow
	// Uses BOTH the caller's context deadline AND the internal auth timeout
	// (TRINO_EXTERNAL_AUTH_TIMEOUT). Whichever expires first will abort auth.
	// Note: Retry after 401 uses context.Background() to give re-auth full timeout.
	token, err := c.authenticator.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("external authentication failed: %w", err)
	}

	// Re-acquire lock to update state
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check state (another goroutine might have connected while we were authenticating)
	if c.initialized && c.db != nil {
		return c.db, nil
	}

	// Connect with the token
	if err := c.connect(token); err != nil {
		// If connection fails due to auth, invalidate token for retry
		if IsAuthenticationError(err) {
			c.authenticator.InvalidateToken()
		}
		return nil, err
	}

	return c.db, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	c.mu.Lock()
	db := c.db
	c.db = nil // Prevent double-close from clearConnectionForReauth()
	c.initialized = false
	c.mu.Unlock()

	if db == nil {
		return nil
	}
	return db.Close()
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
	readOnlyPrefixPatterns := []string{
		`^\s*select\b`,
		`^\s*show\b`,
		`^\s*describe\b`,
		`^\s*explain\b`,
		`^\s*with\b`,
	}

	for _, pattern := range readOnlyPrefixPatterns {
		matched, _ := regexp.MatchString(pattern, queryLower)
		if matched {
			// For queries starting with read-only prefixes, we still need to check
			// for disallowed write operations that might be embedded
			// But we allow common read-only patterns like "SHOW CREATE TABLE"
			if isAllowedReadOnlyPattern(queryLower) {
				return true
			}
		}
	}

	// Check for write operations anywhere in the query using word boundaries
	//  - https://trino.io/docs/current/sql.html - Main SQL reference
	writeOperations := []string{
		"insert", "update", "delete", "drop", "create", "alter", "truncate",
		"merge", "copy", "grant", "revoke", "commit", "rollback",
		"call", "execute", "refresh", "set", "reset",
	}

	for _, op := range writeOperations {
		// Use word boundary regex to catch operations followed by any whitespace
		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(op))
		matched, _ := regexp.MatchString(pattern, queryLower)
		if matched {
			return false
		}
	}

	return false
}

// isAllowedReadOnlyPattern checks if a query matches known safe read-only patterns
// even if it contains keywords that might look like write operations
func isAllowedReadOnlyPattern(queryLower string) bool {
	// SHOW CREATE statements are read-only (they just display DDL)
	showCreatePatterns := []string{
		`^\s*show\s+create\s+table\b`,
		`^\s*show\s+create\s+view\b`,
		`^\s*show\s+create\s+schema\b`,
		`^\s*show\s+create\s+materialized\s+view\b`,
	}

	for _, pattern := range showCreatePatterns {
		matched, _ := regexp.MatchString(pattern, queryLower)
		if matched {
			return true
		}
	}

	// Other SHOW statements without CREATE are safe
	if matched, _ := regexp.MatchString(`^\s*show\b`, queryLower); matched {
		// Check if it doesn't contain any write operation keywords after SHOW
		// (except for "create" which is handled above)
		writeOpsExceptCreate := []string{
			"insert", "update", "delete", "drop", "alter", "truncate",
			"merge", "copy", "grant", "revoke", "commit", "rollback",
			"call", "execute", "refresh", "set", "reset",
		}
		for _, op := range writeOpsExceptCreate {
			pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(op))
			if matched, _ := regexp.MatchString(pattern, queryLower); matched {
				return false
			}
		}
		return true
	}

	// SELECT, DESCRIBE, EXPLAIN, WITH without write operations are safe
	safeStarts := []string{`^\s*select\b`, `^\s*describe\b`, `^\s*explain\b`, `^\s*with\b`}
	for _, pattern := range safeStarts {
		if matched, _ := regexp.MatchString(pattern, queryLower); matched {
			// If it starts with a safe keyword, check there are no write operations
			writeOps := []string{
				"insert", "update", "delete", "drop", "create", "alter", "truncate",
				"merge", "copy", "grant", "revoke", "commit", "rollback",
				"call", "execute", "refresh", "set", "reset",
			}
			for _, op := range writeOps {
				opPattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(op))
				if matched, _ := regexp.MatchString(opPattern, queryLower); matched {
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
	// Remove single-quoted string literals: 'text'
	// Handle escaped quotes: 'don''t' becomes 'don''t'
	query = regexp.MustCompile(`'(?:[^']|'')*'`).ReplaceAllString(query, "'LITERAL'")

	// Remove double-quoted identifiers: "column_name"
	// Handle escaped quotes: "column""name" becomes "column""name"
	query = regexp.MustCompile(`"(?:[^"]|"")*"`).ReplaceAllString(query, "\"IDENTIFIER\"")

	// Remove backtick-quoted identifiers: `column_name`
	query = regexp.MustCompile("`[^`]*`").ReplaceAllString(query, "`IDENTIFIER`")

	// Remove single-line comments: -- comment
	query = regexp.MustCompile(`--[^\r\n]*`).ReplaceAllString(query, "")

	// Remove multi-line comments: /* comment */
	query = regexp.MustCompile(`/\*[^*]*\*+(?:[^/*][^*]*\*+)*/`).ReplaceAllString(query, "")

	return strings.TrimSpace(query)
}

// ExecuteQuery executes a SQL query and returns the results
func (c *Client) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	return c.ExecuteQueryWithContext(context.Background(), query)
}

// ExecuteQueryWithContext executes a SQL query and returns the results
func (c *Client) ExecuteQueryWithContext(ctx context.Context, query string) ([]map[string]interface{}, error) {
	return c.executeQueryWithRetry(ctx, query, false)
}

// executeQueryWithRetry handles query execution with automatic re-authentication on 401 errors
func (c *Client) executeQueryWithRetry(ctx context.Context, query string, isRetry bool) ([]map[string]interface{}, error) {
	// Ensure connection is established (triggers auth if needed)
	// Note: Capturing db prevents nil deref but not concurrent closure by clearConnectionForReauth().
	// If another goroutine closes the connection during re-auth, this query will fail and retry.
	db, err := c.ensureConnected(ctx)
	if err != nil {
		return nil, err
	}

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

	// Execute the query using captured db handle
	rows, err := db.QueryContext(queryCtx, query)
	if err != nil {
		// Check for authentication errors - attempt automatic re-authentication
		if !isRetry && IsAuthenticationError(err) && c.authenticator != nil {
			log.Printf("WARNING: Authentication failed (401) - attempting automatic re-authentication...")
			c.clearConnectionForReauth()
			// Use fresh context for retry to reset deadline, but preserve impersonation
			retryCtx := context.Background()
			if user, ok := GetImpersonatedUser(ctx); ok {
				retryCtx = WithImpersonatedUser(retryCtx, user)
			}
			return c.executeQueryWithRetry(retryCtx, query, true)
		}
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
		// Check for auth errors during result processing
		if !isRetry && IsAuthenticationError(err) && c.authenticator != nil {
			log.Printf("WARNING: Authentication failed during result processing - attempting re-auth...")
			c.clearConnectionForReauth()
			// Use fresh context for retry to reset deadline, but preserve impersonation
			retryCtx := context.Background()
			if user, ok := GetImpersonatedUser(ctx); ok {
				retryCtx = WithImpersonatedUser(retryCtx, user)
			}
			return c.executeQueryWithRetry(retryCtx, query, true)
		}
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// clearConnectionForReauth clears the connection state to allow re-authentication
func (c *Client) clearConnectionForReauth() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticator != nil {
		c.authenticator.InvalidateToken()
	}
	if c.db != nil {
		c.db.Close()
		c.db = nil
	}
	c.initialized = false
}

// ListCatalogs returns a list of available catalogs
func (c *Client) ListCatalogs() ([]string, error) {
	return c.ListCatalogsWithContext(context.Background())
}

// ListCatalogsWithContext returns a list of available catalogs with context
func (c *Client) ListCatalogsWithContext(ctx context.Context) ([]string, error) {
	results, err := c.ExecuteQueryWithContext(ctx, "SHOW CATALOGS")
	if err != nil {
		return nil, err
	}

	catalogs := make([]string, 0, len(results))
	for _, row := range results {
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
	results, err := c.ExecuteQueryWithContext(ctx, query)
	if err != nil {
		return nil, err
	}

	schemas := make([]string, 0, len(results))
	for _, row := range results {
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
	results, err := c.ExecuteQueryWithContext(ctx, query)
	if err != nil {
		return nil, err
	}

	tables := make([]string, 0, len(results))
	for _, row := range results {
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
func (c *Client) GetTableSchema(catalog, schema, table string) ([]map[string]interface{}, error) {
	return c.GetTableSchemaWithContext(context.Background(), catalog, schema, table)
}

// GetTableSchemaWithContext returns the schema of a table with context
func (c *Client) GetTableSchemaWithContext(ctx context.Context, catalog, schema, table string) ([]map[string]interface{}, error) {
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
func (c *Client) ExplainQuery(query string, format string) ([]map[string]interface{}, error) {
	return c.ExplainQueryWithContext(context.Background(), query, format)
}

// ExplainQueryWithContext returns the query execution plan for a given SQL query with context
func (c *Client) ExplainQueryWithContext(ctx context.Context, query string, format string) ([]map[string]interface{}, error) {
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
