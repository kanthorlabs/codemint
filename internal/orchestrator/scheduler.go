// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
	"context"
	"log/slog"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// Scheduler coordinates task execution with story-boundary detection.
// It tracks the last seq_story value and calls ResetContext when transitioning
// to a new User Story, keeping token usage lean without killing the ACP binary.
type Scheduler struct {
	taskRepo      repository.TaskRepository
	executor      *Executor
	acpRegistry   *acp.Registry
	activeSession *ActiveSession

	// lastSeqStory tracks the seq_story of the last processed task.
	// Reset to -1 on scheduler initialization to force reset on first task.
	lastSeqStory int

	// advanceCh receives signals from StatusMapper when a task completes successfully.
	// When a signal is received, the scheduler advances to the next pending task.
	advanceCh <-chan struct{}
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

	// Fetch the next pending task ordered by (seq_epic, seq_story, seq_task).
	task, err := s.taskRepo.Next(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, nil // No pending task
	}

	// Check for story boundary and reset context if needed.
	if err := s.maybeResetContext(ctx, task); err != nil {
		slog.Error("scheduler: context reset failed",
			"task_id", task.ID,
			"seq_story", task.SeqStory,
			"error", err,
		)
		// Continue with task execution despite reset failure.
		// The agent will just use more tokens.
	}

	// Set the current task ID on the worker for StatusMapper (Story 3.7).
	s.setCurrentTaskOnWorker(task.ID)

	// Hand off to the executor.
	if err := s.executor.ExecuteTask(ctx, task); err != nil {
		// Clear the current task on error.
		s.setCurrentTaskOnWorker("")
		return task, err
	}

	return task, nil
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

	slog.Info("scheduler: story boundary reset",
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

// Run starts the scheduler loop that continuously processes tasks.
// It runs until the context is cancelled. When advanceCh is provided,
// the scheduler waits for signals from StatusMapper before processing
// the next task (Task 3.7.3).
func (s *Scheduler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			task, err := s.ProcessNextTask(ctx)
			if err != nil {
				slog.Error("scheduler: task processing error",
					"error", err,
				)
				// Continue processing other tasks.
				continue
			}
			if task == nil {
				// No pending tasks.
				if s.advanceCh != nil {
					// Wait for advance signal or context cancellation.
					slog.Info("acp scheduler idle")
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-s.advanceCh:
						// Received signal to advance, loop continues.
						continue
					}
				}
				// No advance channel, return and let caller decide.
				return nil
			}

			// If we have an advance channel, wait for the task to complete
			// before processing the next one (signal comes from StatusMapper).
			if s.advanceCh != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-s.advanceCh:
					// Task completed, loop continues to next task.
				}
			}
		}
	}
}
