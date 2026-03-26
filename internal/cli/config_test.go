package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCLIConfig(t *testing.T) {
	cfg := DefaultCLIConfig()

	if cfg == nil {
		t.Fatal("DefaultCLIConfig() returned nil")
	}

	if cfg.Output.Format != "table" {
		t.Errorf("expected default format to be 'table', got '%s'", cfg.Output.Format)
	}
}

func TestParseCLIConfig_ValidYAML(t *testing.T) {
	yamlData := []byte(`
trino:
  host: localhost
  port: 8080
  user: testuser
  password: testpass
  catalog: test_catalog
  schema: test_schema
  source: test_source
  ssl:
    enabled: true
    insecure: false
output:
  format: json
`)

	cfg, err := ParseCLIConfig(yamlData)
	if err != nil {
		t.Fatalf("ParseCLIConfig() failed: %v", err)
	}

	if cfg.Trino.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", cfg.Trino.Host)
	}
	if cfg.Trino.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Trino.Port)
	}
	if cfg.Trino.User != "testuser" {
		t.Errorf("expected user 'testuser', got '%s'", cfg.Trino.User)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("expected output format 'json', got '%s'", cfg.Output.Format)
	}

	// Test SSL pointer bool
	if cfg.Trino.SSL.Enabled == nil {
		t.Error("expected SSL.Enabled to be non-nil when explicitly set")
	}
	if cfg.Trino.SSL.Enabled != nil && !*cfg.Trino.SSL.Enabled {
		t.Error("expected SSL.Enabled to be true")
	}
}

