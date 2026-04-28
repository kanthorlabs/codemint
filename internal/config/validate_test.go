package config

import (
	"strings"
	"testing"
)

func init() {
	// Set up builtin provider names for testing.
	BuiltinProviderNames = func() []string {
		return []string{"opencode", "codex", "claude-code"}
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Workflows: []WorkflowConfig{
			{Type: 0, Name: "Project Coding", Description: "Coding tasks"},
			{Type: 1, Name: "Communication", Description: "Inquiries"},
			{Type: 2, Name: "Daily Checking", Description: "Status checks"},
		},
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate returned error for valid config: %v", err)
	}
}

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := &Config{}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate returned error for empty config: %v", err)
	}
}

func TestValidate_NilConfig(t *testing.T) {
	err := Validate(nil)
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
}

func TestValidate_DuplicateType(t *testing.T) {
	cfg := &Config{
		Workflows: []WorkflowConfig{
			{Type: 0, Name: "First", Description: "First workflow"},
			{Type: 0, Name: "Second", Description: "Duplicate type"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate type, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(vErr.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(vErr.Violations), vErr.Violations)
	}

	if !strings.Contains(vErr.Violations[0], "duplicate") {
		t.Errorf("violation should mention duplicate: %s", vErr.Violations[0])
	}
}

func TestValidate_EmptyName(t *testing.T) {
	cfg := &Config{
		Workflows: []WorkflowConfig{
			{Type: 0, Name: "", Description: "Missing name"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty names, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	// Should have 1 violation for empty name.
	if len(vErr.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(vErr.Violations), vErr.Violations)
	}

	if !strings.Contains(vErr.Violations[0], "required") {
		t.Errorf("violation should mention required: %s", vErr.Violations[0])
	}
}

func TestValidate_InvalidType(t *testing.T) {
	cfg := &Config{
		Workflows: []WorkflowConfig{
			{Type: 99, Name: "Invalid Type", Description: "Type out of range"},
			{Type: -1, Name: "Negative Type", Description: "Negative type"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(vErr.Violations) != 2 {
		t.Errorf("expected 2 violations, got %d: %v", len(vErr.Violations), vErr.Violations)
	}

	for _, v := range vErr.Violations {
		if !strings.Contains(v, "must be at") {
			t.Errorf("violation should mention range constraint: %s", v)
		}
	}
}

func TestValidate_MultipleViolations(t *testing.T) {
	cfg := &Config{
		Workflows: []WorkflowConfig{
			{Type: 0, Name: "", Description: "Empty name"},
			{Type: 0, Name: "Duplicate", Description: "Duplicate type"},
			{Type: 99, Name: "Invalid", Description: "Invalid type"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	// Should collect all violations (empty name, duplicate type, invalid type).
	if len(vErr.Violations) < 3 {
		t.Errorf("expected at least 3 violations, got %d: %v", len(vErr.Violations), vErr.Violations)
	}
}

func TestValidationError_Error(t *testing.T) {
	vErr := &ValidationError{
		Violations: []string{"first error", "second error"},
	}

	msg := vErr.Error()
	if !strings.Contains(msg, "first error") || !strings.Contains(msg, "second error") {
		t.Errorf("Error() should contain all violations: %s", msg)
	}
}

func TestValidate_UnknownProvider(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: "unknown-provider"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(vErr.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(vErr.Violations), vErr.Violations)
	}

	if !strings.Contains(vErr.Violations[0], "unknown provider") {
		t.Errorf("violation should mention 'unknown provider': %s", vErr.Violations[0])
	}
	if !strings.Contains(vErr.Violations[0], "unknown-provider") {
		t.Errorf("violation should mention the provider name: %s", vErr.Violations[0])
	}
}

func TestValidate_DefaultAssistantEmptyProvider(t *testing.T) {
	// Empty assistants should be valid (general validation doesn't require sys-default).
	// ValidateSysDefault is used for strict startup validation.
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"custom-assistant": {Provider: ""}, // Empty provider defaults to opencode
		},
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate returned error for empty provider (should be valid): %v", err)
	}
}

func TestValidate_ProviderOverrideKnown(t *testing.T) {
	// Using a custom provider that's declared in providers section.
	cfg := &Config{
		Providers: []ProviderConfig{
			{Name: "custom-ai", Command: "/usr/bin/custom-ai"},
		},
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: "custom-ai"},
		},
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate returned error for custom provider declared in providers: %v", err)
	}
}

func TestValidate_BuiltinProvider(t *testing.T) {
	// Using builtin providers should be valid.
	for _, name := range []string{"opencode", "codex", "claude-code"} {
		cfg := &Config{
			Assistants: map[string]AssistantConfig{
				"sys-default": {Provider: name},
			},
		}

		err := Validate(cfg)
		if err != nil {
			t.Errorf("Validate returned error for builtin provider %q: %v", name, err)
		}
	}
}

func TestValidate_DuplicateProviderNames(t *testing.T) {
	cfg := &Config{
		Providers: []ProviderConfig{
			{Name: "myai", Command: "/bin/ai1"},
			{Name: "myai", Command: "/bin/ai2"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate provider name, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(vErr.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(vErr.Violations), vErr.Violations)
	}

	if !strings.Contains(vErr.Violations[0], "duplicate provider name") {
		t.Errorf("violation should mention 'duplicate provider name': %s", vErr.Violations[0])
	}
}

func TestValidate_ProviderMissingName(t *testing.T) {
	cfg := &Config{
		Providers: []ProviderConfig{
			{Name: "", Command: "/bin/ai"}, // Missing name
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing provider name, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if !strings.Contains(vErr.Violations[0], "required") {
		t.Errorf("violation should mention 'required': %s", vErr.Violations[0])
	}
}

func TestValidate_MultipleAssistantBindings(t *testing.T) {
	// Test that multiple assistant bindings can reference different providers.
	cfg := &Config{
		Providers: []ProviderConfig{
			{Name: "custom-brainstormer", Command: "/bin/brainstorm"},
		},
		Assistants: map[string]AssistantConfig{
			"sys-default":  {Provider: "opencode"},
			"brainstormer": {Provider: "custom-brainstormer"},
			"clarifier":    {Provider: "codex"},
		},
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate returned error for valid multi-assistant config: %v", err)
	}
}

func TestValidate_MultipleUnknownProviders(t *testing.T) {
	// Test that multiple unknown providers are all reported.
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default":  {Provider: "unknown1"},
			"brainstormer": {Provider: "unknown2"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for unknown providers, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(vErr.Violations) != 2 {
		t.Errorf("expected 2 violations, got %d: %v", len(vErr.Violations), vErr.Violations)
	}
}

func TestValidateSysDefault_MissingProvider(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: ""}, // Missing provider
		},
	}

	knownProviders := map[string]bool{"opencode": true}
	err := ValidateSysDefault(cfg, knownProviders)
	if err == nil {
		t.Fatal("expected error for missing sys-default provider, got nil")
	}

	if !strings.Contains(err.Error(), "provider is required") {
		t.Errorf("error should mention provider is required: %v", err)
	}
}

func TestValidateSysDefault_MissingSysDefault(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			// No sys-default configured
			"other": {Provider: "opencode"},
		},
	}

	knownProviders := map[string]bool{"opencode": true}
	err := ValidateSysDefault(cfg, knownProviders)
	if err == nil {
		t.Fatal("expected error for missing sys-default, got nil")
	}

	if !strings.Contains(err.Error(), "provider is required") {
		t.Errorf("error should mention provider is required: %v", err)
	}
}

func TestValidateSysDefault_UnknownProvider(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: "unknown-provider"},
		},
	}

	knownProviders := map[string]bool{"opencode": true}
	err := ValidateSysDefault(cfg, knownProviders)
	if err == nil {
		t.Fatal("expected error for unknown sys-default provider, got nil")
	}

	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("error should mention unknown provider: %v", err)
	}
}

func TestValidateSysDefault_Valid(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: "opencode", Model: "gpt-4"},
		},
	}

	knownProviders := map[string]bool{"opencode": true}
	err := ValidateSysDefault(cfg, knownProviders)
	if err != nil {
		t.Errorf("ValidateSysDefault returned error for valid config: %v", err)
	}
}

