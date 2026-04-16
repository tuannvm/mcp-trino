package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TrinoProfileConfig represents a single Trino connection profile
type TrinoProfileConfig struct {
	Host     string    `json:"host" yaml:"host"`
	Port     int       `json:"port" yaml:"port"`
	User     string    `json:"user" yaml:"user"`
	Password string    `json:"password,omitempty" yaml:"password,omitempty"`
	Catalog  string    `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Schema   string    `json:"schema,omitempty" yaml:"schema,omitempty"`
	Source   string    `json:"source,omitempty" yaml:"source,omitempty"`
	SSL      SSLConfig `json:"ssl,omitempty" yaml:"ssl,omitempty"`
}

// SSLConfig holds SSL configuration for a profile
type SSLConfig struct {
	Enabled  *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"` // pointer to distinguish unset vs false
	Insecure bool  `json:"insecure,omitempty" yaml:"insecure,omitempty"`
}

// CLIConfig represents the configuration file structure (JSON or YAML)
type CLIConfig struct {
	// ConfigPath tracks where this config was loaded from (not serialized)
	ConfigPath string `json:"-" yaml:"-"`

	Current  string                        `json:"current" yaml:"current"`
	Profiles map[string]TrinoProfileConfig `json:"profiles" yaml:"profiles"`
	Output   OutputConfig                  `json:"output,omitempty" yaml:"output,omitempty"`

	// Legacy flat config (YAML only, read-only — auto-migrated to profiles on load, never serialized)
	Trino *TrinoProfileConfig `json:"-" yaml:"trino,omitempty"`
}

// OutputConfig holds output formatting configuration
type OutputConfig struct {
	Format string `json:"format,omitempty" yaml:"format,omitempty"` // table, json, csv
}

// isYAMLPath returns true if the path has a .yaml or .yml extension
func isYAMLPath(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}

// LoadCLIConfig loads the CLI configuration from ~/.config/trino/
// Tries config.json first, then config.yaml. Both formats are supported natively.
func LoadCLIConfig() (*CLIConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "trino")
	jsonPath := filepath.Join(configDir, "config.json")
	yamlPath := filepath.Join(configDir, "config.yaml")

	// Try JSON first
	if _, err := os.Stat(jsonPath); err == nil {
		return loadConfigFromFile(jsonPath)
	}

	// Try YAML
	if _, err := os.Stat(yamlPath); err == nil {
		return loadConfigFromFile(yamlPath)
	}

	// No config file — return defaults with JSON path
	cfg := defaultCLIConfig()
	cfg.ConfigPath = jsonPath
	return cfg, nil
}

// loadConfigFromFile reads and parses a config file, detecting format by extension
func loadConfigFromFile(path string) (*CLIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg *CLIConfig
	if isYAMLPath(path) {
		cfg, err = parseYAMLConfig(data)
	} else {
		cfg, err = parseJSONConfig(data)
	}
	if err != nil {
		return nil, err
	}

	cfg.ConfigPath = path
	return cfg, nil
}

// ParseCLIConfig parses CLI configuration from JSON data
func ParseCLIConfig(data []byte) (*CLIConfig, error) {
	return parseJSONConfig(data)
}

// ParseCLIConfigWithPath parses CLI configuration from data, detecting format by path extension
func ParseCLIConfigWithPath(data []byte, configPath string) (*CLIConfig, error) {
	var cfg *CLIConfig
	var err error
	if isYAMLPath(configPath) {
		cfg, err = parseYAMLConfig(data)
	} else {
		cfg, err = parseJSONConfig(data)
	}
	if err != nil {
		return nil, err
	}
	cfg.ConfigPath = configPath
	return cfg, nil
}

// parseJSONConfig parses and normalizes a JSON config
func parseJSONConfig(data []byte) (*CLIConfig, error) {
	var cfg CLIConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	}
	normalizeConfig(&cfg)
	return &cfg, nil
}

// parseYAMLConfig parses and normalizes a YAML config
func parseYAMLConfig(data []byte) (*CLIConfig, error) {
	var cfg CLIConfig
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	}
	// Migrate legacy flat "trino:" section to profiles
	if cfg.Trino != nil && len(cfg.Profiles) == 0 {
		cfg.Profiles = map[string]TrinoProfileConfig{
			"default": *cfg.Trino,
		}
		if cfg.Current == "" {
			cfg.Current = "default"
		}
		cfg.Trino = nil
	}
	normalizeConfig(&cfg)
	return &cfg, nil
}

