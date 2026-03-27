// Package config handles loading, saving, and managing the shop CLI's
// persistent configuration files: config.json, registry.json, and auth state.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultConfigDir is the default base directory for shop config.
	DefaultConfigDir = ".config/shop"

	configFile = "config.json"
)

// Config is the top-level configuration for the shop CLI.
type Config struct {
	Version   int                       `json:"version"`
	Defaults  Defaults                  `json:"defaults"`
	Providers map[string]ProviderConfig `json:"providers,omitempty"`
}

// Defaults holds user-configurable default values that apply across all
// commands. Matches the `defaults.*` namespace in `shop config set/get`.
type Defaults struct {
	Store   string         `json:"store,omitempty"`
	Timeout string         `json:"timeout,omitempty"`
	Output OutputDefaults `json:"output"`
}

// OutputDefaults controls JSON formatting behavior.
type OutputDefaults struct {
	JSON   bool `json:"json,omitempty"`
	Pretty bool `json:"pretty,omitempty"`
}

// ProviderConfig holds provider-level settings.
type ProviderConfig struct {
	Custom map[string]any `json:"custom,omitempty"`
}

// DefaultDir returns the default config directory path.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfigDir
	}

	return filepath.Join(home, DefaultConfigDir)
}

// Load reads the config from the given directory. If the file doesn't exist,
// returns a default config.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, configFile)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the config to the given directory, creating it if needed.
func Save(dir string, cfg *Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	path := filepath.Join(dir, configFile)

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// EnsureDefaults writes default config and registry files if they don't
// already exist on disk. Called on first run.
func EnsureDefaults(dir string, cfg *Config, reg *Registry) error {
	configPath := filepath.Join(dir, configFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := Save(dir, cfg); err != nil {
			return fmt.Errorf("write default config: %w", err)
		}
	}

	registryPath := filepath.Join(dir, registryFile)
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		if err := SaveRegistry(dir, reg); err != nil {
			return fmt.Errorf("write default registry: %w", err)
		}
	}

	return nil
}

func defaultConfig() *Config {
	return &Config{
		Version: 1,
	}
}
