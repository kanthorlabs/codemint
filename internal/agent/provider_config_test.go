package agent

import (
	"os"
	"testing"

	"codemint.kanthorlabs.com/internal/acp"
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

	if cfg.Env != nil && len(cfg.Env) > 0 {
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
