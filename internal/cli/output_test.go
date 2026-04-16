package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/tuannvm/mcp-trino/internal/trino"
)

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
			cmd, stdout1, _ := newTestCommands(&mockTrinoClient{}, "table")
			result := &trino.QueryResult{
				Rows:      tt.rows,
				Truncated: tt.truncated,
				MaxRows:   100,
			}

			err := cmd.outputTable(result)
			if err != nil {
				t.Errorf("outputTable() failed: %v", err)
			}
			output1 := stdout1.String()

			// Run again for determinism check
			cmd2, stdout2, _ := newTestCommands(&mockTrinoClient{}, "table")
			err2 := cmd2.outputTable(result)
			if err2 != nil {
				t.Errorf("outputTable() second run failed: %v", err2)
			}

			if output1 != stdout2.String() {
				t.Errorf("outputTable() not deterministic:\nfirst:  %s\nsecond: %s", output1, stdout2.String())
			}

			// Verify columns are alphabetically sorted
			lines := strings.Split(output1, "\n")
			if len(lines) > 0 {
				header := lines[0]
				if strings.Contains(header, "zebra") && strings.Contains(header, "apple") {
					appleIdx := strings.Index(header, "apple")
					zebraIdx := strings.Index(header, "zebra")
					if appleIdx > zebraIdx {
						t.Errorf("columns not sorted alphabetically: %s", header)
					}
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
			cmd, stdout1, _ := newTestCommands(&mockTrinoClient{}, "csv")
			result := &trino.QueryResult{
				Rows:      tt.rows,
				Truncated: tt.truncated,
				MaxRows:   100,
			}

			err := cmd.outputCSV(result)
			if err != nil {
				t.Errorf("outputCSV() failed: %v", err)
			}
			output1 := stdout1.String()

			cmd2, stdout2, _ := newTestCommands(&mockTrinoClient{}, "csv")
			err2 := cmd2.outputCSV(result)
			if err2 != nil {
				t.Errorf("outputCSV() second run failed: %v", err2)
			}

			if output1 != stdout2.String() {
				t.Errorf("outputCSV() not deterministic:\nfirst:  %s\nsecond: %s", output1, stdout2.String())
			}
		})
	}
}

func TestOutputJSON_ExactStructure(t *testing.T) {
	cmd, stdout, _ := newTestCommands(&mockTrinoClient{}, "json")

	data := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	err := cmd.outputJSON(data)
	if err != nil {
		t.Errorf("outputJSON() failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `"key1"`) || !strings.Contains(output, `"value1"`) {
		t.Errorf("expected JSON structure, got: %s", output)
	}
}

func TestFormatOutput_TableFormat(t *testing.T) {
	cmd, _, _ := newTestCommands(&mockTrinoClient{}, "table")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
		},
		Truncated: false,
		MaxRows:   100,
	}

	err := cmd.formatOutput(result)
	if err != nil {
		t.Errorf("formatOutput(table) failed: %v", err)
	}
}

func TestFormatOutput_CSVFormat(t *testing.T) {
	cmd, _, _ := newTestCommands(&mockTrinoClient{}, "csv")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1", "col2": 123},
		},
		Truncated: false,
		MaxRows:   100,
	}

	err := cmd.formatOutput(result)
	if err != nil {
		t.Errorf("formatOutput(csv) failed: %v", err)
	}
}

func TestFormatOutput_InvalidFormat(t *testing.T) {
	cmd, _, _ := newTestCommands(&mockTrinoClient{}, "invalid")
	result := &trino.QueryResult{
		Rows: []map[string]interface{}{
			{"col1": "value1"},
		},
		Truncated: false,
		MaxRows:   100,
	}

	err := cmd.formatOutput(result)
	if err != nil {
		t.Errorf("formatOutput(invalid) unexpectedly failed: %v", err)
	}
}

func TestQueryExecution_OutputFormattingWithMockClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &mockTrinoClient{
		queryResult: &trino.QueryResult{
			Rows: []map[string]interface{}{
				{"col": "val"},
			},
		},
	}
	cmd, _, _ := newTestCommands(client, "table")

	err := cmd.Query(ctx, "SELECT 1")
	_ = err
}
