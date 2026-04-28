package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/workflow"
)

// TestGoalCapture_E2E tests the full goal capture workflow:
// 1. Task with exit_on.command="/lock-goal" and output.handler="lock_workflow_goal" is registered
// 2. User types /lock-goal
// 3. ExitOnDispatcher intercepts the command
// 4. Handler parses task output and calls WorkflowRepo.LockGoal
// 5. Task transitions to Success
// 6. Workflow row has goal_text and success_criteria populated
func TestGoalCapture_E2E(t *testing.T) {
	t.Parallel()

	// Create mock repositories.
	taskRepo := &mockGoalCaptureTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockGoalCaptureWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create workflow with goal fields empty.
	wf := &domain.Workflow{
		ID:        "wf-e2e-goal",
		SessionID: "session-e2e",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create the goal capture task with Processing status and output from the agent.
	goalOutput := `{"goal_text":"Implement /lock-goal command","success_criteria":["go test passes","command appears in /help"]}`
	task := &domain.Task{
		ID:         "task-goal-capture",
		SessionID:  "session-e2e",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: goalOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

	// Create handler registry and register the lock_workflow_goal handler.
	handlerRegistry := workflow.NewHandlerRegistry()
	err := workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil, // Not needed for this test
	})
	if err != nil {
		t.Fatalf("RegisterBuiltinHandlers failed: %v", err)
	}

	// Create advance channel to capture scheduler signals.
	advanceCh := make(chan struct{}, 1)

	// Create the ExitOnDispatcher.
	exitOnDispatcher := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo:        taskRepo,
		WorkflowRepo:    workflowRepo,
		HandlerRegistry: handlerRegistry,
		AdvanceCh:       advanceCh,
	})

	// Register the task for exit_on.
	exitOnDispatcher.Register(
		task.ID,
		wf.ID,
		"/lock-goal",
		"lock_workflow_goal",
		"session-e2e",
	)

	// Verify registration.
	if !exitOnDispatcher.HasRegistration("/lock-goal", "session-e2e") {
		t.Fatal("task not registered for /lock-goal")
	}

	// Simulate user typing /lock-goal.
	ctx := context.Background()
	result := exitOnDispatcher.Dispatch(ctx, "/lock-goal", "session-e2e")

	// Verify dispatch result.
	if !result.Handled {
		t.Error("Dispatch() did not handle /lock-goal")
	}
	if result.TaskID != task.ID {
		t.Errorf("TaskID = %q, want %q", result.TaskID, task.ID)
	}
	if result.Error != nil {
		t.Errorf("Error = %v, want nil", result.Error)
	}

	// Verify task status transitioned to Success.
	updatedTask := taskRepo.tasks[task.ID]
	if updatedTask.Status != domain.TaskStatusSuccess {
		t.Errorf("Task.Status = %v, want %v", updatedTask.Status, domain.TaskStatusSuccess)
	}

	// Verify workflow goal was locked.
	updatedWf := workflowRepo.workflows[wf.ID]
	if !updatedWf.GoalText.Valid {
		t.Error("Workflow.GoalText is not valid, expected goal to be locked")
	}
	if updatedWf.GoalText.String != "Implement /lock-goal command" {
		t.Errorf("GoalText = %q, want %q", updatedWf.GoalText.String, "Implement /lock-goal command")
	}
	if !updatedWf.SuccessCriteria.Valid {
		t.Error("Workflow.SuccessCriteria is not valid, expected criteria to be locked")
	}

	// Parse and verify success criteria.
	var criteria []string
	if err := json.Unmarshal([]byte(updatedWf.SuccessCriteria.String), &criteria); err != nil {
		t.Fatalf("Failed to parse SuccessCriteria: %v", err)
	}
	if len(criteria) != 2 {
		t.Errorf("len(criteria) = %d, want 2", len(criteria))
	}
	if criteria[0] != "go test passes" {
		t.Errorf("criteria[0] = %q, want %q", criteria[0], "go test passes")
	}
	if criteria[1] != "command appears in /help" {
		t.Errorf("criteria[1] = %q, want %q", criteria[1], "command appears in /help")
	}

	// Verify task was deregistered.
	if exitOnDispatcher.HasRegistration("/lock-goal", "session-e2e") {
		t.Error("Task should be deregistered after /lock-goal")
	}

	// Verify advance signal was sent.
	select {
	case <-advanceCh:
		// Good - scheduler was signaled.
	default:
		t.Error("Advance signal was not sent to scheduler")
	}
}

