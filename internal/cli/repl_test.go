package cli

import (
	"testing"
)

func TestNewREPL(t *testing.T) {
	mockClient := &mockTrinoClient{}
	cmd := NewCommands(mockClient, "table")

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
	cmd := NewCommands(mockClient, "table")

	repl := NewREPL(cmd, "", "")

	if repl.prompt != "trino>" {
		t.Errorf("expected prompt 'trino>', got '%s'", repl.prompt)
	}
}

func TestNewREPL_CatalogOnly_NoSchema(t *testing.T) {
	mockClient := &mockTrinoClient{}
	cmd := NewCommands(mockClient, "table")

	repl := NewREPL(cmd, "memory", "")

	if repl.prompt != "memory>" {
		t.Errorf("expected prompt 'memory>', got '%s'", repl.prompt)
	}
}

func TestNewREPL_CatalogAndSchema(t *testing.T) {
	mockClient := &mockTrinoClient{}
	cmd := NewCommands(mockClient, "table")

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
	repl := &REPL{}
	// Just verify it doesn't panic
	repl.printHelp()
}

func TestREPL_PrintHistory_Empty(t *testing.T) {
	repl := &REPL{}
	history := []string{}

	// Just verify it doesn't panic
	repl.printHistory(&history)
}

func TestREPL_PrintHistory_WithItems(t *testing.T) {
	repl := &REPL{}
	history := []string{"SELECT 1", "SELECT 2", "SELECT 3"}

	// Just verify it doesn't panic
	repl.printHistory(&history)
}