func TestParseCLIConfig_InvalidYAML(t *testing.T) {
	yamlData := []byte(`trino: host: [invalid`)

	_, err := ParseCLIConfig(yamlData)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestParseCLIConfig_EmptyYAML(t *testing.T) {
	yamlData := []byte(``)

	cfg, err := ParseCLIConfig(yamlData)
	if err != nil {
		t.Fatalf("ParseCLIConfig() failed: %v", err)
	}

	if cfg.Output.Format != "" {
		t.Errorf("expected empty format for empty YAML, got '%s'", cfg.Output.Format)
	}
}

func TestGetOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{"JSON format", "json", "json"},
		{"Table format", "table", "table"},
		{"CSV format", "csv", "csv"},
		{"Empty format", "", "table"},
		{"Unknown format", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &CLIConfig{}
			cfg.Output.Format = tt.format
			result := cfg.GetOutputFormat()
			if result != tt.expected {
				t.Errorf("GetOutputFormat() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestApplyToEnv(t *testing.T) {
	// Clean environment before test
	envVars := []string{"TRINO_HOST", "TRINO_PORT", "TRINO_USER", "TRINO_PASSWORD", "TRINO_CATALOG", "TRINO_SCHEMA", "TRINO_SSL", "TRINO_SOURCE"}
	for _, envVar := range envVars {
		_ = os.Unsetenv(envVar)
	}

	cfg := &CLIConfig{
		Trino: struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Catalog  string `yaml:"catalog"`
			Schema   string `yaml:"schema"`
			Source   string `yaml:"source"`
			SSL      struct {
				Enabled  *bool `yaml:"enabled"`
				Insecure bool  `yaml:"insecure"`
			} `yaml:"ssl"`
		}{
			Host:     "testhost",
			Port:     9000,
			User:     "testuser",
			Password: "testpass",
			Catalog:  "test_catalog",
			Schema:   "test_schema",
			Source:   "test_source",
		},
	}

	// Test SSL enabled = true
	sslEnabled := true
	cfg.Trino.SSL.Enabled = &sslEnabled

	cfg.ApplyToEnv()

	// Verify environment variables were set
	if os.Getenv("TRINO_HOST") != "testhost" {
		t.Errorf("expected TRINO_HOST='testhost', got '%s'", os.Getenv("TRINO_HOST"))
	}
	if os.Getenv("TRINO_PORT") != "9000" {
		t.Errorf("expected TRINO_PORT='9000', got '%s'", os.Getenv("TRINO_PORT"))
	}
	if os.Getenv("TRINO_USER") != "testuser" {
		t.Errorf("expected TRINO_USER='testuser', got '%s'", os.Getenv("TRINO_USER"))
	}
	if os.Getenv("TRINO_SOURCE") != "test_source" {
		t.Errorf("expected TRINO_SOURCE='test_source', got '%s'", os.Getenv("TRINO_SOURCE"))
	}
	if os.Getenv("TRINO_SSL") != "true" {
		t.Errorf("expected TRINO_SSL='true', got '%s'", os.Getenv("TRINO_SSL"))
	}
}

func TestApplyToEnv_SSLDisabled(t *testing.T) {
	// Clean environment before test
	_ = os.Unsetenv("TRINO_SSL")

	cfg := &CLIConfig{
		Trino: struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Catalog  string `yaml:"catalog"`
			Schema   string `yaml:"schema"`
			Source   string `yaml:"source"`
			SSL      struct {
				Enabled  *bool `yaml:"enabled"`
				Insecure bool  `yaml:"insecure"`
			} `yaml:"ssl"`
		}{
			Host: "testhost",
		},
	}

	// Test SSL enabled = false
	sslEnabled := false
	cfg.Trino.SSL.Enabled = &sslEnabled

	cfg.ApplyToEnv()

	// Verify TRINO_SSL is set to false
	if os.Getenv("TRINO_SSL") != "false" {
		t.Errorf("expected TRINO_SSL='false', got '%s'", os.Getenv("TRINO_SSL"))
	}
}

func TestApplyToEnv_SSLNotSet(t *testing.T) {
	// Clean environment before test
	_ = os.Unsetenv("TRINO_SSL")

	cfg := &CLIConfig{
		Trino: struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Catalog  string `yaml:"catalog"`
			Schema   string `yaml:"schema"`
			Source   string `yaml:"source"`
			SSL      struct {
				Enabled  *bool `yaml:"enabled"`
				Insecure bool  `yaml:"insecure"`
			} `yaml:"ssl"`
		}{
			Host: "testhost",
		},
	}

	// SSL.Enabled is nil (not set in config)
	cfg.ApplyToEnv()

	// Verify TRINO_SSL is NOT set (preserves default)
	if ssl := os.Getenv("TRINO_SSL"); ssl != "" {
		t.Errorf("expected TRINO_SSL to not be set when SSL.Enabled is nil, got '%s'", ssl)
	}
}

func TestLoadCLIConfig_MissingFile(t *testing.T) {
	// Use a temp directory to ensure config doesn't exist
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	// Set HOME to temp dir (where no .config/trino/config.yaml exists)
	_ = os.Setenv("HOME", tmpDir)

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

	// Should return default config
	if cfg.Output.Format != "table" {
		t.Errorf("expected default format 'table', got '%s'", cfg.Output.Format)
	}
}

func TestSaveCLIConfig(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tmpDir)

	cfg := &CLIConfig{
		Trino: struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Catalog  string `yaml:"catalog"`
			Schema   string `yaml:"schema"`
			Source   string `yaml:"source"`
			SSL      struct {
				Enabled  *bool `yaml:"enabled"`
				Insecure bool  `yaml:"insecure"`
			} `yaml:"ssl"`
		}{
			Host: "testhost",
		},
		Output: struct {
			Format string `yaml:"format"`
		}{
			Format: "json",
		},
	}

	err := SaveCLIConfig(cfg)
	if err != nil {
		t.Fatalf("SaveCLIConfig() failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".config", "trino", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file was not created at %s", configPath)
	}

	// Verify we can load it back
	loadedCfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

	if loadedCfg.Trino.Host != "testhost" {
		t.Errorf("expected host 'testhost', got '%s'", loadedCfg.Trino.Host)
	}
	if loadedCfg.Output.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", loadedCfg.Output.Format)
	}
}
