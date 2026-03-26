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

	// After migration, data should be in Profiles["default"]
	defaultProfile, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile to exist after migration")
	}

	if defaultProfile.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", defaultProfile.Host)
	}
	if defaultProfile.Port != 8080 {
		t.Errorf("expected port 8080, got %d", defaultProfile.Port)
	}
	if defaultProfile.User != "testuser" {
		t.Errorf("expected user 'testuser', got '%s'", defaultProfile.User)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("expected output format 'json', got '%s'", cfg.Output.Format)
	}

	// Test SSL pointer bool
	if defaultProfile.SSL.Enabled == nil {
		t.Error("expected SSL.Enabled to be non-nil when explicitly set")
	}
	if defaultProfile.SSL.Enabled != nil && !*defaultProfile.SSL.Enabled {
		t.Error("expected SSL.Enabled to be true")
	}

	// Verify current is set to "default" after migration
	if cfg.Current != "default" {
		t.Errorf("expected current to be 'default' after migration, got '%s'", cfg.Current)
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

	sslEnabled := true
	cfg := &CLIConfig{
		Current: "test-profile",
		Profiles: map[string]TrinoProfileConfig{
			"test-profile": {
				Host:     "testhost",
				Port:     9000,
				User:     "testuser",
				Password: "testpass",
				Catalog:  "test_catalog",
				Schema:   "test_schema",
				Source:   "test_source",
				SSL: struct {
					Enabled  *bool `yaml:"enabled"`
					Insecure bool  `yaml:"insecure"`
				}{
					Enabled: &sslEnabled,
				},
			},
		},
	}

	_ = cfg.ApplyToEnv("test-profile")

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

	sslEnabled := false
	cfg := &CLIConfig{
		Current: "test-profile",
		Profiles: map[string]TrinoProfileConfig{
			"test-profile": {
				Host: "testhost",
				SSL: struct {
					Enabled  *bool `yaml:"enabled"`
					Insecure bool  `yaml:"insecure"`
				}{
					Enabled: &sslEnabled,
				},
			},
		},
	}

	_ = cfg.ApplyToEnv("test-profile")

	// Verify TRINO_SSL is set to false
	if os.Getenv("TRINO_SSL") != "false" {
		t.Errorf("expected TRINO_SSL='false', got '%s'", os.Getenv("TRINO_SSL"))
	}
}

func TestApplyToEnv_SSLNotSet(t *testing.T) {
	// Clean environment before test
	_ = os.Unsetenv("TRINO_SSL")

	cfg := &CLIConfig{
		Current: "test-profile",
		Profiles: map[string]TrinoProfileConfig{
			"test-profile": {
				Host: "testhost",
				SSL: struct {
					Enabled  *bool `yaml:"enabled"`
					Insecure bool  `yaml:"insecure"`
				}{
					Enabled: nil, // not set
				},
			},
		},
	}

	// SSL.Enabled is nil (not set in config)
	_ = cfg.ApplyToEnv("test-profile")

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
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {
				Host: "testhost",
				Port: 8080,
				User: "testuser",
			},
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

	// Check the default profile was loaded
	defaultProfile, exists := loadedCfg.Profiles["default"]
	if !exists {
		t.Fatal("default profile not found after loading")
	}

	if defaultProfile.Host != "testhost" {
		t.Errorf("expected host 'testhost', got '%s'", defaultProfile.Host)
	}
	if loadedCfg.Output.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", loadedCfg.Output.Format)
	}
}

// Profile-related tests

func TestGetActiveProfile_Default(t *testing.T) {
	cfg := &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {
				Host: "localhost",
				Port: 8080,
				User: "trino",
			},
			"prod": {
				Host: "prod.example.com",
				Port: 443,
				User: "prod_user",
			},
		},
	}

	profile, err := cfg.GetActiveProfile("")
	if err != nil {
		t.Fatalf("GetActiveProfile() failed: %v", err)
	}

	if profile.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", profile.Host)
	}
}

