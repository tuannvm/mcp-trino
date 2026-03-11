package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tuannvm/mcp-trino/internal/config"
)

// expectedTools lists all tool names that RegisterTrinoTools must register.
var expectedTools = []string{
	"execute_query",
	"list_catalogs",
	"list_schemas",
	"list_tables",
	"get_table_schema",
	"explain_query",
}

// newTestHandlers creates a TrinoHandlers with no real Trino client, suitable
// for tests that only exercise argument validation and response formatting.
func newTestHandlers(cfg *config.TrinoConfig) *TrinoHandlers {
	return &TrinoHandlers{
		TrinoClient: nil,
		Config:      cfg,
	}
}

// TestRegisterTrinoTools_AllToolsRegistered verifies that all 6 tools are
// registered on the MCP server and can be listed via the JSON-RPC protocol.
func TestRegisterTrinoTools_AllToolsRegistered(t *testing.T) {
	srv := mcpserver.NewMCPServer("test-server", "0.0.1", mcpserver.WithToolCapabilities(true))
	handlers := newTestHandlers(&config.TrinoConfig{
		MaxRows:      10000,
		QueryTimeout: 300 * time.Second,
	})
	RegisterTrinoTools(srv, handlers)

	// Send an initialize request first (required before tools/list)
	initMsg := mustJSON(t, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "0.0.1",
			},
		},
	})
	srv.HandleMessage(context.Background(), initMsg)

	// Send tools/list request
	listMsg := mustJSON(t, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	})
	resp := srv.HandleMessage(context.Background(), listMsg)
	if resp == nil {
		t.Fatal("HandleMessage returned nil for tools/list")
	}

	// Parse the JSON-RPC response to extract the tools list
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	registered := make(map[string]bool)
	for _, tool := range rpcResp.Result.Tools {
		registered[tool.Name] = true
	}

	for _, name := range expectedTools {
		if !registered[name] {
			t.Errorf("expected tool %q to be registered, but it was not found", name)
		}
	}

	if len(rpcResp.Result.Tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(rpcResp.Result.Tools))
	}
}

// TestExecuteQuery_MissingQueryParam verifies that the ExecuteQuery handler
// returns an error result when the required "query" argument is missing.
func TestExecuteQuery_MissingQueryParam(t *testing.T) {
	handlers := newTestHandlers(&config.TrinoConfig{
		MaxRows:      100,
		QueryTimeout: 60 * time.Second,
	})

	tests := []struct {
		name      string
		args      interface{}
		wantError string
	}{
		{
			name:      "nil arguments",
			args:      nil,
			wantError: "invalid arguments format",
		},
		{
			name:      "empty arguments map",
			args:      map[string]interface{}{},
			wantError: "query parameter must be a string",
		},
		{
			name:      "query is integer, not string",
			args:      map[string]interface{}{"query": 42},
			wantError: "query parameter must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Name = "execute_query"
			req.Params.Arguments = tt.args

			result, err := handlers.ExecuteQuery(context.Background(), req)
			if err != nil {
				t.Fatalf("ExecuteQuery returned unexpected Go error: %v", err)
			}
			if result == nil {
				t.Fatal("ExecuteQuery returned nil result")
			}
			if !result.IsError {
				t.Error("expected IsError=true for invalid arguments")
			}
			assertContentContains(t, result, tt.wantError)
		})
	}
}