// TestGoalCapture_E2E_HandlerFailure tests that handler errors mark the task as Failure.
func TestGoalCapture_E2E_HandlerFailure(t *testing.T) {
	t.Parallel()

	taskRepo := &mockGoalCaptureTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockGoalCaptureWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create workflow.
	wf := &domain.Workflow{
		ID:        "wf-e2e-fail",
		SessionID: "session-e2e-fail",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create task with INVALID output (empty goal_text).
	invalidOutput := `{"goal_text":"","success_criteria":["criterion"]}`
	task := &domain.Task{
		ID:         "task-goal-invalid",
		SessionID:  "session-e2e-fail",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: invalidOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

	// Create handler registry.
	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})

	// Create the ExitOnDispatcher.
	exitOnDispatcher := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo:        taskRepo,
		WorkflowRepo:    workflowRepo,
		HandlerRegistry: handlerRegistry,
	})

	exitOnDispatcher.Register(task.ID, wf.ID, "/lock-goal", "lock_workflow_goal", "session-e2e-fail")

	// Dispatch /lock-goal.
	result := exitOnDispatcher.Dispatch(context.Background(), "/lock-goal", "session-e2e-fail")

	// Verify dispatch was handled but with error.
	if !result.Handled {
		t.Error("Dispatch() did not handle /lock-goal")
	}
	if result.Error == nil {
		t.Error("Expected handler error for invalid output")
	}

	// Verify task status transitioned to Failure.
	updatedTask := taskRepo.tasks[task.ID]
	if updatedTask.Status != domain.TaskStatusFailure {
		t.Errorf("Task.Status = %v, want %v", updatedTask.Status, domain.TaskStatusFailure)
	}

	// Verify workflow goal was NOT locked.
	updatedWf := workflowRepo.workflows[wf.ID]
	if updatedWf.GoalText.Valid {
		t.Error("Workflow.GoalText should not be set on handler failure")
	}
}

// TestGoalCapture_E2E_AlreadyLocked tests that re-locking a goal fails.
func TestGoalCapture_E2E_AlreadyLocked(t *testing.T) {
	t.Parallel()

	taskRepo := &mockGoalCaptureTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockGoalCaptureWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create workflow with goal ALREADY locked.
	wf := &domain.Workflow{
		ID:              "wf-e2e-locked",
		SessionID:       "session-e2e-locked",
		Status:          domain.WorkflowStatusActive,
		GoalText:        sql.NullString{String: "Existing goal", Valid: true},
		SuccessCriteria: sql.NullString{String: `["existing criterion"]`, Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create task with valid output.
	goalOutput := `{"goal_text":"New goal","success_criteria":["new criterion"]}`
	task := &domain.Task{
		ID:         "task-goal-relocked",
		SessionID:  "session-e2e-locked",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: goalOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

	// Create handler registry.
	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: nil,
	})

	// Create the ExitOnDispatcher.
	exitOnDispatcher := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo:        taskRepo,
		WorkflowRepo:    workflowRepo,
		HandlerRegistry: handlerRegistry,
	})

	exitOnDispatcher.Register(task.ID, wf.ID, "/lock-goal", "lock_workflow_goal", "session-e2e-locked")

	// Dispatch /lock-goal.
	result := exitOnDispatcher.Dispatch(context.Background(), "/lock-goal", "session-e2e-locked")

	// Verify dispatch was handled but with error (goal already locked).
	if !result.Handled {
		t.Error("Dispatch() did not handle /lock-goal")
	}
	if result.Error == nil {
		t.Error("Expected error when goal is already locked")
	}

	// Verify task status transitioned to Failure.
	updatedTask := taskRepo.tasks[task.ID]
	if updatedTask.Status != domain.TaskStatusFailure {
		t.Errorf("Task.Status = %v, want %v", updatedTask.Status, domain.TaskStatusFailure)
	}

	// Verify goal was NOT changed.
	updatedWf := workflowRepo.workflows[wf.ID]
	if updatedWf.GoalText.String != "Existing goal" {
		t.Errorf("GoalText changed to %q, should remain %q", updatedWf.GoalText.String, "Existing goal")
	}
}

