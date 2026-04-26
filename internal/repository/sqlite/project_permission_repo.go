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

// projectPermissionRepo is the SQLite implementation of
// repository.ProjectPermissionRepository.
type projectPermissionRepo struct {
	db *sqlx.DB
}

// NewProjectPermissionRepo constructs a ProjectPermissionRepository backed
// by the given SQLite connection.
func NewProjectPermissionRepo(db *sqlx.DB) repository.ProjectPermissionRepository {
	return &projectPermissionRepo{db: db}
}

// Compile-time interface satisfaction check.
var _ repository.ProjectPermissionRepository = (*projectPermissionRepo)(nil)

// FindByProjectID returns the permission record for the given project.
// Returns nil, nil when no permission record exists (permissive default).
func (r *projectPermissionRepo) FindByProjectID(ctx context.Context, projectID string) (*domain.ProjectPermission, error) {
	const query = `
		SELECT id, project_id, allowed_commands, allowed_directories, blocked_commands
		FROM project_permission
		WHERE project_id = ?`

	var perm domain.ProjectPermission
	err := r.db.GetContext(ctx, &perm, query, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find permission for project %q: %w", projectID, err)
	}
	return &perm, nil
}

// Upsert creates or updates a permission record for a project. Uses SQLite's
// INSERT ... ON CONFLICT DO UPDATE to enable idempotent permission updates.
func (r *projectPermissionRepo) Upsert(ctx context.Context, perm *domain.ProjectPermission) error {
	const query = `
		INSERT INTO project_permission (id, project_id, allowed_commands, allowed_directories, blocked_commands)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			allowed_commands = excluded.allowed_commands,
			allowed_directories = excluded.allowed_directories,
			blocked_commands = excluded.blocked_commands`

	_, err := r.db.ExecContext(ctx, query,
		perm.ID,
		perm.ProjectID,
		perm.AllowedCommands,
		perm.AllowedDirectories,
		perm.BlockedCommands,
	)
	if err != nil {
		return fmt.Errorf("sqlite: upsert permission for project %q: %w", perm.ProjectID, err)
	}
	return nil
}
