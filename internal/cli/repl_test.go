package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tuannvm/mcp-trino/internal/trino"
)

func TestNewREPL(t *testing.T) {
	mockClient := &mockTrinoClient{}
	var buf bytes.Buffer
	cmd := NewCommandsWithWriters(mockClient, "table", &buf, &buf)

	repl := NewREPL(cmd, "memory", "default")

	if repl == nil {
		t.Fatal("NewREPL() returned nil")
	}
	if repl.prompt != "memory.default>" {
		t.Errorf("expected prompt 'memory.default>', got '%s'", repl.prompt)
	}
}

func TestNewREPL_EmptyCatalogSchema(t *testing.T) {
	mockClient := &mockTrinoClient{}
	var buf bytes.Buffer
	cmd := NewCommandsWithWriters(mockClient, "table", &buf, &buf)

	repl := NewREPL(cmd, "", "")

	if repl.prompt != "trino>" {
		t.Errorf("expected prompt 'trino>', got '%s'", repl.prompt)
	}
}

func TestNewREPL_CatalogOnly_NoSchema(t *testing.T) {
	mockClient := &mockTrinoClient{}
	var buf bytes.Buffer
	cmd := NewCommandsWithWriters(mockClient, "table", &buf, &buf)

	repl := NewREPL(cmd, "memory", "")

	if repl.prompt != "memory>" {
		t.Errorf("expected prompt 'memory>', got '%s'", repl.prompt)
	}
}

func TestNewREPL_CatalogAndSchema(t *testing.T) {
	mockClient := &mockTrinoClient{}
	var buf bytes.Buffer
	cmd := NewCommandsWithWriters(mockClient, "table", &buf, &buf)

	repl := NewREPL(cmd, "memory", "default")

	if repl.prompt != "memory.default>" {
		t.Errorf("expected prompt 'memory.default>', got '%s'", repl.prompt)
	}
}

func TestHasMoreInput(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "Complete query with semicolon",
			query:    "SELECT * FROM test;",
			expected: false,
		},
		{
			name:     "Incomplete SELECT",
			query:    "SELECT",
			expected: true,
		},
		{
			name:     "Incomplete FROM",
			query:    "SELECT * FROM",
			expected: true,
		},
		{
			name:     "Incomplete WHERE",
			query:    "SELECT * FROM test WHERE",
			expected: true,
		},
		{
			name:     "Incomplete JOIN",
			query:    "SELECT * FROM test JOIN",
			expected: true,
		},
		{
			name:     "Complete INSERT",
			query:    "INSERT INTO test VALUES (1)",
			expected: false,
		},
		{
			name:     "Simple query without semicolon",
			query:    "SELECT 1",
			expected: false,
		},
		{
			name:     "Empty query",
			query:    "",
			expected: false,
		},
		{
			name:     "Query with trailing whitespace",
			query:    "SELECT * FROM test   ",
			expected: false,
		},
	}

	repl := &REPL{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repl.hasMoreInput(tt.query)
			if result != tt.expected {
				t.Errorf("hasMoreInput(%q) = %v, expected %v", tt.query, result, tt.expected)
			}
		})
	}
}

func TestREPL_PrintHelp(t *testing.T) {
	var buf bytes.Buffer
	repl := &REPL{out: &buf}
	repl.printHelp()

	output := buf.String()
	if !strings.Contains(output, "Meta-commands") {
		t.Errorf("expected help text, got: %s", output)
	}
}

func TestREPL_PrintHistory_Empty(t *testing.T) {
	var buf bytes.Buffer
	repl := &REPL{out: &buf}
	history := []string{}

	repl.printHistory(&history)

	if !strings.Contains(buf.String(), "No history") {
		t.Errorf("expected 'No history', got: %s", buf.String())
	}
}

