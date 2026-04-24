-- +goose Up
-- Story 1.9: Agent Crash Fallback
-- Add timeout column to the task table. The default of 3600000 ms (1 hour)
-- ensures that existing rows and all new tasks without an explicit timeout
-- never hang indefinitely.
ALTER TABLE task ADD COLUMN timeout INTEGER NOT NULL DEFAULT 3600000;

-- +goose Down
-- SQLite does not support DROP COLUMN in older versions; this is a no-op
-- safe rollback marker. The column is benign if the migration is rolled back
-- without a schema change.
SELECT 1;
