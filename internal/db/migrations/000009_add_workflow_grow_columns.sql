-- +goose Up
-- Add GROW Goal/Options columns to workflow table.
-- This supports User Story 2.0.3: Workflow Execution State (GROW alignment).
-- goal_text: one-sentence goal from 2.2.1
-- success_criteria: JSON array of testable strings from 2.2.1
-- chosen_option: JSON of the single option picked in 2.3.1
ALTER TABLE workflow ADD COLUMN goal_text TEXT;
ALTER TABLE workflow ADD COLUMN success_criteria TEXT;
ALTER TABLE workflow ADD COLUMN chosen_option TEXT;

-- +goose Down
-- Forward-only migration (SQLite column drop limitations)
