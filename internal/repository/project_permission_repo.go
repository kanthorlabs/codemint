package repository

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// ProjectPermissionRepository defines atomic persistence operations for
// ProjectPermission entities. This is the central source of truth for the
// Auto-Approval Interceptor (EPIC-03).
type ProjectPermissionRepository interface {
	// FindByProjectID returns the permission record for the given project.
	// Returns nil, nil when no permission record exists (permissive default).
	FindByProjectID(ctx context.Context, projectID string) (*domain.ProjectPermission, error)

	// Upsert creates or updates a permission record for a project. If a record
	// already exists for the project_id, the JSON fields are updated.
	// This enables idempotent permission updates.
	Upsert(ctx context.Context, perm *domain.ProjectPermission) error
}
