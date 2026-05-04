package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tuannvm/mcp-trino/internal/trino"
)

// mockTrinoClient implements TrinoClient for testing
type mockTrinoClient struct {
	catalogs      []string
	schemas       map[string][]string
	tables        map[string][]string
	queryResult   *trino.QueryResult
	schemaResult  *trino.QueryResult
	explainResult *trino.QueryResult
	queryError    error
	catalogError  error
	schemaError   error
	tableError    error
	explainError  error
}

func (m *mockTrinoClient) ExecuteQueryWithContext(ctx context.Context, query string) (*trino.QueryResult, error) {
	if m.queryError != nil {
		return nil, m.queryError
	}
	return m.queryResult, nil
}

func (m *mockTrinoClient) ListCatalogsWithContext(ctx context.Context) ([]string, error) {
	if m.catalogError != nil {
		return nil, m.catalogError
	}
	if m.catalogs != nil {
		return m.catalogs, nil
	}
	return []string{}, nil
}

func (m *mockTrinoClient) ListSchemasWithContext(ctx context.Context, catalog string) ([]string, error) {
	if m.schemaError != nil {
		return nil, m.schemaError
	}
	if m.schemas != nil {
		return m.schemas[catalog], nil
	}
	return []string{}, nil
}

func (m *mockTrinoClient) ListTablesWithContext(ctx context.Context, catalog, schema string) ([]string, error) {
	if m.tableError != nil {
		return nil, m.tableError
	}
	if m.tables != nil {
		key := catalog + "." + schema
		return m.tables[key], nil
	}
	return []string{}, nil
}

func (m *mockTrinoClient) GetTableSchemaWithContext(ctx context.Context, catalog, schema, table string) (*trino.QueryResult, error) {
	if m.schemaError != nil {
		return nil, m.schemaError
	}
	return m.schemaResult, nil
}

func (m *mockTrinoClient) ExplainQueryWithContext(ctx context.Context, query string, format string) (*trino.QueryResult, error) {
	if m.explainError != nil {
		return nil, m.explainError
	}
	return m.explainResult, nil
}

func (m *mockTrinoClient) Close() error {
	return nil
}

func newTestCommands(client TrinoClient, format string) (*Commands, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, format, &stdout, &stderr)
	return cmd, &stdout, &stderr
}

func TestNewCommands(t *testing.T) {
	client := &mockTrinoClient{}
	cmd := NewCommands(client, "table")

	if cmd == nil {
		t.Fatal("NewCommands() returned nil")
	}
	if cmd.format != "table" {
		t.Errorf("expected format 'table', got '%s'", cmd.format)
	}
}

func TestCommands_Query(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		queryResult: &trino.QueryResult{
			Rows: []map[string]interface{}{
				{"col1": "value1", "col2": 123},
				{"col1": "value2", "col2": 456},
			},
			Truncated: false,
			MaxRows:   10000,
		},
	}
	cmd, stdout, _ := newTestCommands(client, "table")

	err := cmd.Query(ctx, "SELECT * FROM test")
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "col1") || !strings.Contains(output, "value1") {
		t.Errorf("expected table output with col1/value1, got: %s", output)
	}
}

func TestCommands_QueryError(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		queryError: fmt.Errorf("query failed"),
	}
	cmd, _, _ := newTestCommands(client, "table")

	err := cmd.Query(ctx, "SELECT * FROM test")
	if err == nil {
		t.Fatal("Query() expected error, got nil")
	}
}

func TestCommands_QueryEmpty(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{}
	cmd, _, _ := newTestCommands(client, "table")

	err := cmd.Query(ctx, "")
	if err == nil {
		t.Fatal("Query() expected error for empty query, got nil")
	}
}

func TestCommands_Catalogs(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		catalogs: []string{"catalog1", "catalog2", "catalog3"},
	}
	cmd, stdout, _ := newTestCommands(client, "table")

	err := cmd.Catalogs(ctx)
	if err != nil {
		t.Fatalf("Catalogs() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "catalog1") {
		t.Errorf("expected catalog1 in output, got: %s", output)
	}
}

func TestCommands_CatalogsError(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		catalogError: fmt.Errorf("catalogs failed"),
	}
	cmd, _, _ := newTestCommands(client, "table")

	err := cmd.Catalogs(ctx)
	if err == nil {
		t.Fatal("Catalogs() expected error, got nil")
	}
}

func TestCommands_Schemas(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		schemas: map[string][]string{
			"catalog1": {"schema1", "schema2"},
			"catalog2": {"schema3"},
		},
	}
	cmd, stdout, _ := newTestCommands(client, "table")

	err := cmd.Schemas(ctx, "catalog1")
	if err != nil {
		t.Fatalf("Schemas() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "schema1") {
		t.Errorf("expected schema1 in output, got: %s", output)
	}
}

func TestCommands_Schemas_DefaultCatalog(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		schemas: map[string][]string{
			"memory": {"default", "information_schema"},
		},
	}
	cmd, _, _ := newTestCommands(client, "table")

	_ = os.Setenv("TRINO_CATALOG", "memory")
	t.Cleanup(func() { _ = os.Unsetenv("TRINO_CATALOG") })

	err := cmd.Schemas(ctx, "")
	if err != nil {
		t.Fatalf("Schemas() with empty catalog failed: %v", err)
	}
}

func TestCommands_Tables(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		tables: map[string][]string{
			"catalog1.schema1": {"table1", "table2"},
			"catalog2.schema3": {"table3"},
		},
	}
	cmd, stdout, _ := newTestCommands(client, "table")

	err := cmd.Tables(ctx, "catalog1", "schema1")
	if err != nil {
		t.Fatalf("Tables() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "table1") {
		t.Errorf("expected table1 in output, got: %s", output)
	}
}

