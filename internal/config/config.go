// Package config provides configuration loading and validation for CodeMint.
// Configuration is loaded from YAML files following the XDG Base Directory
// Specification.
package config

// Config is the root configuration structure for CodeMint.
type Config struct {
	Workflows  []WorkflowConfig `yaml:"workflows" validate:"dive"`
	Providers  []ProviderConfig `yaml:"providers,omitempty" validate:"dive"`
	Assistants AssistantsConfig `yaml:"assistants,omitempty"`
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

// AssistantsConfig holds configuration for different assistant bindings.
type AssistantsConfig struct {
	System       AssistantBindingConfig `yaml:"system,omitempty"`
	Brainstormer AssistantBindingConfig `yaml:"brainstormer,omitempty"` // EPIC-02
	Clarifier    AssistantBindingConfig `yaml:"clarifier,omitempty"`    // EPIC-02 §2.12
	Archivist    AssistantBindingConfig `yaml:"archivist,omitempty"`    // EPIC-05
}

// AssistantBindingConfig configures which provider backs an assistant.
type AssistantBindingConfig struct {
	// Provider is the name of the provider to use (e.g., "opencode", "codex", "claude-code").
	// Must resolve via builtin catalog or an entry in Config.Providers.
	// Defaults to "opencode" if not specified.
	Provider string `yaml:"provider,omitempty"`
	// Model optionally overrides the model to use for this assistant.
	Model string `yaml:"model,omitempty"`
}
