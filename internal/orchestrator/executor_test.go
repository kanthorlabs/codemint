package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// --- Mocks ---

type mockCodingAgent struct {
	executeErr error
}

func (m *mockCodingAgent) ExecuteTask(_ context.Context, _ *domain.Task) error {
	return m.executeErr
}

func (m *mockCodingAgent) Accept(_ context.Context, _ *domain.Task) error { return nil }
func (m *mockCodingAgent) Revert(_ context.Context, _ *domain.Task) error { return nil }

type mockTaskRepo struct {
	updateStatusCalls    []domain.TaskStatus
	updateAssigneeCalled bool
	newAssigneeID        string
	findByIDTask         *domain.Task
	findByIDErr          error
}

func (m *mockTaskRepo) Next(_ context.Context, _ string) (*domain.Task, error) { return nil, nil }
func (m *mockTaskRepo) Claim(_ context.Context, _ string) error                { return nil }
func (m *mockTaskRepo) UpdateStatus(_ context.Context, _ string, _ domain.TaskStatus, _ string) error {
	return nil
}
func (m *mockTaskRepo) FindInterrupted(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepo) FindByID(_ context.Context, _ string) (*domain.Task, error) {
	return m.findByIDTask, m.findByIDErr
}
func (m *mockTaskRepo) UpdateTaskStatus(_ context.Context, _ string, status domain.TaskStatus) error {
	m.updateStatusCalls = append(m.updateStatusCalls, status)
	return nil
}
func (m *mockTaskRepo) UpdateAssignee(_ context.Context, _ string, assigneeID string) error {
	m.updateAssigneeCalled = true
	m.newAssigneeID = assigneeID
	return nil
}
func (m *mockTaskRepo) Create(_ context.Context, _ *domain.Task) error {
	return nil
}
func (m *mockTaskRepo) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepo) ListBySession(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepo) MostRecentActive(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}

type mockAgentRepo struct {
	human *domain.Agent
}

func (m *mockAgentRepo) EnsureSystemAgents(_ context.Context) error { return nil }
func (m *mockAgentRepo) FindByName(_ context.Context, name string) (*domain.Agent, error) {
	if name == "human" && m.human != nil {
		return m.human, nil
	}
	return nil, nil
}

type mockUI struct {
	lastMessage          string
	promptResponse       registry.PromptResponse
	promptRequestCalled  bool
	lastPromptRequest    registry.PromptRequest
	notifyAllCalls       []registry.UIEvent
}

func (m *mockUI) RenderMessage(msg string)     { m.lastMessage = msg }
func (m *mockUI) ClearScreen()                 {}
func (m *mockUI) NotifyAll(ev registry.UIEvent) { m.notifyAllCalls = append(m.notifyAllCalls, ev) }
func (m *mockUI) PromptDecision(_ context.Context, req registry.PromptRequest) registry.PromptResponse {
	m.promptRequestCalled = true
	m.lastPromptRequest = req
	return m.promptResponse
}

// --- Legacy Tests ---

// TestExecuteTask_HappyPath asserts that a successful agent execution returns
// no error and does not trigger the crash-fallback flow.
func TestExecuteTask_HappyPath(t *testing.T) {
	humanID := idgen.MustNew()
	ag := &mockCodingAgent{}
	repo := &mockTaskRepo{}
	agentRepo := &mockAgentRepo{human: &domain.Agent{ID: humanID, Name: "human"}}
	ui := &mockUI{}

	executor := NewExecutor(ag, repo, agentRepo, ui)
	task := &domain.Task{
		ID:      idgen.MustNew(),
		Timeout: domain.DefaultTaskTimeout,
		Status:  domain.TaskStatusProcessing,
	}

	if err := executor.ExecuteTask(context.Background(), task); err != nil {
		t.Fatalf("ExecuteTask returned unexpected error: %v", err)
	}
	if repo.updateAssigneeCalled {
		t.Error("UpdateAssignee must not be called on success")
	}
	if len(repo.updateStatusCalls) != 0 {
		t.Errorf("UpdateTaskStatus must not be called on success, got %d calls", len(repo.updateStatusCalls))
	}
	if ui.lastMessage != "" {
		t.Error("UI must not be notified on success")
	}
}

