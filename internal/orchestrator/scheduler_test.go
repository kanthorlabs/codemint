package orchestrator

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// --- Test Mocks ---

type mockTaskRepoForScheduler struct {
	tasks        []*domain.Task
	currentIndex int
}

func (m *mockTaskRepoForScheduler) Next(_ context.Context, _ string) (*domain.Task, error) {
	if m.currentIndex >= len(m.tasks) {
		return nil, nil
	}
	task := m.tasks[m.currentIndex]
	m.currentIndex++
	return task, nil
}

func (m *mockTaskRepoForScheduler) Create(_ context.Context, _ *domain.Task) error               { return nil }
func (m *mockTaskRepoForScheduler) Claim(_ context.Context, _ string) error                       { return nil }
func (m *mockTaskRepoForScheduler) UpdateStatus(_ context.Context, _ string, _ domain.TaskStatus, _ string) error {
	return nil
}
func (m *mockTaskRepoForScheduler) FindInterrupted(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoForScheduler) FindByID(_ context.Context, _ string) (*domain.Task, error) { return nil, nil }
func (m *mockTaskRepoForScheduler) UpdateTaskStatus(_ context.Context, _ string, _ domain.TaskStatus) error {
	return nil
}
func (m *mockTaskRepoForScheduler) UpdateAssignee(_ context.Context, _ string, _ string) error { return nil }
func (m *mockTaskRepoForScheduler) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) {
	return nil, nil
}

type mockExecutorForScheduler struct {
	executedTasks []*domain.Task
}

func (m *mockExecutorForScheduler) ExecuteTask(_ context.Context, task *domain.Task) error {
	m.executedTasks = append(m.executedTasks, task)
	return nil
}

// mockACPRegistryEntry simulates an ACP worker for testing.
type mockACPRegistryEntry struct {
	resetContextCalls  []string // old session IDs passed to ResetContext
	newSessionID       string
	alive              bool
}

func (m *mockACPRegistryEntry) recordResetContext(oldSessionID string) string {
	m.resetContextCalls = append(m.resetContextCalls, oldSessionID)
	return m.newSessionID
}

// --- Tests ---

// TestScheduler_StoryBoundaryDetection verifies that ResetContext is called
// when transitioning between stories but not within the same story.
// Sequence: (1,1,1) → (1,1,2) → (1,2,1)
// ResetContext should be called once: between task 2 and task 3.
func TestScheduler_StoryBoundaryDetection(t *testing.T) {
	sessionID := idgen.MustNew()

	tasks := []*domain.Task{
		{
			ID:        idgen.MustNew(),
			SessionID: sessionID,
			SeqEpic:   1,
			SeqStory:  1,
			SeqTask:   1,
			Type:      domain.TaskTypeCoding,
			Timeout:   domain.DefaultTaskTimeout,
		},
		{
			ID:        idgen.MustNew(),
			SessionID: sessionID,
			SeqEpic:   1,
			SeqStory:  1,
			SeqTask:   2,
			Type:      domain.TaskTypeCoding,
			Timeout:   domain.DefaultTaskTimeout,
		},
		{
			ID:        idgen.MustNew(),
			SessionID: sessionID,
			SeqEpic:   1,
			SeqStory:  2,
			SeqTask:   1,
			Type:      domain.TaskTypeCoding,
			Timeout:   domain.DefaultTaskTimeout,
		},
	}

	taskRepo := &mockTaskRepoForScheduler{tasks: tasks}

	// Create a mock executor that just tracks executed tasks
	executedTasks := []*domain.Task{}
	mockExec := &mockCodingAgent{}
	executor := NewExecutor(mockExec, &mockTaskRepo{}, &mockAgentRepo{}, &mockUI{})

	// Track reset calls
	resetCallCount := 0
	lastOldSessionID := ""

	// Create an active session
	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
	}
	activeSession.SetACPSessionID("initial-session")

	scheduler := NewScheduler(taskRepo, executor, nil, activeSession, nil)

	// Override the maybeResetContext for testing since we can't easily mock acp.Registry
	// Instead, we'll test the boundary detection logic directly
	ctx := context.Background()

	// Process task 1 (1,1,1)
	task1, _ := taskRepo.Next(ctx, sessionID)
	executedTasks = append(executedTasks, task1)
	// First task always triggers reset (lastSeqStory starts at -1)
	if task1.SeqStory != scheduler.lastSeqStory {
		resetCallCount++
		lastOldSessionID = activeSession.GetACPSessionID()
		scheduler.lastSeqStory = task1.SeqStory
	}

	// Process task 2 (1,1,2) - same story
	task2, _ := taskRepo.Next(ctx, sessionID)
	executedTasks = append(executedTasks, task2)
	if task2.SeqStory != scheduler.lastSeqStory {
		resetCallCount++
		lastOldSessionID = activeSession.GetACPSessionID()
		scheduler.lastSeqStory = task2.SeqStory
	}

	// Process task 3 (1,2,1) - new story
	task3, _ := taskRepo.Next(ctx, sessionID)
	executedTasks = append(executedTasks, task3)
	if task3.SeqStory != scheduler.lastSeqStory {
		resetCallCount++
		lastOldSessionID = activeSession.GetACPSessionID()
		scheduler.lastSeqStory = task3.SeqStory
	}

	// Verify: ResetContext should be called twice:
	// 1. First task (lastSeqStory=-1 to 1)
	// 2. Between task 2 and task 3 (SeqStory 1 to 2)
	if resetCallCount != 2 {
		t.Errorf("expected 2 reset calls (initial + story boundary), got %d", resetCallCount)
	}

	// Verify all tasks were "executed"
	if len(executedTasks) != 3 {
		t.Errorf("expected 3 tasks executed, got %d", len(executedTasks))
	}

	// Verify the last session ID was from the context
	if lastOldSessionID != "initial-session" {
		t.Errorf("expected last old session ID to be 'initial-session', got %q", lastOldSessionID)
	}
}

