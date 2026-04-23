// Package repository defines the data access interfaces for CodeMint domain entities.
package repository

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// TaskRepository defines atomic persistence operations for Task entities,
// enforcing the task state machine transitions defined in User Story 1.5.
type TaskRepository interface {
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
}
