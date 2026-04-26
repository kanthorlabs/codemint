// Package repository defines the data access interfaces for CodeMint domain entities.
//
// # Error Handling Conventions
//
// Repository methods follow these conventions for error handling:
//
//   - Find* methods return (nil, nil) when the requested entity does not exist.
//     This is not an error; the caller should check for nil before using the result.
//
//   - Create methods return wrapped errors with entity context (e.g., "sqlite: project %q: %w").
//     Duplicate key violations return specific errors that can be checked with errors.Is.
//
//   - Sentinel errors (ErrActiveSessionExists, ErrInvalidTransition) should be
//     checked with errors.Is to handle specific failure conditions.
package repository

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// ProjectRepository defines persistence operations for Project entities.
// Projects are the top-level context entity representing a codebase workspace.
type ProjectRepository interface {
	// Create inserts a new project into the repository.
	// Returns an error if a project with the same name already exists.
	Create(ctx context.Context, p *domain.Project) error

	// FindByID retrieves a project by its UUID primary key.
	// Returns nil and no error if the project does not exist.
	FindByID(ctx context.Context, id string) (*domain.Project, error)

	// FindByName retrieves a project by its unique name.
	// Returns nil and no error if the project does not exist.
	FindByName(ctx context.Context, name string) (*domain.Project, error)

	// Update modifies an existing project's name, working_dir, and yolo_mode.
	// Returns an error if the project does not exist.
	Update(ctx context.Context, p *domain.Project) error

	// Delete removes a project and cascades to sessions, tasks, and permissions.
	// Returns an error if the project does not exist.
	Delete(ctx context.Context, id string) error

	// List returns all projects in the repository.
	List(ctx context.Context) ([]*domain.Project, error)
}
