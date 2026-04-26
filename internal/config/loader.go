package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"codemint.kanthorlabs.com/internal/xdg"
)

// Load reads and parses a YAML configuration file from the given path.
// Returns a descriptive error with line number information on parse failure.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File not found is not an error - return empty config.
			return &Config{}, nil
		}
		return nil, fmt.Errorf("config: read file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// yaml.v3 provides line/column info in error messages.
		return nil, fmt.Errorf("config: parse yaml %q: %w", path, err)
	}

	return &cfg, nil
}

// LoadDefault loads configuration from the XDG config directory.
// The default path is $XDG_CONFIG_HOME/codemint/config.yaml.
// Returns an empty Config (not error) if the file does not exist.
func LoadDefault() (*Config, error) {
	path := filepath.Join(xdg.ConfigDir(), "config.yaml")
	return Load(path)
}

// DefaultPath returns the default configuration file path.
func DefaultPath() string {
	return filepath.Join(xdg.ConfigDir(), "config.yaml")
}
