package repository

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// WorkflowRepository defines persistence operations for Workflow entities.
// Workflows group related tasks within a session and track execution progress.
type WorkflowRepository interface {
	// Create inserts a new workflow into the repository.
	Create(ctx context.Context, w *domain.Workflow) error

	// FindByID retrieves a workflow by its UUID primary key.
	// Returns nil and no error if the workflow does not exist.
	FindByID(ctx context.Context, id string) (*domain.Workflow, error)

	// GetActiveForSession returns the currently active workflow execution for a session.
	// A workflow is active when its status is WorkflowStatusActive (0).
	// Returns nil and no error if no active workflow exists.
	GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error)

	// UpdateProgress updates the current epic/story position for a workflow.
	// This is called by the Scheduler as tasks complete.
	UpdateProgress(ctx context.Context, id, epicID, storyID string) error

	// MarkCompleted sets the workflow status to Completed and records the completed_at timestamp.
	MarkCompleted(ctx context.Context, id string) error

	// MarkCancelled sets the workflow status to Cancelled.
	MarkCancelled(ctx context.Context, id string) error

	// ListByFilePath returns all workflow executions for a specific workflow file.
	// Useful for viewing execution history of a workflow.
	ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error)

	// ListBySession returns all workflows for a session, ordered by started_at descending.
	ListBySession(ctx context.Context, sessionID string) ([]*domain.Workflow, error)
}
