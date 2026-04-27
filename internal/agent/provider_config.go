// Package agent defines agent interfaces and implementations for CodeMint.
// This file provides helper functions for converting Provider to acp.WorkerConfig.
package agent

import (
	"log/slog"
	"os"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/config"
)

// WorkerConfigFromProvider creates an acp.WorkerConfig from a Provider.
// The cwd parameter specifies the working directory for the process.
// Deprecated: Use WorkerConfigFromProviderWithBinding for model support.
func WorkerConfigFromProvider(p *Provider, cwd string) acp.WorkerConfig {
	return WorkerConfigFromProviderWithBinding(p, config.AssistantBindingConfig{}, cwd)
}

// WorkerConfigFromProviderWithBinding creates an acp.WorkerConfig from a Provider
// and an AssistantBindingConfig. The binding's Model field is injected into spawn args
// if the Provider supports a model flag.
func WorkerConfigFromProviderWithBinding(p *Provider, binding config.AssistantBindingConfig, cwd string) acp.WorkerConfig {
	if p == nil {
		return acp.DefaultConfig()
	}

	// Copy base args
	args := append([]string{}, p.Args...)

	// Inject model flag if binding has a model specified
	if binding.Model != "" {
		if p.ModelFlag != "" {
			args = append(args, p.ModelFlag, binding.Model)
		} else {
			slog.Debug("provider does not support CLI model selector; ignoring",
				"provider", p.Name, "model", binding.Model)
		}
	}

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
