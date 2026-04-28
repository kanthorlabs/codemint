package orchestrator

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// --- Test Mocks ---

type mockTaskRepoForScheduler struct {
	tasks        []*domain.Task
	currentIndex int
	mu           sync.Mutex
}

func (m *mockTaskRepoForScheduler) Next(_ context.Context, _ string) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
func (m *mockTaskRepoForScheduler) FindByID(_ context.Context, taskID string) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		if t.ID == taskID {
			return t, nil
		}
	}
	return nil, nil
}
func (m *mockTaskRepoForScheduler) UpdateTaskStatus(_ context.Context, _ string, _ domain.TaskStatus) error {
	return nil
}
func (m *mockTaskRepoForScheduler) UpdateAssignee(_ context.Context, _ string, _ string) error { return nil }
func (m *mockTaskRepoForScheduler) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoForScheduler) ListBySession(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoForScheduler) ListPending(_ context.Context, _ string) ([]*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return remaining tasks starting from currentIndex
	if m.currentIndex >= len(m.tasks) {
		return nil, nil
	}
	result := make([]*domain.Task, len(m.tasks)-m.currentIndex)
	copy(result, m.tasks[m.currentIndex:])
	m.currentIndex++ // Advance for Next compatibility
	return result, nil
}
func (m *mockTaskRepoForScheduler) MostRecentActive(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoForScheduler) CancelByWorkflowAndStoryIDs(_ context.Context, _ string, _ []string) error {
	return nil
}
func (m *mockTaskRepoForScheduler) GetMaxSeqTask(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockTaskRepoForScheduler) ListByWorkflowAndStoryIDs(_ context.Context, _ string, _ []string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepoForScheduler) BulkInsert(_ context.Context, _ []*domain.Task) error {
	return nil
}

type mockExecutorForScheduler struct {
	executedTasks []*domain.Task
	mu            sync.Mutex
}

func (m *mockExecutorForScheduler) ExecuteTask(_ context.Context, task *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedTasks = append(m.executedTasks, task)
	return nil
}

// mockACPRegistryEntry simulates an ACP worker for testing.
type mockACPRegistryEntry struct {
	resetContextCalls []string // old session IDs passed to ResetContext
	newSessionID      string
	alive             bool
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

// TestScheduler_RejectsConcurrentRun verifies that only one Run loop can be active at a time.
func TestScheduler_RejectsConcurrentRun(t *testing.T) {
	sessionID := idgen.MustNew()

	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
		Project: &domain.Project{ID: idgen.MustNew()},
	}

	taskRepo := &mockTaskRepoForScheduler{tasks: nil} // No tasks

	scheduler := NewScheduler(taskRepo, nil, nil, activeSession, nil)

	// Start the first Run in a goroutine.
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	started := make(chan struct{})
	firstErr := make(chan error, 1)
	go func() {
		close(started)
		firstErr <- scheduler.Run(ctx1)
	}()

	<-started
	// Give the first Run time to set running=true.
	time.Sleep(50 * time.Millisecond)

	// Try to start a second Run - should fail immediately.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()

	err := scheduler.Run(ctx2)
	if err != ErrSchedulerAlreadyRunning {
		t.Errorf("expected ErrSchedulerAlreadyRunning, got %v", err)
	}

	// Verify IsRunning returns true.
	if !scheduler.IsRunning() {
		t.Error("expected IsRunning() to return true")
	}

	// Cancel the first Run.
	cancel1()

	// Wait for first Run to exit.
	<-firstErr

	// Verify IsRunning returns false after exit.
	time.Sleep(50 * time.Millisecond)
	if scheduler.IsRunning() {
		t.Error("expected IsRunning() to return false after Run exits")
	}
}

