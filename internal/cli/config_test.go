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

func TestParseCLIConfig_ValidJSON(t *testing.T) {
	jsonData := []byte(`{
  "current": "default",
  "profiles": {
    "default": {
      "host": "localhost",
      "port": 8080,
      "user": "testuser",
      "password": "testpass",
      "catalog": "test_catalog",
      "schema": "test_schema",
      "source": "test_source",
      "ssl": {
        "enabled": true,
        "insecure": false
      }
    }
  },
  "output": {
    "format": "json"
  }
}`)

	cfg, err := ParseCLIConfig(jsonData)
	if err != nil {
		t.Fatalf("ParseCLIConfig() failed: %v", err)
	}

	defaultProfile, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile to exist")
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

	if defaultProfile.SSL.Enabled == nil {
		t.Error("expected SSL.Enabled to be non-nil when explicitly set")
	}
	if defaultProfile.SSL.Enabled != nil && !*defaultProfile.SSL.Enabled {
		t.Error("expected SSL.Enabled to be true")
	}
}

func TestParseCLIConfig_InvalidJSON(t *testing.T) {
	jsonData := []byte(`{"profiles": invalid}`)

	_, err := ParseCLIConfig(jsonData)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestParseCLIConfig_EmptyJSON(t *testing.T) {
	jsonData := []byte(``)

	cfg, err := ParseCLIConfig(jsonData)
	if err != nil {
		t.Fatalf("ParseCLIConfig() failed: %v", err)
	}

	// Should get default profiles
	if len(cfg.Profiles) == 0 {
		t.Error("expected default profiles for empty config")
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
				SSL: SSLConfig{
					Enabled: &sslEnabled,
				},
			},
		},
	}

	_ = cfg.ApplyToEnv("test-profile")

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
	_ = os.Unsetenv("TRINO_SSL")

	sslEnabled := false
	cfg := &CLIConfig{
		Current: "test-profile",
		Profiles: map[string]TrinoProfileConfig{
			"test-profile": {
				Host: "testhost",
				SSL: SSLConfig{
					Enabled: &sslEnabled,
				},
			},
		},
	}

	_ = cfg.ApplyToEnv("test-profile")

	if os.Getenv("TRINO_SSL") != "false" {
		t.Errorf("expected TRINO_SSL='false', got '%s'", os.Getenv("TRINO_SSL"))
	}
}

func TestApplyToEnv_SSLNotSet(t *testing.T) {
	_ = os.Unsetenv("TRINO_SSL")

	cfg := &CLIConfig{
		Current: "test-profile",
		Profiles: map[string]TrinoProfileConfig{
			"test-profile": {
				Host: "testhost",
				SSL: SSLConfig{
					Enabled: nil,
				},
			},
		},
	}

	_ = cfg.ApplyToEnv("test-profile")

	if ssl := os.Getenv("TRINO_SSL"); ssl != "" {
		t.Errorf("expected TRINO_SSL to not be set when SSL.Enabled is nil, got '%s'", ssl)
	}
}

func TestLoadCLIConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	_ = os.Setenv("HOME", tmpDir)

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

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
		Output: OutputConfig{
			Format: "json",
		},
	}

	err := SaveCLIConfig(cfg)
	if err != nil {
		t.Fatalf("SaveCLIConfig() failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "trino", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file was not created at %s", configPath)
	}

	loadedCfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

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

func TestGetActiveProfile_Default(t *testing.T) {
	cfg := &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {Host: "localhost", Port: 8080, User: "trino"},
			"prod":    {Host: "prod.example.com", Port: 443, User: "prod_user"},
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
			"default": {Host: "localhost", Port: 8080, User: "trino"},
			"prod":    {Host: "prod.example.com", Port: 443, User: "prod_user"},
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
			"default": {Host: "localhost", Port: 8080, User: "trino"},
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
			"prod": {Host: "prod.example.com", Port: 443, User: "prod_user"},
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
			"prod": {Host: "prod.example.com", Port: 443, User: "prod_user"},
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
			name:    "missing host",
			profile: TrinoProfileConfig{Port: 443, User: "testuser"},
		},
		{
			name:    "invalid port",
			profile: TrinoProfileConfig{Host: "testhost", Port: 0, User: "testuser"},
		},
		{
			name:    "missing user",
			profile: TrinoProfileConfig{Host: "testhost", Port: 443},
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

	loadedCfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

	if loadedCfg.Current != "prod" {
		t.Errorf("expected saved current='prod', got '%s'", loadedCfg.Current)
	}
}

func TestMigrateYAMLConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "trino")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlData := []byte(`current: default
profiles:
  default:
    host: localhost
    port: 8080
    user: testuser
    password: testpass
    catalog: test_catalog
    schema: test_schema
    ssl:
      enabled: true
      insecure: false
output:
  format: json
`)
	yamlPath := filepath.Join(configDir, "config.yaml")
	jsonPath := filepath.Join(configDir, "config.json")

	if err := os.WriteFile(yamlPath, yamlData, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := migrateYAMLConfig(yamlPath, jsonPath)
	if err != nil {
		t.Fatalf("migrateYAMLConfig() failed: %v", err)
	}

	if cfg.Current != "default" {
		t.Errorf("expected current 'default', got '%s'", cfg.Current)
	}
	p, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile")
	}
	if p.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", p.Host)
	}
	if p.Port != 8080 {
		t.Errorf("expected port 8080, got %d", p.Port)
	}
	if p.User != "testuser" {
		t.Errorf("expected user 'testuser', got '%s'", p.User)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", cfg.Output.Format)
	}
	if p.SSL.Enabled == nil || !*p.SSL.Enabled {
		t.Error("expected SSL enabled true")
	}

	// Verify JSON file was created
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON config file was not created")
	}
}

func TestMigrateYAMLConfig_LegacyFlat(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "trino")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlData := []byte(`trino:
  host: legacy-host
  port: 9090
  user: legacy-user
output:
  format: csv
`)
	yamlPath := filepath.Join(configDir, "config.yaml")
	jsonPath := filepath.Join(configDir, "config.json")

	if err := os.WriteFile(yamlPath, yamlData, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := migrateYAMLConfig(yamlPath, jsonPath)
	if err != nil {
		t.Fatalf("migrateYAMLConfig() failed: %v", err)
	}

	p, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile from legacy migration")
	}
	if p.Host != "legacy-host" {
		t.Errorf("expected host 'legacy-host', got '%s'", p.Host)
	}
	if p.Port != 9090 {
		t.Errorf("expected port 9090, got %d", p.Port)
	}
}

func TestLoadCLIConfig_FallbackToYAML(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "trino")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlData := []byte(`current: default
profiles:
  default:
    host: yaml-host
    port: 7070
    user: yaml-user
`)
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), yamlData, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}

	p, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile from YAML fallback")
	}
	if p.Host != "yaml-host" {
		t.Errorf("expected host 'yaml-host', got '%s'", p.Host)
	}

	// Verify JSON was created (migration happened)
	jsonPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON config was not created during migration")
	}
}

func TestMigrateYAMLConfig_FieldsAfterSSL(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "trino")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Fields after ssl block must be parsed correctly
	yamlData := []byte(`trino:
  host: myhost
  ssl:
    enabled: true
    insecure: true
  user: afterssl
  catalog: mycat
`)
	yamlPath := filepath.Join(configDir, "config.yaml")
	jsonPath := filepath.Join(configDir, "config.json")

	if err := os.WriteFile(yamlPath, yamlData, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := migrateYAMLConfig(yamlPath, jsonPath)
	if err != nil {
		t.Fatalf("migrateYAMLConfig() failed: %v", err)
	}

	p := cfg.Profiles["default"]
	if p.User != "afterssl" {
		t.Errorf("expected user 'afterssl' (after ssl block), got '%s'", p.User)
	}
	if p.Catalog != "mycat" {
		t.Errorf("expected catalog 'mycat' (after ssl block), got '%s'", p.Catalog)
	}
	if p.SSL.Enabled == nil || !*p.SSL.Enabled {
		t.Error("expected SSL enabled true")
	}
	if !p.SSL.Insecure {
		t.Error("expected SSL insecure true")
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
