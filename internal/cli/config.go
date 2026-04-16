package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TrinoProfileConfig represents a single Trino connection profile
type TrinoProfileConfig struct {
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	User     string    `json:"user"`
	Password string    `json:"password,omitempty"`
	Catalog  string    `json:"catalog,omitempty"`
	Schema   string    `json:"schema,omitempty"`
	Source   string    `json:"source,omitempty"`
	SSL      SSLConfig `json:"ssl,omitempty"`
}

// SSLConfig holds SSL configuration for a profile
type SSLConfig struct {
	Enabled  *bool `json:"enabled,omitempty"` // pointer to distinguish unset vs false
	Insecure bool  `json:"insecure,omitempty"`
}

// CLIConfig represents the JSON configuration file structure
type CLIConfig struct {
	// ConfigPath tracks where this config was loaded from (not saved to JSON)
	ConfigPath string `json:"-"`

	Current  string                        `json:"current"`            // default profile name
	Profiles map[string]TrinoProfileConfig `json:"profiles"`           // connection profiles
	Output   OutputConfig                  `json:"output,omitempty"`   // output settings
}

// OutputConfig holds output formatting configuration
type OutputConfig struct {
	Format string `json:"format,omitempty"` // table, json, csv
}

// LoadCLIConfig loads the CLI configuration from ~/.config/trino/config.json
// Falls back to reading legacy ~/.config/trino/config.yaml if JSON doesn't exist
func LoadCLIConfig() (*CLIConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "trino")
	jsonPath := filepath.Join(configDir, "config.json")
	yamlPath := filepath.Join(configDir, "config.yaml")

	// Try JSON config first
	if _, err := os.Stat(jsonPath); err == nil {
		data, readErr := os.ReadFile(jsonPath)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read config file: %w", readErr)
		}
		cfg, parseErr := parseCLIConfigFromJSON(data)
		if parseErr != nil {
			return nil, parseErr
		}
		cfg.ConfigPath = jsonPath
		return cfg, nil
	}

	// Fall back to legacy YAML config and auto-migrate to JSON
	if _, err := os.Stat(yamlPath); err == nil {
		cfg, migrateErr := migrateYAMLConfig(yamlPath, jsonPath)
		if migrateErr != nil {
			return nil, fmt.Errorf("failed to migrate YAML config: %w", migrateErr)
		}
		return cfg, nil
	}

	// No config file — return defaults
	cfg := defaultCLIConfig()
	cfg.ConfigPath = jsonPath
	return cfg, nil
}

// migrateYAMLConfig reads a legacy YAML config, converts it to JSON, and saves it
// YAML is parsed with minimal stdlib-only support for the known config structure
func migrateYAMLConfig(yamlPath, jsonPath string) (*CLIConfig, error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config: %w", err)
	}

	cfg := parseSimpleYAML(data)
	cfg.ConfigPath = jsonPath

	// Ensure profiles exist
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}

	// Save as JSON
	if err := SaveCLIConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to save migrated config: %w", err)
	}

	return cfg, nil
}

// parseSimpleYAML does minimal YAML parsing for the known config structure
// Handles the two known formats: flat (trino.host) and profiles-based
func parseSimpleYAML(data []byte) *CLIConfig {
	cfg := &CLIConfig{
		Profiles: make(map[string]TrinoProfileConfig),
	}

	lines := strings.Split(string(data), "\n")
	var currentSection string
	var currentProfile string
	var currentSubsection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Count leading spaces for indent level
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		// Parse key: value
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := cleanYAMLValue(strings.TrimSpace(parts[1]))

		if indent == 0 {
			currentSection = key
			currentSubsection = ""
			currentProfile = ""
			if key == "current" && value != "" {
				cfg.Current = value
			}
			continue
		}

		switch currentSection {
		case "trino":
			// Legacy flat config — populate a "default" profile
			if _, exists := cfg.Profiles["default"]; !exists {
				cfg.Profiles["default"] = TrinoProfileConfig{}
			}
			p := cfg.Profiles["default"]
			// Reset subsection when returning to profile-level indent
			if currentSubsection == "ssl" && indent <= 2 {
				currentSubsection = ""
			}
			if key == "ssl" && value == "" {
				currentSubsection = "ssl"
				cfg.Profiles["default"] = p
				continue
			}
			if currentSubsection == "ssl" {
				applySSLField(&p, key, value)
			} else {
				applyProfileField(&p, key, value)
			}
			cfg.Profiles["default"] = p
			if cfg.Current == "" {
				cfg.Current = "default"
			}

		case "profiles":
			if indent == 2 && value == "" {
				// Profile name header
				currentProfile = key
				cfg.Profiles[currentProfile] = TrinoProfileConfig{}
				currentSubsection = ""
				continue
			}
			if currentProfile != "" {
				p := cfg.Profiles[currentProfile]
				// Reset subsection when returning to profile-field indent
				if currentSubsection == "ssl" && indent <= 4 {
					currentSubsection = ""
				}
				if key == "ssl" && value == "" {
					currentSubsection = "ssl"
					cfg.Profiles[currentProfile] = p
					continue
				}
				if currentSubsection == "ssl" {
					applySSLField(&p, key, value)
				} else {
					applyProfileField(&p, key, value)
				}
				cfg.Profiles[currentProfile] = p
			}

		case "output":
			if key == "format" && value != "" {
				cfg.Output.Format = value
			}
		}
	}

	return cfg
}

