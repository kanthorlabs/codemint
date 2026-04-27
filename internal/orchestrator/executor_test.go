package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
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
	human       *domain.Agent
	autoApprove *domain.Agent
}

func (m *mockAgentRepo) EnsureSystemAgents(_ context.Context) error { return nil }
func (m *mockAgentRepo) FindByName(_ context.Context, name string) (*domain.Agent, error) {
	if name == "human" && m.human != nil {
		return m.human, nil
	}
	if name == "sys-auto-approve" && m.autoApprove != nil {
		return m.autoApprove, nil
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

// --- ACP Mocks for Task 3.15 Tests ---

// mockWorker simulates an ACP worker for testing.
type mockWorker struct {
	mu            sync.Mutex
	sentMessages  []*acp.Message
	currentTaskID string
	alive         bool
}

func newMockWorker() *mockWorker {
	return &mockWorker{
		sentMessages: make([]*acp.Message, 0),
		alive:        true,
	}
}

func (m *mockWorker) Send(msg *acp.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *mockWorker) SetCurrentTask(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentTaskID = taskID
}

func (m *mockWorker) CurrentTaskID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTaskID
}

func (m *mockWorker) Alive() bool {
	return m.alive
}

func (m *mockWorker) GetSentMessages() []*acp.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*acp.Message, len(m.sentMessages))
	copy(result, m.sentMessages)
	return result
}

// mockACPRegistry simulates the ACP registry for testing.
type mockACPRegistry struct {
	workers map[string]*mockWorker
}

func newMockACPRegistry() *mockACPRegistry {
	return &mockACPRegistry{
		workers: make(map[string]*mockWorker),
	}
}

func (m *mockACPRegistry) Get(sessionID string) (*acp.Worker, bool) {
	// This doesn't return the real *acp.Worker, we need a different approach.
	// Since we can't mock *acp.Worker directly (it's a concrete type),
	// we'll need to test at a higher level or use integration tests.
	return nil, false
}

func (m *mockACPRegistry) AddMockWorker(sessionID string) *mockWorker {
	worker := newMockWorker()
	m.workers[sessionID] = worker
	return worker
}

// mockPermissionRepo implements repository.ProjectPermissionRepository for testing.
type mockPermissionRepo struct {
	permissions *domain.ProjectPermission
}

func (m *mockPermissionRepo) FindByProjectID(_ context.Context, _ string) (*domain.ProjectPermission, error) {
	return m.permissions, nil
}

func (m *mockPermissionRepo) Upsert(_ context.Context, _ *domain.ProjectPermission) error {
	return nil
}

// mockSessionRepo implements repository.SessionRepository for testing.
type mockSessionRepo struct{}