func TestGetActiveProfile_Explicit(t *testing.T) {
	cfg := &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {
				Host: "localhost",
				Port: 8080,
				User: "trino",
			},
			"prod": {
				Host: "prod.example.com",
				Port: 443,
				User: "prod_user",
			},
		},
	}

	profile, err := cfg.GetActiveProfile("prod")
	if err != nil {
		t.Fatalf("GetActiveProfile() failed: %v", err)
	}

	if profile.Host != "prod.example.com" {
		t.Errorf("expected host 'prod.example.com', got '%s'", profile.Host)
	}
}

func TestGetActiveProfile_NotFound(t *testing.T) {
	cfg := &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {
				Host: "localhost",
				Port: 8080,
				User: "trino",
			},
		},
	}

	_, err := cfg.GetActiveProfile("nonexistent")
	if err == nil {
		t.Error("GetActiveProfile() should fail for non-existent profile")
	}
}

func TestValidate_CurrentExists(t *testing.T) {
	cfg := &CLIConfig{
		Current: "prod",
		Profiles: map[string]TrinoProfileConfig{
			"prod": {
				Host: "prod.example.com",
				Port: 443,
				User: "prod_user",
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed: %v", err)
	}
}

func TestValidate_CurrentNotExists(t *testing.T) {
	cfg := &CLIConfig{
		Current: "nonexistent",
		Profiles: map[string]TrinoProfileConfig{
			"prod": {
				Host: "prod.example.com",
				Port: 443,
				User: "prod_user",
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail when current profile doesn't exist")
	}
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		profile TrinoProfileConfig
	}{
		{
			name: "missing host",
			profile: TrinoProfileConfig{
				Port: 443,
				User: "testuser",
			},
		},
		{
			name: "invalid port",
			profile: TrinoProfileConfig{
				Host: "testhost",
				Port: 0,
				User: "testuser",
			},
		},
		{
			name: "missing user",
			profile: TrinoProfileConfig{
				Host: "testhost",
				Port: 443,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &CLIConfig{
				Current: "test",
				Profiles: map[string]TrinoProfileConfig{
					"test": tt.profile,
				},
			}

			if err := cfg.Validate(); err == nil {
				t.Error("Validate() should fail for invalid profile")
			}
		})
	}
}

func TestGetProfileNames(t *testing.T) {
	cfg := &CLIConfig{
		Current: "prod",
		Profiles: map[string]TrinoProfileConfig{
			"prod":    {Host: "prod.example.com", Port: 443, User: "prod_user"},
			"staging": {Host: "staging.example.com", Port: 443, User: "staging_user"},
			"dev":     {Host: "localhost", Port: 8080, User: "dev_user"},
		},
	}

	names := cfg.GetProfileNames()

	if len(names) != 3 {
		t.Errorf("expected 3 profiles, got %d", len(names))
	}

	// Check that names are sorted
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("profile names not sorted: %v", names)
		}
	}
}

func TestSetCurrent(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tmpDir)

	cfg := &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {Host: "localhost", Port: 8080, User: "trino"},
			"prod":    {Host: "prod.example.com", Port: 443, User: "prod_user"},
		},
	}

	if err := cfg.SetCurrent("prod"); err != nil {
		t.Fatalf("SetCurrent() failed: %v", err)
	}

	if cfg.Current != "prod" {
		t.Errorf("expected current='prod', got '%s'", cfg.Current)
	}

	// Verify it was saved
	loadedCfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

	if loadedCfg.Current != "prod" {
		t.Errorf("expected saved current='prod', got '%s'", loadedCfg.Current)
	}
}

func TestSetCurrent_NotFound(t *testing.T) {
	cfg := &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {Host: "localhost", Port: 8080, User: "trino"},
		},
	}

	if err := cfg.SetCurrent("nonexistent"); err == nil {
		t.Error("SetCurrent() should fail for non-existent profile")
	}
}