// cleanYAMLValue strips YAML quoting and inline comments from a value
func cleanYAMLValue(v string) string {
	// Strip inline comments (only outside quotes)
	if !strings.HasPrefix(v, "'") && !strings.HasPrefix(v, "\"") {
		if idx := strings.Index(v, " #"); idx >= 0 {
			v = strings.TrimSpace(v[:idx])
		}
	}
	// Strip surrounding quotes
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
	}
	return v
}

func applyProfileField(p *TrinoProfileConfig, key, value string) {
	switch key {
	case "host":
		p.Host = value
	case "port":
		if n, err := fmt.Sscanf(value, "%d", &p.Port); n == 0 || err != nil {
			p.Port = 0
		}
	case "user":
		p.User = value
	case "password":
		p.Password = value
	case "catalog":
		p.Catalog = value
	case "schema":
		p.Schema = value
	case "source":
		p.Source = value
	}
}

func applySSLField(p *TrinoProfileConfig, key, value string) {
	switch key {
	case "enabled":
		b := value == "true"
		p.SSL.Enabled = &b
	case "insecure":
		p.SSL.Insecure = value == "true"
	}
}

// ParseCLIConfig parses CLI configuration from JSON data
func ParseCLIConfig(data []byte) (*CLIConfig, error) {
	return parseCLIConfigFromJSON(data)
}

// ParseCLIConfigWithPath parses CLI configuration from JSON data and sets the config path
func ParseCLIConfigWithPath(data []byte, configPath string) (*CLIConfig, error) {
	cfg, err := parseCLIConfigFromJSON(data)
	if err != nil {
		return nil, err
	}
	cfg.ConfigPath = configPath
	return cfg, nil
}

// ParseYAMLConfigWithPath parses a legacy YAML config and sets the config path
func ParseYAMLConfigWithPath(data []byte, configPath string) (*CLIConfig, error) {
	cfg := parseSimpleYAML(data)
	cfg.ConfigPath = configPath
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}
	return cfg, nil
}

// parseCLIConfigFromJSON parses and normalizes a JSON config
func parseCLIConfigFromJSON(data []byte) (*CLIConfig, error) {
	var cfg CLIConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Ensure profiles map exists with defaults
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultCLIConfig().Profiles
		if cfg.Current == "" {
			cfg.Current = "default"
		}
	}

	return &cfg, nil
}

// SaveCLIConfig saves the CLI configuration to the path it was loaded from,
// or to ~/.config/trino/config.json if no path was set
func SaveCLIConfig(cfg *CLIConfig) error {
	configPath := cfg.ConfigPath
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".config", "trino", "config.json")
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
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
		Output: OutputConfig{
			Format: "table",
		},
	}
}

// GetActiveProfile returns the active profile based on precedence:
// 1. Explicit profile name (from --profile flag or TRINO_PROFILE env)
// 2. Current field in config
// 3. "default" profile fallback
func (c *CLIConfig) GetActiveProfile(profileName string) (*TrinoProfileConfig, error) {
	name := c.resolveProfileName(profileName)

	profile, exists := c.Profiles[name]
	if !exists {
		return nil, fmt.Errorf("profile '%s' not found in config. Available profiles: %v",
			name, c.getProfileNames())
	}

	return &profile, nil
}

// resolveProfileName determines the active profile name based on precedence
func (c *CLIConfig) resolveProfileName(explicitName string) string {
	if explicitName != "" {
		return explicitName
	}
	if c.Current != "" {
		return c.Current
	}
	return "default"
}

// getProfileNames returns a sorted list of profile names
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

// Validate validates the config (e.g., current profile exists)
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
// This applies the active profile values to env vars (profiles override existing env vars)
// CLI flags will later override these env vars (highest priority)
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

// setEnvIfValue sets an environment variable to the given value (if non-empty)
func setEnvIfValue(key, value string) {
	if value == "" {
		return
	}
	_ = os.Setenv(key, value)
}
