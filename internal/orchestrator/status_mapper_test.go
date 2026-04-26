package orchestrator

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository/sqlite"
)

// statusMapperMockTaskRepo implements TaskRepository for StatusMapper tests.
type statusMapperMockTaskRepo struct {
	mu              sync.Mutex
	tasks           map[string]*domain.Task
	statusUpdates   []statusUpdate
	statusUpdateErr error
}

func newStatusMapperMockTaskRepo() *statusMapperMockTaskRepo {
	return &statusMapperMockTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
}

func (m *statusMapperMockTaskRepo) Create(ctx context.Context, task *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
	return nil
}

func (m *statusMapperMockTaskRepo) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task, ok := m.tasks[id]; ok {
		return task, nil
	}
	return nil, nil
}

func (m *statusMapperMockTaskRepo) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}

func (m *statusMapperMockTaskRepo) Claim(ctx context.Context, id string) error {
	return nil
}

func (m *statusMapperMockTaskRepo) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, output string) error {
	return nil
}

func (m *statusMapperMockTaskRepo) UpdateTaskStatus(ctx context.Context, id string, status domain.TaskStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusUpdateErr != nil {
		return m.statusUpdateErr
	}
	// Simulate state machine validation.
	task, ok := m.tasks[id]
	if !ok {
		return sqlite.ErrInvalidTransition
	}
	// Update the task status.
	task.Status = status
	m.statusUpdates = append(m.statusUpdates, statusUpdate{TaskID: id, Status: status})
	return nil
}

func (m *statusMapperMockTaskRepo) UpdateAssignee(ctx context.Context, id string, assigneeID string) error {
	return nil
}

func (m *statusMapperMockTaskRepo) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *statusMapperMockTaskRepo) ListCoordinationAfter(ctx context.Context, sessionID, afterTaskID string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *statusMapperMockTaskRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *statusMapperMockTaskRepo) StatusUpdates() []statusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]statusUpdate(nil), m.statusUpdates...)
}

func (m *statusMapperMockTaskRepo) SetTask(task *domain.Task) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
}

func (m *statusMapperMockTaskRepo) SetStatusUpdateErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdateErr = err
}

func TestStatusMapper_Apply_TurnStart_ToProcessing(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-1",
		Status: domain.TaskStatusPending,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind: acp.EventTurnStart,
	}

	err := mapper.Apply(context.Background(), "task-1", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	updates := repo.StatusUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(updates))
	}
	if updates[0].TaskID != "task-1" {
		t.Errorf("expected task ID 'task-1', got %q", updates[0].TaskID)
	}
	if updates[0].Status != domain.TaskStatusProcessing {
		t.Errorf("expected status Processing, got %d", updates[0].Status)
	}

	// Verify UI event was emitted.
	events := ui.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 UI event, got %d", len(events))
	}
	if events[0].Type != registry.EventTaskStatusChanged {
		t.Errorf("expected event type 'task_status_changed', got %q", events[0].Type)
	}
}

func TestStatusMapper_Apply_PermissionRequest_ToAwaiting(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-2",
		Status: domain.TaskStatusProcessing,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind:      acp.EventPermissionRequest,
		RequestID: "req-1",
		ToolName:  "bash",
		Command:   "rm -rf /",
	}

	err := mapper.Apply(context.Background(), "task-2", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	updates := repo.StatusUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(updates))
	}
	if updates[0].Status != domain.TaskStatusAwaiting {
		t.Errorf("expected status Awaiting, got %d", updates[0].Status)
	}
}

func TestStatusMapper_Apply_TurnEnd_Success(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}
	advanceCh := make(chan struct{}, 1)

	task := &domain.Task{
		ID:     "task-3",
		Status: domain.TaskStatusProcessing,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo:  repo,
		UI:        ui,
		AdvanceCh: advanceCh,
	})

	// turn_end without error.
	ev := acp.Event{
		Kind: acp.EventTurnEnd,
		Raw:  json.RawMessage(`{"result": "success"}`),
	}

	err := mapper.Apply(context.Background(), "task-3", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	updates := repo.StatusUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(updates))
	}
	if updates[0].Status != domain.TaskStatusSuccess {
		t.Errorf("expected status Success, got %d", updates[0].Status)
	}

	// Verify advance signal was sent.
	select {
	case <-advanceCh:
		// Good.
	default:
		t.Error("expected advance signal on success")
	}
}

func TestStatusMapper_Apply_TurnEnd_Failure(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-4",
		Status: domain.TaskStatusProcessing,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	// turn_end with error.
	ev := acp.Event{
		Kind: acp.EventTurnEnd,
		Raw:  json.RawMessage(`{"error": {"message": "something went wrong", "code": 500}}`),
	}

	err := mapper.Apply(context.Background(), "task-4", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	updates := repo.StatusUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(updates))
	}
	if updates[0].Status != domain.TaskStatusFailure {
		t.Errorf("expected status Failure, got %d", updates[0].Status)
	}
}

func TestStatusMapper_Apply_Idempotent(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-5",
		Status: domain.TaskStatusPending,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind: acp.EventTurnStart,
	}

	// Apply first time.
	err := mapper.Apply(context.Background(), "task-5", ev)
	if err != nil {
		t.Fatalf("First Apply returned error: %v", err)
	}

	// Apply second time (idempotent).
	err = mapper.Apply(context.Background(), "task-5", ev)
	if err != nil {
		t.Fatalf("Second Apply returned error: %v", err)
	}

	// Should only have 1 update due to idempotency.
	updates := repo.StatusUpdates()
	if len(updates) != 1 {
		t.Errorf("expected 1 status update (idempotent), got %d", len(updates))
	}

	// Should only have 1 UI event.
	events := ui.Events()
	if len(events) != 1 {
		t.Errorf("expected 1 UI event (idempotent), got %d", len(events))
	}
}

