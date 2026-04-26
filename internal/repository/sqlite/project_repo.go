package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// projectRepo is the SQLite implementation of repository.ProjectRepository.
type projectRepo struct {
	db *sqlx.DB
}

// NewProjectRepo constructs a ProjectRepository backed by the given SQLite connection.
func NewProjectRepo(db *sqlx.DB) repository.ProjectRepository {
	return &projectRepo{db: db}
}

// Compile-time check that projectRepo implements repository.ProjectRepository.
var _ repository.ProjectRepository = (*projectRepo)(nil)

// Create inserts a new project into the database.
// Returns an error if a project with the same name already exists (UNIQUE constraint).
func (r *projectRepo) Create(ctx context.Context, p *domain.Project) error {
	const query = `INSERT INTO project (id, name, working_dir, yolo_mode) VALUES (?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, p.ID, p.Name, p.WorkingDir, p.YoloMode)
	if err != nil {
		// SQLite returns "UNIQUE constraint failed" for duplicate name.
		if isUniqueConstraintError(err) {
			return fmt.Errorf("sqlite: project %q already exists: %w", p.Name, err)
		}
		return fmt.Errorf("sqlite: create project %q: %w", p.Name, err)
	}
	return nil
}

// FindByID retrieves a project by its UUID primary key.
// Returns nil, nil when no matching row exists.
func (r *projectRepo) FindByID(ctx context.Context, id string) (*domain.Project, error) {
	var p domain.Project
	err := r.db.GetContext(ctx, &p, `SELECT id, name, working_dir, yolo_mode FROM project WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find project by id %q: %w", id, err)
	}
	return &p, nil
}

// FindByName retrieves a project by its unique name (case-sensitive).
// Returns nil, nil when no matching row exists.
func (r *projectRepo) FindByName(ctx context.Context, name string) (*domain.Project, error) {
	var p domain.Project
	err := r.db.GetContext(ctx, &p, `SELECT id, name, working_dir, yolo_mode FROM project WHERE name = ?`, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find project by name %q: %w", name, err)
	}
	return &p, nil
}

// Update modifies an existing project's name, working_dir, and yolo_mode.
// Returns an error if the project does not exist.
func (r *projectRepo) Update(ctx context.Context, p *domain.Project) error {
	const query = `UPDATE project SET name = ?, working_dir = ?, yolo_mode = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, p.Name, p.WorkingDir, p.YoloMode, p.ID)
	if err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("sqlite: project name %q already exists: %w", p.Name, err)
		}
		return fmt.Errorf("sqlite: update project %q: %w", p.ID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: update project %q: %w", p.ID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: project %q not found", p.ID)
	}
	return nil
}

// Delete removes a project by ID. CASCADE constraints in the schema handle
// deletion of related sessions, tasks, and permissions.
// Returns an error if the project does not exist.
func (r *projectRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM project WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete project %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete project %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: project %q not found", id)
	}
	return nil
}

// List returns all projects in the repository, ordered by name.
func (r *projectRepo) List(ctx context.Context) ([]*domain.Project, error) {
	var projects []*domain.Project
	err := r.db.SelectContext(ctx, &projects, `SELECT id, name, working_dir, yolo_mode FROM project ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list projects: %w", err)
	}
	return projects, nil
}

// isUniqueConstraintError checks if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite returns error messages containing "UNIQUE constraint failed"
	return errors.Is(err, sql.ErrNoRows) == false && 
		(contains(err.Error(), "UNIQUE constraint failed") ||
		 contains(err.Error(), "constraint failed: UNIQUE"))
}

// contains is a simple substring check helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

// findSubstring returns the index of substr in s, or -1 if not found.
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
