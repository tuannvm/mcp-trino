package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TrinoProfileConfig represents a single Trino connection profile
type TrinoProfileConfig struct {
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
}

// CLIConfig represents the YAML configuration file structure
type CLIConfig struct {
	// ConfigPath tracks where this config was loaded from (not saved to YAML)
	ConfigPath string `yaml:"-"`

	Current  string                       `yaml:"current"` // default profile name
	Profiles map[string]TrinoProfileConfig `yaml:"profiles"`
	Output   struct {
		Format string `yaml:"format"` // table, json, csv
	} `yaml:"output"`

	// Legacy fields for backward compatibility (auto-migrated to profiles)
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
}

// LoadCLIConfig loads the CLI configuration from ~/.config/trino/config.yaml
func LoadCLIConfig() (*CLIConfig, error) {
	// Use XDG config directory: ~/.config/trino/config.yaml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "trino", "config.yaml")

	// If config doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := defaultCLIConfig()
		cfg.ConfigPath = configPath
		return cfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Auto-migrate legacy flat config to profiles
	if err := cfg.migrateLegacyConfig(); err != nil {
		return nil, fmt.Errorf("failed to migrate legacy config: %w", err)
	}

	// If no profiles exist after migration, ensure we have a default profile
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}

	cfg.ConfigPath = configPath
	return &cfg, nil
}

// ParseCLIConfig parses CLI configuration from YAML data
func ParseCLIConfig(data []byte) (*CLIConfig, error) {
	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	// Auto-migrate legacy flat config to profiles for custom config files too
	if err := cfg.migrateLegacyConfig(); err != nil {
		return nil, fmt.Errorf("failed to migrate legacy config: %w", err)
	}
	// If no profiles exist after migration, ensure we have a default profile
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}
	return &cfg, nil
}

// ParseCLIConfigWithPath parses CLI configuration from YAML data and sets the config path
func ParseCLIConfigWithPath(data []byte, configPath string) (*CLIConfig, error) {
	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	// Set ConfigPath before migration so it saves to the correct location
	cfg.ConfigPath = configPath
	// Auto-migrate legacy flat config to profiles
	if err := cfg.migrateLegacyConfig(); err != nil {
		return nil, fmt.Errorf("failed to migrate legacy config: %w", err)
	}
	// If no profiles exist after migration, ensure we have a default profile
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}
	return &cfg, nil
}

// SaveCLIConfig saves the CLI configuration to the path it was loaded from,
// or to ~/.config/trino/config.yaml if no path was set
func SaveCLIConfig(cfg *CLIConfig) error {
	var configPath string
	if cfg.ConfigPath != "" {
		configPath = cfg.ConfigPath
	} else {
		// Use XDG config directory: ~/.config/trino/config.yaml
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".config", "trino", "config.yaml")
	}

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
		Output: struct {
			Format string `yaml:"format"`
		}{
			Format: "table",
		},
	}
}

// migrateLegacyConfig migrates old flat trino config to profiles structure
func (c *CLIConfig) migrateLegacyConfig() error {
	// Check if we have legacy flat config (has trino.host but no profiles)
	hasLegacyConfig := c.Trino.Host != ""
	hasProfiles := len(c.Profiles) > 0

	if hasLegacyConfig && !hasProfiles {
		// Migrate legacy config to "default" profile
		c.Profiles = map[string]TrinoProfileConfig{
			"default": {
				Host:     c.Trino.Host,
				Port:     c.Trino.Port,
				User:     c.Trino.User,
				Password: c.Trino.Password,
				Catalog:  c.Trino.Catalog,
				Schema:   c.Trino.Schema,
				Source:   c.Trino.Source,
				SSL:      c.Trino.SSL,
			},
		}
		// Set current to default if not already set
		if c.Current == "" {
			c.Current = "default"
		}
		// Clear legacy config (not strictly necessary but clean)
		c.Trino = struct {
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
		}{}
		// Save the migrated config
		if err := SaveCLIConfig(c); err != nil {
			return fmt.Errorf("failed to save migrated config: %w", err)
		}
	}
	return nil
}

// GetActiveProfile returns the active profile based on precedence:
// 1. Explicit profile name (from --profile flag or TRINO_PROFILE env)
// 2. Current field in config
// 3. "default" profile fallback
func (c *CLIConfig) GetActiveProfile(profileName string) (*TrinoProfileConfig, error) {
	// Determine which profile to use
	name := c.resolveProfileName(profileName)

	// Validate profile exists
	profile, exists := c.Profiles[name]
	if !exists {
		return nil, fmt.Errorf("profile '%s' not found in config. Available profiles: %v",
			name, c.getProfileNames())
	}

	return &profile, nil
}

// resolveProfileName determines the active profile name based on precedence
func (c *CLIConfig) resolveProfileName(explicitName string) string {
	// 1. Explicit profile name (from --profile flag or TRINO_PROFILE env)
	if explicitName != "" {
		return explicitName
	}

	// 2. Current field in config
	if c.Current != "" {
		return c.Current
	}

	// 3. "default" profile fallback
	return "default"
}

// getProfileNames returns a sorted list of profile names
func (c *CLIConfig) getProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	// Simple sort (could use sort.Strings but avoiding import for now)
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

// GetProfileNames returns a sorted list of profile names (public)
func (c *CLIConfig) GetProfileNames() []string {
	return c.getProfileNames()
}

// Validate validates the config (e.g., current profile exists)
func (c *CLIConfig) Validate() error {
	// If current is set, ensure the profile exists
	if c.Current != "" {
		if _, exists := c.Profiles[c.Current]; !exists {
			return fmt.Errorf("current profile '%s' does not exist. Available profiles: %v",
				c.Current, c.getProfileNames())
		}
	}

	// Validate each profile has required fields
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
// This applies the active profile values to env vars (profiles override existing env vars)
// CLI flags will later override these env vars (highest priority)
func (c *CLIConfig) ApplyToEnv(profileName string) error {
	profile, err := c.GetActiveProfile(profileName)
	if err != nil {
		return err
	}

	setEnvIfValue("TRINO_HOST", profile.Host)
	// Only set port if it's a valid non-zero value
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
	// Only set SSL flags if explicitly configured in the YAML (non-nil pointer)
	if profile.SSL.Enabled != nil {
		setEnvIfValue("TRINO_SSL", fmt.Sprintf("%t", *profile.SSL.Enabled))
		// When SSL is configured, also set INSECURE to match profile (overrides env var)
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

// setEnvIfValue sets an environment variable to the given value (if non-empty)
// This overrides any existing value, allowing profiles to take precedence over env vars
func setEnvIfValue(key, value string) {
	if value == "" {
		return
	}
	_ = os.Setenv(key, value)
}