// normalizeConfig ensures a config has valid defaults
func normalizeConfig(cfg *CLIConfig) {
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}
}

// SaveCLIConfig saves the CLI configuration, using the format matching ConfigPath extension
func SaveCLIConfig(cfg *CLIConfig) error {
	configPath := cfg.ConfigPath
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".config", "trino", "config.json")
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Shallow copy to avoid mutating caller; clear legacy field before serializing
	toSave := *cfg
	toSave.Trino = nil

	var data []byte
	var err error
	if isYAMLPath(configPath) {
		data, err = yaml.Marshal(&toSave)
	} else {
		data, err = json.MarshalIndent(&toSave, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// DefaultCLIConfig returns a default CLI configuration
func DefaultCLIConfig() *CLIConfig {
	return defaultCLIConfig()
}

func defaultCLIConfig() *CLIConfig {
	return &CLIConfig{
		Current: "default",
		Profiles: map[string]TrinoProfileConfig{
			"default": {
				Host:    "localhost",
				Port:    8080,
				User:    "trino",
				Catalog: "memory",
				Schema:  "default",
			},
		},
		Output: OutputConfig{
			Format: "table",
		},
	}
}

// GetActiveProfile returns the active profile based on precedence
func (c *CLIConfig) GetActiveProfile(profileName string) (*TrinoProfileConfig, error) {
	name := c.resolveProfileName(profileName)

	profile, exists := c.Profiles[name]
	if !exists {
		return nil, fmt.Errorf("profile '%s' not found in config. Available profiles: %v",
			name, c.getProfileNames())
	}

	return &profile, nil
}

func (c *CLIConfig) resolveProfileName(explicitName string) string {
	if explicitName != "" {
		return explicitName
	}
	if c.Current != "" {
		return c.Current
	}
	return "default"
}

func (c *CLIConfig) getProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetProfileNames returns a sorted list of profile names (public)
func (c *CLIConfig) GetProfileNames() []string {
	return c.getProfileNames()
}

// Validate validates the config
func (c *CLIConfig) Validate() error {
	if c.Current != "" {
		if _, exists := c.Profiles[c.Current]; !exists {
			return fmt.Errorf("current profile '%s' does not exist. Available profiles: %v",
				c.Current, c.getProfileNames())
		}
	}

	for name, profile := range c.Profiles {
		if profile.Host == "" {
			return fmt.Errorf("profile '%s' is missing required field 'host'", name)
		}
		if profile.Port <= 0 {
			return fmt.Errorf("profile '%s' has invalid port '%d'", name, profile.Port)
		}
		if profile.User == "" {
			return fmt.Errorf("profile '%s' is missing required field 'user'", name)
		}
	}

	return nil
}

// SetCurrent sets the current profile and saves the config
func (c *CLIConfig) SetCurrent(name string) error {
	if _, exists := c.Profiles[name]; !exists {
		return fmt.Errorf("profile '%s' not found. Available profiles: %v",
			name, c.getProfileNames())
	}
	c.Current = name
	return SaveCLIConfig(c)
}

// ApplyToEnv applies CLI config to environment variables
func (c *CLIConfig) ApplyToEnv(profileName string) error {
	profile, err := c.GetActiveProfile(profileName)
	if err != nil {
		return err
	}

	setEnvIfValue("TRINO_HOST", profile.Host)
	if profile.Port > 0 {
		setEnvIfValue("TRINO_PORT", fmt.Sprintf("%d", profile.Port))
	}
	setEnvIfValue("TRINO_USER", profile.User)
	setEnvIfValue("TRINO_PASSWORD", profile.Password)
	setEnvIfValue("TRINO_CATALOG", profile.Catalog)
	setEnvIfValue("TRINO_SCHEMA", profile.Schema)
	if profile.Source != "" {
		setEnvIfValue("TRINO_SOURCE", profile.Source)
	}
	if profile.SSL.Enabled != nil {
		setEnvIfValue("TRINO_SSL", fmt.Sprintf("%t", *profile.SSL.Enabled))
		setEnvIfValue("TRINO_SSL_INSECURE", fmt.Sprintf("%t", profile.SSL.Insecure))
	}
	return nil
}

// GetOutputFormat returns the output format from config or default
func (c *CLIConfig) GetOutputFormat() string {
	if c.Output.Format == "" {
		return "table"
	}
	return c.Output.Format
}

func setEnvIfValue(key, value string) {
	if value == "" {
		return
	}
	_ = os.Setenv(key, value)
}
