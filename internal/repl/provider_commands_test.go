package repl

import (
	"context"
	"strings"
	"testing"

	"codemint.kanthorlabs.com/internal/agent"
	"codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/registry"
)

func TestProvidersHandler_ListProviders(t *testing.T) {
	// Create a registry with default providers
	providerRegistry, err := agent.NewProviderRegistry(nil)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	deps := &ProviderCommandDeps{
		ProviderRegistry:    providerRegistry,
		DefaultProviderName: "opencode",
	}

	handler := providersHandler(deps)
	result, err := handler(context.Background(), mockProviderActiveSession{}, nil, "")
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// Should list providers
	if !strings.Contains(result.Message, "Available Providers") {
		t.Error("expected 'Available Providers' header")
	}

	// Should show opencode as default
	if !strings.Contains(result.Message, "OpenCode") {
		t.Error("expected OpenCode provider in list")
	}
	if !strings.Contains(result.Message, "(default)") {
		t.Error("expected (default) marker for opencode")
	}

	// Should show codex and claude-code
	if !strings.Contains(result.Message, "Codex") {
		t.Error("expected Codex provider in list")
	}
	if !strings.Contains(result.Message, "Claude Code") {
		t.Error("expected Claude Code provider in list")
	}
}

func TestProvidersHandler_NilRegistry(t *testing.T) {
	deps := &ProviderCommandDeps{
		ProviderRegistry: nil,
	}

	handler := providersHandler(deps)
	result, err := handler(context.Background(), mockProviderActiveSession{}, nil, "")
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !strings.Contains(result.Message, "not available") {
		t.Error("expected 'not available' message for nil registry")
	}
}

func TestProvidersHandler_TestUnknownProvider(t *testing.T) {
	providerRegistry, _ := agent.NewProviderRegistry(nil)

	deps := &ProviderCommandDeps{
		ProviderRegistry: providerRegistry,
	}

	handler := providersHandler(deps)
	result, err := handler(context.Background(), mockProviderActiveSession{}, []string{"test", "unknown-provider"}, "")
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !strings.Contains(result.Message, "not found") {
		t.Errorf("expected 'not found' message, got: %s", result.Message)
	}
}

func TestProvidersHandler_TestProviderNotOnPath(t *testing.T) {
	// Create registry with a fake provider
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name:    "fake-provider",
				Command: "/definitely/not/installed/fake-binary-12345",
			},
		},
	}
	providerRegistry, _ := agent.NewProviderRegistry(cfg)

	deps := &ProviderCommandDeps{
		ProviderRegistry: providerRegistry,
	}

	handler := providersHandler(deps)
	result, err := handler(context.Background(), mockProviderActiveSession{}, []string{"test", "fake-provider"}, "")
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !strings.Contains(result.Message, "not found on PATH") {
		t.Errorf("expected 'not found on PATH' message, got: %s", result.Message)
	}
}

func TestRegisterProviderCommands(t *testing.T) {
	cmdRegistry := registry.NewCommandRegistry()
	providerRegistry, _ := agent.NewProviderRegistry(nil)

	deps := &ProviderCommandDeps{
		ProviderRegistry:    providerRegistry,
		DefaultProviderName: "opencode",
	}

	err := RegisterProviderCommands(cmdRegistry, deps)
	if err != nil {
		t.Fatalf("RegisterProviderCommands failed: %v", err)
	}

	// Check that /providers command is registered by looking for it in help text
	help := cmdRegistry.HelpTextForMode(registry.ClientModeCLI)
	if !strings.Contains(help, "/providers") {
		t.Error("expected /providers command to be registered")
	}
}

func TestCheckProviderStatus(t *testing.T) {
	// Test with a command that exists (like /bin/echo or similar)
	provider := &agent.Provider{
		Name:    "test",
		Command: "echo", // echo should exist on most systems
	}

	status := checkProviderStatus(provider)
	if !strings.Contains(status, "✓ found") {
		t.Errorf("expected '✓ found' status for echo, got: %s", status)
	}

	// Test with a command that doesn't exist
	provider2 := &agent.Provider{
		Name:    "test",
		Command: "/definitely/not/a/real/binary",
	}

	status2 := checkProviderStatus(provider2)
	if !strings.Contains(status2, "✗ not found") {
		t.Errorf("expected '✗ not found' status, got: %s", status2)
	}
}

// mockProviderActiveSession implements registry.ActiveSessionInfo for testing.
type mockProviderActiveSession struct{}

func (m mockProviderActiveSession) GetClientMode() registry.ClientMode { return registry.ClientModeCLI }
func (m mockProviderActiveSession) GetIsGlobal() bool                  { return false }
