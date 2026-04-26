// Package config provides configuration loading and validation for CodeMint.
// Configuration is loaded from YAML files following the XDG Base Directory
// Specification.
package config

// Config is the root configuration structure for CodeMint.
type Config struct {
	Workflows  []WorkflowConfig   `yaml:"workflows" validate:"dive"`
	Agents     []AgentConfig      `yaml:"agents,omitempty" validate:"dive"`
	Assistants AssistantsConfig   `yaml:"assistants,omitempty"`
}

// WorkflowConfig defines a workflow entry in the configuration file.
type WorkflowConfig struct {
	Type        int      `yaml:"type" validate:"min=0,max=2"`
	Name        string   `yaml:"name" validate:"required"`
	Description string   `yaml:"description"`
	Triggers    []string `yaml:"triggers,omitempty"`
}

// AgentConfig defines an agent entry in the configuration file.
// Reserved for future use (EPIC-02+).
type AgentConfig struct {
	ID        string `yaml:"id,omitempty"`
	Name      string `yaml:"name" validate:"required"`
	Type      int    `yaml:"type" validate:"min=0,max=2"`
	Assistant string `yaml:"assistant,omitempty"`
}

// AssistantsConfig holds configuration for different assistant bindings.
type AssistantsConfig struct {
	System AssistantBindingConfig `yaml:"system,omitempty"`
}

// AssistantBindingConfig configures which provider backs an assistant.
type AssistantBindingConfig struct {
	// Provider is the name of the provider to use (e.g., "opencode", "codex", "claude-code").
	// Defaults to "opencode" if not specified.
	Provider string `yaml:"provider,omitempty"`
}
