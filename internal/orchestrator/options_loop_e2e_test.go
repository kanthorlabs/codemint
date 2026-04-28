package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/workflow"
)

// mockOptionsTaskRepo implements repository.TaskRepository for options loop tests.
type mockOptionsTaskRepo struct {
	tasks               map[string]*domain.Task
	cancelledStoryIDs   []string
	cancelledWorkflowID string
}

func (m *mockOptionsTaskRepo) Create(_ context.Context, t *domain.Task) error {
	m.tasks[t.ID] = t
	return nil
}

func (m *mockOptionsTaskRepo) Next(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}

func (m *mockOptionsTaskRepo) Claim(_ context.Context, _ string) error {
	return nil
}

func (m *mockOptionsTaskRepo) UpdateStatus(_ context.Context, _ string, _ domain.TaskStatus, _ string) error {
	return nil
}

func (m *mockOptionsTaskRepo) FindInterrupted(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *mockOptionsTaskRepo) FindByID(_ context.Context, id string) (*domain.Task, error) {
	if t, ok := m.tasks[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *mockOptionsTaskRepo) UpdateTaskStatus(_ context.Context, id string, status domain.TaskStatus) error {
	if t, ok := m.tasks[id]; ok {
		t.Status = status
	}
	return nil
}

func (m *mockOptionsTaskRepo) UpdateAssignee(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockOptionsTaskRepo) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *mockOptionsTaskRepo) ListBySession(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *mockOptionsTaskRepo) ListPending(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *mockOptionsTaskRepo) MostRecentActive(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}

func (m *mockOptionsTaskRepo) CancelByWorkflowAndStoryIDs(_ context.Context, workflowID string, storyIDs []string) error {
	m.cancelledWorkflowID = workflowID
	m.cancelledStoryIDs = storyIDs
	// Mark tasks as cancelled.
	for _, task := range m.tasks {
		if task.WorkflowID.String == workflowID {
			for _, storyID := range storyIDs {
				if task.Input.Valid {
					// Check if input contains this story_id.
					if containsOptionsStoryID(task.Input.String, storyID) {
						task.Status = domain.TaskStatusCancelled
					}
				}
			}
		}
	}
	return nil
}

func (m *mockOptionsTaskRepo) GetMaxSeqTask(_ context.Context, workflowID string) (int, error) {
	maxSeq := 0
	for _, t := range m.tasks {
		if t.WorkflowID.String == workflowID && t.SeqTask > maxSeq {
			maxSeq = t.SeqTask
		}
	}
	return maxSeq, nil
}

func (m *mockOptionsTaskRepo) ListByWorkflowAndStoryIDs(_ context.Context, workflowID string, storyIDs []string) ([]*domain.Task, error) {
	var result []*domain.Task
	for _, t := range m.tasks {
		if t.WorkflowID.String == workflowID && t.Input.Valid {
			for _, storyID := range storyIDs {
				if containsOptionsStoryID(t.Input.String, storyID) {
					result = append(result, t)
					break
				}
			}
		}
	}
	return result, nil
}

func (m *mockOptionsTaskRepo) BulkInsert(_ context.Context, _ []*domain.Task) error {
	return nil
}

// containsOptionsStoryID checks if input JSON contains the given story_id.
func containsOptionsStoryID(input string, storyID string) bool {
	// Simple string match for testing purposes.
	return len(input) > 0 && len(storyID) > 0 &&
		(input == `{"story_id":"`+storyID+`"}` ||
			input == `{"story_id": "`+storyID+`"}`)
}

// mockOptionsWorkflowRepo implements repository.WorkflowRepository for options loop tests.
type mockOptionsWorkflowRepo struct {
	workflows        map[string]*domain.Workflow
	resetCalled      bool
	chosenOptionJSON string
}

func (m *mockOptionsWorkflowRepo) Create(_ context.Context, _ *domain.Workflow) error {
	return nil
}

func (m *mockOptionsWorkflowRepo) FindByID(_ context.Context, id string) (*domain.Workflow, error) {
	if wf, ok := m.workflows[id]; ok {
		return wf, nil
	}
	return nil, nil
}

func (m *mockOptionsWorkflowRepo) GetActiveForSession(_ context.Context, _ string) (*domain.Workflow, error) {
	return nil, nil
}

func (m *mockOptionsWorkflowRepo) UpdateProgress(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockOptionsWorkflowRepo) MarkCompleted(_ context.Context, _ string) error {
	return nil
}