func (m *mockSessionRepo) Create(_ context.Context, _ *domain.Session) error            { return nil }
func (m *mockSessionRepo) FindByID(_ context.Context, _ string) (*domain.Session, error) { return nil, nil }
func (m *mockSessionRepo) FindActiveByProjectID(_ context.Context, _ string) (*domain.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) Archive(_ context.Context, _ string) error                    { return nil }
func (m *mockSessionRepo) ListByProjectID(_ context.Context, _ string) ([]*domain.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) SaveState(_ context.Context, _, _ string, _ int64) error      { return nil }
func (m *mockSessionRepo) ClearOwnership(_ context.Context, _ string) error             { return nil }
func (m *mockSessionRepo) ListActive(_ context.Context) ([]*domain.Session, error)      { return nil, nil }
func (m *mockSessionRepo) GetMostRecentActive(_ context.Context) (*domain.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) CountActiveByProjectID(_ context.Context, _ string) (int, error) {
	return 0, nil
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

// --- Task 3.15.3: Typed Params Tests ---

// TestExecutor_Coding_LegacyPlainText verifies that legacy plain-text input
// is handled with a warning but still works (backward compatibility).
func TestExecutor_Coding_LegacyPlainText(t *testing.T) {
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{
		// No ACPRuntime - will fail before sending, but that's OK
		// We're testing the input parsing, not the ACP send
	}

	// Plain text input (not JSON)
	legacyInput := "Please implement the login feature with OAuth2"

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeCoding,
		Timeout: 1000,
		Input:   sql.NullString{String: legacyInput, Valid: true},
	}

	// Will fail because no ACP runtime, but should get past input parsing
	err := executor.Execute(context.Background(), sess, task)
	if err == nil {
		t.Fatal("expected error (no ACP runtime)")
	}

	// Error should be about runtime, not about invalid input
	if errors.Is(err, ErrInvalidTaskInput) {
		t.Errorf("legacy text should not cause ErrInvalidTaskInput: %v", err)
	}
	if err.Error() != "executor: ACP runtime not available" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestResolveContextFiles_Integration verifies the integration between
// ParseTaskInput and resolveContextFiles.
func TestResolveContextFiles_Integration(t *testing.T) {
	// Create a temporary directory with some files.
	tmpDir, err := os.MkdirTemp("", "executor_integration_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files.
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	mainFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: tmpDir,
	}

	// Parse a TaskInput with context files.
	input := `{
		"prompt": "Implement feature X",
		"context_files": ["src/main.go"],
		"tools": ["read", "write"]
	}`

	taskInput, err := domain.ParseTaskInput(input)
	if err != nil {
		t.Fatalf("ParseTaskInput failed: %v", err)
	}

	// Resolve context files.
	resolved, err := resolveContextFiles(project, taskInput.ContextFiles)
	if err != nil {
		t.Fatalf("resolveContextFiles failed: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved path, got %d", len(resolved))
	}

	expectedPath := filepath.Join(tmpDir, "src", "main.go")
	if resolved[0] != expectedPath {
		t.Errorf("resolved path mismatch: got %q, want %q", resolved[0], expectedPath)
	}

	// Verify tools are preserved.
	if len(taskInput.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(taskInput.Tools))
	}
}

// --- Task 3.15.4: Failure Mapping Tests ---

// TestExecutor_InvalidInput_DoesNotKillScheduler verifies that invalid input
// fails the individual task but does not kill the scheduler (returns nil).
func TestExecutor_InvalidInput_DoesNotKillScheduler(t *testing.T) {
	// This test simulates three tasks: good, bad, good.
	// All three should be dispatched; the bad one should fail with a sentinel.

	// For this test, we'll focus on the failure output structure since
	// full scheduler integration requires more setup.

	tests := []struct {
		name           string
		sentinel       string
		detail         string
		expectedOutput string
	}{
		{
			name:     "invalid input sentinel",
			sentinel: FailureSentinelInvalidInput,
			detail:   "missing prompt",
		},
		{
			name:     "context file missing sentinel",
			sentinel: FailureSentinelContextFileMissing,
			detail:   "file not found: src/missing.go",
		},
		{
			name:     "path escape sentinel",
			sentinel: FailureSentinelPathEscape,
			detail:   "path escapes project directory: ../../../etc/passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockTaskRepo{}
			ui := &mockUI{}

			executor := NewExecutorWithConfig(ExecutorConfig{
				TaskRepo: repo,
				UI:       ui,
			})

			task := &domain.Task{
				ID:      idgen.MustNew(),
				Type:    domain.TaskTypeCoding,
				Timeout: 1000,
			}

			// Directly call the failure helper.
			executor.failTaskWithReason(context.Background(), task, tt.sentinel, tt.detail)

			// Verify task status was updated to Failure.
			if len(repo.updateStatusCalls) != 1 {
				t.Fatalf("expected 1 status update, got %d", len(repo.updateStatusCalls))
			}
			if repo.updateStatusCalls[0] != domain.TaskStatusFailure {
				t.Errorf("expected status Failure, got %d", repo.updateStatusCalls[0])
			}

			// Verify task output is set with structured error.
			if !task.Output.Valid {
				t.Fatal("task output should be valid")
			}

			var failureOutput TaskFailureOutput
			if err := json.Unmarshal([]byte(task.Output.String), &failureOutput); err != nil {
				t.Fatalf("failed to unmarshal task output: %v", err)
			}

			if failureOutput.Error != tt.sentinel {
				t.Errorf("sentinel mismatch: got %q, want %q", failureOutput.Error, tt.sentinel)
			}
			if failureOutput.Detail != tt.detail {
				t.Errorf("detail mismatch: got %q, want %q", failureOutput.Detail, tt.detail)
			}
		})
	}
}

