// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// ErrSchedulerAlreadyRunning is returned when Run is called while the scheduler
// is already running. Only one Run loop is permitted at a time.
var ErrSchedulerAlreadyRunning = errors.New("scheduler: already running")

// Scheduler coordinates task execution with story-boundary detection.
// It tracks the last seq_story value and calls ResetContext when transitioning
// to a new User Story, keeping token usage lean without killing the ACP binary.
//
// The scheduler implements Task 3.13.2-3.13.5:
//   - Continuous loop pulling pending tasks in strict (seq_epic, seq_story, seq_task) order
//   - Awaiting state support: pauses when task transitions to awaiting
//   - Exponential backoff on consecutive DB errors
//   - Sequential execution guardrail: only one Run loop at a time
//
// Workflow progress tracking (Story 2.0.3):
//   - Updates current_epic_id and current_story_id as tasks complete
//   - Marks workflow as completed when all tasks in session are done
type Scheduler struct {
	taskRepo      repository.TaskRepository
	workflowRepo  repository.WorkflowRepository
	executor      *Executor
	acpRegistry   *acp.Registry
	acpRuntime    *Runtime
	activeSession *ActiveSession

	// lastSeqStory tracks the seq_story of the last processed task.
	// Reset to -1 on scheduler initialization to force reset on first task.
	lastSeqStory int

	// advanceCh receives signals from StatusMapper when a task completes successfully.
	// When a signal is received, the scheduler advances to the next pending task.
	advanceCh <-chan struct{}

	// awaitingCh receives signals when an awaiting task is resolved.
	// This is triggered by /approve, /deny, or /yolo commands.
	awaitingCh chan struct{}

	// running indicates whether the scheduler loop is currently running.
	// Used to enforce the sequential execution guardrail (Task 3.13.5).
	running atomic.Bool

	// mu protects the dispatch step to ensure sequential execution.
	mu sync.Mutex

	// logger for scheduler operations.
	logger *slog.Logger
}

// SchedulerConfig holds configuration for creating a new Scheduler.
type SchedulerConfig struct {
	TaskRepo      repository.TaskRepository
	WorkflowRepo  repository.WorkflowRepository
	Executor      *Executor
	ACPRegistry   *acp.Registry
	ACPRuntime    *Runtime
	ActiveSession *ActiveSession
	AdvanceCh     <-chan struct{}
	Logger        *slog.Logger
}

// NewScheduler creates a new Scheduler with the provided dependencies.
// The advanceCh parameter receives signals to advance to the next task (from StatusMapper).
// Pass nil if you want the scheduler to run in single-task mode.
func NewScheduler(
	taskRepo repository.TaskRepository,
	executor *Executor,
	acpRegistry *acp.Registry,
	activeSession *ActiveSession,
	advanceCh <-chan struct{},
) *Scheduler {
	return &Scheduler{
		taskRepo:      taskRepo,
		executor:      executor,
		acpRegistry:   acpRegistry,
		activeSession: activeSession,
		lastSeqStory:  -1, // Force reset on first task after restart
		advanceCh:     advanceCh,
		awaitingCh:    make(chan struct{}, 1),
		logger:        slog.Default(),
	}
}

// NewSchedulerWithConfig creates a new Scheduler with the provided configuration.
// This is the preferred constructor for production use.
func NewSchedulerWithConfig(cfg SchedulerConfig) *Scheduler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		taskRepo:      cfg.TaskRepo,
		workflowRepo:  cfg.WorkflowRepo,
		executor:      cfg.Executor,
		acpRegistry:   cfg.ACPRegistry,
		acpRuntime:    cfg.ACPRuntime,
		activeSession: cfg.ActiveSession,
		lastSeqStory:  -1,
		advanceCh:     cfg.AdvanceCh,
		awaitingCh:    make(chan struct{}, 1),
		logger:        logger,
	}
}

// IsRunning returns true if the scheduler loop is currently running.
// Used by /acp-status to display scheduler state.
func (s *Scheduler) IsRunning() bool {
	return s.running.Load()
}

// ResolveAwaiting signals that an awaiting task has been resolved.
// This should be called by /approve, /deny, or /yolo command handlers.
func (s *Scheduler) ResolveAwaiting() {
	select {
	case s.awaitingCh <- struct{}{}:
	default:
		// Channel already has a signal, coalesce.
	}
}

