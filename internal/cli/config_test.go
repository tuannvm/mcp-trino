package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultCLIConfig(t *testing.T) {
	cfg := DefaultCLIConfig()
	if cfg == nil {
		t.Fatal("DefaultCLIConfig() returned nil")
	}
	if cfg.Output.Format != "table" {
		t.Errorf("expected default format 'table', got '%s'", cfg.Output.Format)
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

	p := cfg.Profiles["default"]
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
		t.Error("expected SSL.Enabled to be true")
	}
}

func TestParseCLIConfig_InvalidJSON(t *testing.T) {
	_, err := ParseCLIConfig([]byte(`{"profiles": invalid}`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestParseCLIConfig_EmptyJSON(t *testing.T) {
	cfg, err := ParseCLIConfig([]byte(``))
	if err != nil {
		t.Fatalf("ParseCLIConfig() failed: %v", err)
	}
	if len(cfg.Profiles) == 0 {
		t.Error("expected default profiles for empty config")
	}
}

func TestParseYAMLConfig(t *testing.T) {
	yamlData := []byte(`current: default
profiles:
  default:
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
  prod:
    host: prod.example.com
    port: 443
    user: prod_user
    ssl:
      enabled: true
output:
  format: csv
`)

	cfg, err := ParseCLIConfigWithPath(yamlData, "/tmp/config.yaml")
	if err != nil {
		t.Fatalf("ParseCLIConfigWithPath(yaml) failed: %v", err)
	}

	if cfg.Current != "default" {
		t.Errorf("expected current 'default', got '%s'", cfg.Current)
	}

	p := cfg.Profiles["default"]
	if p.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", p.Host)
	}
	if p.Port != 8080 {
		t.Errorf("expected port 8080, got %d", p.Port)
	}
	if p.User != "testuser" {
		t.Errorf("expected user 'testuser', got '%s'", p.User)
	}
	if p.Password != "testpass" {
		t.Errorf("expected password 'testpass', got '%s'", p.Password)
	}
	if p.Source != "test_source" {
		t.Errorf("expected source 'test_source', got '%s'", p.Source)
	}
	if p.SSL.Enabled == nil || !*p.SSL.Enabled {
		t.Error("expected SSL enabled true")
	}

	prod := cfg.Profiles["prod"]
	if prod.Host != "prod.example.com" {
		t.Errorf("expected prod host 'prod.example.com', got '%s'", prod.Host)
	}

	if cfg.Output.Format != "csv" {
		t.Errorf("expected format 'csv', got '%s'", cfg.Output.Format)
	}
}

func TestParseYAMLConfig_LegacyFlat(t *testing.T) {
	yamlData := []byte(`trino:
  host: legacy-host
  port: 9090
  user: legacy-user
  catalog: mycatalog
  ssl:
    enabled: true
    insecure: true
output:
  format: csv
`)

	cfg, err := ParseCLIConfigWithPath(yamlData, "/tmp/config.yaml")
	if err != nil {
		t.Fatalf("ParseCLIConfigWithPath(yaml legacy) failed: %v", err)
	}

	p, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile from legacy flat migration")
	}
	if p.Host != "legacy-host" {
		t.Errorf("expected host 'legacy-host', got '%s'", p.Host)
	}
	if p.Port != 9090 {
		t.Errorf("expected port 9090, got %d", p.Port)
	}
	if p.Catalog != "mycatalog" {
		t.Errorf("expected catalog 'mycatalog', got '%s'", p.Catalog)
	}
	if p.SSL.Enabled == nil || !*p.SSL.Enabled {
		t.Error("expected SSL enabled true")
	}
	if !p.SSL.Insecure {
		t.Error("expected SSL insecure true")
	}
	if cfg.Output.Format != "csv" {
		t.Errorf("expected format 'csv', got '%s'", cfg.Output.Format)
	}
}

func TestParseYAMLConfig_Invalid(t *testing.T) {
	_, err := ParseCLIConfigWithPath([]byte(`{invalid: yaml: [`), "/tmp/config.yaml")
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestParseYAMLConfig_FieldsAfterSSL(t *testing.T) {
	yamlData := []byte(`trino:
  host: myhost
  ssl:
    enabled: true
    insecure: true
  user: afterssl
  catalog: mycat
`)

	cfg, err := ParseCLIConfigWithPath(yamlData, "/tmp/config.yaml")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	p := cfg.Profiles["default"]
	if p.User != "afterssl" {
		t.Errorf("expected user 'afterssl', got '%s'", p.User)
	}
	if p.Catalog != "mycat" {
		t.Errorf("expected catalog 'mycat', got '%s'", p.Catalog)
	}
	if p.SSL.Enabled == nil || !*p.SSL.Enabled {
		t.Error("expected SSL enabled true")
	}
}

func TestGetOutputFormat(t *testing.T) {
	tests := []struct {
		name, format, expected string
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
			if result := cfg.GetOutputFormat(); result != tt.expected {
				t.Errorf("GetOutputFormat() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestApplyToEnv(t *testing.T) {
	for _, v := range []string{"TRINO_HOST", "TRINO_PORT", "TRINO_USER", "TRINO_PASSWORD", "TRINO_CATALOG", "TRINO_SCHEMA", "TRINO_SSL", "TRINO_SOURCE"} {
		_ = os.Unsetenv(v)
	}

	sslEnabled := true
	cfg := &CLIConfig{
		Current: "test-profile",
		Profiles: map[string]TrinoProfileConfig{
			"test-profile": {
				Host: "testhost", Port: 9000, User: "testuser",
				Password: "testpass", Catalog: "test_catalog", Schema: "test_schema",
				Source: "test_source", SSL: SSLConfig{Enabled: &sslEnabled},
			},
		},
	}

	_ = cfg.ApplyToEnv("test-profile")

	checks := map[string]string{
		"TRINO_HOST": "testhost", "TRINO_PORT": "9000", "TRINO_USER": "testuser",
		"TRINO_SOURCE": "test_source", "TRINO_SSL": "true",
	}
	for k, want := range checks {
		if got := os.Getenv(k); got != want {
			t.Errorf("expected %s='%s', got '%s'", k, want, got)
		}
	}
}

func TestApplyToEnv_SSLDisabled(t *testing.T) {
	_ = os.Unsetenv("TRINO_SSL")
	sslEnabled := false
	cfg := &CLIConfig{
		Current:  "p",
		Profiles: map[string]TrinoProfileConfig{"p": {Host: "h", SSL: SSLConfig{Enabled: &sslEnabled}}},
	}
	_ = cfg.ApplyToEnv("p")
	if os.Getenv("TRINO_SSL") != "false" {
		t.Errorf("expected TRINO_SSL='false', got '%s'", os.Getenv("TRINO_SSL"))
	}
}

func TestApplyToEnv_SSLNotSet(t *testing.T) {
	_ = os.Unsetenv("TRINO_SSL")
	cfg := &CLIConfig{
		Current:  "p",
		Profiles: map[string]TrinoProfileConfig{"p": {Host: "h", SSL: SSLConfig{Enabled: nil}}},
	}
	_ = cfg.ApplyToEnv("p")
	if ssl := os.Getenv("TRINO_SSL"); ssl != "" {
		t.Errorf("expected TRINO_SSL unset, got '%s'", ssl)
	}
}

func TestLoadCLIConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
	_ = os.Setenv("HOME", tmpDir)

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}
	if cfg.Output.Format != "table" {
		t.Errorf("expected default format 'table', got '%s'", cfg.Output.Format)
	}
}

func TestLoadCLIConfig_JSONFirst(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
	_ = os.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "trino")
	_ = os.MkdirAll(configDir, 0755)

	// Write both JSON and YAML — JSON should win
	_ = os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"current":"json-profile","profiles":{"json-profile":{"host":"json-host","port":8080,"user":"u"}}}`), 0600)
	_ = os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("current: yaml-profile\nprofiles:\n  yaml-profile:\n    host: yaml-host\n    port: 8080\n    user: u\n"), 0600)

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}
	if cfg.Current != "json-profile" {
		t.Errorf("expected JSON to take precedence, got current='%s'", cfg.Current)
	}
}

func TestLoadCLIConfig_YAMLFallback(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
	_ = os.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "trino")
	_ = os.MkdirAll(configDir, 0755)

	_ = os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("current: default\nprofiles:\n  default:\n    host: yaml-host\n    port: 7070\n    user: yaml-user\n"), 0600)

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}
	if cfg.Profiles["default"].Host != "yaml-host" {
		t.Errorf("expected host 'yaml-host', got '%s'", cfg.Profiles["default"].Host)
	}
	// ConfigPath should be the YAML file (no forced migration)
	if !strings.HasSuffix(cfg.ConfigPath, "config.yaml") {
		t.Errorf("expected ConfigPath to be yaml, got '%s'", cfg.ConfigPath)
	}
}

func TestSaveCLIConfig_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
	_ = os.Setenv("HOME", tmpDir)

	cfg := &CLIConfig{
		Current:  "default",
		Profiles: map[string]TrinoProfileConfig{"default": {Host: "testhost", Port: 8080, User: "testuser"}},
		Output:   OutputConfig{Format: "json"},
	}

	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatalf("SaveCLIConfig() failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "trino", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file not created at %s", configPath)
	}

	loaded, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}
	if loaded.Profiles["default"].Host != "testhost" {
		t.Errorf("expected host 'testhost', got '%s'", loaded.Profiles["default"].Host)
	}
}

func TestSaveCLIConfig_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &CLIConfig{
		ConfigPath: yamlPath,
		Current:    "default",
		Profiles:   map[string]TrinoProfileConfig{"default": {Host: "yamlhost", Port: 443, User: "yuser"}},
		Output:     OutputConfig{Format: "table"},
	}

	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatalf("SaveCLIConfig(yaml) failed: %v", err)
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("failed to read saved yaml: %v", err)
	}
	if !strings.Contains(string(data), "yamlhost") {
		t.Errorf("expected YAML to contain 'yamlhost', got: %s", string(data))
	}

	// Verify we can load it back
	loaded, err := ParseCLIConfigWithPath(data, yamlPath)
	if err != nil {
		t.Fatalf("failed to parse saved yaml: %v", err)
	}
	if loaded.Profiles["default"].Host != "yamlhost" {
		t.Errorf("roundtrip failed: expected 'yamlhost', got '%s'", loaded.Profiles["default"].Host)
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
		t.Errorf("expected 'localhost', got '%s'", profile.Host)
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
		t.Errorf("expected 'prod.example.com', got '%s'", profile.Host)
	}
}

func TestGetActiveProfile_NotFound(t *testing.T) {
	cfg := &CLIConfig{
		Current:  "default",
		Profiles: map[string]TrinoProfileConfig{"default": {Host: "localhost", Port: 8080, User: "trino"}},
	}
	if _, err := cfg.GetActiveProfile("nonexistent"); err == nil {
		t.Error("should fail for non-existent profile")
	}
}

func TestValidate(t *testing.T) {
	// Valid
	cfg := &CLIConfig{Current: "p", Profiles: map[string]TrinoProfileConfig{"p": {Host: "h", Port: 443, User: "u"}}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed: %v", err)
	}

	// Invalid current
	cfg2 := &CLIConfig{Current: "missing", Profiles: map[string]TrinoProfileConfig{"p": {Host: "h", Port: 443, User: "u"}}}
	if err := cfg2.Validate(); err == nil {
		t.Error("should fail when current doesn't exist")
	}

	// Missing fields
	for _, tc := range []struct {
		name string
		p    TrinoProfileConfig
	}{
		{"missing host", TrinoProfileConfig{Port: 443, User: "u"}},
		{"invalid port", TrinoProfileConfig{Host: "h", Port: 0, User: "u"}},
		{"missing user", TrinoProfileConfig{Host: "h", Port: 443}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &CLIConfig{Current: "t", Profiles: map[string]TrinoProfileConfig{"t": tc.p}}
			if err := c.Validate(); err == nil {
				t.Error("should fail")
			}
		})
	}
}

func TestGetProfileNames(t *testing.T) {
	cfg := &CLIConfig{
		Profiles: map[string]TrinoProfileConfig{
			"prod": {}, "staging": {}, "dev": {},
		},
	}
	names := cfg.GetProfileNames()
	if len(names) != 3 {
		t.Errorf("expected 3 profiles, got %d", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("not sorted: %v", names)
		}
	}
}

func TestSetCurrent(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
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

	loaded, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() failed: %v", err)
	}
	if loaded.Current != "prod" {
		t.Errorf("expected saved current='prod', got '%s'", loaded.Current)
	}
}

func TestSaveCLIConfig_YAML_NoTrinoLeak(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	// Simulate a config that was loaded from legacy flat YAML (Trino field set)
	cfg := &CLIConfig{
		ConfigPath: yamlPath,
		Current:    "default",
		Profiles:   map[string]TrinoProfileConfig{"default": {Host: "h", Port: 80, User: "u"}},
		Trino:      &TrinoProfileConfig{Host: "leaked"},
	}

	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatalf("SaveCLIConfig() failed: %v", err)
	}

	data, _ := os.ReadFile(yamlPath)
	if strings.Contains(string(data), "leaked") {
		t.Errorf("saved YAML should not contain legacy Trino field, got:\n%s", string(data))
	}
}

func TestSetCurrent_NotFound(t *testing.T) {
	cfg := &CLIConfig{
		Current:  "default",
		Profiles: map[string]TrinoProfileConfig{"default": {Host: "localhost", Port: 8080, User: "trino"}},
	}
	if err := cfg.SetCurrent("nonexistent"); err == nil {
		t.Error("should fail for non-existent profile")
	}
}
