-- +goose Up
-- Extend workflow table with execution tracking columns.
-- This supports User Story 2.0.3: Workflow Execution State.
ALTER TABLE workflow ADD COLUMN file_path TEXT;
ALTER TABLE workflow ADD COLUMN current_epic_id TEXT;
ALTER TABLE workflow ADD COLUMN current_story_id TEXT;
ALTER TABLE workflow ADD COLUMN started_at INTEGER;
ALTER TABLE workflow ADD COLUMN completed_at INTEGER;
ALTER TABLE workflow ADD COLUMN status INTEGER DEFAULT 0;

-- +goose Down
-- Forward-only migration (SQLite column drop limitations)