// mockGoalCaptureTaskRepo implements repository.TaskRepository for E2E tests.
type mockGoalCaptureTaskRepo struct {
	tasks map[string]*domain.Task
}

func (m *mockGoalCaptureTaskRepo) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	task, ok := m.tasks[id]
	if !ok {
		return nil, nil
	}
	return task, nil
}

func (m *mockGoalCaptureTaskRepo) UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error {
	if task, ok := m.tasks[taskID]; ok {
		task.Status = status
	}
	return nil
}

// Unused methods to satisfy interface.
func (m *mockGoalCaptureTaskRepo) Create(ctx context.Context, t *domain.Task) error { return nil }
func (m *mockGoalCaptureTaskRepo) ListPending(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) UpdateAssignee(ctx context.Context, taskID, assigneeID string) error {
	return nil
}
func (m *mockGoalCaptureTaskRepo) UpdateOutput(ctx context.Context, taskID, output string) error {
	return nil
}
func (m *mockGoalCaptureTaskRepo) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) Claim(ctx context.Context, taskID string) error {
	return nil
}
func (m *mockGoalCaptureTaskRepo) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error {
	return nil
}
func (m *mockGoalCaptureTaskRepo) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) ListCoordinationAfter(ctx context.Context, sessionID string, afterTaskID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) MostRecentActive(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) CancelByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) error {
	return nil
}
func (m *mockGoalCaptureTaskRepo) GetMaxSeqTask(ctx context.Context, workflowID string) (int, error) {
	return 0, nil
}
func (m *mockGoalCaptureTaskRepo) ListByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockGoalCaptureTaskRepo) BulkInsert(ctx context.Context, tasks []*domain.Task) error {
	return nil
}

// mockGoalCaptureWorkflowRepo implements repository.WorkflowRepository for E2E tests.
type mockGoalCaptureWorkflowRepo struct {
	workflows map[string]*domain.Workflow
}

func (m *mockGoalCaptureWorkflowRepo) LockGoal(ctx context.Context, workflowID, goalText, criteriaJSON string) error {
	wf, ok := m.workflows[workflowID]
	if !ok {
		return nil
	}
	// Simulate one-shot lock semantics.
	if wf.GoalText.Valid {
		return sql.ErrNoRows // Simulates "goal already locked"
	}
	wf.GoalText = sql.NullString{String: goalText, Valid: true}
	wf.SuccessCriteria = sql.NullString{String: criteriaJSON, Valid: true}
	return nil
}

// Unused methods to satisfy interface.
func (m *mockGoalCaptureWorkflowRepo) Create(ctx context.Context, w *domain.Workflow) error {
	return nil
}
func (m *mockGoalCaptureWorkflowRepo) FindByID(ctx context.Context, id string) (*domain.Workflow, error) {
	return m.workflows[id], nil
}
func (m *mockGoalCaptureWorkflowRepo) GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error) {
	return nil, nil
}
func (m *mockGoalCaptureWorkflowRepo) UpdateProgress(ctx context.Context, id, epicID, storyID string) error {
	return nil
}
func (m *mockGoalCaptureWorkflowRepo) MarkCompleted(ctx context.Context, id string) error {
	return nil
}
func (m *mockGoalCaptureWorkflowRepo) MarkCancelled(ctx context.Context, id string) error {
	return nil
}
func (m *mockGoalCaptureWorkflowRepo) ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error) {
	return nil, nil
}
func (m *mockGoalCaptureWorkflowRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Workflow, error) {
	return nil, nil
}
func (m *mockGoalCaptureWorkflowRepo) LockChosenOption(ctx context.Context, workflowID, optionJSON string) error {
	return nil
}
func (m *mockGoalCaptureWorkflowRepo) ResetGOROW(ctx context.Context, workflowID string) error {
	return nil
}