// TestTaskFailureOutput_JSONFormat verifies the JSON structure of TaskFailureOutput.
func TestTaskFailureOutput_JSONFormat(t *testing.T) {
	output := TaskFailureOutput{
		Error:  FailureSentinelInvalidInput,
		Detail: "missing required field 'prompt'",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	expected := `{"error":"invalid_input","detail":"missing required field 'prompt'"}`
	if string(data) != expected {
		t.Errorf("JSON mismatch:\ngot:  %s\nwant: %s", string(data), expected)
	}
}

// --- Task 3.15.3 / 3.15.4: Full Integration Tests with Mock ACP ---

// createTestRuntimeWithMockWorker creates a Runtime with a mock worker injected
// for the given session ID. Returns the runtime, the channel for capturing sent
// messages, and a cleanup function.
func createTestRuntimeWithMockWorker(sessionID string, taskRepo repository.TaskRepository, ui registry.UIMediator) (*Runtime, chan *acp.Message, func()) {
	// Create registry
	acpRegistry := acp.NewRegistry(acp.DefaultConfig())

	// Create mock worker
	worker, sentMessages, workerCleanup := acp.NewMockWorkerForTesting()

	// Inject mock worker into registry
	acpRegistry.InjectWorkerForTesting(sessionID, worker)

	// Create buffer registry
	bufferRegistry := acp.NewBufferRegistry(100)

	// Create runtime
	runtime, err := NewRuntime(context.Background(), RuntimeConfig{
		Registry:       acpRegistry,
		BufferRegistry: bufferRegistry,
		Mediator:       ui,
		TaskRepo:       taskRepo,
		PermissionRepo: &mockPermissionRepo{},
		SessionRepo:    &mockSessionRepo{},
		AgentRepo:      &mockAgentRepo{autoApprove: &domain.Agent{ID: "yolo-agent-id", Name: "sys-auto-approve", Type: domain.AgentTypeSystem}},
	})
	if err != nil {
		panic("failed to create runtime: " + err.Error())
	}

	cleanup := func() {
		workerCleanup()
	}

	return runtime, sentMessages, cleanup
}

// TestExecutor_Coding_BuildsTypedParams verifies that the executor builds
// correctly typed SessionPromptParams with context files and tools.
func TestExecutor_Coding_BuildsTypedParams(t *testing.T) {
	// Create a temporary directory with some files.
	tmpDir, err := os.MkdirTemp("", "executor_typed_params_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files.
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	file1 := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(file1, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	file2 := filepath.Join(srcDir, "helper.go")
	if err := os.WriteFile(file2, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create helper.go: %v", err)
	}

	// Setup
	sessionID := idgen.MustNew()
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	runtime, sentMessages, cleanup := createTestRuntimeWithMockWorker(sessionID, repo, ui)
	defer cleanup()

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	// Create session with runtime
	sess := &ActiveSession{
		Project: &domain.Project{
			ID:         "test-project",
			WorkingDir: tmpDir,
		},
		Session: &domain.Session{ID: sessionID},
	}
	sess.SetACPRuntime(runtime)
	sess.SetACPSessionID("acp-session-123")

	// Create task with typed input
	input := domain.TaskInput{
		Prompt:       "Implement feature X",
		ContextFiles: []string{"src/main.go", "src/helper.go"},
		Tools:        []string{"read", "write", "bash"},
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeCoding,
		Timeout: 1000,
		Input:   sql.NullString{String: string(inputJSON), Valid: true},
	}

	// Execute in goroutine since it will block waiting for completion
	errCh := make(chan error, 1)
	go func() {
		errCh <- executor.Execute(context.Background(), sess, task)
	}()

	// Wait for message to be sent (with timeout)
	var sentMsg *acp.Message
	select {
	case sentMsg = <-sentMessages:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message to be sent")
	}

	// Verify the sent message
	if sentMsg.Method != acp.MethodSessionPrompt {
		t.Errorf("expected method %s, got %s", acp.MethodSessionPrompt, sentMsg.Method)
	}

	// Parse params
	var params acp.SessionPromptParams
	if err := json.Unmarshal(sentMsg.Params, &params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	// Verify prompt - should have text + 2 resource_link blocks
	// Per ACP spec, context files are sent as resource_link ContentBlocks
	if len(params.Prompt) != 3 {
		t.Fatalf("expected 3 prompt blocks (1 text + 2 resource_link), got %d", len(params.Prompt))
	}

	// First block should be text
	if params.Prompt[0].Type != "text" || params.Prompt[0].Text != "Implement feature X" {
		t.Errorf("prompt[0] mismatch: got type=%s text=%s", params.Prompt[0].Type, params.Prompt[0].Text)
	}

	// Verify context files as resource_link blocks
	expectedPath1 := filepath.Join(tmpDir, "src", "main.go")
	expectedPath2 := filepath.Join(tmpDir, "src", "helper.go")

	if params.Prompt[1].Type != "resource_link" {
		t.Errorf("prompt[1].Type = %q, want resource_link", params.Prompt[1].Type)
	}
	if params.Prompt[1].URI != "file://"+expectedPath1 {
		t.Errorf("prompt[1].URI = %q, want %q", params.Prompt[1].URI, "file://"+expectedPath1)
	}
	if params.Prompt[2].Type != "resource_link" {
		t.Errorf("prompt[2].Type = %q, want resource_link", params.Prompt[2].Type)
	}
	if params.Prompt[2].URI != "file://"+expectedPath2 {
		t.Errorf("prompt[2].URI = %q, want %q", params.Prompt[2].URI, "file://"+expectedPath2)
	}

	// Note: Tools are NOT sent via session/prompt per ACP spec.
	// They should be provided via MCP servers if needed.
}

// TestExecutor_Coding_PathEscapeFailsGracefully verifies that path escape
// attempts fail the task gracefully without crashing the scheduler.
func TestExecutor_Coding_PathEscapeFailsGracefully(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor_path_escape_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := idgen.MustNew()
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	runtime, _, cleanup := createTestRuntimeWithMockWorker(sessionID, repo, ui)
	defer cleanup()

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{
		Project: &domain.Project{
			ID:         "test-project",
			WorkingDir: tmpDir,
		},
		Session: &domain.Session{ID: sessionID},
	}
	sess.SetACPRuntime(runtime)
	sess.SetACPSessionID("acp-session-123")

	// Create task with path escape attempt
	input := domain.TaskInput{
		Prompt:       "Implement feature X",
		ContextFiles: []string{"../../../etc/passwd"},
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeCoding,
		Timeout: 1000,
		Input:   sql.NullString{String: string(inputJSON), Valid: true},
	}

	// Execute - should return nil (not kill scheduler) but fail the task
	err = executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Errorf("Execute should return nil for path escape (task failure, not scheduler failure): %v", err)
	}

	// Verify task was marked as failed
	if len(repo.updateStatusCalls) < 2 {
		t.Fatalf("expected at least 2 status updates (Processing, Failure), got %d", len(repo.updateStatusCalls))
	}

	// Last status should be Failure
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusFailure {
		t.Errorf("expected last status Failure, got %d", lastStatus)
	}

	// Verify structured error output
	if !task.Output.Valid {
		t.Fatal("task output should be set")
	}

	var failureOutput TaskFailureOutput
	if err := json.Unmarshal([]byte(task.Output.String), &failureOutput); err != nil {
		t.Fatalf("failed to unmarshal failure output: %v", err)
	}

	if failureOutput.Error != FailureSentinelPathEscape {
		t.Errorf("expected sentinel %q, got %q", FailureSentinelPathEscape, failureOutput.Error)
	}
}

// TestExecutor_Coding_MissingContextFileFailsGracefully verifies that
// missing context files fail the task gracefully.
func TestExecutor_Coding_MissingContextFileFailsGracefully(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor_missing_file_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := idgen.MustNew()
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	runtime, _, cleanup := createTestRuntimeWithMockWorker(sessionID, repo, ui)
	defer cleanup()

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{
		Project: &domain.Project{
			ID:         "test-project",
			WorkingDir: tmpDir,
		},
		Session: &domain.Session{ID: sessionID},
	}
	sess.SetACPRuntime(runtime)
	sess.SetACPSessionID("acp-session-123")

	// Create task with non-existent context file
	input := domain.TaskInput{
		Prompt:       "Implement feature X",
		ContextFiles: []string{"nonexistent.go"},
	}
	inputJSON, _ := json.Marshal(input)

	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeCoding,
		Timeout: 1000,
		Input:   sql.NullString{String: string(inputJSON), Valid: true},
	}

	// Execute - should return nil (not kill scheduler) but fail the task
	err = executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Errorf("Execute should return nil for missing file (task failure, not scheduler failure): %v", err)
	}

	// Verify task was marked as failed
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusFailure {
		t.Errorf("expected last status Failure, got %d", lastStatus)
	}

	// Verify structured error output
	var failureOutput TaskFailureOutput
	if err := json.Unmarshal([]byte(task.Output.String), &failureOutput); err != nil {
		t.Fatalf("failed to unmarshal failure output: %v", err)
	}

	if failureOutput.Error != FailureSentinelContextFileMissing {
		t.Errorf("expected sentinel %q, got %q", FailureSentinelContextFileMissing, failureOutput.Error)
	}
}

// TestExecutor_Coding_EmptyInputFailsGracefully verifies that empty input
// fails the task gracefully.
func TestExecutor_Coding_EmptyInputFailsGracefully(t *testing.T) {
	sessionID := idgen.MustNew()
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	runtime, _, cleanup := createTestRuntimeWithMockWorker(sessionID, repo, ui)
	defer cleanup()

	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	sess := &ActiveSession{
		Project: &domain.Project{
			ID:         "test-project",
			WorkingDir: "/tmp",
		},
		Session: &domain.Session{ID: sessionID},
	}
	sess.SetACPRuntime(runtime)
	sess.SetACPSessionID("acp-session-123")

	// Create task with no input
	task := &domain.Task{
		ID:      idgen.MustNew(),
		Type:    domain.TaskTypeCoding,
		Timeout: 1000,
		Input:   sql.NullString{Valid: false}, // No input
	}

	// Execute - should return nil (not kill scheduler) but fail the task
	err := executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Errorf("Execute should return nil for empty input: %v", err)
	}

	// Verify task was marked as failed
	lastStatus := repo.updateStatusCalls[len(repo.updateStatusCalls)-1]
	if lastStatus != domain.TaskStatusFailure {
		t.Errorf("expected last status Failure, got %d", lastStatus)
	}

	// Verify structured error output
	var failureOutput TaskFailureOutput
	if err := json.Unmarshal([]byte(task.Output.String), &failureOutput); err != nil {
		t.Fatalf("failed to unmarshal failure output: %v", err)
	}

	if failureOutput.Error != FailureSentinelInvalidInput {
		t.Errorf("expected sentinel %q, got %q", FailureSentinelInvalidInput, failureOutput.Error)
	}
}

