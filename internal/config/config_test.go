package config

import (
	"os"
	"testing"
)

func TestNewTrinoConfig_TrinoSource(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedSource string
	}{
		{
			name:           "TRINO_SOURCE set to custom value",
			envValue:       "dataeng-trino-api",
			expectedSource: "dataeng-trino-api",
		},
		{
			name:           "TRINO_SOURCE set to empty string",
			envValue:       "",
			expectedSource: "",
		},
		{
			name:           "TRINO_SOURCE not set",
			envValue:       "UNSET",
			expectedSource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			os.Unsetenv("TRINO_SOURCE")
			
			// Set environment variable if not UNSET
			if tt.envValue != "UNSET" {
				os.Setenv("TRINO_SOURCE", tt.envValue)
				defer os.Unsetenv("TRINO_SOURCE")
			}

			config, err := NewTrinoConfig()
			if err != nil {
				t.Fatalf("NewTrinoConfig() failed: %v", err)
			}

			if config.TrinoSource != tt.expectedSource {
				t.Errorf("TrinoSource = %q, want %q", config.TrinoSource, tt.expectedSource)
			}
		})
	}
}

func TestNewTrinoConfig_DefaultValues(t *testing.T) {
	// Clean up environment
	for _, env := range []string{"TRINO_HOST", "TRINO_PORT", "TRINO_USER", "TRINO_CATALOG", "TRINO_SCHEMA", "TRINO_SOURCE"} {
		os.Unsetenv(env)
	}

	config, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("NewTrinoConfig() failed: %v", err)
	}

	// Check default values
	if config.Host != "localhost" {
		t.Errorf("Host = %q, want %q", config.Host, "localhost")
	}
	if config.Port != 8080 {
		t.Errorf("Port = %d, want %d", config.Port, 8080)
	}
	if config.User != "trino" {
		t.Errorf("User = %q, want %q", config.User, "trino")
	}
	if config.Catalog != "memory" {
		t.Errorf("Catalog = %q, want %q", config.Catalog, "memory")
	}
	if config.Schema != "default" {
		t.Errorf("Schema = %q, want %q", config.Schema, "default")
	}
	if config.TrinoSource != "" {
		t.Errorf("TrinoSource = %q, want empty string", config.TrinoSource)
	}
}
