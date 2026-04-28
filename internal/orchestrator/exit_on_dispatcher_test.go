package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/workflow"
)

// mockTaskRepoExitOn is a mock implementation of the task repository for exit_on tests.
type mockTaskRepoExitOn struct {
	tasks        map[string]*domain.Task
	statusCalls  []statusCall
	returnError  error
}

type statusCall struct {
	taskID string
	status domain.TaskStatus
}

func newMockTaskRepoExitOn() *mockTaskRepoExitOn {
	return &mockTaskRepoExitOn{
		tasks:       make(map[string]*domain.Task),
		statusCalls: make([]statusCall, 0),
	}
}

func (m *mockTaskRepoExitOn) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	task, ok := m.tasks[id]
	if !ok {
		return nil, nil
	}
	return task, nil
}

func (m *mockTaskRepoExitOn) UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error {
	m.statusCalls = append(m.statusCalls, statusCall{taskID: taskID, status: status})
	if task, ok := m.tasks[taskID]; ok {
		task.Status = status
	}
	return m.returnError
}

// Unused methods to satisfy interface.
func (m *mockTaskRepoExitOn) Create(ctx context.Context, t *domain.Task) error           { return nil }
func (m *mockTaskRepoExitOn) ListPending(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) ListBySession(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) UpdateAssignee(ctx context.Context, taskID, assigneeID string) error {
	return nil
}
func (m *mockTaskRepoExitOn) UpdateOutput(ctx context.Context, taskID, output string) error {
	return nil
}
func (m *mockTaskRepoExitOn) CountByStatus(ctx context.Context, sessionID string, status domain.TaskStatus) (int, error) {
	return 0, nil
}
func (m *mockTaskRepoExitOn) ListByWorkflow(ctx context.Context, workflowID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) Claim(ctx context.Context, taskID string) error {
	return nil
}
func (m *mockTaskRepoExitOn) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error {
	return nil
}
func (m *mockTaskRepoExitOn) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) ListCoordinationAfter(ctx context.Context, sessionID string, afterTaskID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) MostRecentActive(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) CancelByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) error {
	return nil
}
func (m *mockTaskRepoExitOn) GetMaxSeqTask(ctx context.Context, workflowID string) (int, error) {
	return 0, nil
}
func (m *mockTaskRepoExitOn) ListByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoExitOn) BulkInsert(ctx context.Context, tasks []*domain.Task) error {
	return nil
}

func TestExitOnDispatcher_Register(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	d.Register("task-1", "wf-1", "/lock-goal", "lock_workflow_goal", "session-1")

	if !d.HasRegistration("/lock-goal", "session-1") {
		t.Error("expected registration for /lock-goal")
	}
	if d.HasRegistration("/lock-goal", "session-2") {
		t.Error("should not have registration for different session")
	}
	if d.HasRegistration("/other-cmd", "session-1") {
		t.Error("should not have registration for different command")
	}
}

func TestExitOnDispatcher_Deregister(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	d.Register("task-1", "wf-1", "/lock-goal", "handler", "session-1")
	d.Register("task-2", "wf-1", "/lock-goal", "handler", "session-2")

	// Deregister task-1.
	d.Deregister("task-1")

	if d.HasRegistration("/lock-goal", "session-1") {
		t.Error("task-1 should be deregistered")
	}
	if !d.HasRegistration("/lock-goal", "session-2") {
		t.Error("task-2 should still be registered")
	}
}

func TestExitOnDispatcher_DeregisterSession(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	d.Register("task-1", "wf-1", "/lock-goal", "handler", "session-1")
	d.Register("task-2", "wf-1", "/other", "handler", "session-1")
	d.Register("task-3", "wf-1", "/lock-goal", "handler", "session-2")

	// Deregister session-1.
	d.DeregisterSession("session-1")

	if d.HasRegistration("/lock-goal", "session-1") {
		t.Error("session-1 /lock-goal should be deregistered")
	}
	if d.HasRegistration("/other", "session-1") {
		t.Error("session-1 /other should be deregistered")
	}
	if !d.HasRegistration("/lock-goal", "session-2") {
		t.Error("session-2 should still be registered")
	}
}

func TestExitOnDispatcher_Dispatch_NoRegistration(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	result := d.Dispatch(context.Background(), "/lock-goal", "session-1")

	if result.Handled {
		t.Error("should not handle unregistered command")
	}
}

