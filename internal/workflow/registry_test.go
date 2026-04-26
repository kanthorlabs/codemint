package workflow

import (
	"strings"
	"testing"

	"codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/domain"
)

// --- Registry Tests ---

func TestNewWorkflowRegistry_Empty(t *testing.T) {
	reg := NewWorkflowRegistry()
	if reg.Len() != 0 {
		t.Errorf("new registry should be empty, got %d", reg.Len())
	}
}

func TestRegister_Success(t *testing.T) {
	reg := NewWorkflowRegistry()
	def := domain.WorkflowDefinition{
		Type:        domain.WorkflowTypeProjectCoding,
		Name:        "Project Coding",
		Description: "Context-aware coding tasks",
		Triggers:    []string{"implement", "fix"},
	}

	err := reg.Register(def)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if reg.Len() != 1 {
		t.Errorf("registry should have 1 workflow, got %d", reg.Len())
	}
}

func TestRegister_Duplicate_ReturnsError(t *testing.T) {
	reg := NewWorkflowRegistry()
	def := domain.WorkflowDefinition{
		Type: domain.WorkflowTypeProjectCoding,
		Name: "Project Coding",
	}

	_ = reg.Register(def)

	err := reg.Register(def)
	if err == nil {
		t.Fatal("expected error for duplicate registration, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate: %v", err)
	}
}

func TestLookup_Success(t *testing.T) {
	reg := NewWorkflowRegistry()
	def := domain.WorkflowDefinition{
		Type:        domain.WorkflowTypeCommunication,
		Name:        "Communication",
		Description: "General inquiries",
	}
	_ = reg.Register(def)

	got, err := reg.Lookup(domain.WorkflowTypeCommunication)
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}

	if got.Name != def.Name {
		t.Errorf("Lookup returned %q, want %q", got.Name, def.Name)
	}
}

func TestLookup_NotFound(t *testing.T) {
	reg := NewWorkflowRegistry()

	_, err := reg.Lookup(domain.WorkflowTypeProjectCoding)
	if err == nil {
		t.Fatal("expected ErrWorkflowNotFound, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestAll_SortedByType(t *testing.T) {
	reg := NewWorkflowRegistry()

	// Register in reverse order.
	_ = reg.Register(domain.WorkflowDefinition{Type: domain.WorkflowTypeDailyChecking, Name: "Daily"})
	_ = reg.Register(domain.WorkflowDefinition{Type: domain.WorkflowTypeProjectCoding, Name: "Coding"})
	_ = reg.Register(domain.WorkflowDefinition{Type: domain.WorkflowTypeCommunication, Name: "Comm"})

	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("All returned %d items, want 3", len(all))
	}

	wantOrder := []domain.WorkflowType{
		domain.WorkflowTypeProjectCoding,
		domain.WorkflowTypeCommunication,
		domain.WorkflowTypeDailyChecking,
	}

	for i, w := range all {
		if w.Type != wantOrder[i] {
			t.Errorf("All()[%d].Type = %d, want %d", i, w.Type, wantOrder[i])
		}
	}
}

func TestFindByTrigger_Matches(t *testing.T) {
	reg := NewWorkflowRegistry()
	_ = reg.Register(domain.WorkflowDefinition{
		Type:     domain.WorkflowTypeProjectCoding,
		Name:     "Coding",
		Triggers: []string{"implement", "fix"},
	})
	_ = reg.Register(domain.WorkflowDefinition{
		Type:     domain.WorkflowTypeCommunication,
		Name:     "Communication",
		Triggers: []string{"explain", "what is"},
	})

	tests := []struct {
		input    string
		wantName string
		wantOK   bool
	}{
		{"implement a feature", "Coding", true},
		{"Fix the bug", "Coding", true},          // case-insensitive
		{"can you explain this?", "Communication", true},
		{"what is the status?", "Communication", true},
		{"random input", "", false},
	}

	for _, tt := range tests {
		def, ok := reg.FindByTrigger(tt.input)
		if ok != tt.wantOK {
			t.Errorf("FindByTrigger(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok && def.Name != tt.wantName {
			t.Errorf("FindByTrigger(%q) = %q, want %q", tt.input, def.Name, tt.wantName)
		}
	}
}

// --- LoadFromConfig Tests ---

func TestLoadFromConfig_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		Workflows: []config.WorkflowConfig{
			{Type: 0, Name: "Project Coding", Description: "Coding tasks", Triggers: []string{"implement"}},
			{Type: 1, Name: "Communication", Description: "Inquiries", Triggers: []string{"explain"}},
			{Type: 2, Name: "Daily Checking", Description: "Status checks", Triggers: []string{"status"}},
		},
	}

	reg, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig returned error: %v", err)
	}

	if reg.Len() != 3 {
		t.Errorf("registry should have 3 workflows, got %d", reg.Len())
	}

	// Verify each workflow is accessible.
	for _, wc := range cfg.Workflows {
		def, err := reg.Lookup(domain.WorkflowType(wc.Type))
		if err != nil {
			t.Errorf("Lookup(%d) failed: %v", wc.Type, err)
		}
		if def.Name != wc.Name {
			t.Errorf("Workflow %d name = %q, want %q", wc.Type, def.Name, wc.Name)
		}
	}
}