// TestExecuteTask_AgentCrash asserts that when the agent returns an error:
//  1. The task is reassigned to the human agent.
//  2. Status transitions: Failure → Awaiting.
//  3. The UI renders CrashMessage.
func TestExecuteTask_AgentCrash(t *testing.T) {
	humanID := idgen.MustNew()
	ag := &mockCodingAgent{executeErr: errors.New("agent panic")}
	repo := &mockTaskRepo{}
	agentRepo := &mockAgentRepo{human: &domain.Agent{ID: humanID, Name: "human"}}
	ui := &mockUI{}

	executor := NewExecutor(ag, repo, agentRepo, ui)
	task := &domain.Task{
		ID:      idgen.MustNew(),
		Timeout: domain.DefaultTaskTimeout,
		Status:  domain.TaskStatusProcessing,
	}

	err := executor.ExecuteTask(context.Background(), task)
	if err == nil {
		t.Fatal("ExecuteTask must return an error on agent crash")
	}

	// Reassignment to human.
	if !repo.updateAssigneeCalled {
		t.Error("UpdateAssignee must be called to reassign to human")
	}
	if repo.newAssigneeID != humanID {
		t.Errorf("expected reassignment to human %q, got %q", humanID, repo.newAssigneeID)
	}

	// Status sequence: Failure then Awaiting.
	if len(repo.updateStatusCalls) != 2 {
		t.Fatalf("expected 2 status transitions, got %d: %v", len(repo.updateStatusCalls), repo.updateStatusCalls)
	}
	if repo.updateStatusCalls[0] != domain.TaskStatusFailure {
		t.Errorf("first transition: want Failure(%d), got %d", domain.TaskStatusFailure, repo.updateStatusCalls[0])
	}
	if repo.updateStatusCalls[1] != domain.TaskStatusAwaiting {
		t.Errorf("second transition: want Awaiting(%d), got %d", domain.TaskStatusAwaiting, repo.updateStatusCalls[1])
	}

	// UI notification.
	if ui.lastMessage != crashMessageWithDiscard(task.ID) {
		t.Errorf("UI message: got %q, want %q", ui.lastMessage, crashMessageWithDiscard(task.ID))
	}
}

// --- Task 3.14.1: Execute Dispatch Tests ---

// TestExecutor_Execute_Routes verifies that Execute dispatches to the correct handler
// based on task.Type.
func TestExecutor_Execute_Routes(t *testing.T) {
	tests := []struct {
		name     string
		taskType domain.TaskType
		wantErr  bool
	}{
		{
			name:     "Coding - requires session (returns error without runtime)",
			taskType: domain.TaskTypeCoding,
			wantErr:  true, // No ACP runtime configured
		},
		{
			name:     "Verification - requires command",
			taskType: domain.TaskTypeVerification,
			wantErr:  true, // No input
		},
		{
			name:     "Confirmation - prompts user",
			taskType: domain.TaskTypeConfirmation,
			wantErr:  false,
		},
		{
			name:     "Coordination - skipped",
			taskType: domain.TaskTypeCoordination,
			wantErr:  false,
		},
		{
			name:     "Retrospective - prompts user",
			taskType: domain.TaskTypeRetrospective,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockTaskRepo{}
			ui := &mockUI{
				promptResponse: registry.PromptResponse{
					SelectedOptionID: ConfirmationOptionApprove,
				},
			}
			executor := NewExecutor(nil, repo, nil, ui)
			sess := &ActiveSession{}

			task := &domain.Task{
				ID:      idgen.MustNew(),
				Type:    tt.taskType,
				Timeout: 1000,
			}

			err := executor.Execute(context.Background(), sess, task)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestExecutor_Execute_UnknownType verifies that unknown task types return an error.
func TestExecutor_Execute_UnknownType(t *testing.T) {
	executor := NewExecutor(nil, &mockTaskRepo{}, nil, &mockUI{})
	sess := &ActiveSession{}

	task := &domain.Task{
		ID:   idgen.MustNew(),
		Type: domain.TaskType(99), // Unknown type
	}

	err := executor.Execute(context.Background(), sess, task)
	if err == nil {
		t.Error("Execute() should return error for unknown task type")
	}
	if err.Error() != "executor: unknown task type 99" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- Task 3.14.3: Verification Tests ---

// TestExecutor_Verification_PassesOnZeroExit verifies that verification tasks
// succeed when the command exits with code 0.
func TestExecutor_Verification_PassesOnZeroExit(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	// Create executor with a custom runner that always succeeds
	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
		Runner:   NewLocalRunner(),
	})

	sess := &ActiveSession{
		Project: &domain.Project{WorkingDir: "/tmp"},
	}

	input := VerificationInputSchema{
		Command: "true", // Shell true command - exits 0
		Cwd:     "/tmp",
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeVerification,
		Timeout: 5000,
		Input:   sql.NullString{String: string(inputJSON), Valid: true},
	}

	err := executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("verification task failed unexpectedly: %v", err)
	}

	// Should have Processing → Success
	if len(repo.updateStatusCalls) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d", len(repo.updateStatusCalls))
	}

	// Last status should be Success
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusSuccess {
		t.Errorf("expected final status Success, got %d", lastStatus)
	}
}