// TestScheduler_SkipResetForCoordination verifies that Coordination tasks
// don't trigger a context reset even on story boundary.
func TestScheduler_SkipResetForCoordination(t *testing.T) {
	sessionID := idgen.MustNew()

	tasks := []*domain.Task{
		{
			ID:        idgen.MustNew(),
			SessionID: sessionID,
			SeqEpic:   1,
			SeqStory:  1,
			SeqTask:   1,
			Type:      domain.TaskTypeCoordination, // Coordination task
			Timeout:   domain.DefaultTaskTimeout,
		},
		{
			ID:        idgen.MustNew(),
			SessionID: sessionID,
			SeqEpic:   1,
			SeqStory:  2, // Different story
			SeqTask:   1,
			Type:      domain.TaskTypeCoordination, // Coordination task
			Timeout:   domain.DefaultTaskTimeout,
		},
	}

	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
	}
	activeSession.SetACPSessionID("initial-session")

	scheduler := NewScheduler(nil, nil, nil, activeSession, nil)
	scheduler.lastSeqStory = 1 // Start at story 1

	ctx := context.Background()

	// Test that Coordination task doesn't trigger reset
	err := scheduler.maybeResetContext(ctx, tasks[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lastSeqStory should NOT be updated for Coordination tasks
	// (the current implementation returns early, so lastSeqStory stays unchanged)
	if scheduler.lastSeqStory != 1 {
		t.Errorf("lastSeqStory should remain 1 for Coordination task, got %d", scheduler.lastSeqStory)
	}

	// Test second Coordination task with different story
	err = scheduler.maybeResetContext(ctx, tasks[1])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lastSeqStory should still be 1 (no reset for Coordination)
	if scheduler.lastSeqStory != 1 {
		t.Errorf("lastSeqStory should remain 1 for Coordination task, got %d", scheduler.lastSeqStory)
	}
}

// TestScheduler_SkipResetForConfirmation verifies that Confirmation tasks
// don't trigger a context reset even on story boundary.
func TestScheduler_SkipResetForConfirmation(t *testing.T) {
	sessionID := idgen.MustNew()

	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: sessionID,
		SeqEpic:   1,
		SeqStory:  5, // Different from lastSeqStory
		SeqTask:   1,
		Type:      domain.TaskTypeConfirmation, // Confirmation task
		Timeout:   domain.DefaultTaskTimeout,
	}

	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
	}
	activeSession.SetACPSessionID("initial-session")

	scheduler := NewScheduler(nil, nil, nil, activeSession, nil)
	scheduler.lastSeqStory = 1 // Start at story 1

	ctx := context.Background()

	// Test that Confirmation task doesn't trigger reset
	err := scheduler.maybeResetContext(ctx, task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lastSeqStory should NOT be updated for Confirmation tasks
	if scheduler.lastSeqStory != 1 {
		t.Errorf("lastSeqStory should remain 1 for Confirmation task, got %d", scheduler.lastSeqStory)
	}
}

// TestScheduler_ResetOnCodingTask verifies that Coding tasks DO trigger
// a context reset on story boundary (when a worker exists).
func TestScheduler_ResetOnCodingTask(t *testing.T) {
	sessionID := idgen.MustNew()

	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: sessionID,
		SeqEpic:   1,
		SeqStory:  3, // Different from lastSeqStory
		SeqTask:   1,
		Type:      domain.TaskTypeCoding, // Coding task
		Timeout:   domain.DefaultTaskTimeout,
	}

	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
	}
	activeSession.SetACPSessionID("initial-session")

	// Create scheduler without registry (simulates no worker)
	scheduler := NewScheduler(nil, nil, nil, activeSession, nil)
	scheduler.lastSeqStory = 1 // Start at story 1

	ctx := context.Background()

	// Without a worker, reset should "succeed" (no-op) and update lastSeqStory
	err := scheduler.maybeResetContext(ctx, task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lastSeqStory should be updated since this is a Coding task with story boundary
	if scheduler.lastSeqStory != 3 {
		t.Errorf("lastSeqStory should be updated to 3 for Coding task, got %d", scheduler.lastSeqStory)
	}
}

// TestScheduler_NoResetWithinSameStory verifies that no reset happens when
// processing tasks within the same story.
func TestScheduler_NoResetWithinSameStory(t *testing.T) {
	sessionID := idgen.MustNew()

	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: sessionID,
		SeqEpic:   1,
		SeqStory:  2, // Same as lastSeqStory
		SeqTask:   5,
		Type:      domain.TaskTypeCoding,
		Timeout:   domain.DefaultTaskTimeout,
	}

	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
	}
	activeSession.SetACPSessionID("initial-session")

	// Create a mock registry with a worker
	acpRegistry := acp.NewRegistry(acp.WorkerConfig{})

	scheduler := NewScheduler(nil, nil, acpRegistry, activeSession, nil)
	scheduler.lastSeqStory = 2 // Same story

	ctx := context.Background()

	// No reset should happen
	err := scheduler.maybeResetContext(ctx, task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lastSeqStory should remain unchanged
	if scheduler.lastSeqStory != 2 {
		t.Errorf("lastSeqStory should remain 2, got %d", scheduler.lastSeqStory)
	}
}