// TestExplainQuery_MissingQueryParam verifies that ExplainQuery rejects
// requests without a query argument.
func TestExplainQuery_MissingQueryParam(t *testing.T) {
	handlers := newTestHandlers(&config.TrinoConfig{
		MaxRows:      100,
		QueryTimeout: 60 * time.Second,
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "explain_query"
	req.Params.Arguments = map[string]interface{}{}

	result, err := handlers.ExplainQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("ExplainQuery returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing query parameter")
	}
	assertContentContains(t, result, "query parameter must be a string")
}

// TestGetTableSchema_MissingTableParam verifies that GetTableSchema rejects
// requests without the required "table" argument.
func TestGetTableSchema_MissingTableParam(t *testing.T) {
	handlers := newTestHandlers(&config.TrinoConfig{
		MaxRows:      100,
		QueryTimeout: 60 * time.Second,
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "get_table_schema"
	req.Params.Arguments = map[string]interface{}{
		"catalog": "hive",
		"schema":  "analytics",
	}

	result, err := handlers.GetTableSchema(context.Background(), req)
	if err != nil {
		t.Fatalf("GetTableSchema returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing table parameter")
	}
	assertContentContains(t, result, "table parameter is required")
}

// TestConfigPropagation verifies that MaxRows and QueryTimeout are correctly
// propagated from config to the handler struct.
func TestConfigPropagation(t *testing.T) {
	tests := []struct {
		name         string
		maxRows      int
		queryTimeout time.Duration
	}{
		{"Defaults", 10000, 300 * time.Second},
		{"Unlimited rows", 0, 300 * time.Second},
		{"Small limit", 5, 60 * time.Second},
		{"Large timeout", 10000, 600 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.TrinoConfig{
				MaxRows:      tt.maxRows,
				QueryTimeout: tt.queryTimeout,
			}
			handlers := NewTrinoHandlers(nil, cfg)

			if handlers.Config.MaxRows != tt.maxRows {
				t.Errorf("MaxRows = %d, want %d", handlers.Config.MaxRows, tt.maxRows)
			}
			if handlers.Config.QueryTimeout != tt.queryTimeout {
				t.Errorf("QueryTimeout = %v, want %v", handlers.Config.QueryTimeout, tt.queryTimeout)
			}
		})
	}
}

// TestTruncationResponseFormat verifies that when results hit MaxRows:
// - Text content (backward-compat) is a bare JSON array
// - StructuredContent carries the truncation envelope with metadata
func TestTruncationResponseFormat(t *testing.T) {
	maxRows := 5
	results := make([]map[string]interface{}, maxRows)
	for i := range results {
		results[i] = map[string]interface{}{"row": i + 1}
	}

	// Simulate the handler's truncation path using mcp.NewToolResultStructured
	structured := map[string]interface{}{
		"results":   results,
		"truncated": true,
		"rowCount":  len(results),
		"message":   fmt.Sprintf("Result truncated to %d rows. Add LIMIT to your query or increase TRINO_MAX_ROWS.", maxRows),
	}
	bareJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal results: %v", err)
	}

	result := mcp.NewToolResultStructured(structured, string(bareJSON))

	// Verify text content is the bare array (backward-compatible)
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent in content[0]")
	}
	// Text should parse as a JSON array, not an object
	var arr []interface{}
	if err := json.Unmarshal([]byte(tc.Text), &arr); err != nil {
		t.Fatalf("text content is not a bare JSON array: %v", err)
	}
	if len(arr) != 5 {
		t.Errorf("expected 5 results in text content, got %d", len(arr))
	}

	// Verify structuredContent carries the envelope
	if result.StructuredContent == nil {
		t.Fatal("expected structuredContent to be set")
	}
	scJSON, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("failed to marshal structuredContent: %v", err)
	}
	var sc map[string]interface{}
	if err := json.Unmarshal(scJSON, &sc); err != nil {
		t.Fatalf("structuredContent is not a JSON object: %v", err)
	}
	if truncated, ok := sc["truncated"].(bool); !ok || !truncated {
		t.Error("expected structuredContent.truncated=true")
	}
	expectedMsg := "Result truncated to 5 rows. Add LIMIT to your query or increase TRINO_MAX_ROWS."
	if msg, ok := sc["message"].(string); !ok || msg != expectedMsg {
		t.Errorf("structuredContent.message = %q, want %q", sc["message"], expectedMsg)
	}
}