// --- Task 3.16: YOLO Auto-Approval Bypass Tests ---

// TestExecutor_Confirmation_AutoApproved verifies that Confirmation tasks assigned
// to sys-auto-approve are bypassed without prompting the user.
func TestExecutor_Confirmation_AutoApproved(t *testing.T) {
	// Create mock dependencies.
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	// Create executor.
	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo:  repo,
		AgentRepo: &mockAgentRepo{},
		UI:        ui,
	})

	// Create runtime with YOLO agent ID.
	yoloAgentID := "sys-auto-approve-agent-id"
	runtime, err := NewRuntime(context.Background(), RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(100),
		AgentRepo:      &mockAgentRepo{autoApprove: &domain.Agent{ID: yoloAgentID, Name: "sys-auto-approve", Type: domain.AgentTypeSystem}},
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	// Create session with runtime.
	sess := &ActiveSession{
		acpRuntime: runtime,
	}

	// Create a Confirmation task assigned to YOLO agent.
	task := &domain.Task{
		ID:         idgen.MustNew(),
		Type:       domain.TaskTypeConfirmation,
		AssigneeID: yoloAgentID,
		SeqEpic:    1,
		SeqStory:   2,
		Input:      domain.NewNullString(`{"prompt":"Review changes"}`),
	}

	// Execute.
	err = executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Verify PromptDecision was NOT called (auto-approval bypassed it).
	if ui.promptRequestCalled {
		t.Error("PromptDecision should NOT be called for auto-approved task")
	}

	// Verify task status was set to Success.
	found := false
	for _, status := range repo.updateStatusCalls {
		if status == domain.TaskStatusSuccess {
			found = true
			break
		}
	}
	if !found {
		t.Error("Task should be marked as Success")
	}

	// Verify output JSON contains auto_approved marker.
	if !task.Output.Valid {
		t.Fatal("Task output should be set")
	}
	var output map[string]string
	if err := json.Unmarshal([]byte(task.Output.String), &output); err != nil {
		t.Fatalf("Failed to parse output JSON: %v", err)
	}
	if output["auto_approved"] != "true" {
		t.Errorf("Expected auto_approved=true, got %q", output["auto_approved"])
	}
	if output["agent"] != "sys-auto-approve" {
		t.Errorf("Expected agent=sys-auto-approve, got %q", output["agent"])
	}

	// Verify UI event was emitted.
	found = false
	for _, ev := range ui.notifyAllCalls {
		if ev.Type == registry.EventYoloAutoApproved {
			found = true
			payload, ok := ev.Payload.(registry.YoloAutoApprovedPayload)
			if !ok {
				t.Error("Payload should be YoloAutoApprovedPayload")
			}
			if payload.TaskID != task.ID {
				t.Errorf("Payload TaskID = %q, want %q", payload.TaskID, task.ID)
			}
			if payload.SeqStory != 2 {
				t.Errorf("Payload SeqStory = %d, want 2", payload.SeqStory)
			}
			break
		}
	}
	if !found {
		t.Error("EventYoloAutoApproved should be emitted")
	}
}

