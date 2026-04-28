package agent

import (
	"os"
	"testing"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/config"
)

func TestWorkerConfigFromProvider(t *testing.T) {
	provider := &Provider{
		Name:    "test-provider",
		Command: "/usr/bin/test",
		Args:    []string{"arg1", "arg2"},
		Env:     map[string]string{"KEY": "value"},
	}

	cfg := WorkerConfigFromProvider(provider, "/work/dir")

	if cfg.Command != "/usr/bin/test" {
		t.Errorf("Command = %q; want %q", cfg.Command, "/usr/bin/test")
	}
	if len(cfg.Args) != 2 || cfg.Args[0] != "arg1" || cfg.Args[1] != "arg2" {
		t.Errorf("Args = %v; want [arg1 arg2]", cfg.Args)
	}
	if cfg.Cwd != "/work/dir" {
		t.Errorf("Cwd = %q; want %q", cfg.Cwd, "/work/dir")
	}
	if len(cfg.Env) != 1 || cfg.Env[0] != "KEY=value" {
		t.Errorf("Env = %v; want [KEY=value]", cfg.Env)
	}
	if cfg.HandshakeTimeout != acp.DefaultHandshakeTimeout {
		t.Errorf("HandshakeTimeout = %v; want %v", cfg.HandshakeTimeout, acp.DefaultHandshakeTimeout)
	}
}

func TestWorkerConfigFromProvider_NilProvider(t *testing.T) {
	cfg := WorkerConfigFromProvider(nil, "/work/dir")

	defaultCfg := acp.DefaultConfig()
	if cfg.Command != defaultCfg.Command {
		t.Errorf("Command = %q; want %q (default)", cfg.Command, defaultCfg.Command)
	}
}

func TestWorkerConfigFromProvider_EmptyEnv(t *testing.T) {
	provider := &Provider{
		Name:    "test",
		Command: "test-cmd",
		Args:    []string{"acp"},
		Env:     nil, // Empty env
	}

	cfg := WorkerConfigFromProvider(provider, "/dir")

	if len(cfg.Env) > 0 {
		t.Errorf("Env should be empty, got %v", cfg.Env)
	}
}

func TestWorkerConfigFromProvider_ArgsIndependent(t *testing.T) {
	provider := &Provider{
		Name:    "test",
		Command: "test-cmd",
		Args:    []string{"original"},
	}

	cfg := WorkerConfigFromProvider(provider, "/dir")
	cfg.Args[0] = "modified"

	if provider.Args[0] == "modified" {
		t.Error("modifying cfg.Args should not affect provider.Args")
	}
}

func TestResolveSystemAssistantProvider_EnvOverride(t *testing.T) {
	// Save and restore env
	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)

	// Set env override
	os.Setenv(acp.EnvACPCommand, "/custom/command")

	reg, _ := NewProviderRegistry(nil)

	provider, err := ResolveSystemAssistantProvider(reg, "opencode")
	if err != nil {
		t.Fatalf("ResolveSystemAssistantProvider failed: %v", err)
	}

	if provider.Name != "env-override" {
		t.Errorf("Name = %q; want %q", provider.Name, "env-override")
	}
	if provider.Command != "/custom/command" {
		t.Errorf("Command = %q; want %q", provider.Command, "/custom/command")
	}
	if len(provider.Args) != 0 {
		t.Errorf("Args should be empty when using env override, got %v", provider.Args)
	}
}

func TestResolveSystemAssistantProvider_RegistryFallback(t *testing.T) {
	// Clear env override
	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)
	os.Unsetenv(acp.EnvACPCommand)

	reg, _ := NewProviderRegistry(nil)

	provider, err := ResolveSystemAssistantProvider(reg, "codex")
	if err != nil {
		t.Fatalf("ResolveSystemAssistantProvider failed: %v", err)
	}

	if provider.Name != "codex" {
		t.Errorf("Name = %q; want %q", provider.Name, "codex")
	}
}

func TestResolveSystemAssistantProvider_DefaultProvider(t *testing.T) {
	// Clear env override
	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)
	os.Unsetenv(acp.EnvACPCommand)

	reg, _ := NewProviderRegistry(nil)

	// Empty provider name should default to opencode
	provider, err := ResolveSystemAssistantProvider(reg, "")
	if err != nil {
		t.Fatalf("ResolveSystemAssistantProvider failed: %v", err)
	}

	if provider.Name != "opencode" {
		t.Errorf("Name = %q; want %q (default)", provider.Name, "opencode")
	}
}

func TestResolveSystemAssistantProvider_UnknownProvider(t *testing.T) {
	// Clear env override
	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)
	os.Unsetenv(acp.EnvACPCommand)

	reg, _ := NewProviderRegistry(nil)

	_, err := ResolveSystemAssistantProvider(reg, "unknown-provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestWorkerConfig_SetsModelField(t *testing.T) {
	provider := &Provider{
		Name:    "test",
		Command: "test-cmd",
		Args:    []string{"acp"},
	}
	binding := config.AssistantConfig{
		Provider: "test",
		Model:    "gpt-5",
	}

	cfg := WorkerConfigFromProviderWithBinding(provider, binding, "/dir")

	// Model should be set on WorkerConfig.Model (not as CLI args)
	if cfg.Model != "gpt-5" {
		t.Errorf("Model = %q; want %q", cfg.Model, "gpt-5")
	}

	// Args should NOT contain model flag (model is set via ACP protocol)
	if len(cfg.Args) != 1 || cfg.Args[0] != "acp" {
		t.Errorf("Args = %v; want [acp] (model should not be in args)", cfg.Args)
	}
}