func (m *mockOptionsWorkflowRepo) MarkCancelled(_ context.Context, _ string) error {
	return nil
}

func (m *mockOptionsWorkflowRepo) ListByFilePath(_ context.Context, _ string) ([]*domain.Workflow, error) {
	return nil, nil
}

func (m *mockOptionsWorkflowRepo) ListBySession(_ context.Context, _ string) ([]*domain.Workflow, error) {
	return nil, nil
}

func (m *mockOptionsWorkflowRepo) LockGoal(_ context.Context, id, goalText, successCriteria string) error {
	if wf, ok := m.workflows[id]; ok {
		wf.GoalText = sql.NullString{String: goalText, Valid: true}
		wf.SuccessCriteria = sql.NullString{String: successCriteria, Valid: true}
	}
	return nil
}

func (m *mockOptionsWorkflowRepo) LockChosenOption(_ context.Context, id, optionJSON string) error {
	m.chosenOptionJSON = optionJSON
	if wf, ok := m.workflows[id]; ok {
		wf.ChosenOption = sql.NullString{String: optionJSON, Valid: true}
	}
	return nil
}

func (m *mockOptionsWorkflowRepo) ResetGOROW(_ context.Context, id string) error {
	m.resetCalled = true
	if wf, ok := m.workflows[id]; ok {
		wf.GoalText = sql.NullString{}
		wf.SuccessCriteria = sql.NullString{}
		wf.ChosenOption = sql.NullString{}
	}
	return nil
}

// TestOptionsLoop_E2E_PickOption tests the full pick-option flow:
// 1. Task with output.handler="lock_chosen_option" receives /pick-option A
// 2. Handler validates the options output JSON
// 3. Workflow columns are populated with chosen option details
func TestOptionsLoop_E2E_PickOption(t *testing.T) {
	t.Parallel()

	// Create mock repositories.
	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create workflow with goal already set.
	wf := &domain.Workflow{
		ID:              "wf-options",
		SessionID:       "session-options",
		Status:          domain.WorkflowStatusActive,
		GoalText:        sql.NullString{String: "Add email validation to user registration", Valid: true},
		SuccessCriteria: sql.NullString{String: `["go test passes","email format is validated"]`, Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create the propose_options task with valid output.
	optionsOutput := `{
		"options": [
			{
				"id": "A",
				"name": "Regex Validation",
				"summary": "Use regex pattern matching for email validation",
				"files_touched_estimate": 3,
				"pros": ["Simple to implement", "No external dependencies"],
				"cons": ["May not catch all edge cases"],
				"risk_level": "low"
			},
			{
				"id": "B",
				"name": "RFC 5322 Compliance",
				"summary": "Full RFC 5322 compliant validation with library",
				"files_touched_estimate": 5,
				"pros": ["Industry standard", "Handles all edge cases"],
				"cons": ["External dependency", "More complex"],
				"risk_level": "medium"
			}
		]
	}`

	task := &domain.Task{
		ID:         "task-propose-options",
		SessionID:  "session-options",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: optionsOutput, Valid: true},
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

	// Verify lock_chosen_option handler is registered.
	if !handlerRegistry.Has("lock_chosen_option") {
		t.Fatal("lock_chosen_option handler not registered")
	}

	// Invoke the handler with /pick-option A.
	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       task,
		Output:     task.Output.String,
		ExitCmd:    "/pick-option A",
	}

	err = handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err != nil {
		t.Fatalf("lock_chosen_option handler failed: %v", err)
	}

	// Verify workflow ChosenOption was updated with full option JSON.
	if !wf.ChosenOption.Valid {
		t.Fatal("ChosenOption should be set")
	}

	// Parse the stored option to verify it's correct.
	var storedOption struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(wf.ChosenOption.String), &storedOption); err != nil {
		t.Fatalf("Failed to parse stored option: %v", err)
	}
	if storedOption.ID != "A" {
		t.Errorf("Stored option ID = %q, want %q", storedOption.ID, "A")
	}
	if storedOption.Name != "Regex Validation" {
		t.Errorf("Stored option name = %q, want %q", storedOption.Name, "Regex Validation")
	}
}