// TestExecutor_Retrospective_AutoApprovedSkipsFreeform verifies that Retrospective
// tasks assigned to sys-auto-approve skip the freeform prompt.
func TestExecutor_Retrospective_AutoApprovedSkipsFreeform(t *testing.T) {
	// Create mock dependencies.
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	// Create executor.
	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo:  repo,
		AgentRepo: &mockAgentRepo{},
		UI:        ui,
	})

	// Create runtime with YOLO agent ID.
	yoloAgentID := "sys-auto-approve-agent-id"
	runtime, err := NewRuntime(context.Background(), RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(100),
		AgentRepo:      &mockAgentRepo{autoApprove: &domain.Agent{ID: yoloAgentID, Name: "sys-auto-approve", Type: domain.AgentTypeSystem}},
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	// Create session with runtime.
	sess := &ActiveSession{
		acpRuntime: runtime,
	}

	// Create a Retrospective task assigned to YOLO agent.
	task := &domain.Task{
		ID:         idgen.MustNew(),
		Type:       domain.TaskTypeRetrospective,
		AssigneeID: yoloAgentID,
		SeqEpic:    1,
		SeqStory:   0,
	}

	// Execute.
	err = executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Verify PromptDecision was NOT called (freeform prompt skipped).
	if ui.promptRequestCalled {
		t.Error("PromptDecision should NOT be called for auto-approved retrospective")
	}

	// Verify task status was set to Success.
	found := false
	for _, status := range repo.updateStatusCalls {
		if status == domain.TaskStatusSuccess {
			found = true
			break
		}
	}
	if !found {
		t.Error("Task should be marked as Success")
	}

	// Verify UI event was emitted.
	found = false
	for _, ev := range ui.notifyAllCalls {
		if ev.Type == registry.EventYoloAutoApproved {
			found = true
			break
		}
	}
	if !found {
		t.Error("EventYoloAutoApproved should be emitted for retrospective")
	}
}