// ProcessNextTask fetches the next pending task for the session, performs
// story-boundary detection, and hands the task off to the executor.
// Returns the processed task or nil if no task is available.
func (s *Scheduler) ProcessNextTask(ctx context.Context) (*domain.Task, error) {
	sessionID := s.activeSession.GetSessionID()
	if sessionID == "" {
		return nil, nil
	}

	// Fetch all pending tasks to evaluate eligibility.
	pendingTasks, err := s.taskRepo.ListPending(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(pendingTasks) == 0 {
		return nil, nil // No pending tasks
	}

	// Find the first eligible task.
	var task *domain.Task
	for _, t := range pendingTasks {
		if s.isTaskEligible(ctx, t) {
			task = t
			break
		}
	}
	if task == nil {
		return nil, nil // No eligible task
	}

	// Check for story boundary and reset context if needed.
	if err := s.maybeResetContext(ctx, task); err != nil {
		s.logger.Error("scheduler: context reset failed",
			"task_id", task.ID,
			"seq_story", task.SeqStory,
			"error", err,
		)
		// Continue with task execution despite reset failure.
		// The agent will just use more tokens.
	}

	// Set the current task ID on the worker for StatusMapper (Story 3.7).
	s.setCurrentTaskOnWorker(task.ID)

	// Hand off to the executor with task-type routing.
	if err := s.executor.Execute(ctx, s.activeSession, task); err != nil {
		// Clear the current task on error.
		s.setCurrentTaskOnWorker("")
		return task, err
	}

	return task, nil
}

// isTaskEligible checks if a task is eligible for execution based on its
// depends_on and condition fields (Story 2.0.2).
//
// Eligibility rules:
//   - Task with no depends_on is always eligible (seq order alone).
//   - Task with depends_on waits for predecessor to reach terminal state.
//   - Task with condition only eligible when predecessor matches that specific status.
func (s *Scheduler) isTaskEligible(ctx context.Context, task *domain.Task) bool {
	// No dependency - eligible based on seq order alone.
	if !task.DependsOn.Valid {
		return true
	}

	// Fetch the predecessor task.
	predecessor, err := s.taskRepo.FindByID(ctx, task.DependsOn.String)
	if err != nil {
		s.logger.Warn("scheduler: failed to fetch predecessor task",
			"task_id", task.ID,
			"depends_on", task.DependsOn.String,
			"error", err,
		)
		return false
	}
	if predecessor == nil {
		s.logger.Warn("scheduler: predecessor task not found",
			"task_id", task.ID,
			"depends_on", task.DependsOn.String,
		)
		return false
	}

	// Check if predecessor is in terminal state.
	if !predecessor.Status.IsTerminal() {
		return false
	}

	// No specific condition - any terminal state is acceptable.
	if !task.Condition.Valid {
		return true
	}

	// Check if predecessor status matches the required condition.
	requiredStatus := domain.TaskStatus(task.Condition.Int64)
	return predecessor.Status == requiredStatus
}

// maybeResetContext checks if the task represents a story boundary transition
// and calls ResetContext on the ACP worker if so.
// Skip reset for Coordination and Confirmation tasks since they don't consume agent context.
func (s *Scheduler) maybeResetContext(ctx context.Context, task *domain.Task) error {
	// Skip reset for Coordination and Confirmation tasks.
	if task.Type == domain.TaskTypeCoordination || task.Type == domain.TaskTypeConfirmation {
		return nil
	}

	// Check if we're transitioning to a new story.
	if task.SeqStory == s.lastSeqStory {
		return nil // Same story, no reset needed
	}

	// Get the ACP worker for this session.
	session := s.activeSession.GetSession()
	if session == nil {
		// No session, just update lastSeqStory and continue.
		s.lastSeqStory = task.SeqStory
		return nil
	}

	// Check if we have a registry.
	if s.acpRegistry == nil {
		// No registry, just update lastSeqStory and continue.
		s.lastSeqStory = task.SeqStory
		return nil
	}

	worker, ok := s.acpRegistry.Get(session.ID)
	if !ok || !worker.Alive() {
		// No worker or worker is dead, update lastSeqStory and continue.
		s.lastSeqStory = task.SeqStory
		return nil
	}

	// Reset the context.
	oldSessionID := s.activeSession.GetACPSessionID()
	newSessionID, err := worker.ResetContext(ctx, oldSessionID)
	if err != nil {
		return err
	}

	// Update the stored ACP session ID.
	s.activeSession.SetACPSessionID(newSessionID)

	// Update lastSeqStory only after successful reset.
	s.lastSeqStory = task.SeqStory

	s.logger.Info("scheduler: story boundary reset",
		"task_id", task.ID,
		"old_seq_story", s.lastSeqStory,
		"new_seq_story", task.SeqStory,
	)

	return nil
}

// setCurrentTaskOnWorker sets the current task ID on the ACP worker.
// This allows StatusMapper to associate events with the correct task.
func (s *Scheduler) setCurrentTaskOnWorker(taskID string) {
	session := s.activeSession.GetSession()
	if session == nil || s.acpRegistry == nil {
		return
	}

	worker, ok := s.acpRegistry.Get(session.ID)
	if !ok || !worker.Alive() {
		return
	}

	worker.SetCurrentTask(taskID)
}

// ensureWorkerAttached lazily spawns the ACP worker if not already attached.
// This implements the lazy spawn requirement from Task 3.13.2.
func (s *Scheduler) ensureWorkerAttached(ctx context.Context) (*acp.Worker, error) {
	if s.acpRuntime == nil {
		return nil, nil
	}

	session := s.activeSession.GetSession()
	project := s.activeSession.GetProject()
	if session == nil || project == nil {
		return nil, nil
	}

	return s.acpRuntime.AttachWorker(ctx, session, project)
}

// backoffDuration calculates the exponential backoff duration.
// Starts at 500ms and doubles up to a maximum of 5 seconds.
func backoffDuration(consecutiveErrors int) time.Duration {
	base := 500 * time.Millisecond
	max := 5 * time.Second

	// Calculate 2^(consecutiveErrors-1) * base.
	duration := base
	for i := 1; i < consecutiveErrors && duration < max; i++ {
		duration *= 2
	}
	if duration > max {
		duration = max
	}
	return duration
}

// Run starts the scheduler loop that continuously processes tasks.
// It runs until the context is cancelled.
//
// This implements Task 3.13.2-3.13.5:
//   - Lazy worker spawn via runtime.AttachWorker
//   - Blocks on wakeupCh or a 1-minute fallback ticker when idle
//   - Blocks on awaitingCh when a task is in awaiting state
//   - Exponential backoff on consecutive DB errors
//   - Sequential execution guardrail (only one Run loop at a time)
//
// Returns ErrSchedulerAlreadyRunning if another Run loop is already active.
func (s *Scheduler) Run(ctx context.Context) error {
	// Task 3.13.5: Reject concurrent Run calls.
	if !s.running.CompareAndSwap(false, true) {
		return ErrSchedulerAlreadyRunning
	}
	defer s.running.Store(false)

	s.logger.Info("scheduler: starting")

	// Lazy spawn the ACP worker (Task 3.13.2).
	if _, err := s.ensureWorkerAttached(ctx); err != nil {
		s.logger.Error("scheduler: failed to attach worker",
			"error", err,
			"hint", "Check that the configured provider binary is working. Try running it manually: opencode acp",
		)
		// Continue without worker - tasks will fail but loop keeps running.
	}

	// Create fallback ticker (1 minute).
	fallbackTicker := time.NewTicker(time.Minute)
	defer fallbackTicker.Stop()

	var consecutiveErrors int

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler: shutting down")
			return ctx.Err()
		default:
		}

		// Task 3.13.5: Lock around dispatch step for sequential execution.
		s.mu.Lock()
		task, err := s.processWithBackoff(ctx, &consecutiveErrors)
		s.mu.Unlock()

		if err != nil {
			// If context is cancelled, exit.
			if ctx.Err() != nil {
				s.logger.Info("scheduler: shutting down")
				return ctx.Err()
			}
			// Otherwise continue with backoff (already applied in processWithBackoff).
			continue
		}

		if task == nil {
			// No pending tasks - wait for wakeup signal or fallback ticker.
			s.logger.Info("scheduler: idle, waiting for wakeup")
			select {
			case <-ctx.Done():
				s.logger.Info("scheduler: shutting down")
				return ctx.Err()
			case <-s.activeSession.WakeupCh():
				s.logger.Debug("scheduler: wakeup received")
			case <-fallbackTicker.C:
				s.logger.Debug("scheduler: fallback ticker fired")
			case <-s.advanceCh:
				s.logger.Debug("scheduler: advance signal received")
			}
			continue
		}

		// Task was dispatched. Check if it's in awaiting state.
		// Re-fetch the task to get the current status (executor may have changed it).
		currentTask, err := s.taskRepo.FindByID(ctx, task.ID)
		if err != nil {
			s.logger.Warn("scheduler: failed to re-fetch task status",
				"task_id", task.ID,
				"error", err,
			)
			// Continue to next iteration.
			continue
		}

		if currentTask != nil && currentTask.Status == domain.TaskStatusAwaiting {
			// Block until the awaiting task is resolved.
			s.logger.Info("scheduler: task awaiting approval",
				"task_id", task.ID,
			)
			select {
			case <-ctx.Done():
				s.logger.Info("scheduler: shutting down")
				return ctx.Err()
			case <-s.awaitingCh:
				s.logger.Info("scheduler: awaiting resolved, continuing",
					"task_id", task.ID,
				)
			case <-s.activeSession.WakeupCh():
				s.logger.Debug("scheduler: wakeup during await")
			}
		}

		// Wait for advance signal if provided (task completion from StatusMapper).
		if s.advanceCh != nil {
			select {
			case <-ctx.Done():
				s.logger.Info("scheduler: shutting down")
				return ctx.Err()
			case <-s.advanceCh:
				// Task completed, update workflow progress (Story 2.0.3).
				s.updateWorkflowProgress(ctx, task)
			}
		}
	}
}