// TestOptionsLoop_E2E_PickOptionB tests picking the second option.
func TestOptionsLoop_E2E_PickOptionB(t *testing.T) {
	t.Parallel()

	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:              "wf-options-b",
		SessionID:       "session-options-b",
		Status:          domain.WorkflowStatusActive,
		GoalText:        sql.NullString{String: "Goal", Valid: true},
		SuccessCriteria: sql.NullString{String: `[]`, Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	optionsOutput := `{
		"options": [
			{"id": "A", "name": "Option A", "summary": "First option", "files_touched_estimate": 1, "pros": ["pro"], "cons": ["con"], "risk_level": "low"},
			{"id": "B", "name": "Option B", "summary": "Second option", "files_touched_estimate": 2, "pros": ["pro"], "cons": ["con"], "risk_level": "medium"}
		]
	}`

	task := &domain.Task{
		ID:         "task-options-b",
		SessionID:  "session-options-b",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: optionsOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

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
		ExitCmd:    "/pick-option B",
	}

	err := handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err != nil {
		t.Fatalf("lock_chosen_option handler failed: %v", err)
	}

	var storedOption struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(wf.ChosenOption.String), &storedOption); err != nil {
		t.Fatalf("Failed to parse stored option: %v", err)
	}
	if storedOption.ID != "B" {
		t.Errorf("Stored option ID = %q, want %q", storedOption.ID, "B")
	}
	if storedOption.Name != "Option B" {
		t.Errorf("Stored option name = %q, want %q", storedOption.Name, "Option B")
	}
}

// TestOptionsLoop_E2E_Modify tests the /modify flow that resets to goal capture.
func TestOptionsLoop_E2E_Modify(t *testing.T) {
	t.Parallel()

	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	// Create a file registry with the workflow spec.
	fileRegistry := workflow.NewFileRegistry()
	// Load the embedded brainstorming workflow which has the stories we need.
	if err := fileRegistry.LoadAll(nil); err != nil {
		t.Fatalf("Failed to load workflows: %v", err)
	}

	// Get the source path from the loaded workflow.
	brainstormingWf, ok := fileRegistry.Get("brainstorming")
	if !ok {
		t.Fatal("brainstorming workflow not found in registry")
	}

	wf := &domain.Workflow{
		ID:              "wf-modify",
		SessionID:       "session-modify",
		Status:          domain.WorkflowStatusActive,
		GoalText:        sql.NullString{String: "Original goal", Valid: true},
		SuccessCriteria: sql.NullString{String: `["criterion"]`, Valid: true},
		FilePath:        sql.NullString{String: brainstormingWf.SourcePath, Valid: true},
	}
	workflowRepo.workflows[wf.ID] = wf

	// Create tasks for stories that should be cancelled.
	captureGoalTask := &domain.Task{
		ID:         "task-capture-goal",
		ProjectID:  "project-modify",
		SessionID:  "session-modify",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusSuccess,
		Input:      sql.NullString{String: `{"story_id":"capture_goal"}`, Valid: true},
		SeqTask:    1,
	}
	taskRepo.tasks[captureGoalTask.ID] = captureGoalTask

	gatherTargetedTask := &domain.Task{
		ID:         "task-gather-targeted",
		ProjectID:  "project-modify",
		SessionID:  "session-modify",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusSuccess,
		Input:      sql.NullString{String: `{"story_id":"gather_targeted"}`, Valid: true},
		SeqTask:    2,
	}
	taskRepo.tasks[gatherTargetedTask.ID] = gatherTargetedTask

	proposeOptionsTask := &domain.Task{
		ID:         "task-propose-options",
		ProjectID:  "project-modify",
		SessionID:  "session-modify",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Input:      sql.NullString{String: `{"story_id":"propose_options"}`, Valid: true},
		Output:     sql.NullString{String: `{"options": [{"id": "A", "name": "X", "summary": "Y", "files_touched_estimate": 1, "pros": [], "cons": [], "risk_level": "low"}]}`, Valid: true},
		SeqTask:    3,
	}
	taskRepo.tasks[proposeOptionsTask.ID] = proposeOptionsTask

	handlerRegistry := workflow.NewHandlerRegistry()
	_ = workflow.RegisterBuiltinHandlers(handlerRegistry, workflow.RegisterBuiltinHandlersDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		FileRegistry: fileRegistry,
	})

	ctx := context.Background()
	handlerArgs := workflow.HandlerArgs{
		WorkflowID: wf.ID,
		Task:       proposeOptionsTask,
		Output:     proposeOptionsTask.Output.String,
		ExitCmd:    "/modify",
	}

	err := handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err != nil {
		t.Fatalf("lock_chosen_option handler with /modify failed: %v", err)
	}

	// Verify workflow GOROW columns were reset.
	if workflowRepo.resetCalled != true {
		t.Error("ResetGOROW was not called")
	}
	if wf.GoalText.Valid {
		t.Error("GoalText should be cleared after /modify")
	}
	if wf.SuccessCriteria.Valid {
		t.Error("SuccessCriteria should be cleared after /modify")
	}

	// Verify tasks were cancelled.
	if taskRepo.cancelledWorkflowID != wf.ID {
		t.Errorf("CancelByWorkflowAndStoryIDs workflow ID = %q, want %q", taskRepo.cancelledWorkflowID, wf.ID)
	}
	expectedStoryIDs := []string{"capture_goal", "gather_targeted", "propose_options"}
	if len(taskRepo.cancelledStoryIDs) != len(expectedStoryIDs) {
		t.Errorf("CancelByWorkflowAndStoryIDs story IDs count = %d, want %d", len(taskRepo.cancelledStoryIDs), len(expectedStoryIDs))
	}
}

