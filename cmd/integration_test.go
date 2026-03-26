package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// getBinaryPath returns the path to the built binary
func getBinaryPath() string {
	if os.Getenv("GO_BINARY") != "" {
		return os.Getenv("GO_BINARY")
	}
	return "./bin/mcp-trino"
}

// buildTestBinary builds the binary for testing if it doesn't exist
// Set FORCE_REBUILD=1 to force a rebuild even if binary exists
func buildTestBinary(t *testing.T) string {
	binaryPath := getBinaryPath()

	// Check if we should force rebuild
	forceRebuild := os.Getenv("FORCE_REBUILD") == "1"

	if !forceRebuild {
		if _, err := os.Stat(binaryPath); err == nil {
			t.Logf("Using existing binary: %s", binaryPath)
			return binaryPath
		}
	}

	// Build the binary
	// We need to build from the module root, not the cmd directory
	// When running tests in cmd/, "." refers to the cmd package
	t.Log("Building test binary...")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build test binary: %v\nOutput: %s", err, output)
	}
	t.Logf("Built test binary: %s", binaryPath)
	return binaryPath
}

func TestIntegration_VersionFlag(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version flag failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "mcp-trino version") {
		t.Errorf("Expected version output to contain 'mcp-trino version', got: %s", outputStr)
	}
}

func TestIntegration_HelpFlag(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Help flag failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Usage of mcp-trino") {
		t.Errorf("Expected help output to contain usage, got: %s", outputStr)
	}
}

func TestIntegration_CLICommandWithBadHost(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "catalogs")
	// Set a non-existent host to trigger connection error without trying to connect to real services
	cmd.Env = append(os.Environ(), "TRINO_HOST=invalid-host-that-does-not-exist.local", "TRINO_PORT=9999")

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail (non-zero exit)
	if err == nil {
		t.Error("Expected CLI to fail with bad host, but it succeeded")
	}

	// Should contain error message
	if !strings.Contains(outputStr, "failed") && !strings.Contains(outputStr, "error") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected error message in output for bad host")
	}
}

func TestIntegration_UnknownArgPreservesMCPMode(t *testing.T) {
	binary := buildTestBinary(t)

	// Unknown argument should trigger MCP mode (backward compatibility)
	// We can't test full MCP mode without complex setup, but we can verify
	// it doesn't crash and shows MCP server startup behavior
	cmd := exec.Command(binary, "unknown-argument")
	cmd.Env = append(os.Environ(), "MCP_PROTOCOL_VERSION=1.0")

	// Add a timeout since MCP server will hang waiting for stdin
	timeout := time.AfterFunc(2*time.Second, func() {
		_ = cmd.Process.Kill()
	})

	output, err := cmd.CombinedOutput()
	timeout.Stop()

	outputStr := string(output)

	// Should show MCP server startup
	if !strings.Contains(outputStr, "Starting Trino MCP Server") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected MCP server startup message for unknown argument")
	}

	// The command will be killed, so we expect an error
	if err == nil {
		t.Error("Expected process to be terminated, but it completed")
	}
}

func TestIntegration_FormatFlagValidation(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--format", "invalid-format", "catalogs")
	cmd.Env = append(os.Environ(), "TRINO_HOST=localhost")

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail with invalid format error
	if err == nil {
		t.Error("Expected CLI to fail with invalid format, but it succeeded")
	}

	if !strings.Contains(outputStr, "invalid output format") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected 'invalid output format' error message")
	}
}

