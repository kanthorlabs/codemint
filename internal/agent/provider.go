// Package agent defines agent interfaces and implementations for CodeMint.
// This file defines the Provider domain type for ACP-compatible CLI agents.
package agent

// PromptStrategy defines how system prompts are injected into a provider.
type PromptStrategy int

const (
	// PromptStrategyStdin injects the system prompt via stdin as the first message.
	// This is the default for OpenCode and Claude Code.
	PromptStrategyStdin PromptStrategy = iota
	// PromptStrategyFlag injects the system prompt via a --system-prompt-file flag.
	// Used by Codex CLI.
	PromptStrategyFlag
	// PromptStrategyEnv injects the system prompt via an environment variable.
	PromptStrategyEnv
)

// String returns the string representation of a PromptStrategy.
func (s PromptStrategy) String() string {
	switch s {
	case PromptStrategyStdin:
		return "stdin"
	case PromptStrategyFlag:
		return "flag"
	case PromptStrategyEnv:
		return "env"
	default:
		return "unknown"
	}
}

// ProviderCaps defines the capabilities of an ACP provider.
type ProviderCaps struct {
	// Streaming indicates whether the provider supports streaming responses.
	Streaming bool
	// ToolCalls indicates whether the provider supports tool calls.
	ToolCalls bool
	// Planning indicates whether the provider supports planning mode.
	Planning bool
	// ContextReset indicates whether the provider supports session/new for context reset.
	ContextReset bool
}

// Provider represents an ACP-compatible CLI agent configuration.
// This is the full provider type for Story 3.22; it replaces the simplified
// Provider struct from Story 3.19.
type Provider struct {
	// Name is the unique provider identifier (e.g., "opencode", "codex", "claude-code").
	Name string
	// DisplayName is a human-readable name for display purposes.
	DisplayName string
	// Command is the binary on PATH, or an absolute path to the executable.
	Command string
	// Args are the default ACP-mode arguments (e.g., ["acp"]).
	Args []string
	// Env holds additional environment variables for the provider process.
	Env map[string]string
	// Capabilities defines what features the provider supports.
	Capabilities ProviderCaps
	// SystemPromptStrategy defines how to inject memory/system prompts.
	SystemPromptStrategy PromptStrategy
	// VersionArgs are arguments to get version info (for /providers test command).
	VersionArgs []string
}

// Clone creates a deep copy of the Provider.
func (p *Provider) Clone() *Provider {
	if p == nil {
		return nil
	}
	clone := &Provider{
		Name:                 p.Name,
		DisplayName:          p.DisplayName,
		Command:              p.Command,
		Args:                 make([]string, len(p.Args)),
		Env:                  make(map[string]string, len(p.Env)),
		Capabilities:         p.Capabilities,
		SystemPromptStrategy: p.SystemPromptStrategy,
		VersionArgs:          make([]string, len(p.VersionArgs)),
	}
	copy(clone.Args, p.Args)
	copy(clone.VersionArgs, p.VersionArgs)
	for k, v := range p.Env {
		clone.Env[k] = v
	}
	return clone
}

// Merge applies non-zero fields from override onto the Provider.
// Used when config overrides builtin catalog defaults.
func (p *Provider) Merge(override *Provider) {
	if override == nil {
		return
	}
	if override.DisplayName != "" {
		p.DisplayName = override.DisplayName
	}
	if override.Command != "" {
		p.Command = override.Command
	}
	if len(override.Args) > 0 {
		p.Args = make([]string, len(override.Args))
		copy(p.Args, override.Args)
	}
	if len(override.Env) > 0 {
		if p.Env == nil {
			p.Env = make(map[string]string)
		}
		for k, v := range override.Env {
			p.Env[k] = v
		}
	}
	if len(override.VersionArgs) > 0 {
		p.VersionArgs = make([]string, len(override.VersionArgs))
		copy(p.VersionArgs, override.VersionArgs)
	}
}