// TestExecutor_Verification_FailsOnNonZeroExit verifies that verification tasks
// fail when the command exits with a non-zero code.
func TestExecutor_Verification_FailsOnNonZeroExit(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
		Runner:   NewLocalRunner(),
	})

	sess := &ActiveSession{
		Project: &domain.Project{WorkingDir: "/tmp"},
	}

	input := VerificationInputSchema{
		Command: "false", // Shell false command - exits 1
		Cwd:     "/tmp",
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeVerification,
		Timeout: 5000,
		Input:   sql.NullString{String: string(inputJSON), Valid: true},
	}

	err := executor.Execute(context.Background(), sess, task)
	if err == nil {
		t.Fatal("verification task should have failed")
	}

	// Last status should be Failure
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusFailure {
		t.Errorf("expected final status Failure, got %d", lastStatus)
	}
}

// TestExecutor_Verification_InvalidInput verifies that verification tasks fail
// when input is missing or invalid.
func TestExecutor_Verification_InvalidInput(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
		Runner:   NewLocalRunner(),
	})

	sess := &ActiveSession{}

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeVerification,
		Timeout: 5000,
		// No input - should fail
	}

	err := executor.Execute(context.Background(), sess, task)
	if err == nil {
		t.Fatal("verification task should fail without command")
	}
	if !errors.Is(err, ErrInvalidTaskInput) {
		t.Errorf("expected ErrInvalidTaskInput, got %v", err)
	}
}

// --- Task 3.14.4: Confirmation Tests ---

// TestExecutor_Confirmation_BlocksUntilApproval verifies that confirmation tasks
// wait for user input and succeed on approval.
func TestExecutor_Confirmation_BlocksUntilApproval(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{
		promptResponse: registry.PromptResponse{
			SelectedOptionID: ConfirmationOptionApprove,
		},
	}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{}

	input := ConfirmationInputSchema{
		Prompt: "Please review the changes for Story 1.1",
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:       idgen.MustNew(),
		Type:     domain.TaskTypeConfirmation,
		SeqEpic:  1,
		SeqStory: 1,
		Input:    sql.NullString{String: string(inputJSON), Valid: true},
	}

	start := time.Now()
	err := executor.Execute(context.Background(), sess, task)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("confirmation task failed: %v", err)
	}

	// Should complete quickly (mocked response)
	if elapsed > 100*time.Millisecond {
		t.Errorf("confirmation took too long: %v", elapsed)
	}

	// Prompt should have been called
	if !ui.promptRequestCalled {
		t.Error("PromptDecision should have been called")
	}

	// Check prompt options
	if len(ui.lastPromptRequest.PromptOptions) != 3 {
		t.Errorf("expected 3 prompt options, got %d", len(ui.lastPromptRequest.PromptOptions))
	}

	// Final status should be Success
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusSuccess {
		t.Errorf("expected final status Success, got %d", lastStatus)
	}
}

// TestExecutor_Confirmation_AbortPropagates verifies that abort selection
// fails the task and returns ErrUserAbort.
func TestExecutor_Confirmation_AbortPropagates(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{
		promptResponse: registry.PromptResponse{
			SelectedOptionID: ConfirmationOptionAbort,
		},
	}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{}

	task := &domain.Task{
		ID:   idgen.MustNew(),
		Type: domain.TaskTypeConfirmation,
	}

	err := executor.Execute(context.Background(), sess, task)
	if !errors.Is(err, ErrUserAbort) {
		t.Errorf("expected ErrUserAbort, got %v", err)
	}

	// Final status should be Failure
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusFailure {
		t.Errorf("expected final status Failure, got %d", lastStatus)
	}
}

// TestExecutor_Confirmation_ReviseEmitsEvent verifies that revise selection
// emits EventReviseRequested and keeps task awaiting.
func TestExecutor_Confirmation_ReviseEmitsEvent(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{
		promptResponse: registry.PromptResponse{
			SelectedOptionID: ConfirmationOptionRevise,
		},
	}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{}

	task := &domain.Task{
		ID:   idgen.MustNew(),
		Type: domain.TaskTypeConfirmation,
	}

	err := executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("revise should not error: %v", err)
	}

	// Should have emitted EventReviseRequested
	found := false
	for _, ev := range ui.notifyAllCalls {
		if ev.Type == registry.EventReviseRequested {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected EventReviseRequested to be emitted")
	}
}

// --- Task 3.14.5: Retrospective Tests ---