// processWithBackoff processes the next task with exponential backoff on errors.
func (s *Scheduler) processWithBackoff(ctx context.Context, consecutiveErrors *int) (*domain.Task, error) {
	task, err := s.ProcessNextTask(ctx)
	if err != nil {
		*consecutiveErrors++
		backoff := backoffDuration(*consecutiveErrors)
		s.logger.Warn("scheduler: task processing error, backing off",
			"error", err,
			"consecutive_errors", *consecutiveErrors,
			"backoff", backoff,
		)

		// Sleep with context awareness.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		return nil, err
	}

	// Reset consecutive errors on success.
	*consecutiveErrors = 0
	return task, nil
}

// Restart stops the current scheduler loop and starts a new one for a different session.
// This should be called when /project-open switches to a new project.
func (s *Scheduler) Restart(ctx context.Context, newSession *ActiveSession) error {
	// The current Run loop will exit when its context is cancelled.
	// This method updates the active session so subsequent calls use the new session.
	s.activeSession = newSession
	s.lastSeqStory = -1 // Reset story tracking for new session.

	// Wake up the scheduler to pick up the new session.
	if newSession != nil {
		newSession.Wakeup()
	}

	return nil
}

// updateWorkflowProgress updates the workflow's current epic/story position
// based on the completed task, and marks the workflow as completed if all tasks are done.
// This implements Story 2.0.3 workflow execution state tracking.
func (s *Scheduler) updateWorkflowProgress(ctx context.Context, task *domain.Task) {
	if s.workflowRepo == nil {
		return
	}

	// Skip if task doesn't belong to a workflow.
	if !task.WorkflowID.Valid {
		return
	}

	workflowID := task.WorkflowID.String

	// Update the current epic/story position.
	epicID := fmt.Sprintf("epic-%d", task.SeqEpic)
	storyID := fmt.Sprintf("story-%d", task.SeqStory)

	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, epicID, storyID); err != nil {
		s.logger.Warn("scheduler: failed to update workflow progress",
			"workflow_id", workflowID,
			"task_id", task.ID,
			"error", err,
		)
		// Continue even if progress update fails - non-critical.
		return
	}

	s.logger.Debug("scheduler: updated workflow progress",
		"workflow_id", workflowID,
		"epic_id", epicID,
		"story_id", storyID,
	)

	// Check if all tasks in the workflow are complete.
	s.checkWorkflowCompletion(ctx, workflowID)
}

// checkWorkflowCompletion checks if all tasks in a workflow are in terminal state
// and marks the workflow as completed if so.
func (s *Scheduler) checkWorkflowCompletion(ctx context.Context, workflowID string) {
	sessionID := s.activeSession.GetSessionID()
	if sessionID == "" {
		return
	}

	// Get all tasks for the session that belong to this workflow.
	// We check if there are any pending tasks remaining.
	pendingTasks, err := s.taskRepo.ListPending(ctx, sessionID)
	if err != nil {
		s.logger.Warn("scheduler: failed to check workflow completion",
			"workflow_id", workflowID,
			"error", err,
		)
		return
	}

	// Check if any pending task belongs to this workflow.
	for _, t := range pendingTasks {
		if t.WorkflowID.Valid && t.WorkflowID.String == workflowID {
			// Still have pending tasks in this workflow.
			return
		}
	}

	// No more pending tasks in this workflow - mark as completed.
	if err := s.workflowRepo.MarkCompleted(ctx, workflowID); err != nil {
		s.logger.Warn("scheduler: failed to mark workflow completed",
			"workflow_id", workflowID,
			"error", err,
		)
		return
	}

	s.logger.Info("scheduler: workflow completed",
		"workflow_id", workflowID,
	)
}
