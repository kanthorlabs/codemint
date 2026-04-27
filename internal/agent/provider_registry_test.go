package agent

import (
	"testing"

	"codemint.kanthorlabs.com/internal/config"
)

func TestNewProviderRegistry_LoadsBuiltins(t *testing.T) {
	reg, err := NewProviderRegistry(nil)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	// Should have all builtin providers.
	for _, name := range []string{"opencode", "codex", "claude-code"} {
		if !reg.Has(name) {
			t.Errorf("expected builtin provider %q to be registered", name)
		}
	}
}

func TestProviderRegistry_Resolve_Builtin(t *testing.T) {
	reg, _ := NewProviderRegistry(nil)

	provider, err := reg.Resolve("opencode")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if provider.Name != "opencode" {
		t.Errorf("Name = %q; want %q", provider.Name, "opencode")
	}
	if provider.Command != "opencode" {
		t.Errorf("Command = %q; want %q", provider.Command, "opencode")
	}
	if len(provider.Args) != 1 || provider.Args[0] != "acp" {
		t.Errorf("Args = %v; want [acp]", provider.Args)
	}
}

func TestProviderRegistry_Resolve_Unknown(t *testing.T) {
	reg, _ := NewProviderRegistry(nil)

	_, err := reg.Resolve("unknown-provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !containsString(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestProviderRegistry_BuiltinPlusOverride(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "opencode",
				Args: []string{"--debug", "acp"},
				Env:  map[string]string{"DEBUG": "1"},
			},
		},
	}

	reg, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	provider, err := reg.Resolve("opencode")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Args should be overridden.
	if len(provider.Args) != 2 || provider.Args[0] != "--debug" {
		t.Errorf("Args = %v; want [--debug acp]", provider.Args)
	}

	// Env should be merged.
	if provider.Env["DEBUG"] != "1" {
		t.Errorf("Env[DEBUG] = %q; want %q", provider.Env["DEBUG"], "1")
	}

	// Command should still be the builtin default.
	if provider.Command != "opencode" {
		t.Errorf("Command = %q; want %q (from builtin)", provider.Command, "opencode")
	}
}

func TestProviderRegistry_DisabledProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "codex", Disabled: true},
		},
	}

	reg, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	// codex should not be resolvable.
	if reg.Has("codex") {
		t.Error("disabled provider should not be registered")
	}

	_, err = reg.Resolve("codex")
	if err == nil {
		t.Error("expected error resolving disabled provider")
	}

	// List should not include disabled provider.
	for _, p := range reg.List() {
		if p.Name == "codex" {
			t.Error("List() should not include disabled provider")
		}
	}
}

func TestProviderRegistry_CustomProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name:    "custom-ai",
				Command: "/usr/local/bin/custom-ai",
				Args:    []string{"serve", "--acp"},
				Env:     map[string]string{"API_KEY": "secret"},
			},
		},
	}

	reg, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	provider, err := reg.Resolve("custom-ai")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if provider.Name != "custom-ai" {
		t.Errorf("Name = %q; want %q", provider.Name, "custom-ai")
	}
	if provider.Command != "/usr/local/bin/custom-ai" {
		t.Errorf("Command = %q; want %q", provider.Command, "/usr/local/bin/custom-ai")
	}
	if len(provider.Args) != 2 || provider.Args[0] != "serve" {
		t.Errorf("Args = %v; want [serve --acp]", provider.Args)
	}
	if provider.Env["API_KEY"] != "secret" {
		t.Errorf("Env[API_KEY] = %q; want %q", provider.Env["API_KEY"], "secret")
	}
}

func TestProviderRegistry_MustExist_MissingBinary(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name:    "fake-provider",
				Command: "/definitely/not/installed/fake-binary-12345",
			},
		},
	}

	reg, err := NewProviderRegistry(cfg)
	if err != nil {
		t.Fatalf("NewProviderRegistry failed: %v", err)
	}

	err = reg.MustExist("fake-provider")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}

	binErr, ok := err.(*BinaryNotFoundError)
	if !ok {
		t.Fatalf("expected *BinaryNotFoundError, got %T: %v", err, err)
	}

	if binErr.ProviderName != "fake-provider" {
		t.Errorf("ProviderName = %q; want %q", binErr.ProviderName, "fake-provider")
	}
	if binErr.Command != "/definitely/not/installed/fake-binary-12345" {
		t.Errorf("Command = %q; want the fake path", binErr.Command)
	}
}

func TestProviderRegistry_MustExist_UnknownProvider(t *testing.T) {
	reg, _ := NewProviderRegistry(nil)

	err := reg.MustExist("unknown-provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !containsString(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestProviderRegistry_List(t *testing.T) {
	reg, _ := NewProviderRegistry(nil)

	providers := reg.List()

	if len(providers) < 3 {
		t.Errorf("expected at least 3 providers, got %d", len(providers))
	}

	// Check that list is sorted.
	for i := 1; i < len(providers); i++ {
		if providers[i-1].Name > providers[i].Name {
			t.Errorf("List() not sorted: %q > %q", providers[i-1].Name, providers[i].Name)
		}
	}
}

func TestProviderRegistry_Names(t *testing.T) {
	reg, _ := NewProviderRegistry(nil)

	names := reg.Names()

	if len(names) < 3 {
		t.Errorf("expected at least 3 names, got %d", len(names))
	}

	// Check that names are sorted.
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Names() not sorted: %q > %q", names[i-1], names[i])
		}
	}
}

func TestProviderRegistry_ResolveReturnsClone(t *testing.T) {
	reg, _ := NewProviderRegistry(nil)

	provider1, _ := reg.Resolve("opencode")
	provider2, _ := reg.Resolve("opencode")

	// Modify provider1.
	provider1.Command = "modified"
	provider1.Args = append(provider1.Args, "extra")

	// provider2 should be unaffected.
	if provider2.Command == "modified" {
		t.Error("Resolve should return independent clones - Command was shared")
	}
	if len(provider2.Args) != 1 {
		t.Error("Resolve should return independent clones - Args was shared")
	}
}

func TestProviderRegistry_CommandOverride(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name:    "claude-code",
				Command: "/opt/claude/bin/claude", // Override path
			},
		},
	}

	reg, _ := NewProviderRegistry(cfg)

	provider, err := reg.Resolve("claude-code")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if provider.Command != "/opt/claude/bin/claude" {
		t.Errorf("Command = %q; want %q", provider.Command, "/opt/claude/bin/claude")
	}

	// Other fields should retain builtin defaults.
	if provider.DisplayName != "Claude Code" {
		t.Errorf("DisplayName = %q; want %q (from builtin)", provider.DisplayName, "Claude Code")
	}
}

// containsString is a helper to check if s contains substr.
func containsString(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