func TestREPL_RunWithInput(t *testing.T) {
	client := &mockTrinoClient{
		catalogs: []string{"cat1", "cat2"},
	}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	input := strings.NewReader("\\catalogs\n\\quit\n")
	repl := NewREPLWithReader(cmd, "", "", input)

	ctx := context.Background()
	err := repl.Run(ctx)
	if err != nil {
		t.Fatalf("REPL.Run() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "cat1") {
		t.Errorf("expected catalogs in output, got: %s", output)
	}
}

func TestREPL_RunQuery(t *testing.T) {
	client := &mockTrinoClient{
		queryResult: &trino.QueryResult{
			Rows: []map[string]interface{}{
				{"val": 42},
			},
			MaxRows: 10000,
		},
	}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	input := strings.NewReader("SELECT 42;\n\\quit\n")
	repl := NewREPLWithReader(cmd, "", "", input)

	err := repl.Run(context.Background())
	if err != nil {
		t.Fatalf("REPL.Run() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "42") {
		t.Errorf("expected query result in output, got: %s", output)
	}
}

func TestREPL_QueryError_ToStderr(t *testing.T) {
	client := &mockTrinoClient{
		queryError: fmt.Errorf("connection refused"),
	}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	input := strings.NewReader("SELECT 1;\n\\quit\n")
	repl := NewREPLWithReader(cmd, "", "", input)

	err := repl.Run(context.Background())
	if err != nil {
		t.Fatalf("REPL.Run() failed: %v", err)
	}

	if !strings.Contains(stderr.String(), "connection refused") {
		t.Errorf("expected error on stderr, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	// Error should NOT appear on stdout
	if strings.Contains(stdout.String(), "connection refused") {
		t.Error("error message should not appear on stdout")
	}
}

func TestREPL_MetaCommandError_ToStderr(t *testing.T) {
	client := &mockTrinoClient{}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	input := strings.NewReader("\\describe\n\\quit\n")
	repl := NewREPLWithReader(cmd, "", "", input)

	err := repl.Run(context.Background())
	if err != nil {
		t.Fatalf("REPL.Run() failed: %v", err)
	}

	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage error on stderr, got stderr=%q", stderr.String())
	}
}

func TestREPL_FormatCommand(t *testing.T) {
	client := &mockTrinoClient{}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	input := strings.NewReader("\\format json\n\\format\n\\quit\n")
	repl := NewREPLWithReader(cmd, "", "", input)

	err := repl.Run(context.Background())
	if err != nil {
		t.Fatalf("REPL.Run() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Output format set to: json") {
		t.Errorf("expected format change confirmation, got: %s", output)
	}
	if !strings.Contains(output, "Current format: json") {
		t.Errorf("expected current format display, got: %s", output)
	}
}

func TestREPL_InvalidFormatCommand(t *testing.T) {
	client := &mockTrinoClient{}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	input := strings.NewReader("\\format xml\n\\quit\n")
	repl := NewREPLWithReader(cmd, "", "", input)

	_ = repl.Run(context.Background())

	if !strings.Contains(stderr.String(), "invalid format") {
		t.Errorf("expected 'invalid format' on stderr, got: %s", stderr.String())
	}
}

func TestREPL_EOF_GracefulExit(t *testing.T) {
	client := &mockTrinoClient{}
	var stdout, stderr bytes.Buffer
	cmd := NewCommandsWithWriters(client, "table", &stdout, &stderr)

	// Empty input = immediate EOF
	input := strings.NewReader("")
	repl := NewREPLWithReader(cmd, "", "", input)

	err := repl.Run(context.Background())
	if err != nil {
		t.Fatalf("REPL.Run() should exit gracefully on EOF, got: %v", err)
	}
}

func TestREPL_PrintHistory_WithItems(t *testing.T) {
	var buf bytes.Buffer
	repl := &REPL{out: &buf}
	history := []string{"SELECT 1", "SELECT 2", "SELECT 3"}

	repl.printHistory(&history)

	output := buf.String()
	if !strings.Contains(output, "SELECT 1") || !strings.Contains(output, "SELECT 3") {
		t.Errorf("expected history items in output, got: %s", output)
	}
}
