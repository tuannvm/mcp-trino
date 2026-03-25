package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// CLIConfig represents the YAML configuration file structure
type CLIConfig struct {
	Trino struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Catalog  string `yaml:"catalog"`
		Schema   string `yaml:"schema"`
		Source   string `yaml:"source"`
		SSL      struct {
			Enabled  *bool `yaml:"enabled"` // pointer to distinguish unset vs false
			Insecure bool  `yaml:"insecure"`
		} `yaml:"ssl"`
	} `yaml:"trino"`
	Output struct {
		Format string `yaml:"format"` // table, json, csv
	} `yaml:"output"`
}

// LoadCLIConfig loads the CLI configuration from ~/.mcp-trino/config.yaml
func LoadCLIConfig() (*CLIConfig, error) {
	// Use home directory directly for ~/.mcp-trino/config.yaml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".mcp-trino", "config.yaml")

	// If config doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return defaultCLIConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// ParseCLIConfig parses CLI configuration from YAML data
func ParseCLIConfig(data []byte) (*CLIConfig, error) {
	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return &cfg, nil
}

// SaveCLIConfig saves the CLI configuration to ~/.mcp-trino/config.yaml
func SaveCLIConfig(cfg *CLIConfig) error {
	// Use home directory directly for ~/.mcp-trino/config.yaml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".mcp-trino", "config.yaml")

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// DefaultCLIConfig returns a default CLI configuration
func DefaultCLIConfig() *CLIConfig {
	return defaultCLIConfig()
}

// defaultCLIConfig returns a default CLI configuration
func defaultCLIConfig() *CLIConfig {
	return &CLIConfig{
		Output: struct {
			Format string `yaml:"format"`
		}{
			Format: "table",
		},
	}
}

// ApplyToEnv applies CLI config to environment variables (only if not already set)
func (c *CLIConfig) ApplyToEnv() {
	setEnvIfAbsent("TRINO_HOST", c.Trino.Host)
	// Only set port if it's a valid non-zero value
	if c.Trino.Port > 0 {
		setEnvIfAbsent("TRINO_PORT", fmt.Sprintf("%d", c.Trino.Port))
	}
	setEnvIfAbsent("TRINO_USER", c.Trino.User)
	setEnvIfAbsent("TRINO_PASSWORD", c.Trino.Password)
	setEnvIfAbsent("TRINO_CATALOG", c.Trino.Catalog)
	setEnvIfAbsent("TRINO_SCHEMA", c.Trino.Schema)
	if c.Trino.Source != "" {
		setEnvIfAbsent("TRINO_SOURCE", c.Trino.Source)
	}
	// Only set SSL flags if explicitly configured in the YAML (non-nil pointer)
	if c.Trino.SSL.Enabled != nil {
		setEnvIfAbsent("TRINO_SSL", fmt.Sprintf("%t", *c.Trino.SSL.Enabled))
	}
	if c.Trino.SSL.Insecure {
		setEnvIfAbsent("TRINO_SSL_INSECURE", "true")
	}
}

// GetOutputFormat returns the output format from config or default
func (c *CLIConfig) GetOutputFormat() string {
	if c.Output.Format == "" {
		return "table"
	}
	return c.Output.Format
}

// setEnvIfAbsent sets an environment variable only if it's not already set
func setEnvIfAbsent(key, value string) {
	if value == "" {
		return
	}
	if _, exists := os.LookupEnv(key); !exists {
		os.Setenv(key, value)
	}
}
