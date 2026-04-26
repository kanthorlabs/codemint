package repository

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// TaskRepository defines atomic persistence operations for Task entities,
// enforcing the task state machine transitions defined in User Story 1.5.
type TaskRepository interface {
	// Create inserts a new task into the repository.
	Create(ctx context.Context, t *domain.Task) error

	// Next returns the first actionable task in the given session, ordered
	// by (seq_epic, seq_story, seq_task) ASC. Only tasks with status
	// Pending (0) or Awaiting (2) are considered.
	// Returns nil, nil when no actionable task exists.
	Next(ctx context.Context, sessionID string) (*domain.Task, error)

	// Claim atomically transitions a Pending task to Processing (1).
	// It uses a BEGIN IMMEDIATE transaction to prevent concurrent double-claims.
	// Returns an error if the task is not in Pending state.
	Claim(ctx context.Context, taskID string) error

	// UpdateStatus updates the status and output of a task, enforcing the
	// state machine: terminal states (>= Success/3) are immutable.
	// Returns an error if the transition is invalid or the task is already terminal.
	UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error

	// FindInterrupted returns all tasks in the given session that are stuck
	// in the Processing (1) state, indicating a possible crash mid-execution.
	// This method is read-only; it does not modify any task state.
	FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error)

	// FindByID returns the Task with the given ID, or an error if it does not
	// exist. This is used by review command handlers to validate pre-conditions
	// before delegating to the CodingAgent.
	FindByID(ctx context.Context, taskID string) (*domain.Task, error)

	// UpdateTaskStatus transitions a task to the given status, enforcing the
	// state machine defined in validFromStates. Unlike UpdateStatus it does not
	// modify the task output field, making it suitable for pure state transitions
	// such as those triggered by the Accept/Revert review commands.
	UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error

	// UpdateAssignee reassigns a task to a different agent. This is used by the
	// crash fallback flow (Story 1.9) to hand a failed task back to the human.
	UpdateAssignee(ctx context.Context, taskID string, assigneeID string) error

	// ListCoordinationAfter returns all Coordination tasks (type=3) in the session
	// with IDs greater than afterTaskID, ordered by ID (ascending).
	// Used to show "missed activity" when a client reclaims a session.
	// If afterTaskID is empty, returns all Coordination tasks.
	ListCoordinationAfter(ctx context.Context, sessionID string, afterTaskID string) ([]*domain.Task, error)
}