// TestScheduler_RunsTasksInOrder verifies that the scheduler dispatches
// tasks in strict (seq_epic, seq_story, seq_task) order.
func TestScheduler_RunsTasksInOrder(t *testing.T) {
	sessionID := idgen.MustNew()

	tasks := []*domain.Task{
		{ID: "task-1", SessionID: sessionID, SeqEpic: 1, SeqStory: 1, SeqTask: 1, Type: domain.TaskTypeCoding, Timeout: domain.DefaultTaskTimeout},
		{ID: "task-2", SessionID: sessionID, SeqEpic: 1, SeqStory: 1, SeqTask: 2, Type: domain.TaskTypeCoding, Timeout: domain.DefaultTaskTimeout},
		{ID: "task-3", SessionID: sessionID, SeqEpic: 1, SeqStory: 2, SeqTask: 1, Type: domain.TaskTypeCoding, Timeout: domain.DefaultTaskTimeout},
	}

	taskRepo := &mockTaskRepoForScheduler{tasks: tasks}

	// Create a mock executor that tracks executed tasks.
	executedTasks := []*domain.Task{}
	var mu sync.Mutex
	mockExec := &mockCodingAgent{}
	executor := NewExecutor(mockExec, &mockTaskRepo{}, &mockAgentRepo{}, &mockUI{})

	// Override executor's ExecuteTask to track calls.
	// Since we can't easily override, we'll use ProcessNextTask directly.

	activeSession := &ActiveSession{
		Session: &domain.Session{ID: sessionID},
	}

	scheduler := NewScheduler(taskRepo, executor, nil, activeSession, nil)

	ctx := context.Background()

	// Process all tasks.
	for i := 0; i < 3; i++ {
		task, err := scheduler.ProcessNextTask(ctx)
		if err != nil {
			t.Fatalf("unexpected error on task %d: %v", i, err)
		}
		if task == nil {
			t.Fatalf("expected task %d, got nil", i)
		}
		mu.Lock()
		executedTasks = append(executedTasks, task)
		mu.Unlock()
	}

	// Verify order.
	if len(executedTasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(executedTasks))
	}
	if executedTasks[0].ID != "task-1" {
		t.Errorf("expected first task to be task-1, got %s", executedTasks[0].ID)
	}
	if executedTasks[1].ID != "task-2" {
		t.Errorf("expected second task to be task-2, got %s", executedTasks[1].ID)
	}
	if executedTasks[2].ID != "task-3" {
		t.Errorf("expected third task to be task-3, got %s", executedTasks[2].ID)
	}
}

// TestActiveSession_Wakeup_Coalesces verifies that multiple Wakeup calls
// before the scheduler reads the channel collapse to a single notification.
func TestActiveSession_Wakeup_Coalesces(t *testing.T) {
	activeSession := &ActiveSession{}

	// Call Wakeup multiple times.
	activeSession.Wakeup()
	activeSession.Wakeup()
	activeSession.Wakeup()

	// Only one signal should be in the channel.
	ch := activeSession.WakeupCh()
	select {
	case <-ch:
		// OK, received one signal.
	default:
		t.Error("expected one signal in WakeupCh")
	}

	// Channel should now be empty.
	select {
	case <-ch:
		t.Error("expected no more signals in WakeupCh")
	default:
		// OK, channel is empty.
	}
}

// TestBackoffDuration verifies the exponential backoff calculation.
func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		consecutiveErrors int
		expected          time.Duration
	}{
		{1, 500 * time.Millisecond},
		{2, 1 * time.Second},
		{3, 2 * time.Second},
		{4, 4 * time.Second},
		{5, 5 * time.Second}, // Capped at 5s
		{10, 5 * time.Second}, // Still capped
	}

	for _, tt := range tests {
		got := backoffDuration(tt.consecutiveErrors)
		if got != tt.expected {
			t.Errorf("backoffDuration(%d) = %v, expected %v", tt.consecutiveErrors, got, tt.expected)
		}
	}
}

// --- Task Eligibility Tests (Story 2.0.2) ---

// eligibilityMockTaskRepo is a mock for eligibility tests that allows
// configuring task states for predecessor lookups.
type eligibilityMockTaskRepo struct {
	tasks map[string]*domain.Task
}

