package project

import (
	"context"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// --- Mock TaskRepository ---

type mockTaskRepo struct {
	task            *domain.Task
	findErr         error
	updateStatusErr error
	updatedStatus   domain.TaskStatus
	updateCalled    bool
}

func (m *mockTaskRepo) Next(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}

func (m *mockTaskRepo) Claim(_ context.Context, _ string) error {
	return nil
}

func (m *mockTaskRepo) UpdateStatus(_ context.Context, _ string, _ domain.TaskStatus, _ string) error {
	return nil
}

func (m *mockTaskRepo) FindInterrupted(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *mockTaskRepo) FindByID(_ context.Context, _ string) (*domain.Task, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.task, nil
}

func (m *mockTaskRepo) UpdateTaskStatus(_ context.Context, _ string, status domain.TaskStatus) error {
	m.updateCalled = true
	m.updatedStatus = status
	return m.updateStatusErr
}

func (m *mockTaskRepo) UpdateAssignee(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockTaskRepo) Create(_ context.Context, _ *domain.Task) error {
	return nil
}

func (m *mockTaskRepo) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) {
	return nil, nil
}

// --- Mock CodingAgent ---

type mockCodingAgent struct {
	acceptErr error
	revertErr error
	acceptCalled bool
	revertCalled bool
}

func (m *mockCodingAgent) ExecuteTask(_ context.Context, _ *domain.Task) error {
	return nil
}

func (m *mockCodingAgent) Accept(_ context.Context, _ *domain.Task) error {
	m.acceptCalled = true
	return m.acceptErr
}

func (m *mockCodingAgent) Revert(_ context.Context, _ *domain.Task) error {
	m.revertCalled = true
	return m.revertErr
}

// --- Helpers ---

func awaitingTask(projectID, sessionID, agentID string) *domain.Task {
	return &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		AssigneeID: agentID,
		SeqEpic:    1,
		SeqStory:   1,
		SeqTask:    1,
		Type:       domain.TaskTypeCoding,
		Status:     domain.TaskStatusAwaiting,
	}
}

// --- Test A: Happy Path Accept ---

// TestAcceptHandler_HappyPath asserts that a task in Awaiting state is
// accepted: the mock agent's Accept method is called and the DB status is
// updated to Success.
func TestAcceptHandler_HappyPath(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{}

	rc := NewReviewCommands(repo, ag)
	handler := rc.acceptHandler()

	result, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err != nil {
		t.Fatalf("acceptHandler returned unexpected error: %v", err)
	}
	if !ag.acceptCalled {
		t.Error("expected CodingAgent.Accept to be called, but it was not")
	}
	if !repo.updateCalled {
		t.Error("expected UpdateTaskStatus to be called, but it was not")
	}
	if repo.updatedStatus != domain.TaskStatusSuccess {
		t.Errorf("expected status TaskStatusSuccess (%d), got %d",
			domain.TaskStatusSuccess, repo.updatedStatus)
	}
	if result.Message == "" {
		t.Error("expected a non-empty result message")
	}
}

// --- Test B: Happy Path Revert ---

// TestRevertHandler_HappyPath asserts that a task in Awaiting state is
// reverted: the mock agent's Revert method is called and the DB status is
// updated to Reverted.
func TestRevertHandler_HappyPath(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{}

	rc := NewReviewCommands(repo, ag)
	handler := rc.revertHandler()

	result, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err != nil {
		t.Fatalf("revertHandler returned unexpected error: %v", err)
	}
	if !ag.revertCalled {
		t.Error("expected CodingAgent.Revert to be called, but it was not")
	}
	if !repo.updateCalled {
		t.Error("expected UpdateTaskStatus to be called, but it was not")
	}
	if repo.updatedStatus != domain.TaskStatusReverted {
		t.Errorf("expected status TaskStatusReverted (%d), got %d",
			domain.TaskStatusReverted, repo.updatedStatus)
	}
	if result.Message == "" {
		t.Error("expected a non-empty result message")
	}
}

// --- Test C: Agent Crash on Revert ---