func TestCommands_Tables_DefaultCatalogSchema(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		tables: map[string][]string{
			"memory.default": {"table1"},
		},
	}
	cmd, _, _ := newTestCommands(client, "table")

	_ = os.Setenv("TRINO_CATALOG", "memory")
	_ = os.Setenv("TRINO_SCHEMA", "default")
	t.Cleanup(func() {
		_ = os.Unsetenv("TRINO_CATALOG")
		_ = os.Unsetenv("TRINO_SCHEMA")
	})

	err := cmd.Tables(ctx, "", "")
	if err != nil {
		t.Fatalf("Tables() with empty catalog/schema failed: %v", err)
	}
}

func TestCommands_Describe(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		schemaResult: &trino.QueryResult{
			Rows: []map[string]interface{}{
				{"Column": "col1", "Type": "varchar", "Extra": "", "Comment": "column 1"},
				{"Column": "col2", "Type": "integer", "Extra": "NOT NULL", "Comment": ""},
			},
			Truncated: false,
			MaxRows:   10000,
		},
	}
	cmd, stdout, _ := newTestCommands(client, "table")

	err := cmd.Describe(ctx, "catalog.schema.table")
	if err != nil {
		t.Fatalf("Describe() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "col1") || !strings.Contains(output, "varchar") {
		t.Errorf("expected column info in output, got: %s", output)
	}
}

func TestCommands_Describe_EmptyTable(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{}
	cmd, _, _ := newTestCommands(client, "table")

	err := cmd.Describe(ctx, "")
	if err == nil {
		t.Fatal("Describe() expected error for empty table, got nil")
	}
}

func TestCommands_Explain(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{
		explainResult: &trino.QueryResult{
			Rows: []map[string]interface{}{
				{"Plan": "SELECT col1, col2 FROM table"},
			},
			Truncated: false,
			MaxRows:   10000,
		},
	}
	cmd, stdout, _ := newTestCommands(client, "table")

	err := cmd.Explain(ctx, "SELECT * FROM test", "")
	if err != nil {
		t.Fatalf("Explain() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Query Plan") {
		t.Errorf("expected 'Query Plan' in output, got: %s", output)
	}
}

func TestCommands_Explain_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	client := &mockTrinoClient{}
	cmd, _, _ := newTestCommands(client, "table")

	err := cmd.Explain(ctx, "", "")
	if err == nil {
		t.Fatal("Explain() expected error for empty query, got nil")
	}
}

func TestOutputJSON(t *testing.T) {
	cmd, stdout, _ := newTestCommands(&mockTrinoClient{}, "json")
	data := map[string]interface{}{
		"key":    "value",
		"number": 123,
	}

	err := cmd.outputJSON(data)
	if err != nil {
		t.Fatalf("outputJSON() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `"key"`) || !strings.Contains(output, `"value"`) {
		t.Errorf("expected JSON output, got: %s", output)
	}
}

func TestOutputTable_EmptyResults(t *testing.T) {
	cmd, stdout, _ := newTestCommands(&mockTrinoClient{}, "table")
	result := &trino.QueryResult{
		Rows:      []map[string]interface{}{},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.outputTable(result)
	if err != nil {
		t.Fatalf("outputTable() failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "No results") {
		t.Errorf("expected 'No results', got: %s", stdout.String())
	}
}

func TestOutputTable_WithResults(t *testing.T) {
	cmd, stdout, _ := newTestCommands(&mockTrinoClient{}, "table")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
			{"col1": "value2", "col2": 456},
		},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.outputTable(result)
	if err != nil {
		t.Fatalf("outputTable() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "col1") || !strings.Contains(output, "value1") {
		t.Errorf("expected table with col1/value1, got: %s", output)
	}
	if !strings.Contains(output, "2 row(s)") {
		t.Errorf("expected row count in output, got: %s", output)
	}
}

func TestOutputCSV_EmptyResults(t *testing.T) {
	cmd, stdout, _ := newTestCommands(&mockTrinoClient{}, "csv")
	result := &trino.QueryResult{
		Rows:      []map[string]interface{}{},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.outputCSV(result)
	if err != nil {
		t.Fatalf("outputCSV() failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "No results") {
		t.Errorf("expected 'No results', got: %s", stdout.String())
	}
}

func TestOutputCSV_WithResults(t *testing.T) {
	cmd, stdout, _ := newTestCommands(&mockTrinoClient{}, "csv")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
			{"col1": "value2", "col2": 456},
		},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.outputCSV(result)
	if err != nil {
		t.Fatalf("outputCSV() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "col1") || !strings.Contains(output, "value1") {
		t.Errorf("expected CSV with col1/value1, got: %s", output)
	}
}

func TestFormatOutput_Table(t *testing.T) {
	cmd, _, _ := newTestCommands(&mockTrinoClient{}, "table")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1"},
		},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.formatOutput(result)
	if err != nil {
		t.Fatalf("formatOutput() failed: %v", err)
	}
}

func TestFormatOutput_JSON(t *testing.T) {
	cmd, _, _ := newTestCommands(&mockTrinoClient{}, "json")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1"},
		},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.formatOutput(result)
	if err != nil {
		t.Fatalf("formatOutput() failed: %v", err)
	}
}

func TestFormatOutput_CSV(t *testing.T) {
	cmd, _, _ := newTestCommands(&mockTrinoClient{}, "csv")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
		},
		Truncated: false,
		MaxRows:   10000,
	}

	err := cmd.formatOutput(result)
	if err != nil {
		t.Fatalf("formatOutput() failed: %v", err)
	}
}
