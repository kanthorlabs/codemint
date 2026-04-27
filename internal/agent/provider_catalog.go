// Package agent defines agent interfaces and implementations for CodeMint.
// This file provides a built-in catalog of known ACP providers.
package agent

// builtinProviders is the catalog of known ACP-compatible providers.
// These serve as defaults that can be overridden by config.
var builtinProviders = map[string]*Provider{
	"opencode": {
		Name:        "opencode",
		DisplayName: "OpenCode",
		Command:     "opencode",
		Args:        []string{"acp"},
		Capabilities: ProviderCaps{
			Streaming:    true,
			ToolCalls:    true,
			Planning:     true,
			ContextReset: true,
		},
		SystemPromptStrategy: PromptStrategyStdin,
		VersionArgs:          []string{"--version"},
		ModelFlag:            "--model",
	},
}

// DefaultProviderName is the name of the default provider.
const DefaultProviderName = "opencode"

// LookupBuiltinProvider returns a clone of the builtin provider by name.
// Returns nil, false if the provider is not found in the catalog.
func LookupBuiltinProvider(name string) (*Provider, bool) {
	p, ok := builtinProviders[name]
	if !ok {
		return nil, false
	}
	return p.Clone(), true
}

// BuiltinProviderNames returns the names of all builtin providers.
func BuiltinProviderNames() []string {
	names := make([]string, 0, len(builtinProviders))
	for name := range builtinProviders {
		names = append(names, name)
	}
	return names
}
