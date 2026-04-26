package repl

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/ui"
)

// mockTaskRepoForDaemon implements minimal TaskRepository for daemon command tests.
type mockTaskRepoForDaemon struct {
	tasks []*domain.Task
}

func (m *mockTaskRepoForDaemon) Create(_ context.Context, t *domain.Task) error {
	m.tasks = append(m.tasks, t)
	return nil
}

func (m *mockTaskRepoForDaemon) Next(_ context.Context, _ string) (*domain.Task, error) {
	for _, t := range m.tasks {
		if t.Status == domain.TaskStatusPending || t.Status == domain.TaskStatusAwaiting {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockTaskRepoForDaemon) Claim(_ context.Context, _ string) error                              { return nil }
func (m *mockTaskRepoForDaemon) UpdateStatus(_ context.Context, _ string, _ domain.TaskStatus, _ string) error { return nil }
func (m *mockTaskRepoForDaemon) FindInterrupted(_ context.Context, _ string) ([]*domain.Task, error)  { return nil, nil }
func (m *mockTaskRepoForDaemon) FindByID(_ context.Context, id string) (*domain.Task, error) {
	for _, t := range m.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, nil
}
func (m *mockTaskRepoForDaemon) UpdateTaskStatus(_ context.Context, _ string, _ domain.TaskStatus) error { return nil }
func (m *mockTaskRepoForDaemon) UpdateAssignee(_ context.Context, _ string, _ string) error              { return nil }
func (m *mockTaskRepoForDaemon) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) { return nil, nil }
func (m *mockTaskRepoForDaemon) ListBySession(_ context.Context, sessionID string) ([]*domain.Task, error) {
	var result []*domain.Task
	for _, t := range m.tasks {
		if t.SessionID == sessionID {
			result = append(result, t)
		}
	}
	return result, nil
}

// mockActiveSessionForDaemon implements MutableSessionInfo for testing.
type mockActiveSessionForDaemon struct {
	sessionID  string
	clientMode registry.ClientMode
}

func (m *mockActiveSessionForDaemon) GetClientMode() registry.ClientMode { return m.clientMode }
func (m *mockActiveSessionForDaemon) GetIsGlobal() bool                  { return m.sessionID == "" }
func (m *mockActiveSessionForDaemon) GetSessionID() string               { return m.sessionID }
func (m *mockActiveSessionForDaemon) GetClientID() string                { return "test-client" }
func (m *mockActiveSessionForDaemon) SetSession(_ any, _ any, _ bool)    {}
func (m *mockActiveSessionForDaemon) SetSuspended(_ bool)                {}
func (m *mockActiveSessionForDaemon) SetClientMode(_ registry.ClientMode) {}

// TestTasksHandler_NoSession tests /tasks with no active session.
func TestTasksHandler_NoSession(t *testing.T) {
	deps := &DaemonCommandDeps{
		TaskRepo:      &mockTaskRepoForDaemon{},
		ActiveSession: &mockActiveSessionForDaemon{sessionID: ""},
	}

	handler := tasksHandler(deps)
	result, err := handler(context.Background(), deps.ActiveSession, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message != "No active session. Use /project-open to start." {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

// TestTasksHandler_NoTasks tests /tasks with no tasks in session.
func TestTasksHandler_NoTasks(t *testing.T) {
	deps := &DaemonCommandDeps{
		TaskRepo:      &mockTaskRepoForDaemon{},
		ActiveSession: &mockActiveSessionForDaemon{sessionID: "session-123"},
	}

	handler := tasksHandler(deps)
	result, err := handler(context.Background(), deps.ActiveSession, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message != "No tasks in this session." {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

// TestTasksHandler_WithTasks tests /tasks displays task hierarchy.
func TestTasksHandler_WithTasks(t *testing.T) {
	repo := &mockTaskRepoForDaemon{
		tasks: []*domain.Task{
			{
				ID:        "task-1",
				SessionID: "session-123",
				SeqEpic:   1,
				SeqStory:  1,
				SeqTask:   1,
				Type:      domain.TaskTypeCoding,
				Status:    domain.TaskStatusSuccess,
			},
			{
				ID:        "task-2",
				SessionID: "session-123",
				SeqEpic:   1,
				SeqStory:  1,
				SeqTask:   2,
				Type:      domain.TaskTypeVerification,
				Status:    domain.TaskStatusPending,
			},
			{
				ID:        "task-3",
				SessionID: "session-123",
				SeqEpic:   1,
				SeqStory:  2,
				SeqTask:   1,
				Type:      domain.TaskTypeCoding,
				Status:    domain.TaskStatusAwaiting,
			},
		},
	}

	deps := &DaemonCommandDeps{
		TaskRepo:      repo,
		ActiveSession: &mockActiveSessionForDaemon{sessionID: "session-123"},
	}

	handler := tasksHandler(deps)
	result, err := handler(context.Background(), deps.ActiveSession, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that output contains expected content.
	if !contains(result.Message, "Epic 1 / Story 1") {
		t.Errorf("missing Epic 1 / Story 1 in output: %s", result.Message)
	}
	if !contains(result.Message, "Epic 1 / Story 2") {
		t.Errorf("missing Epic 1 / Story 2 in output: %s", result.Message)
	}
	if !contains(result.Message, "[S]") {
		t.Errorf("missing Success indicator [S] in output: %s", result.Message)
	}
	if !contains(result.Message, "[P]") {
		t.Errorf("missing Pending indicator [P] in output: %s", result.Message)
	}
	if !contains(result.Message, "[A]") {
		t.Errorf("missing Awaiting indicator [A] in output: %s", result.Message)
	}
}

// TestStatusHandler_NoSession tests /status with no active session.
func TestStatusHandler_NoSession(t *testing.T) {
	deps := &DaemonCommandDeps{
		TaskRepo:      &mockTaskRepoForDaemon{},
		ActiveSession: &mockActiveSessionForDaemon{sessionID: ""},
	}

	handler := statusHandler(deps)
	result, err := handler(context.Background(), deps.ActiveSession, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Message, "Session: (none - global mode)") {
		t.Errorf("missing global mode indicator in output: %s", result.Message)
	}
}

// TestStatusHandler_WithSession tests /status with an active session.
func TestStatusHandler_WithSession(t *testing.T) {
	repo := &mockTaskRepoForDaemon{
		tasks: []*domain.Task{
			{
				ID:        "task-1",
				SessionID: "session-123",
				Status:    domain.TaskStatusPending,
			},
		},
	}

	deps := &DaemonCommandDeps{
		TaskRepo:      repo,
		ActiveSession: &mockActiveSessionForDaemon{sessionID: "session-123"},
	}

	handler := statusHandler(deps)
	result, err := handler(context.Background(), deps.ActiveSession, nil, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session ID is truncated to 8 chars.
	if !contains(result.Message, "Session: session-") {
		t.Errorf("missing session ID in output: %s", result.Message)
	}
	if !contains(result.Message, "Task:") {
		t.Errorf("missing task info in output: %s", result.Message)
	}
}

// TestApproveHandler_NoAdapter tests /approve without CUIAdapter.
func TestApproveHandler_NoAdapter(t *testing.T) {
	deps := &DaemonCommandDeps{
		CUIAdapter: nil,
	}

	handler := approveHandler(deps)
	result, err := handler(context.Background(), nil, []string{"1", "allow"}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Message, "Approval not available") {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

// TestApproveHandler_InvalidArgs tests /approve with missing arguments.
func TestApproveHandler_InvalidArgs(t *testing.T) {
	adapter := ui.NewCUIAdapter(ui.CUIAdapterConfig{})
	defer adapter.Close()

	deps := &DaemonCommandDeps{
		CUIAdapter: adapter,
	}

	handler := approveHandler(deps)
	result, err := handler(context.Background(), nil, []string{}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Message, "Usage:") {
		t.Errorf("expected usage message, got: %s", result.Message)
	}
}

// TestApproveHandler_NotFound tests /approve for non-existent prompt.
func TestApproveHandler_NotFound(t *testing.T) {
	adapter := ui.NewCUIAdapter(ui.CUIAdapterConfig{})
	defer adapter.Close()

	deps := &DaemonCommandDeps{
		CUIAdapter: adapter,
	}

	handler := approveHandler(deps)
	result, err := handler(context.Background(), nil, []string{"999", "allow"}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Message, "not found") {
		t.Errorf("expected 'not found' message, got: %s", result.Message)
	}
}

// TestDenyHandler_NoAdapter tests /deny without CUIAdapter.
func TestDenyHandler_NoAdapter(t *testing.T) {
	deps := &DaemonCommandDeps{
		CUIAdapter: nil,
	}

	handler := denyHandler(deps)
	result, err := handler(context.Background(), nil, []string{"1"}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Message, "Approval not available") {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

// TestStatusIndicator tests the status indicator mapping.
func TestStatusIndicator(t *testing.T) {
	tests := []struct {
		status   domain.TaskStatus
		expected string
	}{
		{domain.TaskStatusPending, "P"},
		{domain.TaskStatusProcessing, "R"},
		{domain.TaskStatusAwaiting, "A"},
		{domain.TaskStatusSuccess, "S"},
		{domain.TaskStatusFailure, "F"},
		{domain.TaskStatusCompleted, "C"},
		{domain.TaskStatusReverted, "V"},
		{domain.TaskStatusCancelled, "X"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := statusIndicator(tt.status)
			if got != tt.expected {
				t.Errorf("statusIndicator(%d) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

// TestFormatTaskHierarchy tests the task hierarchy formatting.
func TestFormatTaskHierarchy(t *testing.T) {
	tasks := []*domain.Task{
		{
			ID:       "task-1",
			SeqEpic:  1,
			SeqStory: 1,
			SeqTask:  1,
			Type:     domain.TaskTypeCoding,
			Status:   domain.TaskStatusSuccess,
		},
		{
			ID:       "task-2",
			SeqEpic:  2,
			SeqStory: 1,
			SeqTask:  1,
			Type:     domain.TaskTypeCoordination,
			Status:   domain.TaskStatusPending,
		},
	}

	output := formatTaskHierarchy(tasks)

	if !contains(output, "Epic 1 / Story 1") {
		t.Errorf("missing Epic 1 / Story 1 in output")
	}
	if !contains(output, "Epic 2 / Story 1") {
		t.Errorf("missing Epic 2 / Story 1 in output")
	}
	if !contains(output, "Legend:") {
		t.Errorf("missing legend in output")
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
