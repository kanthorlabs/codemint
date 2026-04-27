-- +goose Up
-- Task routing columns for conditional execution (Story 2.0.2)
ALTER TABLE task ADD COLUMN depends_on TEXT;
ALTER TABLE task ADD COLUMN condition INTEGER;

-- +goose Down
-- Note: SQLite < 3.35.0 doesn't support DROP COLUMN
-- For compatibility, this is a forward-only migration.
-- If rollback is required on SQLite >= 3.35.0, uncomment below:
-- ALTER TABLE task DROP COLUMN depends_on;
-- ALTER TABLE task DROP COLUMN condition;
