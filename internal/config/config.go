// Package config provides configuration loading and validation for CodeMint.
// Configuration is loaded from YAML files following the XDG Base Directory
// Specification.
package config

// SysDefaultAssistant is the canonical name for the default system assistant.
// This assistant must always be configured and seeded in the database.
const SysDefaultAssistant = "sys-default"

// SysCodingAssistant is the canonical name for the coding assistant.
// This assistant is used for executing coding tasks via the Executor.
// If not configured, falls back to sys-default.
const SysCodingAssistant = "sys-coding"

// Config is the root configuration structure for CodeMint.
type Config struct {
	Workflows  []WorkflowConfig           `yaml:"workflows" validate:"dive"`
	Providers  []ProviderConfig           `yaml:"providers,omitempty" validate:"dive"`
	Assistants map[string]AssistantConfig `yaml:"assistants,omitempty"`
}

// WorkflowConfig defines a workflow entry in the configuration file.
type WorkflowConfig struct {
	Type        int      `yaml:"type" validate:"min=0,max=2"`
	Name        string   `yaml:"name" validate:"required"`
	Description string   `yaml:"description"`
	Triggers    []string `yaml:"triggers,omitempty"`
}

// ProviderConfig defines a provider entry in the configuration file.
// Provider names matching the built-in catalog inherit defaults.
type ProviderConfig struct {
	// Name is the unique provider identifier (required).
	// Use builtin names (opencode, codex, claude-code) to override their defaults.
	Name string `yaml:"name" validate:"required"`
	// Command overrides the binary path or executable name.
	Command string `yaml:"command,omitempty"`
	// Args overrides the default arguments.
	Args []string `yaml:"args,omitempty"`
	// Env provides additional environment variables for the provider process.
	Env map[string]string `yaml:"env,omitempty"`
	// Disabled excludes this provider from resolution.
	Disabled bool `yaml:"disabled,omitempty"`
	// ModelFlag overrides the CLI flag for model selection (e.g., "--model", "-m").
	// Empty string disables model injection for this provider.
	ModelFlag string `yaml:"model_flag,omitempty"`
}

// AssistantConfig configures which provider backs an assistant.
type AssistantConfig struct {
	// Provider is the name of the provider to use (e.g., "opencode", "codex", "claude-code").
	// Must resolve via builtin catalog or an entry in Config.Providers.
	// Required for sys-default, defaults to "opencode" for others if not specified.
	Provider string `yaml:"provider,omitempty"`
	// Model optionally overrides the model to use for this assistant.
	Model string `yaml:"model,omitempty"`
}

// GetAssistant returns the configuration for the named assistant.
// Returns an empty AssistantConfig if the assistant is not configured.
func (c *Config) GetAssistant(name string) AssistantConfig {
	if c.Assistants == nil {
		return AssistantConfig{}
	}
	return c.Assistants[name]
}

// GetSysDefault returns the sys-default assistant configuration.
// This is the primary assistant used for workflow task execution.
func (c *Config) GetSysDefault() AssistantConfig {
	return c.GetAssistant(SysDefaultAssistant)
}

// GetSysCoding returns the sys-coding assistant configuration.
// This assistant is used for executing coding tasks.
// If not configured, falls back to sys-default configuration.
func (c *Config) GetSysCoding() AssistantConfig {
	cfg := c.GetAssistant(SysCodingAssistant)
	if cfg.Provider == "" {
		// Fallback to sys-default if sys-coding is not configured.
		return c.GetSysDefault()
	}
	return cfg
}
