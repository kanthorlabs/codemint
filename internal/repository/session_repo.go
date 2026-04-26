package repository

import (
	"context"
	"errors"

	"codemint.kanthorlabs.com/internal/domain"
)

// ErrActiveSessionExists is returned when attempting to create a new active session
// for a project that already has one. The database enforces this via a partial
// unique index: CREATE UNIQUE INDEX idx_active_session ON session(project_id) WHERE status = 0.
var ErrActiveSessionExists = errors.New("repository: active session already exists for this project")

// SessionRepository defines persistence operations for Session entities.
// Sessions represent a single execution instance tied to a project.
type SessionRepository interface {
	// Create inserts a new session into the repository.
	// Returns ErrActiveSessionExists if the project already has an active session.
	Create(ctx context.Context, s *domain.Session) error

	// FindByID retrieves a session by its UUID primary key.
	// Returns nil and no error if the session does not exist.
	FindByID(ctx context.Context, id string) (*domain.Session, error)

	// FindActiveByProjectID retrieves the active session (status=0) for a project.
	// Returns nil and no error if no active session exists.
	FindActiveByProjectID(ctx context.Context, projectID string) (*domain.Session, error)

	// Archive transitions a session from Active (0) to Archived (1).
	// This releases the single-active-session constraint for the project.
	// Returns an error if the session does not exist or is already archived.
	Archive(ctx context.Context, id string) error

	// ListByProjectID returns all sessions for a project, ordered by ID (creation order).
	ListByProjectID(ctx context.Context, projectID string) ([]*domain.Session, error)
}
