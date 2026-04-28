package workflow

import (
	"context"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

// mockWorkflowRepo is a mock implementation of repository.WorkflowRepository.
type mockWorkflowRepo struct {
	lockGoalCalled    bool
	lockGoalWorkflow  string
	lockGoalText      string
	lockGoalCriteria  string
	lockGoalErr       error
}

func (m *mockWorkflowRepo) Create(ctx context.Context, w *domain.Workflow) error {
	return nil
}

func (m *mockWorkflowRepo) FindByID(ctx context.Context, id string) (*domain.Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepo) GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepo) UpdateProgress(ctx context.Context, id, epicID, storyID string) error {
	return nil
}

func (m *mockWorkflowRepo) MarkCompleted(ctx context.Context, id string) error {
	return nil
}

func (m *mockWorkflowRepo) MarkCancelled(ctx context.Context, id string) error {
	return nil
}

func (m *mockWorkflowRepo) ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepo) LockGoal(ctx context.Context, workflowID, goalText, criteriaJSON string) error {
	m.lockGoalCalled = true
	m.lockGoalWorkflow = workflowID
	m.lockGoalText = goalText
	m.lockGoalCriteria = criteriaJSON
	return m.lockGoalErr
}

func (m *mockWorkflowRepo) LockChosenOption(ctx context.Context, workflowID, optionJSON string) error {
	return nil
}

func TestLockWorkflowGoalHandler_HappyPath(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output:     `{"goal_text":"Add email validation to user registration","success_criteria":["go test passes","email format is validated"]}`,
		ExitCmd:    "/lock-goal",
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify repo was called correctly.
	if !repo.lockGoalCalled {
		t.Error("LockGoal was not called")
	}
	if repo.lockGoalWorkflow != "wf-123" {
		t.Errorf("WorkflowID = %q, want %q", repo.lockGoalWorkflow, "wf-123")
	}
	if repo.lockGoalText != "Add email validation to user registration" {
		t.Errorf("GoalText = %q, want %q", repo.lockGoalText, "Add email validation to user registration")
	}
	// Criteria should be marshaled as JSON array.
	expected := `["go test passes","email format is validated"]`
	if repo.lockGoalCriteria != expected {
		t.Errorf("CriteriaJSON = %q, want %q", repo.lockGoalCriteria, expected)
	}
}

func TestLockWorkflowGoalHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{invalid json`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if !strContains(err.Error(), "invalid JSON") {
		t.Errorf("Error = %q, want to contain 'invalid JSON'", err.Error())
	}
	if repo.lockGoalCalled {
		t.Error("LockGoal should not be called on invalid JSON")
	}
}

func TestLockWorkflowGoalHandler_EmptyGoalText(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"","success_criteria":["criterion"]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty goal_text")
	}
	if err.Error() != "lock_workflow_goal: goal_text is required" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: goal_text is required'", err.Error())
	}
	if repo.lockGoalCalled {
		t.Error("LockGoal should not be called on empty goal")
	}
}

func TestLockWorkflowGoalHandler_WhitespaceOnlyGoalText(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"   ","success_criteria":["criterion"]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for whitespace-only goal_text")
	}
	if err.Error() != "lock_workflow_goal: goal_text is required" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: goal_text is required'", err.Error())
	}
}

func TestLockWorkflowGoalHandler_EmptyCriteriaArray(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"A valid goal","success_criteria":[]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty criteria array")
	}
	if err.Error() != "lock_workflow_goal: at least one success criterion required" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: at least one success criterion required'", err.Error())
	}
	if repo.lockGoalCalled {
		t.Error("LockGoal should not be called on empty criteria")
	}
}

func TestLockWorkflowGoalHandler_AllEmptyCriteria(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"A valid goal","success_criteria":["","   ",""]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for all empty criteria")
	}
	if err.Error() != "lock_workflow_goal: all success criteria are empty" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: all success criteria are empty'", err.Error())
	}
}

func TestLockWorkflowGoalHandler_FiltersEmptyCriteria(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"A valid goal","success_criteria":["valid","","  ","also valid"]}`,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Only non-empty criteria should be stored.
	expected := `["valid","also valid"]`
	if repo.lockGoalCriteria != expected {
		t.Errorf("CriteriaJSON = %q, want %q", repo.lockGoalCriteria, expected)
	}
}

func TestLockWorkflowGoalHandler_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"  trimmed goal  ","success_criteria":["  trimmed criterion  "]}`,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if repo.lockGoalText != "trimmed goal" {
		t.Errorf("GoalText = %q, want %q", repo.lockGoalText, "trimmed goal")
	}
	expected := `["trimmed criterion"]`
	if repo.lockGoalCriteria != expected {
		t.Errorf("CriteriaJSON = %q, want %q", repo.lockGoalCriteria, expected)
	}
}

func TestLockWorkflowGoalHandler_MissingWorkflowID(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "",
		Output:     `{"goal_text":"goal","success_criteria":["criterion"]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for missing workflow ID")
	}
	if err.Error() != "lock_workflow_goal: workflow ID is required" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: workflow ID is required'", err.Error())
	}
}

func TestLockWorkflowGoalHandler_EmptyOutput(t *testing.T) {
	t.Parallel()

	repo := &mockWorkflowRepo{}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     "",
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty output")
	}
	if err.Error() != "lock_workflow_goal: output is empty" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: output is empty'", err.Error())
	}
}

func TestLockWorkflowGoalHandler_NilRepo(t *testing.T) {
	t.Parallel()

	handler := LockWorkflowGoalHandler(nil)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"goal","success_criteria":["criterion"]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for nil repo")
	}
	if err.Error() != "lock_workflow_goal: workflow repository is nil" {
		t.Errorf("Error = %q, want 'lock_workflow_goal: workflow repository is nil'", err.Error())
	}
}

func TestLockWorkflowGoalHandler_RepoError(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("goal already locked; use /revise-goal to change")
	repo := &mockWorkflowRepo{lockGoalErr: repoErr}
	handler := LockWorkflowGoalHandler(repo)

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{"goal_text":"goal","success_criteria":["criterion"]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error from repo")
	}
	if !strContains(err.Error(), "goal already locked") {
		t.Errorf("Error = %q, want to contain 'goal already locked'", err.Error())
	}
}

func TestRegisterBuiltinHandlers(t *testing.T) {
	t.Parallel()

	registry := NewHandlerRegistry()
	repo := &mockWorkflowRepo{}

	err := RegisterBuiltinHandlers(registry, repo)
	if err != nil {
		t.Fatalf("RegisterBuiltinHandlers failed: %v", err)
	}

	// Verify lock_workflow_goal is registered.
	if !registry.Has("lock_workflow_goal") {
		t.Error("lock_workflow_goal handler not registered")
	}
}

func TestRegisterBuiltinHandlers_Idempotent(t *testing.T) {
	t.Parallel()

	registry := NewHandlerRegistry()
	repo := &mockWorkflowRepo{}

	err := RegisterBuiltinHandlers(registry, repo)
	if err != nil {
		t.Fatalf("First RegisterBuiltinHandlers failed: %v", err)
	}

	// Second registration should fail due to duplicate.
	err = RegisterBuiltinHandlers(registry, repo)
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}
}

// strContains is a helper to check if s contains substr.
func strContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strContainsAt(s, substr))
}

func strContainsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
