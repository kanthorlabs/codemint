package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// workflowRepo is the SQLite implementation of repository.WorkflowRepository.
type workflowRepo struct {
	db *sqlx.DB
}

// NewWorkflowRepo constructs a WorkflowRepository backed by the given SQLite connection.
func NewWorkflowRepo(db *sqlx.DB) repository.WorkflowRepository {
	return &workflowRepo{db: db}
}

// Compile-time check that workflowRepo implements repository.WorkflowRepository.
var _ repository.WorkflowRepository = (*workflowRepo)(nil)

// Create inserts a new workflow into the database.
func (r *workflowRepo) Create(ctx context.Context, w *domain.Workflow) error {
	const query = `INSERT INTO workflow (id, session_id, type, file_path, current_epic_id, current_story_id, started_at, completed_at, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		w.ID,
		w.SessionID,
		w.Type,
		w.FilePath,
		w.CurrentEpicID,
		w.CurrentStoryID,
		w.StartedAt,
		w.CompletedAt,
		int(w.Status),
	)
	if err != nil {
		return fmt.Errorf("sqlite: create workflow: %w", err)
	}
	return nil
}

// FindByID retrieves a workflow by its UUID primary key.
// Returns nil, nil when no matching row exists.
func (r *workflowRepo) FindByID(ctx context.Context, id string) (*domain.Workflow, error) {
	var w domain.Workflow
	const query = `SELECT id, session_id, type, file_path, current_epic_id, current_story_id, started_at, completed_at, status, goal_text, success_criteria, chosen_option FROM workflow WHERE id = ?`
	err := r.db.GetContext(ctx, &w, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find workflow by id %q: %w", id, err)
	}
	return &w, nil
}

// GetActiveForSession returns the currently active workflow execution for a session.
// Returns nil, nil when no active workflow exists.
func (r *workflowRepo) GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error) {
	var w domain.Workflow
	const query = `
		SELECT id, session_id, type, file_path, current_epic_id, current_story_id, started_at, completed_at, status, goal_text, success_criteria, chosen_option 
		FROM workflow 
		WHERE session_id = ? AND status = ? 
		ORDER BY started_at DESC 
		LIMIT 1
	`
	err := r.db.GetContext(ctx, &w, query, sessionID, int(domain.WorkflowStatusActive))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get active workflow for session %q: %w", sessionID, err)
	}
	return &w, nil
}

// UpdateProgress updates the current epic/story position for a workflow.
func (r *workflowRepo) UpdateProgress(ctx context.Context, id, epicID, storyID string) error {
	const query = `UPDATE workflow SET current_epic_id = ?, current_story_id = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, epicID, storyID, id)
	if err != nil {
		return fmt.Errorf("sqlite: update workflow progress %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: update workflow progress %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: workflow %q not found", id)
	}
	return nil
}

// MarkCompleted sets the workflow status to Completed and records the completed_at timestamp.
func (r *workflowRepo) MarkCompleted(ctx context.Context, id string) error {
	const query = `UPDATE workflow SET status = ?, completed_at = ? WHERE id = ? AND status = ?`
	completedAt := time.Now().Unix()
	result, err := r.db.ExecContext(ctx, query,
		int(domain.WorkflowStatusCompleted),
		completedAt,
		id,
		int(domain.WorkflowStatusActive),
	)
	if err != nil {
		return fmt.Errorf("sqlite: mark workflow completed %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: mark workflow completed %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: workflow %q not found or not active", id)
	}
	return nil
}

// MarkCancelled sets the workflow status to Cancelled.
func (r *workflowRepo) MarkCancelled(ctx context.Context, id string) error {
	const query = `UPDATE workflow SET status = ? WHERE id = ? AND status = ?`
	result, err := r.db.ExecContext(ctx, query,
		int(domain.WorkflowStatusCancelled),
		id,
		int(domain.WorkflowStatusActive),
	)
	if err != nil {
		return fmt.Errorf("sqlite: mark workflow cancelled %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: mark workflow cancelled %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: workflow %q not found or not active", id)
	}
	return nil
}

// ListByFilePath returns all workflow executions for a specific workflow file.
func (r *workflowRepo) ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error) {
	var workflows []*domain.Workflow
	const query = `
		SELECT id, session_id, type, file_path, current_epic_id, current_story_id, started_at, completed_at, status, goal_text, success_criteria, chosen_option 
		FROM workflow 
		WHERE file_path = ? 
		ORDER BY started_at DESC
	`
	err := r.db.SelectContext(ctx, &workflows, query, filePath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list workflows by file path %q: %w", filePath, err)
	}
	return workflows, nil
}

// ListBySession returns all workflows for a session, ordered by started_at descending.
func (r *workflowRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Workflow, error) {
	var workflows []*domain.Workflow
	const query = `
		SELECT id, session_id, type, file_path, current_epic_id, current_story_id, started_at, completed_at, status, goal_text, success_criteria, chosen_option 
		FROM workflow 
		WHERE session_id = ? 
		ORDER BY started_at DESC
	`
	err := r.db.SelectContext(ctx, &workflows, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list workflows by session %q: %w", sessionID, err)
	}
	return workflows, nil
}

// LockGoal writes goal_text and success_criteria for a workflow.
// Returns an error if these fields are already set (one-shot lock semantics).
func (r *workflowRepo) LockGoal(ctx context.Context, workflowID, goalText, criteriaJSON string) error {
	const query = `UPDATE workflow SET goal_text = ?, success_criteria = ? WHERE id = ? AND goal_text IS NULL`
	result, err := r.db.ExecContext(ctx, query, goalText, criteriaJSON, workflowID)
	if err != nil {
		return fmt.Errorf("sqlite: lock goal for workflow %q: %w", workflowID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: lock goal for workflow %q: %w", workflowID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("goal already locked; use /revise-goal to change")
	}
	return nil
}

// LockChosenOption writes chosen_option for a workflow.
// Returns an error if the field is already set (one-shot lock semantics).
func (r *workflowRepo) LockChosenOption(ctx context.Context, workflowID, optionJSON string) error {
	const query = `UPDATE workflow SET chosen_option = ? WHERE id = ? AND chosen_option IS NULL`
	result, err := r.db.ExecContext(ctx, query, optionJSON, workflowID)
	if err != nil {
		return fmt.Errorf("sqlite: lock chosen option for workflow %q: %w", workflowID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: lock chosen option for workflow %q: %w", workflowID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("option already chosen for this workflow")
	}
	return nil
}

// ResetGOROW clears goal_text, success_criteria, and chosen_option back to NULL.
// Used by /modify to loop back to Goal Capture.
func (r *workflowRepo) ResetGOROW(ctx context.Context, workflowID string) error {
	const query = `UPDATE workflow SET goal_text = NULL, success_criteria = NULL, chosen_option = NULL WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, workflowID)
	if err != nil {
		return fmt.Errorf("sqlite: reset GOROW for workflow %q: %w", workflowID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: reset GOROW for workflow %q: %w", workflowID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: workflow %q not found", workflowID)
	}
	return nil
}
