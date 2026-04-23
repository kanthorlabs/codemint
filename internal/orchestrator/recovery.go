// Package orchestrator contains startup and recovery logic for the CodeMint
// task execution engine.
package orchestrator

import (
	"context"
	"fmt"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/workspace"
)

// RecoveryIntent describes a task that was interrupted mid-execution and
// requires manual intervention because the workspace is dirty.
type RecoveryIntent struct {
	Task    *domain.Task
	Message string
}

// Recovery handles interrupted tasks discovered at startup.
// For each Processing task it checks whether the workspace is clean:
//   - Clean workspace  → the task is safely reset to Pending.
//   - Dirty workspace  → a RecoveryIntent is emitted for the caller to surface
//     to the user via the UI mediator.
type Recovery struct {
	taskRepo repository.TaskRepository
	verifier *workspace.Verifier
}

// NewRecovery constructs a Recovery handler.
func NewRecovery(taskRepo repository.TaskRepository, verifier *workspace.Verifier) *Recovery {
	return &Recovery{taskRepo: taskRepo, verifier: verifier}
}

// Run inspects the given session for interrupted tasks and either resets them
// or returns RecoveryIntents for user-facing resolution.
func (r *Recovery) Run(ctx context.Context, sessionID string) ([]RecoveryIntent, error) {
	interrupted, err := r.taskRepo.FindInterrupted(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: find interrupted tasks: %w", err)
	}
	if len(interrupted) == 0 {
		return nil, nil
	}

	dirty, err := r.verifier.IsDirty()
	if err != nil {
		return nil, fmt.Errorf("orchestrator: check workspace: %w", err)
	}

	if dirty {
		intents := make([]RecoveryIntent, len(interrupted))
		for i, t := range interrupted {
			intents[i] = RecoveryIntent{
				Task: t,
				Message: fmt.Sprintf(
					"task %q was interrupted while the workspace is dirty; manual resolution required",
					t.ID,
				),
			}
		}
		return intents, nil
	}

	// Workspace is clean — safely reset each interrupted task to Pending.
	for _, t := range interrupted {
		if err := r.taskRepo.UpdateStatus(ctx, t.ID, domain.TaskStatusPending, ""); err != nil {
			return nil, fmt.Errorf("orchestrator: reset interrupted task %q: %w", t.ID, err)
		}
	}
	return nil, nil
}
