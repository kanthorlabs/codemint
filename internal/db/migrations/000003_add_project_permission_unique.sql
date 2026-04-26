-- +goose Up
-- Add UNIQUE constraint on project_id for idempotent upserts (Story 1.12)
CREATE UNIQUE INDEX IF NOT EXISTS idx_project_permission_project_id ON project_permission(project_id);

-- +goose Down
DROP INDEX IF EXISTS idx_project_permission_project_id;
