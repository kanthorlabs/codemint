package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"codemint.kanthorlabs.com/internal/xdg"
)

// knownAssistantRoles lists valid assistant role names for strict validation.
var knownAssistantRoles = []string{"system", "brainstormer", "clarifier", "archivist"}

// Load reads and parses a YAML configuration file from the given path.
// Returns a descriptive error with line number information on parse failure.
// Logs a deprecation warning if the legacy "agents:" key is present.
// Returns an error if the assistants block contains unknown role names.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File not found is not an error - return empty config.
			return &Config{}, nil
		}
		return nil, fmt.Errorf("config: read file %q: %w", path, err)
	}

	// First pass: peek at top-level keys for validation.
	var rawMap map[string]any
	if err := yaml.Unmarshal(data, &rawMap); err == nil {
		// Check for legacy "agents:" key.
		if _, hasAgents := rawMap["agents"]; hasAgents {
			slog.Warn("config: 'agents:' key is deprecated; use 'assistants:' instead",
				"path", path)
		}

		// Validate assistants block for unknown role names.
		if assistants, ok := rawMap["assistants"].(map[string]any); ok {
			if err := validateAssistantRoles(assistants); err != nil {
				return nil, fmt.Errorf("config: %s in %q", err.Error(), path)
			}
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// yaml.v3 provides line/column info in error messages.
		return nil, fmt.Errorf("config: parse yaml %q: %w", path, err)
	}

	return &cfg, nil
}

// validateAssistantRoles checks that all keys in the assistants map are known roles.
func validateAssistantRoles(assistants map[string]any) error {
	known := make(map[string]bool)
	for _, role := range knownAssistantRoles {
		known[role] = true
	}

	for key := range assistants {
		if !known[key] {
			return fmt.Errorf("unknown assistant role %q (known: %s)",
				key, strings.Join(knownAssistantRoles, ", "))
		}
	}
	return nil
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