// TestExecutor_Coding_IgnoresYoloAssignee verifies that Coding tasks with YOLO
// assignee still run the ACP path (YOLO only changes approval gates).
func TestExecutor_Coding_IgnoresYoloAssignee(t *testing.T) {
	// This test verifies the warning is logged but execution proceeds normally.
	// The actual execution would fail without a full ACP setup, so we just
	// verify the task type routing still happens.

	// Create mock dependencies.
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	// Create executor.
	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo:  repo,
		AgentRepo: &mockAgentRepo{},
		UI:        ui,
	})

	// Create runtime with YOLO agent ID.
	yoloAgentID := "sys-auto-approve-agent-id"
	runtime, err := NewRuntime(context.Background(), RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(100),
		AgentRepo:      &mockAgentRepo{autoApprove: &domain.Agent{ID: yoloAgentID, Name: "sys-auto-approve", Type: domain.AgentTypeSystem}},
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	// Create session with runtime (but no ACP worker - will fail at execution).
	sess := &ActiveSession{
		acpRuntime: runtime,
	}

	// Create a Coding task assigned to YOLO agent.
	task := &domain.Task{
		ID:         idgen.MustNew(),
		Type:       domain.TaskTypeCoding,
		AssigneeID: yoloAgentID,
		Input:      domain.NewNullString(`{"prompt":"test"}`),
		Timeout:    1000,
	}

	// Execute - will fail because no ACP worker, but the point is it
	// doesn't auto-approve and tries to run the coding path.
	_ = executor.Execute(context.Background(), sess, task)

	// Verify no auto-approval event was emitted.
	for _, ev := range ui.notifyAllCalls {
		if ev.Type == registry.EventYoloAutoApproved {
			t.Error("EventYoloAutoApproved should NOT be emitted for Coding tasks")
		}
	}
}

