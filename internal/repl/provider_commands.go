// Package repl provides slash command handlers for the CodeMint REPL.
// This file implements the /providers command for listing and testing providers.
package repl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"codemint.kanthorlabs.com/internal/agent"
	"codemint.kanthorlabs.com/internal/registry"
)

// ProviderCommandDeps holds the dependencies needed for provider-related commands.
type ProviderCommandDeps struct {
	ProviderRegistry *agent.ProviderRegistry
	// DefaultProviderName is the name of the default provider (from config).
	DefaultProviderName string
}

// RegisterProviderCommands registers provider-specific commands (/providers).
func RegisterProviderCommands(r *registry.CommandRegistry, deps *ProviderCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "providers",
			Description:    "List available ACP providers and their status.",
			Usage:          "/providers [test <name>]",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon, registry.ClientModeHybrid},
			Handler:        providersHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register provider command %q: %w", c.Name, err)
		}
	}
	return nil
}

// providersHandler handles the /providers command.
// Without arguments, it lists all available providers with their status.
// With "test <name>", it runs the provider's version command to verify installation.
func providersHandler(deps *ProviderCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		if deps.ProviderRegistry == nil {
			return registry.CommandResult{
				Message: "Provider registry not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Check for subcommand
		if len(args) >= 2 && args[0] == "test" {
			return testProvider(ctx, deps, args[1])
		}

		// List all providers
		return listProviders(deps)
	}
}

// listProviders returns a formatted list of all available providers with their status.
func listProviders(deps *ProviderCommandDeps) (registry.CommandResult, error) {
	providers := deps.ProviderRegistry.List()

	if len(providers) == 0 {
		return registry.CommandResult{
			Message: "No providers configured.",
			Action:  registry.ActionNone,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("Available Providers:\n")
	sb.WriteString("──────────────────────────────────────────\n")

	for _, p := range providers {
		// Check if binary exists
		status := checkProviderStatus(p)

		// Mark default provider
		defaultMark := ""
		if p.Name == deps.DefaultProviderName {
			defaultMark = " (default)"
		}

		sb.WriteString(fmt.Sprintf("  %s%s\n", p.DisplayName, defaultMark))
		sb.WriteString(fmt.Sprintf("    Name:    %s\n", p.Name))
		sb.WriteString(fmt.Sprintf("    Command: %s %s\n", p.Command, strings.Join(p.Args, " ")))
		sb.WriteString(fmt.Sprintf("    Status:  %s\n", status))
		sb.WriteString("\n")
	}

	return registry.CommandResult{
		Message: sb.String(),
		Action:  registry.ActionNone,
	}, nil
}

// checkProviderStatus checks if a provider's binary is available on PATH.
func checkProviderStatus(p *agent.Provider) string {
	path, err := exec.LookPath(p.Command)
	if err != nil {
		return "✗ not found on PATH"
	}
	return fmt.Sprintf("✓ found at %s", path)
}

// testProvider runs the provider's version command to verify it's working.
func testProvider(ctx context.Context, deps *ProviderCommandDeps, name string) (registry.CommandResult, error) {
	provider, err := deps.ProviderRegistry.Resolve(name)
	if err != nil {
		return registry.CommandResult{
			Message: fmt.Sprintf("Provider %q not found. Use /providers to see available providers.", name),
			Action:  registry.ActionNone,
		}, nil
	}

	// Check if binary exists first
	cmdPath, err := exec.LookPath(provider.Command)
	if err != nil {
		return registry.CommandResult{
			Message: fmt.Sprintf("Provider %q: binary %q not found on PATH.", name, provider.Command),
			Action:  registry.ActionNone,
		}, nil
	}

	// Run version command
	versionArgs := provider.VersionArgs
	if len(versionArgs) == 0 {
		versionArgs = []string{"--version"}
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdPath, versionArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return registry.CommandResult{
			Message: fmt.Sprintf("Provider %q test failed:\n  Command: %s %s\n  Error: %v\n  Output: %s",
				name, provider.Command, strings.Join(versionArgs, " "), err, strings.TrimSpace(string(output))),
			Action: registry.ActionNone,
		}, nil
	}

	return registry.CommandResult{
		Message: fmt.Sprintf("Provider %q test successful:\n  Command: %s %s\n  Output:\n%s",
			name, provider.Command, strings.Join(versionArgs, " "), indent(strings.TrimSpace(string(output)), "    ")),
		Action: registry.ActionNone,
	}, nil
}

// indent adds a prefix to each line of the input string.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
