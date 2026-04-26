-- +goose Up
-- Add session ownership columns for multi-client coordination.
-- active_client: Identifies the current owning client (format: "{mode}:{uuid}").
-- last_activity_at: Unix timestamp (seconds) for staleness detection.

ALTER TABLE session ADD COLUMN active_client TEXT;
ALTER TABLE session ADD COLUMN last_activity_at INTEGER;

-- +goose Down
-- SQLite does not support DROP COLUMN directly; recreate table would be needed.
-- For simplicity, we leave columns in place on downgrade.