func TestExitOnDispatcher_Dispatch_Success(t *testing.T) {
	t.Parallel()

	taskRepo := newMockTaskRepoExitOn()
	task := &domain.Task{
		ID:     "task-123",
		Status: domain.TaskStatusProcessing,
		Output: sql.NullString{String: `{"goal_text":"test","success_criteria":["a"]}`, Valid: true},
	}
	taskRepo.tasks["task-123"] = task

	// Create handler registry with a test handler.
	handlerRegistry := workflow.NewHandlerRegistry()
	handlerCalled := false
	var receivedArgs workflow.HandlerArgs
	_ = handlerRegistry.Register("test_handler", func(ctx context.Context, args workflow.HandlerArgs) error {
		handlerCalled = true
		receivedArgs = args
		return nil
	})

	advanceCh := make(chan struct{}, 1)

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo:        taskRepo,
		HandlerRegistry: handlerRegistry,
		AdvanceCh:       advanceCh,
	})

	d.Register("task-123", "wf-456", "/lock-goal", "test_handler", "session-1")

	result := d.Dispatch(context.Background(), "/lock-goal", "session-1")

	if !result.Handled {
		t.Error("expected command to be handled")
	}
	if result.TaskID != "task-123" {
		t.Errorf("TaskID = %q, want %q", result.TaskID, "task-123")
	}
	if result.Error != nil {
		t.Errorf("Error = %v, want nil", result.Error)
	}

	// Verify handler was called.
	if !handlerCalled {
		t.Error("handler was not called")
	}
	if receivedArgs.WorkflowID != "wf-456" {
		t.Errorf("WorkflowID = %q, want %q", receivedArgs.WorkflowID, "wf-456")
	}
	if receivedArgs.ExitCmd != "/lock-goal" {
		t.Errorf("ExitCmd = %q, want %q", receivedArgs.ExitCmd, "/lock-goal")
	}

	// Verify task status was updated to Success.
	if len(taskRepo.statusCalls) != 1 {
		t.Errorf("expected 1 status call, got %d", len(taskRepo.statusCalls))
	}
	if taskRepo.statusCalls[0].status != domain.TaskStatusSuccess {
		t.Errorf("status = %v, want Success", taskRepo.statusCalls[0].status)
	}

	// Verify advance signal was sent.
	select {
	case <-advanceCh:
		// Good.
	default:
		t.Error("expected advance signal")
	}

	// Verify task was deregistered.
	if d.HasRegistration("/lock-goal", "session-1") {
		t.Error("task should be deregistered after dispatch")
	}
}

func TestExitOnDispatcher_Dispatch_HandlerError(t *testing.T) {
	t.Parallel()

	taskRepo := newMockTaskRepoExitOn()
	task := &domain.Task{
		ID:     "task-123",
		Status: domain.TaskStatusProcessing,
	}
	taskRepo.tasks["task-123"] = task

	// Create handler registry with a failing handler.
	handlerRegistry := workflow.NewHandlerRegistry()
	handlerErr := errors.New("handler failed")
	_ = handlerRegistry.Register("failing_handler", func(ctx context.Context, args workflow.HandlerArgs) error {
		return handlerErr
	})

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo:        taskRepo,
		HandlerRegistry: handlerRegistry,
	})

	d.Register("task-123", "wf-456", "/lock-goal", "failing_handler", "session-1")

	result := d.Dispatch(context.Background(), "/lock-goal", "session-1")

	if !result.Handled {
		t.Error("expected command to be handled")
	}
	if result.Error == nil {
		t.Error("expected error from handler")
	}

	// Verify task status was updated to Failure.
	if len(taskRepo.statusCalls) != 1 {
		t.Errorf("expected 1 status call, got %d", len(taskRepo.statusCalls))
	}
	if taskRepo.statusCalls[0].status != domain.TaskStatusFailure {
		t.Errorf("status = %v, want Failure", taskRepo.statusCalls[0].status)
	}
}

func TestExitOnDispatcher_Dispatch_TaskNotProcessing(t *testing.T) {
	t.Parallel()

	taskRepo := newMockTaskRepoExitOn()
	task := &domain.Task{
		ID:     "task-123",
		Status: domain.TaskStatusSuccess, // Not processing.
	}
	taskRepo.tasks["task-123"] = task

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo: taskRepo,
	})

	d.Register("task-123", "wf-456", "/lock-goal", "", "session-1")

	result := d.Dispatch(context.Background(), "/lock-goal", "session-1")

	// Should not handle because task is not processing.
	if result.Handled {
		t.Error("should not handle command for non-processing task")
	}
}