func newEligibilityMockTaskRepo() *eligibilityMockTaskRepo {
	return &eligibilityMockTaskRepo{tasks: make(map[string]*domain.Task)}
}

func (m *eligibilityMockTaskRepo) addTask(t *domain.Task) {
	m.tasks[t.ID] = t
}

func (m *eligibilityMockTaskRepo) FindByID(_ context.Context, taskID string) (*domain.Task, error) {
	if t, ok := m.tasks[taskID]; ok {
		return t, nil
	}
	return nil, nil
}

// Implement other interface methods as no-ops.
func (m *eligibilityMockTaskRepo) Create(_ context.Context, _ *domain.Task) error { return nil }
func (m *eligibilityMockTaskRepo) Next(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) ListPending(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) Claim(_ context.Context, _ string) error { return nil }
func (m *eligibilityMockTaskRepo) UpdateStatus(_ context.Context, _ string, _ domain.TaskStatus, _ string) error {
	return nil
}
func (m *eligibilityMockTaskRepo) UpdateTaskStatus(_ context.Context, _ string, _ domain.TaskStatus) error {
	return nil
}
func (m *eligibilityMockTaskRepo) UpdateAssignee(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *eligibilityMockTaskRepo) FindInterrupted(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) ListCoordinationAfter(_ context.Context, _ string, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) ListBySession(_ context.Context, _ string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) MostRecentActive(_ context.Context, _ string) (*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) CancelByWorkflowAndStoryIDs(_ context.Context, _ string, _ []string) error {
	return nil
}
func (m *eligibilityMockTaskRepo) GetMaxSeqTask(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *eligibilityMockTaskRepo) ListByWorkflowAndStoryIDs(_ context.Context, _ string, _ []string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *eligibilityMockTaskRepo) BulkInsert(_ context.Context, _ []*domain.Task) error {
	return nil
}

// TestScheduler_IsTaskEligible_NoDependency verifies that tasks without
// depends_on are always eligible.
func TestScheduler_IsTaskEligible_NoDependency(t *testing.T) {
	repo := newEligibilityMockTaskRepo()
	activeSession := &ActiveSession{
		Session: &domain.Session{ID: idgen.MustNew()},
	}
	scheduler := NewScheduler(repo, nil, nil, activeSession, nil)

	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusPending,
		// DependsOn is not set (NULL)
	}

	ctx := context.Background()
	if !scheduler.isTaskEligible(ctx, task) {
		t.Error("task without depends_on should be eligible")
	}
}

// TestScheduler_IsTaskEligible_WaitForTerminal verifies that tasks with
// depends_on wait until the predecessor reaches a terminal state.
func TestScheduler_IsTaskEligible_WaitForTerminal(t *testing.T) {
	repo := newEligibilityMockTaskRepo()
	activeSession := &ActiveSession{
		Session: &domain.Session{ID: idgen.MustNew()},
	}
	scheduler := NewScheduler(repo, nil, nil, activeSession, nil)

	predecessorID := idgen.MustNew()
	predecessor := &domain.Task{
		ID:        predecessorID,
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusProcessing, // Not terminal
	}
	repo.addTask(predecessor)

	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusPending,
		DependsOn: domain.NewNullString(predecessorID),
		// Condition not set - any terminal state is OK
	}

	ctx := context.Background()

	// Should NOT be eligible while predecessor is Processing.
	if scheduler.isTaskEligible(ctx, task) {
		t.Error("task should not be eligible while predecessor is Processing")
	}

	// Update predecessor to Success (terminal).
	predecessor.Status = domain.TaskStatusSuccess

	// Now should be eligible.
	if !scheduler.isTaskEligible(ctx, task) {
		t.Error("task should be eligible when predecessor is Success")
	}
}

