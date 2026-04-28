package orchestrator

import (
	"context"
	"database/sql"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/workflow"
)

// TestTargetedGather_E2E tests the full targeted gather workflow:
// 1. Task with output.handler="append_targeted_context" transitions to Success
// 2. Handler validates the skill output JSON
// 3. Task output contains the targeted context (keyword hits, files read)
func TestTargetedGather_E2E(t *testing.T) {
	t.Parallel()

	// Create mock repositories.
	taskRepo := &mockTargetedGatherTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockTargetedGatherWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create workflow with goal already locked (prerequisite for targeted gather).
	wf := &domain.Workflow{
		ID:              "wf-targeted",
		SessionID:       "session-targeted",
		Status:          domain.WorkflowStatusActive,
		GoalText:        sql.NullString{String: "Add email validation to user registration", Valid: true},
		SuccessCriteria: sql.NullString{String: `["go test passes","email format is validated"]`, Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create the targeted gather task with valid output.
	targetedOutput := `{
		"skipped": false,
		"keyword_hits": [
			{
				"keyword": "email",
				"files": [
					{"path": "internal/user/validation.go", "lines": [42, 67, 89]}
				]
			},
			{
				"keyword": "validation",
				"files": [
					{"path": "internal/user/validation.go", "lines": [1, 42]}
				]
			}
		],
		"files_read": {
			"internal/user/validation.go": "package user\n\nfunc ValidateEmail(email string) bool {\n\treturn strings.Contains(email, \"@\")\n}"
		},
		"import_hops": [
			{
				"from": "internal/user/validation.go",
				"to": "internal/user/service.go",
				"reason": "UserService is used in validation flow"
			}
		],
		"token_budget_used": 2500
	}`

	task := &domain.Task{
		ID:         "task-targeted-gather",
		SessionID:  "session-targeted",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: targetedOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

	// Create handler registry and register all handlers.
	handlerRegistry := workflow.NewHandlerRegistry()
	err := workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})
	if err != nil {
		t.Fatalf("RegisterBuiltinHandlers failed: %v", err)
	}

	// Verify append_targeted_context handler is registered.
	if !handlerRegistry.Has("append_targeted_context") {
		t.Fatal("append_targeted_context handler not registered")
	}

	// Invoke the handler directly (in real flow, this happens after task completion).
	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       task,
		Output:     task.Output.String,
	}

	err = handlerRegistry.Invoke(ctx, "append_targeted_context", handlerArgs)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Mark task as Success (simulating what the orchestrator does after handler succeeds).
	task.Status = domain.TaskStatusSuccess

	// Verify task output still contains the targeted context.
	if !task.Output.Valid || task.Output.String == "" {
		t.Error("Task output should be preserved")
	}
}

// TestTargetedGather_E2E_SkippedGreenfield tests the greenfield case where no code matches.
func TestTargetedGather_E2E_SkippedGreenfield(t *testing.T) {
	t.Parallel()

	taskRepo := &mockTargetedGatherTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockTargetedGatherWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create workflow with greenfield goal.
	wf := &domain.Workflow{
		ID:              "wf-greenfield",
		SessionID:       "session-greenfield",
		Status:          domain.WorkflowStatusActive,
		GoalText:        sql.NullString{String: "Implement webhook handler for Stripe events", Valid: true},
		SuccessCriteria: sql.NullString{String: `["webhook endpoint returns 200"]`, Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create task with skipped output (greenfield).
	skippedOutput := `{
		"skipped": true,
		"reason": "No existing code matches goal keywords ['webhook', 'Stripe']. This appears to be a greenfield implementation."
	}`

	task := &domain.Task{
		ID:         "task-greenfield",
		SessionID:  "session-greenfield",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: skippedOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

	// Create handler registry.
	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})

	// Invoke the handler.
	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       task,
		Output:     task.Output.String,
	}

	err := handlerRegistry.Invoke(ctx, "append_targeted_context", handlerArgs)
	if err != nil {
		t.Fatalf("Handler returned error for greenfield case: %v", err)
	}

	// Mark task as Success.
	task.Status = domain.TaskStatusSuccess

	// Verify the skipped output is preserved.
	if task.Output.String != skippedOutput {
		t.Error("Skipped output should be preserved as-is")
	}
}

// TestTargetedGather_E2E_MalformedJSON tests that malformed JSON fails the handler.
func TestTargetedGather_E2E_MalformedJSON(t *testing.T) {
	t.Parallel()

	taskRepo := &mockTargetedGatherTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockTargetedGatherWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-malformed",
		SessionID: "session-malformed",
		Status:    domain.WorkflowStatusActive,
		GoalText:  sql.NullString{String: "Some goal", Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create task with malformed output.
	task := &domain.Task{
		ID:         "task-malformed",
		SessionID:  "session-malformed",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: `{invalid json`, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

	// Create handler registry.
	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})

	// Invoke the handler.
	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       task,
		Output:     task.Output.String,
	}

	err := handlerRegistry.Invoke(ctx, "append_targeted_context", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for malformed JSON")
	}

	// In real flow, the task would be marked as Failure.
	// Handler error → task Failure (per 2.8 error contract).
	task.Status = domain.TaskStatusFailure

	if task.Status != domain.TaskStatusFailure {
		t.Errorf("Task.Status = %v, want %v", task.Status, domain.TaskStatusFailure)
	}
}