// TestRevertHandler_AgentCrash asserts that when the agent returns an error
// during Revert the task status is set to TaskStatusFailure (triggering the
// crash fallback flow in Story 1.9) and the handler returns an error.
func TestRevertHandler_AgentCrash(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{revertErr: errors.New("agent internal error")}

	rc := NewReviewCommands(repo, ag)
	handler := rc.revertHandler()

	_, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err == nil {
		t.Fatal("expected revertHandler to return an error on agent crash, got nil")
	}
	if !ag.revertCalled {
		t.Error("expected CodingAgent.Revert to be called, but it was not")
	}
	if !repo.updateCalled {
		t.Error("expected UpdateTaskStatus to be called (to set Failure), but it was not")
	}
	if repo.updatedStatus != domain.TaskStatusFailure {
		t.Errorf("expected status TaskStatusFailure (%d) after agent crash, got %d",
			domain.TaskStatusFailure, repo.updatedStatus)
	}
}

// --- Additional edge-case tests ---

// TestAcceptHandler_NotAwaiting asserts that the accept handler rejects tasks
// that are not in Awaiting state.
func TestAcceptHandler_NotAwaiting(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	task.Status = domain.TaskStatusPending // not Awaiting
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{}

	rc := NewReviewCommands(repo, ag)
	handler := rc.acceptHandler()

	_, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err == nil {
		t.Fatal("expected error for non-Awaiting task, got nil")
	}
	if !errors.Is(err, ErrTaskNotAwaiting) {
		t.Errorf("expected ErrTaskNotAwaiting, got: %v", err)
	}
	if ag.acceptCalled {
		t.Error("CodingAgent.Accept must not be called when precondition fails")
	}
}

// TestRevertHandler_NotAwaiting asserts that the revert handler rejects tasks
// that are not in Awaiting state.
func TestRevertHandler_NotAwaiting(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	task.Status = domain.TaskStatusProcessing // not Awaiting
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{}

	rc := NewReviewCommands(repo, ag)
	handler := rc.revertHandler()

	_, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err == nil {
		t.Fatal("expected error for non-Awaiting task, got nil")
	}
	if !errors.Is(err, ErrTaskNotAwaiting) {
		t.Errorf("expected ErrTaskNotAwaiting, got: %v", err)
	}
	if ag.revertCalled {
		t.Error("CodingAgent.Revert must not be called when precondition fails")
	}
}

// TestAcceptHandler_MissingTaskID asserts that the accept handler returns an
// error when no task_id argument is supplied.
func TestAcceptHandler_MissingTaskID(t *testing.T) {
	rc := NewReviewCommands(&mockTaskRepo{}, &mockCodingAgent{})
	handler := rc.acceptHandler()

	_, err := handler(context.Background(), nil, []string{}, "")
	if err == nil {
		t.Fatal("expected error for missing task_id, got nil")
	}
}

// --- Test: Discard Happy Path ---

// TestDiscardHandler_HappyPath asserts that a Coding task returns the stub
// placeholder message without error.
func TestDiscardHandler_HappyPath(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	task.Type = domain.TaskTypeCoding
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{}

	rc := NewReviewCommands(repo, ag)
	handler := rc.discardHandler()

	result, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err != nil {
		t.Fatalf("discardHandler returned unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected a non-empty result message")
	}
}

// TestDiscardHandler_NonCodingTask asserts that the discard handler rejects
// tasks that are not of type TaskTypeCoding.
func TestDiscardHandler_NonCodingTask(t *testing.T) {
	task := awaitingTask(idgen.MustNew(), idgen.MustNew(), idgen.MustNew())
	task.Type = domain.TaskTypeVerification
	repo := &mockTaskRepo{task: task}
	ag := &mockCodingAgent{}

	rc := NewReviewCommands(repo, ag)
	handler := rc.discardHandler()

	_, err := handler(context.Background(), nil, []string{task.ID}, "")
	if err == nil {
		t.Fatal("expected error for non-Coding task, got nil")
	}
	if !errors.Is(err, ErrTaskNotCodingType) {
		t.Errorf("expected ErrTaskNotCodingType, got: %v", err)
	}
}

// TestDiscardHandler_MissingTaskID asserts that the discard handler returns an
// error when no task_id argument is supplied.
func TestDiscardHandler_MissingTaskID(t *testing.T) {
	rc := NewReviewCommands(&mockTaskRepo{}, &mockCodingAgent{})
	handler := rc.discardHandler()

	_, err := handler(context.Background(), nil, []string{}, "")
	if err == nil {
		t.Fatal("expected error for missing task_id, got nil")
	}
}
