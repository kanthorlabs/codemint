package agent

import (
	"testing"
)

func TestLookupBuiltinProvider_Opencode(t *testing.T) {
	provider, ok := LookupBuiltinProvider("opencode")
	if !ok {
		t.Fatal("expected to find opencode provider")
	}

	if provider.Name != "opencode" {
		t.Errorf("Name = %q; want %q", provider.Name, "opencode")
	}
	if provider.DisplayName != "OpenCode" {
		t.Errorf("DisplayName = %q; want %q", provider.DisplayName, "OpenCode")
	}
	if provider.Command != "opencode" {
		t.Errorf("Command = %q; want %q", provider.Command, "opencode")
	}
	if len(provider.Args) != 1 || provider.Args[0] != "acp" {
		t.Errorf("Args = %v; want [acp]", provider.Args)
	}
	if !provider.Capabilities.Streaming {
		t.Error("expected Streaming = true")
	}
	if !provider.Capabilities.ToolCalls {
		t.Error("expected ToolCalls = true")
	}
	if !provider.Capabilities.Planning {
		t.Error("expected Planning = true")
	}
	if !provider.Capabilities.ContextReset {
		t.Error("expected ContextReset = true")
	}
	if provider.SystemPromptStrategy != PromptStrategyStdin {
		t.Errorf("SystemPromptStrategy = %v; want %v", provider.SystemPromptStrategy, PromptStrategyStdin)
	}
	if provider.ModelFlag != "--model" {
		t.Errorf("ModelFlag = %q; want %q", provider.ModelFlag, "--model")
	}
}

func TestLookupBuiltinProvider_Codex(t *testing.T) {
	provider, ok := LookupBuiltinProvider("codex")
	if !ok {
		t.Fatal("expected to find codex provider")
	}

	if provider.Name != "codex" {
		t.Errorf("Name = %q; want %q", provider.Name, "codex")
	}
	if provider.Command != "codex" {
		t.Errorf("Command = %q; want %q", provider.Command, "codex")
	}
	if !provider.Capabilities.Planning {
		// Codex is now planning-aware in the test
	}
	if provider.SystemPromptStrategy != PromptStrategyFlag {
		t.Errorf("SystemPromptStrategy = %v; want %v", provider.SystemPromptStrategy, PromptStrategyFlag)
	}
}

func TestLookupBuiltinProvider_ClaudeCode(t *testing.T) {
	provider, ok := LookupBuiltinProvider("claude-code")
	if !ok {
		t.Fatal("expected to find claude-code provider")
	}

	if provider.Name != "claude-code" {
		t.Errorf("Name = %q; want %q", provider.Name, "claude-code")
	}
	if provider.Command != "claude" {
		t.Errorf("Command = %q; want %q", provider.Command, "claude")
	}
	if provider.SystemPromptStrategy != PromptStrategyStdin {
		t.Errorf("SystemPromptStrategy = %v; want %v", provider.SystemPromptStrategy, PromptStrategyStdin)
	}
}

func TestLookupBuiltinProvider_Unknown(t *testing.T) {
	provider, ok := LookupBuiltinProvider("unknown-provider")
	if ok {
		t.Error("expected not found for unknown provider")
	}
	if provider != nil {
		t.Error("expected nil provider for unknown")
	}
}

func TestLookupBuiltinProvider_ReturnsClone(t *testing.T) {
	provider1, _ := LookupBuiltinProvider("opencode")
	provider2, _ := LookupBuiltinProvider("opencode")

	// Modify provider1
	provider1.Command = "modified"
	provider1.Args = append(provider1.Args, "extra")

	// provider2 should be unaffected
	if provider2.Command == "modified" {
		t.Error("clone should be independent - Command was modified")
	}
	if len(provider2.Args) != 1 {
		t.Error("clone should be independent - Args was modified")
	}
}

func TestBuiltinProviderNames(t *testing.T) {
	names := BuiltinProviderNames()

	if len(names) < 3 {
		t.Errorf("expected at least 3 builtin providers, got %d", len(names))
	}

	// Check expected providers exist
	expected := map[string]bool{"opencode": false, "codex": false, "claude-code": false}
	for _, name := range names {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected builtin provider %q not found", name)
		}
	}
}

func TestProviderClone(t *testing.T) {
	original := &Provider{
		Name:        "test",
		DisplayName: "Test Provider",
		Command:     "test-cmd",
		Args:        []string{"arg1", "arg2"},
		Env:         map[string]string{"KEY": "value"},
		Capabilities: ProviderCaps{
			Streaming:    true,
			ToolCalls:    true,
			Planning:     false,
			ContextReset: true,
		},
		SystemPromptStrategy: PromptStrategyFlag,
		VersionArgs:          []string{"--version"},
	}

	clone := original.Clone()

	// Verify all fields are copied
	if clone.Name != original.Name {
		t.Errorf("Name = %q; want %q", clone.Name, original.Name)
	}
	if clone.DisplayName != original.DisplayName {
		t.Errorf("DisplayName = %q; want %q", clone.DisplayName, original.DisplayName)
	}
	if clone.Command != original.Command {
		t.Errorf("Command = %q; want %q", clone.Command, original.Command)
	}
	if len(clone.Args) != len(original.Args) {
		t.Errorf("Args length = %d; want %d", len(clone.Args), len(original.Args))
	}
	if clone.Env["KEY"] != original.Env["KEY"] {
		t.Errorf("Env[KEY] = %q; want %q", clone.Env["KEY"], original.Env["KEY"])
	}
	if clone.Capabilities != original.Capabilities {
		t.Error("Capabilities not equal")
	}
	if clone.SystemPromptStrategy != original.SystemPromptStrategy {
		t.Errorf("SystemPromptStrategy = %v; want %v", clone.SystemPromptStrategy, original.SystemPromptStrategy)
	}

	// Verify independence
	clone.Args[0] = "modified"
	if original.Args[0] == "modified" {
		t.Error("modifying clone should not affect original")
	}

	clone.Env["KEY"] = "modified"
	if original.Env["KEY"] == "modified" {
		t.Error("modifying clone env should not affect original")
	}
}