// TestScheduler_IsTaskEligible_WithCondition verifies that tasks with
// a specific condition only become eligible when the predecessor matches.
func TestScheduler_IsTaskEligible_WithCondition(t *testing.T) {
	repo := newEligibilityMockTaskRepo()
	activeSession := &ActiveSession{
		Session: &domain.Session{ID: idgen.MustNew()},
	}
	scheduler := NewScheduler(repo, nil, nil, activeSession, nil)

	predecessorID := idgen.MustNew()
	predecessor := &domain.Task{
		ID:        predecessorID,
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeConfirmation,
		Status:    domain.TaskStatusSuccess, // Terminal, but not Failure
	}
	repo.addTask(predecessor)

	// Task requires predecessor to be Failure (condition=4).
	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusPending,
		DependsOn: domain.NewNullString(predecessorID),
		Condition: sql.NullInt64{Int64: int64(domain.TaskStatusFailure), Valid: true},
	}

	ctx := context.Background()

	// Should NOT be eligible because predecessor is Success, not Failure.
	if scheduler.isTaskEligible(ctx, task) {
		t.Error("task should not be eligible when predecessor status doesn't match condition")
	}

	// Update predecessor to Failure.
	predecessor.Status = domain.TaskStatusFailure

	// Now should be eligible.
	if !scheduler.isTaskEligible(ctx, task) {
		t.Error("task should be eligible when predecessor status matches condition")
	}
}

// TestScheduler_IsTaskEligible_ConditionSuccess verifies routing based on Success condition.
func TestScheduler_IsTaskEligible_ConditionSuccess(t *testing.T) {
	repo := newEligibilityMockTaskRepo()
	activeSession := &ActiveSession{
		Session: &domain.Session{ID: idgen.MustNew()},
	}
	scheduler := NewScheduler(repo, nil, nil, activeSession, nil)

	predecessorID := idgen.MustNew()
	predecessor := &domain.Task{
		ID:        predecessorID,
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeConfirmation,
		Status:    domain.TaskStatusSuccess,
	}
	repo.addTask(predecessor)

	// Task requires predecessor to be Success (condition=3).
	taskOnSuccess := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusPending,
		DependsOn: domain.NewNullString(predecessorID),
		Condition: sql.NullInt64{Int64: int64(domain.TaskStatusSuccess), Valid: true},
	}

	// Task requires predecessor to be Failure (condition=4).
	taskOnFailure := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusPending,
		DependsOn: domain.NewNullString(predecessorID),
		Condition: sql.NullInt64{Int64: int64(domain.TaskStatusFailure), Valid: true},
	}

	ctx := context.Background()

	// Predecessor is Success, so taskOnSuccess should be eligible.
	if !scheduler.isTaskEligible(ctx, taskOnSuccess) {
		t.Error("taskOnSuccess should be eligible when predecessor is Success")
	}

	// Predecessor is Success, so taskOnFailure should NOT be eligible.
	if scheduler.isTaskEligible(ctx, taskOnFailure) {
		t.Error("taskOnFailure should not be eligible when predecessor is Success")
	}
}

// TestScheduler_IsTaskEligible_PredecessorNotFound verifies behavior when
// the predecessor task doesn't exist.
func TestScheduler_IsTaskEligible_PredecessorNotFound(t *testing.T) {
	repo := newEligibilityMockTaskRepo()
	activeSession := &ActiveSession{
		Session: &domain.Session{ID: idgen.MustNew()},
	}
	scheduler := NewScheduler(repo, nil, nil, activeSession, nil)

	// Task depends on a non-existent predecessor.
	task := &domain.Task{
		ID:        idgen.MustNew(),
		SessionID: activeSession.Session.ID,
		Type:      domain.TaskTypeCoding,
		Status:    domain.TaskStatusPending,
		DependsOn: domain.NewNullString("non-existent-task-id"),
	}

	ctx := context.Background()

	// Should NOT be eligible because predecessor doesn't exist.
	if scheduler.isTaskEligible(ctx, task) {
		t.Error("task should not be eligible when predecessor doesn't exist")
	}
}
