package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParseAllowlist(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "Single item",
			input:    "hive",
			expected: []string{"hive"},
		},
		{
			name:     "Multiple items",
			input:    "hive,postgresql,mysql",
			expected: []string{"hive", "postgresql", "mysql"},
		},
		{
			name:     "Items with whitespace",
			input:    " hive , postgresql , mysql ",
			expected: []string{"hive", "postgresql", "mysql"},
		},
		{
			name:     "Items with empty entries",
			input:    "hive,,postgresql,,mysql,",
			expected: []string{"hive", "postgresql", "mysql"},
		},
		{
			name:     "Schema format",
			input:    "hive.analytics,hive.marts,postgresql.public",
			expected: []string{"hive.analytics", "hive.marts", "postgresql.public"},
		},
		{
			name:     "Table format",
			input:    "hive.analytics.users,hive.marts.sales",
			expected: []string{"hive.analytics.users", "hive.marts.sales"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAllowlist(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseAllowlist(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewTrinoConfigWithAllowlists(t *testing.T) {
	// Save original environment
	originalCatalogs := os.Getenv("TRINO_ALLOWED_CATALOGS")
	originalSchemas := os.Getenv("TRINO_ALLOWED_SCHEMAS")
	originalTables := os.Getenv("TRINO_ALLOWED_TABLES")
	originalOAuth := os.Getenv("OAUTH_ENABLED")

	// Clean up after test
	defer func() {
		_ = os.Setenv("TRINO_ALLOWED_CATALOGS", originalCatalogs)
		_ = os.Setenv("TRINO_ALLOWED_SCHEMAS", originalSchemas)
		_ = os.Setenv("TRINO_ALLOWED_TABLES", originalTables)
		_ = os.Setenv("OAUTH_ENABLED", originalOAuth)
	}()

	// Test with allowlists configured
	_ = os.Setenv("TRINO_ALLOWED_CATALOGS", "hive,postgresql")
	_ = os.Setenv("TRINO_ALLOWED_SCHEMAS", "hive.analytics,postgresql.public")
	_ = os.Setenv("TRINO_ALLOWED_TABLES", "hive.analytics.users")
	_ = os.Setenv("OAUTH_ENABLED", "false") // Disable OAuth for this test

	config, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("NewTrinoConfig() error = %v", err)
	}

	expectedCatalogs := []string{"hive", "postgresql"}
	if !reflect.DeepEqual(config.AllowedCatalogs, expectedCatalogs) {
		t.Errorf("AllowedCatalogs = %v, want %v", config.AllowedCatalogs, expectedCatalogs)
	}

	expectedSchemas := []string{"hive.analytics", "postgresql.public"}
	if !reflect.DeepEqual(config.AllowedSchemas, expectedSchemas) {
		t.Errorf("AllowedSchemas = %v, want %v", config.AllowedSchemas, expectedSchemas)
	}

	expectedTables := []string{"hive.analytics.users"}
	if !reflect.DeepEqual(config.AllowedTables, expectedTables) {
		t.Errorf("AllowedTables = %v, want %v", config.AllowedTables, expectedTables)
	}
}

func TestNewTrinoConfigWithoutAllowlists(t *testing.T) {
	// Save original environment
	originalCatalogs := os.Getenv("TRINO_ALLOWED_CATALOGS")
	originalSchemas := os.Getenv("TRINO_ALLOWED_SCHEMAS")
	originalTables := os.Getenv("TRINO_ALLOWED_TABLES")
	originalOAuth := os.Getenv("OAUTH_ENABLED")

	// Clean up after test
	defer func() {
		_ = os.Setenv("TRINO_ALLOWED_CATALOGS", originalCatalogs)
		_ = os.Setenv("TRINO_ALLOWED_SCHEMAS", originalSchemas)
		_ = os.Setenv("TRINO_ALLOWED_TABLES", originalTables)
		_ = os.Setenv("OAUTH_ENABLED", originalOAuth)
	}()

	// Clear allowlist environment variables
	_ = os.Unsetenv("TRINO_ALLOWED_CATALOGS")
	_ = os.Unsetenv("TRINO_ALLOWED_SCHEMAS")
	_ = os.Unsetenv("TRINO_ALLOWED_TABLES")
	_ = os.Setenv("OAUTH_ENABLED", "false") // Disable OAuth for this test

	config, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("NewTrinoConfig() error = %v", err)
	}

	if config.AllowedCatalogs != nil {
		t.Errorf("AllowedCatalogs = %v, want nil", config.AllowedCatalogs)
	}

	if config.AllowedSchemas != nil {
		t.Errorf("AllowedSchemas = %v, want nil", config.AllowedSchemas)
	}

	if config.AllowedTables != nil {
		t.Errorf("AllowedTables = %v, want nil", config.AllowedTables)
	}
}

func TestValidateAllowlist(t *testing.T) {
	tests := []struct {
		name         string
		allowlist    []string
		expectedDots int
		expectedErr  string
	}{
		{
			name:         "Valid schema format",
			allowlist:    []string{"hive.analytics", "postgresql.public"},
			expectedDots: 1,
			expectedErr:  "",
		},
		{
			name:         "Valid table format",
			allowlist:    []string{"hive.analytics.users", "postgresql.public.orders"},
			expectedDots: 2,
			expectedErr:  "",
		},
		{
			name:         "Invalid schema format - no dots",
			allowlist:    []string{"hive", "postgresql"},
			expectedDots: 1,
			expectedErr:  "invalid format in TEST_ALLOWLIST: 'hive' (expected 1 dots, found 0)",
		},
		{
			name:         "Invalid schema format - too many dots",
			allowlist:    []string{"hive.analytics.users"},
			expectedDots: 1,
			expectedErr:  "invalid format in TEST_ALLOWLIST: 'hive.analytics.users' (expected 1 dots, found 2)",
		},
		{
			name:         "Invalid table format - not enough dots",
			allowlist:    []string{"hive.analytics"},
			expectedDots: 2,
			expectedErr:  "invalid format in TEST_ALLOWLIST: 'hive.analytics' (expected 2 dots, found 1)",
		},
		{
			name:         "Mixed valid and invalid",
			allowlist:    []string{"hive.analytics", "postgresql"},
			expectedDots: 1,
			expectedErr:  "invalid format in TEST_ALLOWLIST: 'postgresql' (expected 1 dots, found 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAllowlist("TEST_ALLOWLIST", tt.allowlist, tt.expectedDots)
			if tt.expectedErr == "" {
				if err != nil {
					t.Errorf("validateAllowlist() expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("validateAllowlist() expected error %q, got nil", tt.expectedErr)
				} else if err.Error() != tt.expectedErr {
					t.Errorf("validateAllowlist() error = %q, want %q", err.Error(), tt.expectedErr)
				}
			}
		})
	}
}

func TestNewTrinoConfigMaxRows(t *testing.T) {
	// Save and restore env
	origMaxRows := os.Getenv("TRINO_MAX_ROWS")
	origOAuth := os.Getenv("OAUTH_ENABLED")
	defer func() {
		_ = os.Setenv("TRINO_MAX_ROWS", origMaxRows)
		_ = os.Setenv("OAUTH_ENABLED", origOAuth)
	}()
	_ = os.Setenv("OAUTH_ENABLED", "false")

	tests := []struct {
		name     string
		envValue string
		unset    bool
		expected int
	}{
		{"Default (unset)", "", true, 10000},
		{"Explicit zero (unlimited)", "0", false, 0},
		{"Custom value", "500", false, 500},
		{"Large value", "1000000", false, 1000000},
		{"Negative falls back to default", "-1", false, 10000},
		{"Non-integer falls back to default", "abc", false, 10000},
		{"MaxRows=1", "1", false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unset {
				_ = os.Unsetenv("TRINO_MAX_ROWS")
			} else {
				_ = os.Setenv("TRINO_MAX_ROWS", tt.envValue)
			}
			cfg, err := NewTrinoConfig()
			if err != nil {
				t.Fatalf("NewTrinoConfig() error = %v", err)
			}
			if cfg.MaxRows != tt.expected {
				t.Errorf("MaxRows = %d, want %d", cfg.MaxRows, tt.expected)
			}
		})
	}
}

func TestNewTrinoConfigDefaultTimeout(t *testing.T) {
	// Save and restore env
	origTimeout := os.Getenv("TRINO_QUERY_TIMEOUT")
	origOAuth := os.Getenv("OAUTH_ENABLED")
	defer func() {
		_ = os.Setenv("TRINO_QUERY_TIMEOUT", origTimeout)
		_ = os.Setenv("OAUTH_ENABLED", origOAuth)
	}()
	_ = os.Setenv("OAUTH_ENABLED", "false")

	// Test default timeout is 300s
	_ = os.Unsetenv("TRINO_QUERY_TIMEOUT")
	cfg, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("NewTrinoConfig() error = %v", err)
	}
	expected := 300 * time.Second
	if cfg.QueryTimeout != expected {
		t.Errorf("QueryTimeout = %v, want %v", cfg.QueryTimeout, expected)
	}

	// Test custom timeout
	_ = os.Setenv("TRINO_QUERY_TIMEOUT", "60")
	cfg, err = NewTrinoConfig()
	if err != nil {
		t.Fatalf("NewTrinoConfig() error = %v", err)
	}
	expected = 60 * time.Second
	if cfg.QueryTimeout != expected {
		t.Errorf("QueryTimeout = %v, want %v", cfg.QueryTimeout, expected)
	}
}

