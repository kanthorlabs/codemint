package config

import (
	"strings"
	"testing"
)

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

func TestValidate_AgentConfig(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{Name: "", Type: 0}, // Missing name
			{Name: "Valid Agent", Type: 99}, // Invalid type
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid agent config, got nil")
	}

	vErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(vErr.Violations) != 2 {
		t.Errorf("expected 2 violations, got %d: %v", len(vErr.Violations), vErr.Violations)
	}
}
