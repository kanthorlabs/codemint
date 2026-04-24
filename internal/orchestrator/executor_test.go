package orchestrator

import (
	"context"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
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
	updateStatusCalls  []domain.TaskStatus
	updateAssigneeCalled bool
	newAssigneeID      string
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
	return nil, nil
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
	lastMessage string
}

func (m *mockUI) RenderMessage(msg string) { m.lastMessage = msg }
func (m *mockUI) ClearScreen()             {}

// --- Tests ---

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
