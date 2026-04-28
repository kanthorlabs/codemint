// Package agent defines agent interfaces and implementations for CodeMint.
// This file provides helper functions for converting Provider to acp.WorkerConfig.
package agent

import (
	"os"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/config"
)

// WorkerConfigFromProvider creates an acp.WorkerConfig from a Provider.
// The cwd parameter specifies the working directory for the process.
// Deprecated: Use WorkerConfigFromProviderWithBinding for model support.
func WorkerConfigFromProvider(p *Provider, cwd string) acp.WorkerConfig {
	return WorkerConfigFromProviderWithBinding(p, config.AssistantConfig{}, cwd)
}

// WorkerConfigFromProviderWithBinding creates an acp.WorkerConfig from a Provider
// and an AssistantConfig. The binding's Model field is passed to WorkerConfig.Model
// for configuration via ACP session/set_config_option after session creation.
func WorkerConfigFromProviderWithBinding(p *Provider, binding config.AssistantConfig, cwd string) acp.WorkerConfig {
	if p == nil {
		return acp.DefaultConfig()
	}

	// Copy base args (model is no longer injected here - it's set via ACP protocol)
	args := append([]string{}, p.Args...)

	// Merge provider env with current environment
	var env []string
	if len(p.Env) > 0 {
		for k, v := range p.Env {
			env = append(env, k+"="+v)
		}
	}

	return acp.WorkerConfig{
		Command:          p.Command,
		Args:             args,
		Cwd:              cwd,
		Env:              env,
		HandshakeTimeout: acp.DefaultHandshakeTimeout,
		Model:            binding.Model, // Set via ACP session/set_config_option
	}
}

// ResolveSystemAssistantProvider resolves the provider for the system assistant.
// It checks for CODEMINT_ACP_CMD env override first (Story 3.1 compatibility),
// then falls back to the registry resolution.
func ResolveSystemAssistantProvider(registry *ProviderRegistry, providerName string) (*Provider, error) {
	// Check for env override (Story 3.1 legacy support)
	if cmd := os.Getenv(acp.EnvACPCommand); cmd != "" {
		return &Provider{
			Name:        "env-override",
			DisplayName: "Environment Override",
			Command:     cmd,
			Args:        nil, // When using env override, args are empty
			Capabilities: ProviderCaps{
				Streaming:    true,
				ToolCalls:    true,
				Planning:     true,
				ContextReset: true,
			},
			SystemPromptStrategy: PromptStrategyStdin,
		}, nil
	}

	// Use default provider name if not specified
	if providerName == "" {
		providerName = DefaultProviderName
	}

	return registry.Resolve(providerName)
}