func TestIntegration_ConfigFilePrecedence(t *testing.T) {
	binary := buildTestBinary(t)

	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := tempDir + "/config.yaml"
	configContent := `
trino:
  host: from-config-host
  port: 9999
  user: testuser
output:
  format: json
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test: Config file values are used
	// Clear TRINO environment variables to ensure config file is used
	cmd := exec.Command(binary, "--config", configFile, "catalogs")
	cmd.Env = []string{
		"HOME=" + tempDir,
		"PATH=" + os.Getenv("PATH"),
	}

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Verify config file host is being used (will fail to connect, but that's expected)
	if !strings.Contains(outputStr, "from-config-host") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected config file host 'from-config-host' to be used, but it wasn't found in output")
	}
}

func TestIntegration_ConfigOverridesEnvVar(t *testing.T) {
	binary := buildTestBinary(t)

	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := tempDir + "/config.yaml"
	configContent := `
trino:
  host: from-config-host
  port: 9999
  user: testuser
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test: Config file overrides env var (env vars are lowest priority)
	cmd := exec.Command(binary, "--config", configFile, "catalogs")
	cmd.Env = append(os.Environ(),
		"HOME="+tempDir,
		"TRINO_HOST=from-env-host",
	)

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Verify config host is used and env host is NOT used
	// Precedence: CLI flags > --profile > TRINO_PROFILE > current > default > env vars
	if !strings.Contains(outputStr, "from-config-host") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected config file host 'from-config-host' to be used, but it wasn't found in output")
	}
	if strings.Contains(outputStr, "from-env-host") {
		t.Logf("Output: %s", outputStr)
		t.Error("Env var host 'from-env-host' should NOT be used when config file is set")
	}
}

func TestIntegration_FlagOverridesEnv(t *testing.T) {
	binary := buildTestBinary(t)

	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := tempDir + "/config.yaml"
	configContent := `trino:
  host: from-config-host
  port: 9999
  user: testuser
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test: Flag overrides env var
	cmd := exec.Command(binary, "--config", configFile, "--host", "from-flag-host", "catalogs")
	cmd.Env = append(os.Environ(),
		"HOME="+tempDir,
		"TRINO_HOST=from-env-host",
	)

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Verify flag host is used and env/config hosts are NOT used
	if !strings.Contains(outputStr, "from-flag-host") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected flag host 'from-flag-host' to be used, but it wasn't found in output")
	}
	if strings.Contains(outputStr, "from-env-host") {
		t.Logf("Output: %s", outputStr)
		t.Error("Env var host 'from-env-host' should NOT be used when flag is set")
	}
	if strings.Contains(outputStr, "from-config-host") {
		t.Logf("Output: %s", outputStr)
		t.Error("Config file host 'from-config-host' should NOT be used when flag is set")
	}
}

func TestIntegration_ErrorPropagation(t *testing.T) {
	binary := buildTestBinary(t)

	// Test error propagation for bad host
	cmd := exec.Command(binary, "query", "SELECT 1")
	cmd.Env = append(os.Environ(),
		"TRINO_HOST=invalid-test-host.invalid",
		"TRINO_PORT=9999",
	)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err == nil {
		t.Error("Expected query to fail with invalid host, but it succeeded")
	}

	// Check that error message is informative
	if !strings.Contains(outputStr, "failed") && !strings.Contains(outputStr, "error") {
		t.Logf("Error propagation test output: %s", outputStr)
		t.Error("Expected error message in output")
	}
}

func TestIntegration_ModeSelection_ExplicitMCP(t *testing.T) {
	binary := buildTestBinary(t)

	// --mcp flag should force MCP mode even with CLI command
	cmd := exec.Command(binary, "--mcp", "query", "SELECT 1")
	cmd.Env = append(os.Environ(), "MCP_PROTOCOL_VERSION=1.0")

	// Add timeout since MCP server waits for stdin
	timeout := time.AfterFunc(2*time.Second, func() {
		_ = cmd.Process.Kill()
	})
	defer timeout.Stop()

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should start MCP server
	if !strings.Contains(outputStr, "Starting Trino MCP Server") {
		t.Logf("Mode selection test output: %s", outputStr)
		t.Error("Expected MCP server startup with --mcp flag")
	}

	// Process was killed, so error is expected
	if err == nil {
		t.Error("Expected process to be terminated")
	}
}

func TestIntegration_ModeSelection_ExplicitCLI(t *testing.T) {
	binary := buildTestBinary(t)

	// --cli flag should force CLI mode even with MCP_PROTOCOL_VERSION
	cmd := exec.Command(binary, "--cli", "catalogs")
	cmd.Env = append(os.Environ(),
		"MCP_PROTOCOL_VERSION=1.0",
		"TRINO_HOST=localhost",
	)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should run CLI command (and fail to connect, which is expected)
	if !strings.Contains(outputStr, "failed") && !strings.Contains(outputStr, "error") {
		t.Logf("CLI mode test output: %s", outputStr)
		t.Error("Expected CLI command to execute")
	}

	// Should have an error (connection failure)
	if err == nil {
		t.Error("Expected CLI command to fail with connection error")
	}
}