func TestNewTrinoConfigMalformedAllowlist(t *testing.T) {
	// Save original environment
	originalSchemas := os.Getenv("TRINO_ALLOWED_SCHEMAS")
	originalTables := os.Getenv("TRINO_ALLOWED_TABLES")
	originalOAuth := os.Getenv("OAUTH_ENABLED")

	// Clean up after test
	defer func() {
		_ = os.Setenv("TRINO_ALLOWED_SCHEMAS", originalSchemas)
		_ = os.Setenv("TRINO_ALLOWED_TABLES", originalTables)
		_ = os.Setenv("OAUTH_ENABLED", originalOAuth)
	}()

	tests := []struct {
		name          string
		envVar        string
		value         string
		expectedError string
	}{
		{
			name:          "Malformed schema entry (no dots)",
			envVar:        "TRINO_ALLOWED_SCHEMAS",
			value:         "hive,postgresql.public",
			expectedError: "invalid format in TRINO_ALLOWED_SCHEMAS: 'hive' (expected 1 dots, found 0)",
		},
		{
			name:          "Malformed schema entry (too many dots)",
			envVar:        "TRINO_ALLOWED_SCHEMAS",
			value:         "hive.analytics.users,postgresql.public",
			expectedError: "invalid format in TRINO_ALLOWED_SCHEMAS: 'hive.analytics.users' (expected 1 dots, found 2)",
		},
		{
			name:          "Malformed table entry (not enough dots)",
			envVar:        "TRINO_ALLOWED_TABLES",
			value:         "hive.analytics,hive.analytics.users",
			expectedError: "invalid format in TRINO_ALLOWED_TABLES: 'hive.analytics' (expected 2 dots, found 1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv(tt.envVar, tt.value)
			_ = os.Setenv("OAUTH_ENABLED", "false") // Disable OAuth for this test
			_, err := NewTrinoConfig()

			if err == nil {
				t.Fatalf("NewTrinoConfig() expected an error, got nil")
			}
			if err.Error() != tt.expectedError {
				t.Errorf("NewTrinoConfig() error = %q, want %q", err.Error(), tt.expectedError)
			}
			_ = os.Unsetenv(tt.envVar) // Clean up for next test
		})
	}
}