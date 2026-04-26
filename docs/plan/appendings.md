# Appeding Tasks

TODO: 

1. Project should have type enum: Coding, Research, etc. Only Coding projects have the TaskType enum with Coding/Verification/Confirmation/Coordination.
2. System Agent: do fallback parsing for command input, do slack+jira checking, so on.
3. Session Continuity: User Story 1.19 — TUI ↔ CUI mode switching with session persistence and handoff.

## Prerequiment Check

1. Coding project must have git initialized, check by running `git rev-parse --is-inside-work-tree` in the project root.

## Coding Convention

- ID pattern: <entity-id>-<uuid-v7>. For example, `task-123e4567-e89b-12d3-a456-426614174000`, `session-123e4567-e89b-12d3-a456-426614174000`, `project-123e4567-e89b-12d3-a456-426614174000`.