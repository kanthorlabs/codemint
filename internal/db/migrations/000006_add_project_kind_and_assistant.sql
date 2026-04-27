-- +goose Up
-- Add kind column to distinguish CodeMint sentinel project from Coding projects.
-- Add assistant_provider and assistant_model columns for per-project assistant override.

ALTER TABLE project ADD COLUMN kind TEXT NOT NULL DEFAULT 'coding';
ALTER TABLE project ADD COLUMN assistant_provider TEXT;
ALTER TABLE project ADD COLUMN assistant_model TEXT;

-- +goose Down
-- SQLite does not support DROP COLUMN cleanly; columns remain on downgrade.
