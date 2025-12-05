package config

import (
	"os"
	"reflect"
	"testing"
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

func TestExternalAuthConfiguration(t *testing.T) {
	// Save original environment
	originalExtAuth := os.Getenv("TRINO_EXTERNAL_AUTH")
	originalExtAuthTimeout := os.Getenv("TRINO_EXTERNAL_AUTH_TIMEOUT")
	originalOAuth := os.Getenv("OAUTH_ENABLED")

	// Clean up after test
	defer func() {
		os.Setenv("TRINO_EXTERNAL_AUTH", originalExtAuth)
		os.Setenv("TRINO_EXTERNAL_AUTH_TIMEOUT", originalExtAuthTimeout)
		os.Setenv("OAUTH_ENABLED", originalOAuth)
	}()

	tests := []struct {
		name            string
		extAuth         string
		extAuthTimeout  string
		wantExtAuth     bool
		wantTimeout     int
	}{
		{
			name:        "External auth disabled by default",
			extAuth:     "",
			wantExtAuth: false,
			wantTimeout: 300,
		},
		{
			name:        "External auth enabled",
			extAuth:     "true",
			wantExtAuth: true,
			wantTimeout: 300,
		},
		{
			name:           "Custom timeout",
			extAuth:        "true",
			extAuthTimeout: "600",
			wantExtAuth:    true,
			wantTimeout:    600,
		},
		{
			name:           "Invalid timeout uses default",
			extAuth:        "true",
			extAuthTimeout: "invalid",
			wantExtAuth:    true,
			wantTimeout:    300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("TRINO_EXTERNAL_AUTH")
			os.Unsetenv("TRINO_EXTERNAL_AUTH_TIMEOUT")
			os.Setenv("OAUTH_ENABLED", "false")

			if tt.extAuth != "" {
				os.Setenv("TRINO_EXTERNAL_AUTH", tt.extAuth)
			}
			if tt.extAuthTimeout != "" {
				os.Setenv("TRINO_EXTERNAL_AUTH_TIMEOUT", tt.extAuthTimeout)
			}

			config, err := NewTrinoConfig()
			if err != nil {
				t.Fatalf("NewTrinoConfig() error = %v", err)
			}

			if config.ExternalAuth != tt.wantExtAuth {
				t.Errorf("ExternalAuth = %v, want %v", config.ExternalAuth, tt.wantExtAuth)
			}
			if config.ExternalAuthTimeout != tt.wantTimeout {
				t.Errorf("ExternalAuthTimeout = %v, want %v", config.ExternalAuthTimeout, tt.wantTimeout)
			}
		})
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