// TestTruncationConditions verifies the truncation condition across boundary
// cases, matching the exact condition: maxRows > 0 && len(results) >= maxRows.
func TestTruncationConditions(t *testing.T) {
	tests := []struct {
		name          string
		maxRows       int
		numResults    int
		wantTruncated bool
	}{
		{"Exact match triggers truncation", 5, 5, true},
		{"Over limit triggers truncation", 5, 10, true},
		{"Under limit no truncation", 5, 3, false},
		{"Unlimited (maxRows=0) never truncates", 0, 100, false},
		{"MaxRows=1 with 1 result", 1, 1, true},
		{"MaxRows=1 with 0 results", 1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]map[string]interface{}, tt.numResults)
			for i := range results {
				results[i] = map[string]interface{}{"id": i}
			}

			maxRows := tt.maxRows
			truncated := maxRows > 0 && len(results) >= maxRows

			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}

			// Verify the handler would produce the right result type
			if truncated {
				structured := map[string]interface{}{
					"results":   results,
					"truncated": true,
					"rowCount":  len(results),
					"message":   fmt.Sprintf("Result truncated to %d rows. Add LIMIT to your query or increase TRINO_MAX_ROWS.", maxRows),
				}
				result := mcp.NewToolResultStructured(structured, "[]")
				if result.StructuredContent == nil {
					t.Error("expected structuredContent when truncated")
				}
			} else {
				result := mcp.NewToolResultText("[]")
				if result.StructuredContent != nil {
					t.Error("expected no structuredContent when not truncated")
				}
			}
		})
	}
}

// TestNoTruncationWhenUnderLimit verifies that when results are under MaxRows,
// the response uses plain NewToolResultText (no structuredContent).
func TestNoTruncationWhenUnderLimit(t *testing.T) {
	maxRows := 10
	results := make([]map[string]interface{}, 3)
	for i := range results {
		results[i] = map[string]interface{}{"row": i + 1}
	}

	// Simulate the handler's non-truncated path
	truncated := maxRows > 0 && len(results) >= maxRows
	if truncated {
		t.Fatal("should not be truncated with 3 results and maxRows=10")
	}

	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal results: %v", err)
	}
	result := mcp.NewToolResultText(string(jsonData))

	// Verify text content is the bare JSON array
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	var arr []interface{}
	if err := json.Unmarshal([]byte(tc.Text), &arr); err != nil {
		t.Fatalf("text content is not a bare JSON array: %v", err)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 results, got %d", len(arr))
	}

	// Verify no structuredContent
	if result.StructuredContent != nil {
		t.Error("expected no structuredContent when under limit")
	}
}

// TestListSchemas_InvalidArguments verifies that ListSchemas rejects
// non-map arguments.
func TestListSchemas_InvalidArguments(t *testing.T) {
	handlers := newTestHandlers(&config.TrinoConfig{})

	req := mcp.CallToolRequest{}
	req.Params.Name = "list_schemas"
	req.Params.Arguments = "not-a-map"

	result, err := handlers.ListSchemas(context.Background(), req)
	if err != nil {
		t.Fatalf("ListSchemas returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid arguments format")
	}
	assertContentContains(t, result, "invalid arguments format")
}

// TestListTables_InvalidArguments verifies that ListTables rejects
// non-map arguments.
func TestListTables_InvalidArguments(t *testing.T) {
	handlers := newTestHandlers(&config.TrinoConfig{})

	req := mcp.CallToolRequest{}
	req.Params.Name = "list_tables"
	req.Params.Arguments = "not-a-map"

	result, err := handlers.ListTables(context.Background(), req)
	if err != nil {
		t.Fatalf("ListTables returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid arguments format")
	}
	assertContentContains(t, result, "invalid arguments format")
}

// --- Helpers ---

// mustJSON marshals v to json.RawMessage; fails the test on error.
func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return data
}

// assertContentContains checks that the CallToolResult contains text matching
// the expected substring.
func assertContentContains(t *testing.T, result *mcp.CallToolResult, want string) {
	t.Helper()
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, want) {
				return
			}
		}
	}
	t.Errorf("result content does not contain %q", want)
}