func TestLoadFromConfig_InvalidConfig_ReturnsValidationError(t *testing.T) {
	cfg := &config.Config{
		Workflows: []config.WorkflowConfig{
			{Type: 0, Name: "Workflow A", Description: "First"},
			{Type: 0, Name: "Workflow B", Description: "Duplicate type"},
		},
	}

	_, err := LoadFromConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for duplicate type, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate: %v", err)
	}
}

func TestLoadFromConfig_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}

	reg, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig returned error: %v", err)
	}

	if reg.Len() != 0 {
		t.Errorf("empty config should create empty registry, got %d", reg.Len())
	}
}

// --- End-to-End Test ---

func TestEndToEnd_ConfigToRegistryToLookup(t *testing.T) {
	// Simulate full flow: YAML string → Config → Registry → Lookup → FindByTrigger.
	cfg := &config.Config{
		Workflows: []config.WorkflowConfig{
			{
				Type:        0,
				Name:        "Project Coding",
				Description: "Context-aware coding tasks within a project",
				Triggers:    []string{"implement", "fix", "refactor", "add feature"},
			},
			{
				Type:        1,
				Name:        "Communication",
				Description: "General inquiries and explanations",
				Triggers:    []string{"explain", "what is", "how does", "tell me"},
			},
			{
				Type:        2,
				Name:        "Daily Checking",
				Description: "Status checks and routine operations",
				Triggers:    []string{"status", "check", "verify", "test"},
			},
		},
	}

	// Validate config.
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Load into registry.
	reg, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	// Verify all three workflow types are routable.
	tests := []struct {
		input    string
		wantType domain.WorkflowType
	}{
		{"please implement this feature", domain.WorkflowTypeProjectCoding},
		{"fix the login bug", domain.WorkflowTypeProjectCoding},
		{"what is dependency injection?", domain.WorkflowTypeCommunication},
		{"explain how goroutines work", domain.WorkflowTypeCommunication},
		{"check the test results", domain.WorkflowTypeDailyChecking},
		{"verify the build status", domain.WorkflowTypeDailyChecking},
	}

	for _, tt := range tests {
		def, ok := reg.FindByTrigger(tt.input)
		if !ok {
			t.Errorf("FindByTrigger(%q) returned false, expected match", tt.input)
			continue
		}
		if def.Type != tt.wantType {
			t.Errorf("FindByTrigger(%q) = %s, want %s", tt.input, def.Type, tt.wantType)
		}
	}

	// Verify lookup by type.
	for i := 0; i <= 2; i++ {
		_, err := reg.Lookup(domain.WorkflowType(i))
		if err != nil {
			t.Errorf("Lookup(%d) failed: %v", i, err)
		}
	}
}