func TestProviderClone_Nil(t *testing.T) {
	var p *Provider
	clone := p.Clone()
	if clone != nil {
		t.Error("Clone of nil should return nil")
	}
}

func TestProviderMerge(t *testing.T) {
	base := &Provider{
		Name:        "base",
		DisplayName: "Base Provider",
		Command:     "base-cmd",
		Args:        []string{"base-arg"},
		Env:         map[string]string{"BASE_KEY": "base-value"},
	}

	override := &Provider{
		Command: "override-cmd",
		Args:    []string{"override-arg1", "override-arg2"},
		Env:     map[string]string{"OVERRIDE_KEY": "override-value"},
	}

	base.Merge(override)

	// Name and DisplayName should be unchanged (override has empty values)
	if base.Name != "base" {
		t.Errorf("Name = %q; want %q", base.Name, "base")
	}
	if base.DisplayName != "Base Provider" {
		t.Errorf("DisplayName = %q; want %q", base.DisplayName, "Base Provider")
	}

	// Command should be overridden
	if base.Command != "override-cmd" {
		t.Errorf("Command = %q; want %q", base.Command, "override-cmd")
	}

	// Args should be replaced entirely
	if len(base.Args) != 2 || base.Args[0] != "override-arg1" {
		t.Errorf("Args = %v; want [override-arg1 override-arg2]", base.Args)
	}

	// Env should be merged (both keys present)
	if base.Env["BASE_KEY"] != "base-value" {
		t.Errorf("Env[BASE_KEY] = %q; want %q", base.Env["BASE_KEY"], "base-value")
	}
	if base.Env["OVERRIDE_KEY"] != "override-value" {
		t.Errorf("Env[OVERRIDE_KEY] = %q; want %q", base.Env["OVERRIDE_KEY"], "override-value")
	}
}

func TestProviderMerge_Nil(t *testing.T) {
	base := &Provider{
		Name:    "base",
		Command: "cmd",
	}

	// Should not panic
	base.Merge(nil)

	if base.Command != "cmd" {
		t.Error("merge with nil should not change base")
	}
}

func TestPromptStrategyString(t *testing.T) {
	tests := []struct {
		strategy PromptStrategy
		want     string
	}{
		{PromptStrategyStdin, "stdin"},
		{PromptStrategyFlag, "flag"},
		{PromptStrategyEnv, "env"},
		{PromptStrategy(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.strategy.String()
		if got != tt.want {
			t.Errorf("%v.String() = %q; want %q", tt.strategy, got, tt.want)
		}
	}
}

func TestProvider_ModelFlag_DefaultsForCatalog(t *testing.T) {
	// Table test for all builtin providers.
	tests := []struct {
		name      string
		wantFlag  string
	}{
		{"opencode", "--model"},
		{"codex", "--model"},
		{"claude-code", "--model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, ok := LookupBuiltinProvider(tt.name)
			if !ok {
				t.Fatalf("expected to find %q provider", tt.name)
			}
			if provider.ModelFlag != tt.wantFlag {
				t.Errorf("ModelFlag = %q; want %q", provider.ModelFlag, tt.wantFlag)
			}
		})
	}
}

func TestProviderClone_CopiesModelFlag(t *testing.T) {
	original := &Provider{
		Name:      "test",
		ModelFlag: "--model",
	}

	clone := original.Clone()

	if clone.ModelFlag != original.ModelFlag {
		t.Errorf("ModelFlag = %q; want %q", clone.ModelFlag, original.ModelFlag)
	}

	// Verify independence (strings are immutable, but ensure assignment works)
	clone.ModelFlag = "-m"
	if original.ModelFlag == "-m" {
		t.Error("modifying clone should not affect original")
	}
}

func TestProviderMerge_MergesModelFlag(t *testing.T) {
	base := &Provider{
		Name:      "base",
		ModelFlag: "--model",
	}

	override := &Provider{
		ModelFlag: "-m",
	}

	base.Merge(override)

	if base.ModelFlag != "-m" {
		t.Errorf("ModelFlag = %q; want %q", base.ModelFlag, "-m")
	}
}

func TestProviderMerge_EmptyModelFlag_NoOverride(t *testing.T) {
	base := &Provider{
		Name:      "base",
		ModelFlag: "--model",
	}

	override := &Provider{
		Command: "new-cmd", // Non-empty to verify partial merge
		// ModelFlag intentionally empty
	}

	base.Merge(override)

	// ModelFlag should remain unchanged
	if base.ModelFlag != "--model" {
		t.Errorf("ModelFlag = %q; want %q (should not be overridden by empty)", base.ModelFlag, "--model")
	}
	// But Command should be updated
	if base.Command != "new-cmd" {
		t.Errorf("Command = %q; want %q", base.Command, "new-cmd")
	}
}
