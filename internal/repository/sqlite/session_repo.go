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

// sessionRepo is the SQLite implementation of repository.SessionRepository.
type sessionRepo struct {
	db *sqlx.DB
}

// NewSessionRepo constructs a SessionRepository backed by the given SQLite connection.
func NewSessionRepo(db *sqlx.DB) repository.SessionRepository {
	return &sessionRepo{db: db}
}

// Compile-time check that sessionRepo implements repository.SessionRepository.
var _ repository.SessionRepository = (*sessionRepo)(nil)

// Create inserts a new session into the database.
// The partial unique index idx_active_session enforces at most one active session per project.
// Returns ErrActiveSessionExists if the project already has an active session.
func (r *sessionRepo) Create(ctx context.Context, s *domain.Session) error {
	const query = `INSERT INTO session (id, project_id, status, active_client, last_activity_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, s.ID, s.ProjectID, int(s.Status), s.ActiveClient, s.LastActivityAt)
	if err != nil {
		// SQLite returns "UNIQUE constraint failed" for the partial index violation.
		if isUniqueConstraintError(err) {
			return repository.ErrActiveSessionExists
		}
		return fmt.Errorf("sqlite: create session for project %q: %w", s.ProjectID, err)
	}
	return nil
}

// FindByID retrieves a session by its UUID primary key.
// Returns nil, nil when no matching row exists.
func (r *sessionRepo) FindByID(ctx context.Context, id string) (*domain.Session, error) {
	var s domain.Session
	err := r.db.GetContext(ctx, &s, `SELECT id, project_id, status, active_client, last_activity_at FROM session WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find session by id %q: %w", id, err)
	}
	return &s, nil
}

// FindActiveByProjectID retrieves the active session (status=0) for a project.
// Returns nil, nil when no active session exists.
func (r *sessionRepo) FindActiveByProjectID(ctx context.Context, projectID string) (*domain.Session, error) {
	var s domain.Session
	err := r.db.GetContext(ctx, &s,
		`SELECT id, project_id, status, active_client, last_activity_at FROM session WHERE project_id = ? AND status = ?`,
		projectID, int(domain.SessionStatusActive))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find active session for project %q: %w", projectID, err)
	}
	return &s, nil
}

// Archive transitions a session from Active (0) to Archived (1).
// This releases the single-active-session constraint for the project.
// Returns an error if the session does not exist or is already archived.
func (r *sessionRepo) Archive(ctx context.Context, id string) error {
	const query = `UPDATE session SET status = ? WHERE id = ? AND status = ?`
	result, err := r.db.ExecContext(ctx, query,
		int(domain.SessionStatusArchived), id, int(domain.SessionStatusActive))
	if err != nil {
		return fmt.Errorf("sqlite: archive session %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: archive session %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: session %q not found or already archived", id)
	}
	return nil
}

// ListByProjectID returns all sessions for a project, ordered by ID (creation order).
func (r *sessionRepo) ListByProjectID(ctx context.Context, projectID string) ([]*domain.Session, error) {
	var sessions []*domain.Session
	err := r.db.SelectContext(ctx, &sessions,
		`SELECT id, project_id, status, active_client, last_activity_at FROM session WHERE project_id = ? ORDER BY id`,
		projectID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sessions for project %q: %w", projectID, err)
	}
	return sessions, nil
}

// SaveState updates the session's active_client and last_activity_at columns.
// Used for client ownership tracking and heartbeat updates.
func (r *sessionRepo) SaveState(ctx context.Context, sessionID, activeClient string, lastActivityAt int64) error {
	const query = `UPDATE session SET active_client = ?, last_activity_at = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, activeClient, lastActivityAt, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: save session state %q: %w", sessionID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: save session state %q: %w", sessionID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: session %q not found", sessionID)
	}
	return nil
}

// GetMostRecentActive returns the most recently active session across all projects.
// Returns nil, nil if no active sessions exist.
func (r *sessionRepo) GetMostRecentActive(ctx context.Context) (*domain.Session, error) {
	var s domain.Session
	const query = `
		SELECT id, project_id, status, active_client, last_activity_at 
		FROM session 
		WHERE status = ? 
		ORDER BY last_activity_at DESC NULLS LAST 
		LIMIT 1
	`
	err := r.db.GetContext(ctx, &s, query, int(domain.SessionStatusActive))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get most recent active session: %w", err)
	}
	return &s, nil
}

// ClearOwnership sets active_client to NULL for the given session.
// Used when a client releases a session or switches to another session.
func (r *sessionRepo) ClearOwnership(ctx context.Context, sessionID string) error {
	const query = `UPDATE session SET active_client = NULL WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: clear session ownership %q: %w", sessionID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: clear session ownership %q: %w", sessionID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("sqlite: session %q not found", sessionID)
	}
	return nil
}

// ListActive returns all active sessions (status=0) ordered by last_activity_at descending.
func (r *sessionRepo) ListActive(ctx context.Context) ([]*domain.Session, error) {
	var sessions []*domain.Session
	const query = `
		SELECT id, project_id, status, active_client, last_activity_at 
		FROM session 
		WHERE status = ? 
		ORDER BY last_activity_at DESC NULLS LAST
	`
	err := r.db.SelectContext(ctx, &sessions, query, int(domain.SessionStatusActive))
	if err != nil {
		return nil, fmt.Errorf("sqlite: list active sessions: %w", err)
	}
	return sessions, nil
}