func TestWorkerConfig_NoModel_EmptyModelField(t *testing.T) {
	provider := &Provider{
		Name:    "test",
		Command: "test-cmd",
		Args:    []string{"acp"},
	}
	binding := config.AssistantConfig{
		Provider: "test",
		// Model intentionally empty
	}

	cfg := WorkerConfigFromProviderWithBinding(provider, binding, "/dir")

	// Model field should be empty
	if cfg.Model != "" {
		t.Errorf("Model = %q; want empty", cfg.Model)
	}

	// Args should be unchanged
	if len(cfg.Args) != 1 || cfg.Args[0] != "acp" {
		t.Errorf("Args = %v; want [acp]", cfg.Args)
	}
}

func TestWorkerConfig_ArgsIndependentWithBinding(t *testing.T) {
	provider := &Provider{
		Name:    "test",
		Command: "test-cmd",
		Args:    []string{"original"},
	}
	binding := config.AssistantConfig{
		Model: "test-model",
	}

	cfg := WorkerConfigFromProviderWithBinding(provider, binding, "/dir")
	cfg.Args[0] = "modified"

	if provider.Args[0] == "modified" {
		t.Error("modifying cfg.Args should not affect provider.Args")
	}
}

// TestAssistant_E2E_ModelInWorkerConfig is an integration test that proves the full
// path from config.yaml to WorkerConfig.Model includes the model value.
func TestAssistant_E2E_ModelInWorkerConfig(t *testing.T) {
	// Simulate config with assistants.sys-default.model specified.
	cfg := &config.Config{
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {
				Provider: "opencode",
				Model:    "github-copilot/claude-sonnet-4.6",
			},
		},
	}

	// Create registry from config.
	registry, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	// Resolve the provider for system assistant.
	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)
	os.Unsetenv(acp.EnvACPCommand) // Ensure env override doesn't interfere

	sysDefault := cfg.GetSysDefault()
	provider, err := ResolveSystemAssistantProvider(registry, sysDefault.Provider)
	if err != nil {
		t.Fatalf("ResolveSystemAssistantProvider failed: %v", err)
	}

	// Create worker config with binding.
	workerCfg := WorkerConfigFromProviderWithBinding(provider, sysDefault, "/workspace")

	// Verify Model is set on WorkerConfig (not in args).
	if workerCfg.Model != "github-copilot/claude-sonnet-4.6" {
		t.Errorf("Model = %q; want %q", workerCfg.Model, "github-copilot/claude-sonnet-4.6")
	}

	// Args should NOT contain model flag.
	expectedArgs := []string{"acp"}
	if len(workerCfg.Args) != len(expectedArgs) {
		t.Fatalf("Args = %v; want %v (model should not be in args)", workerCfg.Args, expectedArgs)
	}
}

// TestAssistant_E2E_NoModel_EmptyModelField verifies that omitting model leaves Model field empty.
func TestAssistant_E2E_NoModel_EmptyModelField(t *testing.T) {
	cfg := &config.Config{
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {
				Provider: "opencode",
				// Model intentionally empty - use provider default
			},
		},
	}

	registry, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)
	os.Unsetenv(acp.EnvACPCommand)

	sysDefault := cfg.GetSysDefault()
	provider, err := ResolveSystemAssistantProvider(registry, sysDefault.Provider)
	if err != nil {
		t.Fatalf("ResolveSystemAssistantProvider failed: %v", err)
	}

	workerCfg := WorkerConfigFromProviderWithBinding(provider, sysDefault, "/workspace")

	// Model should be empty.
	if workerCfg.Model != "" {
		t.Errorf("Model = %q; want empty", workerCfg.Model)
	}

	// Args should be just ["acp"].
	if len(workerCfg.Args) != 1 || workerCfg.Args[0] != "acp" {
		t.Errorf("Args = %v; want [acp]", workerCfg.Args)
	}
}

// TestAssistant_E2E_CustomProviderWithModel verifies model works with custom providers.
func TestAssistant_E2E_CustomProviderWithModel(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name:    "custom-ai",
				Command: "/usr/bin/custom-ai",
				Args:    []string{"serve"},
			},
		},
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {
				Provider: "custom-ai",
				Model:    "custom-model-v1",
			},
		},
	}

	registry, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	oldVal := os.Getenv(acp.EnvACPCommand)
	defer os.Setenv(acp.EnvACPCommand, oldVal)
	os.Unsetenv(acp.EnvACPCommand)

	sysDefault := cfg.GetSysDefault()
	provider, err := ResolveSystemAssistantProvider(registry, sysDefault.Provider)
	if err != nil {
		t.Fatalf("ResolveSystemAssistantProvider failed: %v", err)
	}

	workerCfg := WorkerConfigFromProviderWithBinding(provider, sysDefault, "/workspace")

	// Model should be set on WorkerConfig.
	if workerCfg.Model != "custom-model-v1" {
		t.Errorf("Model = %q; want %q", workerCfg.Model, "custom-model-v1")
	}

	// Args should NOT contain model flag (only base args).
	expectedArgs := []string{"serve"}
	if len(workerCfg.Args) != len(expectedArgs) {
		t.Fatalf("Args = %v; want %v (model should not be in args)", workerCfg.Args, expectedArgs)
	}
}