func TestStatusMapper_Apply_EmptyTaskID_ShortCircuit(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind: acp.EventTurnStart,
	}

	// Apply with empty task ID (ad-hoc /acp prompt).
	err := mapper.Apply(context.Background(), "", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	// Should have no updates.
	updates := repo.StatusUpdates()
	if len(updates) != 0 {
		t.Errorf("expected 0 status updates for empty task ID, got %d", len(updates))
	}

	// Should have no UI events.
	events := ui.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 UI events for empty task ID, got %d", len(events))
	}
}

func TestStatusMapper_Apply_InvalidTransition_LogsAndSkips(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	// Task in Pending status cannot go to Success directly.
	task := &domain.Task{
		ID:     "task-6",
		Status: domain.TaskStatusPending,
	}
	repo.SetTask(task)

	// Force the repo to return an invalid transition error.
	repo.SetStatusUpdateErr(sqlite.ErrInvalidTransition)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind: acp.EventTurnEnd,
		Raw:  json.RawMessage(`{"result": "success"}`),
	}

	// Apply should NOT return an error - it logs and skips.
	err := mapper.Apply(context.Background(), "task-6", ev)
	if err != nil {
		t.Errorf("Apply should not return error for invalid transition, got: %v", err)
	}

	// Should have no UI events since transition was rejected.
	events := ui.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 UI events for rejected transition, got %d", len(events))
	}
}

func TestStatusMapper_Apply_NonMappingEvent_NoOp(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-7",
		Status: domain.TaskStatusProcessing,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	// Events that don't map to status transitions.
	nonMappingEvents := []acp.EventKind{
		acp.EventThinking,
		acp.EventMessage,
		acp.EventPlan,
		acp.EventToolCall,
		acp.EventToolUpdate,
		acp.EventUnknown,
	}

	for _, kind := range nonMappingEvents {
		ev := acp.Event{Kind: kind}

		err := mapper.Apply(context.Background(), "task-7", ev)
		if err != nil {
			t.Errorf("Apply for %s returned error: %v", kind.String(), err)
		}
	}

	// Should have no updates.
	updates := repo.StatusUpdates()
	if len(updates) != 0 {
		t.Errorf("expected 0 status updates for non-mapping events, got %d", len(updates))
	}
}

func TestStatusMapper_ClearTask(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-8",
		Status: domain.TaskStatusPending,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind: acp.EventTurnStart,
	}

	// Apply first time.
	err := mapper.Apply(context.Background(), "task-8", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	// Clear the task from idempotency tracking.
	mapper.ClearTask("task-8")

	// Reset task status to Pending for the test.
	task.Status = domain.TaskStatusPending

	// Apply second time should work now.
	err = mapper.Apply(context.Background(), "task-8", ev)
	if err != nil {
		t.Fatalf("Apply after ClearTask returned error: %v", err)
	}

	// Should have 2 updates now.
	updates := repo.StatusUpdates()
	if len(updates) != 2 {
		t.Errorf("expected 2 status updates after ClearTask, got %d", len(updates))
	}
}

func TestStatusMapper_Apply_TurnEnd_ErrorInParams(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-9",
		Status: domain.TaskStatusProcessing,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	// turn_end with error in params.
	ev := acp.Event{
		Kind: acp.EventTurnEnd,
		Raw:  json.RawMessage(`{"params": {"error": {"message": "agent crashed"}}}`),
	}

	err := mapper.Apply(context.Background(), "task-9", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	updates := repo.StatusUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(updates))
	}
	if updates[0].Status != domain.TaskStatusFailure {
		t.Errorf("expected status Failure for params.error, got %d", updates[0].Status)
	}
}

func TestStatusMapper_StatusChangedPayload(t *testing.T) {
	repo := newStatusMapperMockTaskRepo()
	ui := &mockUIMediator{}

	task := &domain.Task{
		ID:     "task-10",
		Status: domain.TaskStatusPending,
	}
	repo.SetTask(task)

	mapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo: repo,
		UI:       ui,
	})

	ev := acp.Event{
		Kind: acp.EventTurnStart,
	}

	err := mapper.Apply(context.Background(), "task-10", ev)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	events := ui.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 UI event, got %d", len(events))
	}

	payload, ok := events[0].Payload.(registry.TaskStatusChangedPayload)
	if !ok {
		t.Fatalf("expected registry.TaskStatusChangedPayload, got %T", events[0].Payload)
	}

	if payload.TaskID != "task-10" {
		t.Errorf("expected TaskID 'task-10', got %q", payload.TaskID)
	}
	if payload.From != int(domain.TaskStatusPending) {
		t.Errorf("expected From %d, got %d", domain.TaskStatusPending, payload.From)
	}
	if payload.To != int(domain.TaskStatusProcessing) {
		t.Errorf("expected To %d, got %d", domain.TaskStatusProcessing, payload.To)
	}
	if payload.Reason != "turn_start" {
		t.Errorf("expected Reason 'turn_start', got %q", payload.Reason)
	}
}