func TestExitOnDispatcher_Dispatch_NoHandler(t *testing.T) {
	t.Parallel()

	taskRepo := newMockTaskRepoExitOn()
	task := &domain.Task{
		ID:     "task-123",
		Status: domain.TaskStatusProcessing,
	}
	taskRepo.tasks["task-123"] = task

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo: taskRepo,
	})

	// Register without a handler.
	d.Register("task-123", "wf-456", "/lock-goal", "", "session-1")

	result := d.Dispatch(context.Background(), "/lock-goal", "session-1")

	if !result.Handled {
		t.Error("expected command to be handled")
	}
	if result.Error != nil {
		t.Errorf("Error = %v, want nil", result.Error)
	}

	// Verify task status was updated to Success (no handler means success).
	if len(taskRepo.statusCalls) != 1 {
		t.Errorf("expected 1 status call, got %d", len(taskRepo.statusCalls))
	}
	if taskRepo.statusCalls[0].status != domain.TaskStatusSuccess {
		t.Errorf("status = %v, want Success", taskRepo.statusCalls[0].status)
	}
}

func TestExitOnDispatcher_RegisterFromStory(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	story := &domain.StoryDefinition{
		ID: "capture_goal",
		ExitOn: &domain.ExitCondition{
			Command: "/lock-goal",
		},
		Output: &domain.OutputConfig{
			Handler: "lock_workflow_goal",
		},
	}

	d.RegisterFromStory("task-1", "wf-1", "session-1", story)

	if !d.HasRegistration("/lock-goal", "session-1") {
		t.Error("expected registration from story")
	}

	reg, ok := d.GetRegistration("task-1")
	if !ok {
		t.Fatal("expected to find registration")
	}
	if reg.HandlerName != "lock_workflow_goal" {
		t.Errorf("HandlerName = %q, want %q", reg.HandlerName, "lock_workflow_goal")
	}
}

func TestExitOnDispatcher_RegisterFromStory_NoExitOn(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	story := &domain.StoryDefinition{
		ID: "gather",
		// No ExitOn.
	}

	d.RegisterFromStory("task-1", "wf-1", "session-1", story)

	if !d.IsEmpty() {
		t.Error("should not register without exit_on")
	}
}

func TestExitOnDispatcher_Count(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	if d.Count() != 0 {
		t.Errorf("Count() = %d, want 0", d.Count())
	}

	d.Register("task-1", "wf-1", "/cmd1", "", "session-1")
	d.Register("task-2", "wf-1", "/cmd2", "", "session-1")
	d.Register("task-3", "wf-1", "/cmd1", "", "session-2")

	if d.Count() != 3 {
		t.Errorf("Count() = %d, want 3", d.Count())
	}
}

func TestExitOnDispatcher_ActiveRegistrations(t *testing.T) {
	t.Parallel()

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{})

	d.Register("task-1", "wf-1", "/lock-goal", "", "session-1")
	d.Register("task-2", "wf-1", "/lock-goal", "", "session-2")
	d.Register("task-3", "wf-1", "/other", "", "session-1")

	active := d.ActiveRegistrations()

	if len(active) != 2 {
		t.Errorf("expected 2 commands, got %d", len(active))
	}
	if len(active["/lock-goal"]) != 2 {
		t.Errorf("expected 2 tasks for /lock-goal, got %d", len(active["/lock-goal"]))
	}
	if len(active["/other"]) != 1 {
		t.Errorf("expected 1 task for /other, got %d", len(active["/other"]))
	}
}

func TestExitOnDispatcher_Dispatch_WrongSession(t *testing.T) {
	t.Parallel()

	taskRepo := newMockTaskRepoExitOn()
	task := &domain.Task{
		ID:     "task-123",
		Status: domain.TaskStatusProcessing,
	}
	taskRepo.tasks["task-123"] = task

	d := NewExitOnDispatcher(ExitOnDispatcherConfig{
		TaskRepo: taskRepo,
	})

	d.Register("task-123", "wf-456", "/lock-goal", "", "session-1")

	// Dispatch from different session.
	result := d.Dispatch(context.Background(), "/lock-goal", "session-2")

	if result.Handled {
		t.Error("should not handle command for different session")
	}
}
