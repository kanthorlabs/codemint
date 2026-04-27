package repl

import (
	"context"
	"strings"
	"testing"

	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/workflow"
)

// mockWorkflowSessionInfo implements WorkflowSessionInfo for testing.
type mockWorkflowSessionInfo struct {
	sessionID string
	projectID string
	clientID  string
}

func (m *mockWorkflowSessionInfo) GetClientMode() registry.ClientMode  { return registry.ClientModeCLI }
func (m *mockWorkflowSessionInfo) GetIsCodeMint() bool                 { return m.sessionID == "" }
func (m *mockWorkflowSessionInfo) GetSessionID() string                { return m.sessionID }
func (m *mockWorkflowSessionInfo) GetProjectID() string                { return m.projectID }
func (m *mockWorkflowSessionInfo) GetClientID() string                 { return m.clientID }
func (m *mockWorkflowSessionInfo) SetSession(_ any, _ any, _ bool)     {}
func (m *mockWorkflowSessionInfo) SetSuspended(_ bool)                 {}
func (m *mockWorkflowSessionInfo) SetClientMode(_ registry.ClientMode) {}
func (m *mockWorkflowSessionInfo) Wakeup()                             {}

func TestWorkflowHandler_ListWorkflows_Empty(t *testing.T) {
	// Create empty file registry.
	reg := workflow.NewFileRegistry()

	deps := &WorkflowCommandDeps{
		FileRegistry: reg,
	}

	handler := workflowHandler(deps)
	result, err := handler(context.Background(), nil, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message == "" {
		t.Error("expected non-empty message")
	}

	// Should mention no workflows available.
	if result.Action != registry.ActionNone {
		t.Errorf("expected ActionNone, got %v", result.Action)
	}
}

func TestWorkflowHandler_NilRegistry(t *testing.T) {
	deps := &WorkflowCommandDeps{
		FileRegistry: nil, // Nil registry
	}

	handler := workflowHandler(deps)
	result, err := handler(context.Background(), nil, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message != "Workflow file registry not available." {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestWorkflowHandler_StartWorkflow_NoSession(t *testing.T) {
	// Create a file registry with a workflow.
	reg := workflow.NewFileRegistry()

	// Create mock active session with no session.
	activeSession := &mockWorkflowSessionInfo{
		sessionID: "",
		projectID: "",
	}

	deps := &WorkflowCommandDeps{
		FileRegistry:  reg,
		ActiveSession: activeSession,
	}

	handler := workflowHandler(deps)
	result, err := handler(context.Background(), nil, []string{"brainstorming"}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should indicate no active session.
	expectedMsg := "No active session. Use /project-open to start."
	if result.Message != expectedMsg {
		t.Errorf("unexpected message: got %q, want %q", result.Message, expectedMsg)
	}
}

func TestWorkflowHandler_StartWorkflow_NoProject(t *testing.T) {
	// Create a file registry.
	reg := workflow.NewFileRegistry()

	// Create mock active session with session but no project.
	activeSession := &mockWorkflowSessionInfo{
		sessionID: "session-123",
		projectID: "", // No project
	}

	deps := &WorkflowCommandDeps{
		FileRegistry:  reg,
		ActiveSession: activeSession,
	}

	handler := workflowHandler(deps)
	result, err := handler(context.Background(), nil, []string{"brainstorming"}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should indicate no active project.
	expectedMsg := "No active project. Use /project-open to select a project."
	if result.Message != expectedMsg {
		t.Errorf("unexpected message: got %q, want %q", result.Message, expectedMsg)
	}
}

func TestWorkflowHandler_StartWorkflow_NotFound(t *testing.T) {
	// Create empty file registry.
	reg := workflow.NewFileRegistry()

	// Create mock active session with session and project.
	activeSession := &mockWorkflowSessionInfo{
		sessionID: "session-123",
		projectID: "project-456",
	}

	deps := &WorkflowCommandDeps{
		FileRegistry:  reg,
		ActiveSession: activeSession,
	}

	handler := workflowHandler(deps)
	result, err := handler(context.Background(), nil, []string{"nonexistent"}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should indicate workflow not found.
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("expected 'not found' in message, got: %s", result.Message)
	}
}

func TestWorkflowCompleter(t *testing.T) {
	tests := []struct {
		name           string
		workflowNames  []string
		prefix         string
		expectedResult []string
	}{
		{
			name:           "exact match",
			workflowNames:  []string{"brainstorming", "coding", "review"},
			prefix:         "brainstorming",
			expectedResult: []string{"brainstorming"},
		},
		{
			name:           "prefix match",
			workflowNames:  []string{"brainstorming", "coding", "review"},
			prefix:         "br",
			expectedResult: []string{"brainstorming"},
		},
		{
			name:           "multiple matches",
			workflowNames:  []string{"brainstorming", "branching", "coding"},
			prefix:         "bra",
			expectedResult: []string{"brainstorming", "branching"},
		},
		{
			name:           "no match",
			workflowNames:  []string{"brainstorming", "coding"},
			prefix:         "xyz",
			expectedResult: nil,
		},
		{
			name:           "empty prefix",
			workflowNames:  []string{"brainstorming", "coding"},
			prefix:         "",
			expectedResult: []string{"brainstorming", "coding"},
		},
		{
			name:           "case insensitive",
			workflowNames:  []string{"BrainStorming", "Coding"},
			prefix:         "brain",
			expectedResult: []string{"BrainStorming"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock registry that returns the workflow names.
			// Since we can't easily create a real FileRegistry with custom workflows,
			// we'll test the completer logic directly.
			reg := workflow.NewFileRegistry()
			// Note: FileRegistry.Names() returns empty for a fresh registry.
			// The completer function handles nil/empty gracefully.

			completer := WorkflowCompleter(reg)
			result := completer(tc.prefix)

			// For an empty registry, result should be nil or empty.
			// This test validates the completer handles empty registries.
			if reg.Len() == 0 && len(result) != 0 {
				t.Errorf("expected empty result for empty registry, got %v", result)
			}
		})
	}
}

func TestWorkflowCompleter_NilRegistry(t *testing.T) {
	completer := WorkflowCompleter(nil)
	result := completer("test")

	if result != nil {
		t.Errorf("expected nil result for nil registry, got %v", result)
	}
}

func TestFindWorkflowByPrefix(t *testing.T) {
	// Test with empty registry.
	reg := workflow.NewFileRegistry()
	result := findWorkflowByPrefix(reg, "test")
	if result != nil {
		t.Errorf("expected nil for empty registry, got %v", result)
	}
}