func TestGetAssistant(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: "opencode", Model: "gpt-4"},
			"custom":      {Provider: "codex"},
		},
	}

	// Test getting existing assistant.
	sysDefault := cfg.GetAssistant("sys-default")
	if sysDefault.Provider != "opencode" {
		t.Errorf("GetAssistant(sys-default).Provider = %q; want %q", sysDefault.Provider, "opencode")
	}
	if sysDefault.Model != "gpt-4" {
		t.Errorf("GetAssistant(sys-default).Model = %q; want %q", sysDefault.Model, "gpt-4")
	}

	// Test getting non-existent assistant.
	missing := cfg.GetAssistant("missing")
	if missing.Provider != "" || missing.Model != "" {
		t.Errorf("GetAssistant(missing) should return empty config, got %+v", missing)
	}
}

func TestGetSysDefault(t *testing.T) {
	cfg := &Config{
		Assistants: map[string]AssistantConfig{
			"sys-default": {Provider: "opencode", Model: "gpt-4"},
		},
	}

	sysDefault := cfg.GetSysDefault()
	if sysDefault.Provider != "opencode" {
		t.Errorf("GetSysDefault().Provider = %q; want %q", sysDefault.Provider, "opencode")
	}
	if sysDefault.Model != "gpt-4" {
		t.Errorf("GetSysDefault().Model = %q; want %q", sysDefault.Model, "gpt-4")
	}
}