// TestExecutor_Retrospective_StoresFeedback verifies that feedback is stored
// when the user chooses to share.
func TestExecutor_Retrospective_StoresFeedback(t *testing.T) {
	repo := &mockTaskRepo{}

	// First call returns "share", second call returns the feedback text
	callCount := 0
	ui := &mockUI{}
	ui.promptResponse = registry.PromptResponse{
		SelectedOptionID: RetrospectiveOptionShare,
		SelectedOption:   "Great job on the refactoring!",
	}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})
	// Override to simulate freeform prompt
	executor.ui = &mockUIWithFreeform{
		firstResponse:  registry.PromptResponse{SelectedOptionID: RetrospectiveOptionShare},
		secondResponse: registry.PromptResponse{SelectedOption: "Great job on the refactoring!"},
		callCount:      &callCount,
	}

	sess := &ActiveSession{}

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeRetrospective,
		SeqEpic: 1,
	}

	err := executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("retrospective task failed: %v", err)
	}

	// Final status should be Success
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusSuccess {
		t.Errorf("expected final status Success, got %d", lastStatus)
	}
}

// mockUIWithFreeform simulates a UI that returns different responses
// for the initial prompt and the freeform follow-up.
type mockUIWithFreeform struct {
	firstResponse  registry.PromptResponse
	secondResponse registry.PromptResponse
	callCount      *int
}

func (m *mockUIWithFreeform) RenderMessage(_ string) {}
func (m *mockUIWithFreeform) ClearScreen()           {}
func (m *mockUIWithFreeform) NotifyAll(_ registry.UIEvent) {}
func (m *mockUIWithFreeform) PromptDecision(_ context.Context, _ registry.PromptRequest) registry.PromptResponse {
	*m.callCount++
	if *m.callCount == 1 {
		return m.firstResponse
	}
	return m.secondResponse
}

// TestExecutor_Retrospective_SkipStillSucceeds verifies that skipping feedback
// still marks the task as success.
func TestExecutor_Retrospective_SkipStillSucceeds(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{
		promptResponse: registry.PromptResponse{
			SelectedOptionID: RetrospectiveOptionSkip,
		},
	}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{}

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeRetrospective,
		SeqEpic: 1,
	}

	err := executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("retrospective task failed: %v", err)
	}

	// Final status should be Success
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusSuccess {
		t.Errorf("expected final status Success, got %d", lastStatus)
	}
}

// --- Task 3.14.6: Coordination Tests ---

// TestExecutor_Coordination_DoesNothing verifies that coordination tasks
// do nothing and return immediately.
func TestExecutor_Coordination_DoesNothing(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{}

	task := &domain.Task{
		ID:   idgen.MustNew(),
		Type: domain.TaskTypeCoordination,
	}

	err := executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("coordination task failed: %v", err)
	}

	// No status updates should occur (task is already complete)
	if len(repo.updateStatusCalls) != 0 {
		t.Errorf("coordination should not update status, got %d calls", len(repo.updateStatusCalls))
	}

	// Prompt should not be called
	if ui.promptRequestCalled {
		t.Error("coordination should not prompt user")
	}
}

// --- Coding Task Tests (limited without full ACP setup) ---

// TestExecutor_Coding_ReturnsErrorWithoutRuntime verifies that coding tasks
// return an error when no ACP runtime is available.
func TestExecutor_Coding_ReturnsErrorWithoutRuntime(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{
		// No ACPRuntime set
	}

	input := CodingInputSchema{
		Prompt: "Write a function to add two numbers",
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeCoding,
		Timeout: 1000,
		Input:   sql.NullString{String: string(inputJSON), Valid: true},
	}

	err := executor.Execute(context.Background(), sess, task)
	if err == nil {
		t.Fatal("coding task should fail without ACP runtime")
	}
	if err.Error() != "executor: ACP runtime not available" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestExecutor_Coding_InvalidInput verifies that coding tasks fail
// when input is missing or invalid.
func TestExecutor_Coding_InvalidInput(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	// Create a minimal session with mock runtime
	sess := &ActiveSession{}

	tests := []struct {
		name    string
		input   sql.NullString
		wantErr string
	}{
		{
			name:    "no input",
			input:   sql.NullString{Valid: false},
			wantErr: "missing prompt",
		},
		{
			name:    "empty prompt",
			input:   sql.NullString{String: `{"prompt":""}`, Valid: true},
			wantErr: "missing prompt",
		},
		{
			name:    "invalid JSON",
			input:   sql.NullString{String: `{invalid}`, Valid: true},
			wantErr: "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID:      idgen.MustNew(),
				Type:    domain.TaskTypeCoding,
				Timeout: 1000,
				Input:   tt.input,
			}

			// This will fail before reaching ACP runtime because of nil runtime
			err := executor.Execute(context.Background(), sess, task)
			if err == nil {
				t.Fatal("should have failed")
			}
		})
	}
}