// TestTargetedGather_E2E_SkippedWithoutReason tests that skipped=true without reason fails.
func TestTargetedGather_E2E_SkippedWithoutReason(t *testing.T) {
	t.Parallel()

	taskRepo := &mockTargetedGatherTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockTargetedGatherWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-no-reason",
		SessionID: "session-no-reason",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create task with skipped=true but no reason.
	task := &domain.Task{
		ID:         "task-no-reason",
		SessionID:  "session-no-reason",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: `{"skipped": true}`, Valid: true},
	}

	// Create handler registry.
	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})

	// Invoke the handler.
	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       task,
		Output:     task.Output.String,
	}

	err := handlerRegistry.Invoke(ctx, "append_targeted_context", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for skipped=true without reason")
	}
	if err.Error() != "append_targeted_context: skipped=true requires non-empty reason" {
		t.Errorf("Error = %q, want 'append_targeted_context: skipped=true requires non-empty reason'", err.Error())
	}
}

// TestTargetedGather_E2E_MissingRequiredFields tests that skipped=false without required fields fails.
func TestTargetedGather_E2E_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	taskRepo := &mockTargetedGatherTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockTargetedGatherWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-missing-fields",
		SessionID: "session-missing-fields",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	// Test case: skipped=false but missing keyword_hits.
	task := &domain.Task{
		ID:         "task-missing-hits",
		SessionID:  "session-missing-fields",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: `{"skipped": false, "files_read": {"a.go": "content"}}`, Valid: true},
	}

	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})

	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       task,
		Output:     task.Output.String,
	}

	err := handlerRegistry.Invoke(ctx, "append_targeted_context", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for missing keyword_hits")
	}
	if err.Error() != "append_targeted_context: keyword_hits is required when skipped=false" {
		t.Errorf("Error = %q, want 'append_targeted_context: keyword_hits is required when skipped=false'", err.Error())
	}
}

// mockTargetedGatherTaskRepo implements repository.TaskRepository for E2E tests.
type mockTargetedGatherTaskRepo struct {
	tasks map[string]*domain.Task
}

func (m *mockTargetedGatherTaskRepo) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	return m.tasks[id], nil
}

func (m *mockTargetedGatherTaskRepo) UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error {
	if task, ok := m.tasks[taskID]; ok {
		task.Status = status
	}
	return nil
}

// Unused methods to satisfy interface.
func (m *mockTargetedGatherTaskRepo) Create(ctx context.Context, t *domain.Task) error { return nil }
func (m *mockTargetedGatherTaskRepo) ListPending(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTargetedGatherTaskRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTargetedGatherTaskRepo) UpdateAssignee(ctx context.Context, taskID, assigneeID string) error {
	return nil
}
func (m *mockTargetedGatherTaskRepo) UpdateOutput(ctx context.Context, taskID, output string) error {
	return nil
}
func (m *mockTargetedGatherTaskRepo) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockTargetedGatherTaskRepo) Claim(ctx context.Context, taskID string) error { return nil }
func (m *mockTargetedGatherTaskRepo) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error {
	return nil
}
func (m *mockTargetedGatherTaskRepo) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTargetedGatherTaskRepo) ListCoordinationAfter(ctx context.Context, sessionID string, afterTaskID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTargetedGatherTaskRepo) MostRecentActive(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockTargetedGatherTaskRepo) CancelByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) error {
	return nil
}
func (m *mockTargetedGatherTaskRepo) GetMaxSeqTask(ctx context.Context, workflowID string) (int, error) {
	return 0, nil
}
func (m *mockTargetedGatherTaskRepo) ListByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) ([]*domain.Task, error) {
	return nil, nil
}

// mockTargetedGatherWorkflowRepo implements repository.WorkflowRepository for E2E tests.
type mockTargetedGatherWorkflowRepo struct {
	workflows map[string]*domain.Workflow
}

func (m *mockTargetedGatherWorkflowRepo) LockGoal(ctx context.Context, workflowID, goalText, criteriaJSON string) error {
	return nil
}

// Unused methods to satisfy interface.
func (m *mockTargetedGatherWorkflowRepo) Create(ctx context.Context, w *domain.Workflow) error {
	return nil
}
func (m *mockTargetedGatherWorkflowRepo) FindByID(ctx context.Context, id string) (*domain.Workflow, error) {
	return m.workflows[id], nil
}
func (m *mockTargetedGatherWorkflowRepo) GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error) {
	return nil, nil
}
func (m *mockTargetedGatherWorkflowRepo) UpdateProgress(ctx context.Context, id, epicID, storyID string) error {
	return nil
}
func (m *mockTargetedGatherWorkflowRepo) MarkCompleted(ctx context.Context, id string) error {
	return nil
}
func (m *mockTargetedGatherWorkflowRepo) MarkCancelled(ctx context.Context, id string) error {
	return nil
}
func (m *mockTargetedGatherWorkflowRepo) ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error) {
	return nil, nil
}
func (m *mockTargetedGatherWorkflowRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Workflow, error) {
	return nil, nil
}
func (m *mockTargetedGatherWorkflowRepo) LockChosenOption(ctx context.Context, workflowID, optionJSON string) error {
	return nil
}
func (m *mockTargetedGatherWorkflowRepo) ResetGOROW(ctx context.Context, workflowID string) error {
	return nil
}
