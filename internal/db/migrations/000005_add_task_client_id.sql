-- +goose Up
-- Add client_id column to task table for tracking which client created each task.
-- Format: "{mode}:{uuid}" e.g. "cli:abc123" or "daemon:xyz789".
-- NULL for tasks created by AI agents (not user-initiated).

ALTER TABLE task ADD COLUMN client_id TEXT;

-- +goose Down
-- SQLite does not support DROP COLUMN directly; leave in place on downgrade.