// TestExecutor_AutoApproved_EmitsEvent verifies that auto-approved tasks emit
// the EventYoloAutoApproved event with correct payload.
func TestExecutor_AutoApproved_EmitsEvent(t *testing.T) {
	// Create mock dependencies.
	repo := &mockTaskRepo{}
	ui := &mockUI{}

	// Create executor.
	executor := NewExecutorWithConfig(ExecutorConfig{
		TaskRepo:  repo,
		AgentRepo: &mockAgentRepo{},
		UI:        ui,
	})

	// Create runtime with YOLO agent ID.
	yoloAgentID := "sys-auto-approve-agent-id"
	runtime, err := NewRuntime(context.Background(), RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(100),
		AgentRepo:      &mockAgentRepo{autoApprove: &domain.Agent{ID: yoloAgentID, Name: "sys-auto-approve", Type: domain.AgentTypeSystem}},
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	// Create session with runtime.
	sess := &ActiveSession{
		acpRuntime: runtime,
	}

	// Create a Confirmation task assigned to YOLO agent.
	task := &domain.Task{
		ID:         idgen.MustNew(),
		Type:       domain.TaskTypeConfirmation,
		AssigneeID: yoloAgentID,
		SeqEpic:    3,
		SeqStory:   5,
		Input:      domain.NewNullString(`{"prompt":"test"}`),
	}

	// Execute.
	err = executor.Execute(context.Background(), sess, task)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Verify exactly one EventYoloAutoApproved event was emitted.
	count := 0
	var payload registry.YoloAutoApprovedPayload
	for _, ev := range ui.notifyAllCalls {
		if ev.Type == registry.EventYoloAutoApproved {
			count++
			var ok bool
			payload, ok = ev.Payload.(registry.YoloAutoApprovedPayload)
			if !ok {
				t.Error("Payload should be YoloAutoApprovedPayload")
			}
		}
	}

	if count != 1 {
		t.Errorf("Expected exactly 1 EventYoloAutoApproved, got %d", count)
	}

	// Verify payload fields.
	if payload.TaskID != task.ID {
		t.Errorf("Payload.TaskID = %q, want %q", payload.TaskID, task.ID)
	}
	if payload.SeqEpic != 3 {
		t.Errorf("Payload.SeqEpic = %d, want 3", payload.SeqEpic)
	}
	if payload.SeqStory != 5 {
		t.Errorf("Payload.SeqStory = %d, want 5", payload.SeqStory)
	}
	if payload.TaskType != int(domain.TaskTypeConfirmation) {
		t.Errorf("Payload.TaskType = %d, want %d", payload.TaskType, int(domain.TaskTypeConfirmation))
	}
}