// TestOptionsLoop_E2E_InvalidOptionID tests that an invalid option ID fails.
func TestOptionsLoop_E2E_InvalidOptionID(t *testing.T) {
	t.Parallel()

	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-invalid",
		SessionID: "session-invalid",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	optionsOutput := `{
		"options": [
			{"id": "A", "name": "Only A", "summary": "Only option A exists", "files_touched_estimate": 1, "pros": [], "cons": [], "risk_level": "low"}
		]
	}`

	task := &domain.Task{
		ID:         "task-invalid-option",
		SessionID:  "session-invalid",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: optionsOutput, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

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
		ExitCmd:    "/pick-option C", // C doesn't exist in options
	}

	err := handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for invalid option ID")
	}
	if err.Error() != "lock_chosen_option: option \"C\" not found in proposed options" {
		t.Errorf("Error = %q, want 'lock_chosen_option: option \"C\" not found in proposed options'", err.Error())
	}
}

// TestOptionsLoop_E2E_MissingOptions tests that missing options array fails.
func TestOptionsLoop_E2E_MissingOptions(t *testing.T) {
	t.Parallel()

	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-no-options",
		SessionID: "session-no-options",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	task := &domain.Task{
		ID:         "task-no-options",
		SessionID:  "session-no-options",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: `{}`, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

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
		ExitCmd:    "/pick-option A",
	}

	err := handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for missing options")
	}
	if err.Error() != "lock_chosen_option: no options in output" {
		t.Errorf("Error = %q, want 'lock_chosen_option: no options in output'", err.Error())
	}
}

// TestOptionsLoop_E2E_MissingExitCommand tests that missing exit command fails.
func TestOptionsLoop_E2E_MissingExitCommand(t *testing.T) {
	t.Parallel()

	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-no-cmd",
		SessionID: "session-no-cmd",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	task := &domain.Task{
		ID:         "task-no-cmd",
		SessionID:  "session-no-cmd",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: `{"options": []}`, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

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
		// ExitCmd is empty
	}

	err := handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for missing exit command")
	}
	// Should get error about expected format since empty string doesn't match "/pick-option <id>"
	if !containsOptionsStr(err.Error(), "expected '/pick-option <id>'") {
		t.Errorf("Error = %q, want error mentioning expected format", err.Error())
	}
}

// TestOptionsLoop_E2E_InvalidJSON tests that invalid JSON output fails.
func TestOptionsLoop_E2E_InvalidJSON(t *testing.T) {
	t.Parallel()

	taskRepo := &mockOptionsTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
	workflowRepo := &mockOptionsWorkflowRepo{
		workflows: make(map[string]*domain.Workflow),
	}

	wf := &domain.Workflow{
		ID:        "wf-bad-json",
		SessionID: "session-bad-json",
		Status:    domain.WorkflowStatusActive,
	}
	workflowRepo.workflows[wf.ID] = wf

	task := &domain.Task{
		ID:         "task-bad-json",
		SessionID:  "session-bad-json",
		WorkflowID: sql.NullString{String: wf.ID, Valid: true},
		Status:     domain.TaskStatusProcessing,
		Output:     sql.NullString{String: `{invalid json}`, Valid: true},
	}
	taskRepo.tasks[task.ID] = task

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
		ExitCmd:    "/pick-option A",
	}

	err := handlerRegistry.Invoke(ctx, "lock_chosen_option", handlerArgs)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	// Error should mention JSON parsing.
	if !containsOptionsStr(err.Error(), "invalid JSON") {
		t.Errorf("Error = %q, want error mentioning 'invalid JSON'", err.Error())
	}
}

// containsOptionsStr checks if substr is in s (helper to avoid conflict with runtime_test.go).
func containsOptionsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
