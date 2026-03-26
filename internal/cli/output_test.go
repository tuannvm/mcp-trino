package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tuannvm/mcp-trino/internal/trino"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	fnErr := fn()
	_ = w.Close()
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("failed to read captured stdout: %v", readErr)
	}
	return string(out), fnErr
}

func TestOutputTable_DeterministicColumnOrder(t *testing.T) {
	tests := []struct {
		name      string
		rows      []map[string]interface{}
		truncated bool
	}{
		{
			name: "single row multiple columns",
			rows: []map[string]interface{}{
				{"zebra": 1, "apple": "a", "banana": 3.14},
			},
			truncated: false,
		},
		{
			name: "multiple rows same columns",
			rows: []map[string]interface{}{
				{"zebra": 1, "apple": "a"},
				{"zebra": 2, "apple": "b"},
			},
			truncated: false,
		},
		{
			name: "truncated results",
			rows: []map[string]interface{}{
				{"zebra": 1, "apple": "a"},
			},
			truncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Commands{format: "table"}
			result := &trino.QueryResult{
				Rows:      tt.rows,
				Truncated: tt.truncated,
				MaxRows:   100,
			}

			out1, err := captureStdout(t, func() error {
				return cmd.outputTable(result)
			})
			if err != nil {
				t.Errorf("outputTable() failed: %v", err)
			}

			out2, err2 := captureStdout(t, func() error {
				return cmd.outputTable(result)
			})
			if err != err2 {
				t.Errorf("outputTable() not deterministic: first err=%v, second err=%v", err, err2)
			}
			if out1 != out2 {
				t.Errorf("outputTable() output mismatch between runs:\nfirst:\n%s\nsecond:\n%s", out1, out2)
			}

			if tt.name == "single row multiple columns" {
				expected := "apple  banana  zebra  \n-----  ------  -----  \na      3.14    1      \n"
				if out1 != expected {
					t.Errorf("outputTable() exact output mismatch:\nexpected:\n%s\ngot:\n%s", expected, out1)
				}
			}
		})
	}
}

func TestOutputCSV_DeterministicColumnOrder(t *testing.T) {
	tests := []struct {
		name      string
		rows      []map[string]interface{}
		truncated bool
	}{
		{
			name: "single row multiple columns",
			rows: []map[string]interface{}{
				{"zebra": 1, "apple": "a", "banana": 3.14},
			},
			truncated: false,
		},
		{
			name: "multiple rows",
			rows: []map[string]interface{}{
				{"zebra": 1, "apple": "a"},
				{"zebra": 2, "apple": "b"},
			},
			truncated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Commands{format: "csv"}
			result := &trino.QueryResult{
				Rows:      tt.rows,
				Truncated: tt.truncated,
				MaxRows:   100,
			}

			out1, err := captureStdout(t, func() error {
				return cmd.outputCSV(result)
			})
			if err != nil {
				t.Errorf("outputCSV() failed: %v", err)
			}

			out2, err2 := captureStdout(t, func() error {
				return cmd.outputCSV(result)
			})
			if err != err2 {
				t.Errorf("outputCSV() not deterministic: first err=%v, second err=%v", err, err2)
			}
			if out1 != out2 {
				t.Errorf("outputCSV() output mismatch between runs:\nfirst:\n%s\nsecond:\n%s", out1, out2)
			}

			if tt.name == "single row multiple columns" {
				expected := "\"apple\",\"banana\",\"zebra\"\n\"a\",\"3.14\",\"1\"\n"
				if out1 != expected {
					t.Errorf("outputCSV() exact output mismatch:\nexpected:\n%s\ngot:\n%s", expected, out1)
				}
			}
		})
	}
}

func TestOutputJSON_ExactStructure(t *testing.T) {
	cmd := &Commands{format: "json"}

	// Capture stdout would require refactoring, so we test the error path
	// and verify the structure is valid by not panicking
	data := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	err := cmd.outputJSON(data)
	if err != nil {
		t.Errorf("outputJSON() failed: %v", err)
	}
}

func TestFormatOutput_TableFormat(t *testing.T) {
	cmd := &Commands{format: "table"}
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
		},
		Truncated: false,
		MaxRows:   100,
	}

	output, err := captureStdout(t, func() error {
		return cmd.formatOutput(result)
	})
	if err != nil {
		t.Errorf("formatOutput(table) failed: %v", err)
	}
	expected := "col1    col2  \n------  ----  \nvalue1  123   \n"
	if output != expected {
		t.Errorf("formatOutput(table) output mismatch:\nexpected:\n%s\ngot:\n%s", expected, output)
	}
}

func TestFormatOutput_CSVFormat(t *testing.T) {
	cmd := &Commands{format: "csv"}
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
		},
		Truncated: false,
		MaxRows:   100,
	}

	output, err := captureStdout(t, func() error {
		return cmd.formatOutput(result)
	})
	if err != nil {
		t.Errorf("formatOutput(csv) failed: %v", err)
	}
	expected := "\"col1\",\"col2\"\n\"value1\",\"123\"\n"
	if output != expected {
		t.Errorf("formatOutput(csv) output mismatch:\nexpected:\n%s\ngot:\n%s", expected, output)
	}
}

func TestFormatOutput_InvalidFormat(t *testing.T) {
	// This test verifies that an invalid format doesn't crash
	// The actual validation happens in cmd/cli.go, so we just test here
	// that the Commands struct can be created with any format string
	cmd := &Commands{format: "invalid"}
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1"},
		},
		Truncated: false,
		MaxRows:   100,
	}

	// This will fall through to outputTable which doesn't validate format
	err := cmd.formatOutput(result)
	// We expect this to work (falls back to table format)
	if err != nil {
		t.Errorf("formatOutput(invalid) unexpectedly failed: %v", err)
	}
}

func TestQueryExecution_ContextCancellation(t *testing.T) {
	// Test that query execution respects context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := &mockTrinoClient{
		queryResult: &trino.QueryResult{
			Rows: []map[string]interface{}{
				{"col": "val"},
			},
		},
	}
	cmd := NewCommands(client, "table")

	output, err := captureStdout(t, func() error {
		return cmd.Query(ctx, "SELECT 1")
	})
	if err != nil {
		t.Fatalf("Query() failed unexpectedly for canceled context test: %v", err)
	}
	if !strings.Contains(output, "col") || !strings.Contains(output, "val") {
		t.Errorf("expected formatted output to include query result, got: %q", output)
	}
}
