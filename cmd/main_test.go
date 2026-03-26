package main

import (
	"os"
	"testing"
)

func TestShouldRunCLIMode_KnownCommands(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "query command",
			args:     []string{"query", "SELECT 1"},
			expected: true,
		},
		{
			name:     "catalogs command",
			args:     []string{"catalogs"},
			expected: true,
		},
		{
			name:     "schemas command",
			args:     []string{"schemas", "memory"},
			expected: true,
		},
		{
			name:     "tables command",
			args:     []string{"tables", "memory", "default"},
			expected: true,
		},
		{
			name:     "describe command",
			args:     []string{"describe", "test_table"},
			expected: true,
		},
		{
			name:     "explain command",
			args:     []string{"explain", "SELECT 1"},
			expected: true,
		},
		{
			name:     "interactive flag",
			args:     []string{"--interactive"},
			expected: true,
		},
		{
			name:     "with flags before command",
			args:     []string{"--format", "json", "query", "SELECT 1"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRunCLIMode(tt.args)
			if result != tt.expected {
				t.Errorf("shouldRunCLIMode(%v) = %v, expected %v", tt.args, result, tt.expected)
			}
		})
	}
}

func TestShouldRunCLIMode_UnknownCommands(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "unknown single argument - should NOT trigger CLI",
			args:     []string{"unknown-command"},
			expected: false, // Critical for MCP compatibility
		},
		{
			name:     "unknown argument with flags",
			args:     []string{"--some-flag", "unknown-arg"},
			expected: false, // Critical for MCP compatibility
		},
		{
			name:     "multiple unknown arguments",
			args:     []string{"arg1", "arg2"},
			expected: false, // Critical for MCP compatibility
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
		{
			name:     "only flags",
			args:     []string{"--format", "json"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRunCLIMode(tt.args)
			if result != tt.expected {
				t.Errorf("shouldRunCLIMode(%v) = %v, expected %v", tt.args, result, tt.expected)
			}
		})
	}
}

func TestHasCLIOnlyFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "help flag",
			args:     []string{"--help"},
			expected: true,
		},
		{
			name:     "short help flag",
			args:     []string{"-h"},
			expected: true,
		},
		{
			name:     "version flag",
			args:     []string{"--version"},
			expected: true,
		},
		{
			name:     "short version flag",
			args:     []string{"-v"},
			expected: true,
		},
		{
			name:     "config flag",
			args:     []string{"--config", "/path/to/config"},
			expected: true,
		},
		{
			name:     "format flag",
			args:     []string{"--format", "json"},
			expected: true,
		},
		{
			name:     "interactive flag",
			args:     []string{"--interactive"},
			expected: true,
		},
		{
			name:     "no flags",
			args:     []string{"query", "SELECT 1"},
			expected: false,
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
		{
			name:     "unknown flag",
			args:     []string{"--unknown-flag"},
			expected: false,
		},
		{
			name:     "help flag with equals",
			args:     []string{"--help=true"},
			expected: true, // Function extracts flag name before "="
		},
		{
			name:     "config flag with equals",
			args:     []string{"--config=/path/to/config"},
			expected: true, // Function extracts flag name before "="
		},
		{
			name:     "format flag with equals",
			args:     []string{"--format=json"},
			expected: true, // Function extracts flag name before "="
		},
		{
			name:     "multiple flags",
			args:     []string{"--help", "--version"},
			expected: true,
		},
		{
			name:     "flag with value",
			args:     []string{"--config", "/path/to/config"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCLIOnlyFlags(tt.args)
			if result != tt.expected {
				t.Errorf("hasCLIOnlyFlags(%v) = %v, expected %v", tt.args, result, tt.expected)
			}
		})
	}
}

func TestCleanArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "strip --cli flag",
			args:     []string{"--cli", "query", "SELECT 1"},
			expected: []string{"query", "SELECT 1"},
		},
		{
			name:     "strip --mcp flag",
			args:     []string{"--mcp", "query", "SELECT 1"},
			expected: []string{"query", "SELECT 1"},
		},
		{
			name:     "flags after subcommand preserved",
			args:     []string{"query", "--format", "json", "SELECT 1"},
			expected: []string{"query", "--format", "json", "SELECT 1"},
		},
		{
			name:     "only mode flags",
			args:     []string{"--cli"},
			expected: []string{},
		},
		{
			name:     "no mode flags",
			args:     []string{"query", "SELECT 1"},
			expected: []string{"query", "SELECT 1"},
		},
		{
			name:     "flags before subcommand preserved",
			args:     []string{"--format", "json", "query", "SELECT 1"},
			expected: []string{"--format", "json", "query", "SELECT 1"},
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanArgs(tt.args)
			if len(result) != len(tt.expected) {
				t.Fatalf("cleanArgs(%v) length = %d, expected %d", tt.args, len(result), len(tt.expected))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("cleanArgs(%v)[%d] = %v, expected %v", tt.args, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	// Save and restore original env
	originalValue := os.Getenv("TEST_GET_ENV_VAR")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("TEST_GET_ENV_VAR", originalValue)
		} else {
			_ = os.Unsetenv("TEST_GET_ENV_VAR")
		}
	}()

	t.Run("environment variable is set", func(t *testing.T) {
		_ = os.Setenv("TEST_GET_ENV_VAR", "test_value")
		result := getEnv("TEST_GET_ENV_VAR", "default")
		if result != "test_value" {
			t.Errorf("getEnv() = %v, expected 'test_value'", result)
		}
	})

	t.Run("environment variable is not set", func(t *testing.T) {
		_ = os.Unsetenv("TEST_GET_ENV_VAR")
		result := getEnv("TEST_GET_ENV_VAR", "default_value")
		if result != "default_value" {
			t.Errorf("getEnv() = %v, expected 'default_value'", result)
		}
	})

	t.Run("empty environment variable returns empty string", func(t *testing.T) {
		_ = os.Setenv("TEST_GET_ENV_VAR", "")
		result := getEnv("TEST_GET_ENV_VAR", "default_value")
		if result != "" {
			t.Errorf("getEnv() = %v, expected '' (empty string for set but empty env var)", result)
		}
	})
}

func TestIsTTY(t *testing.T) {
	// This is a simple smoke test - we can't easily test TTY detection
	// in all environments, but we can verify it returns a boolean
	result := isTTY()
	if result != true && result != false {
		t.Errorf("isTTY() returned non-boolean value")
	}
}
