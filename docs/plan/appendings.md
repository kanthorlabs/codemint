# Appeding Tasks

DONE (Story 2.0):

1. ~~Project should have type enum: Coding, Research, etc. Only Coding projects have the TaskType enum with Coding/Verification/Confirmation/Coordination.~~
   - Implemented `ProjectKind` enum with `ProjectKindCoding` and `ProjectKindCodeMint` values.
   - CodeMint is the sentinel project for non-project work (chat, research, blogging).
   - Added `/project-open`, `/project-list`, `/project-assistant` commands.
   - CodeMint sessions bypass Interceptor permission checks (auto-allow all tool calls).
   - Per-project assistant override via `assistant_provider` and `assistant_model` columns.

TODO: 

2. System Agent: do fallback parsing for command input, do slack+jira checking, so on.
3. Session Continuity: User Story 1.19 — TUI ↔ CUI mode switching with session persistence and handoff.

## Prerequiment Check

1. Coding project must have git initialized, check by running `git rev-parse --is-inside-work-tree` in the project root.

## Coding Convention

- ID pattern: <entity-id>-<uuid-v7>. For example, `task-123e4567-e89b-12d3-a456-426614174000`, `session-123e4567-e89b-12d3-a456-426614174000`, `project-123e4567-e89b-12d3-a456-426614174000`.
- Don't use harcode text, use constants or enums instead. For example, use `ProjectKindCoding` instead of "coding", `TaskTypeCoding` instead of "coding", `ProviderOpenCode` instead of "opencode", etc.
- Use [Go Validator](https://github.com/go-playground/validator) for struct validation, and define custom validation tags if needed. For example, `validate:"required,uuidv7"` for UUID fields, `validate:"required,oneof=coding code_mint"` for project kind, etc.

## Agent Notes

- Always find relevant tests of the code you are working on, and run them to ensure your changes do not break existing functionality.

## TODO

## ACP Spec Conformance

EPIC-02-scoped. See `docs/plan/epic-02/appendings.md` for the full task list (A: schema audit, B: coverage map, C: wire conformance harness, D: planning guardrail). Top priority — blocks the Coding Workflow spine (2.1 → 2.8).

## Coding Workflow cross-cuts (EPIC-02)

- **Verbosity respect.** Every story (2.1–2.8) renders user-visible progress through a single `/verbosity` filter (`quiet | normal | verbose | debug`). Default `normal`. Level setter is a separate command, not a story. See `docs/plan/epic-02/appendings.md` § "Cross-cutting concern A".
- **Error escalation.** Any task `Failure` is reassigned to the session's Human Agent and pauses the workflow until `/resolve retry|skip|abort`. Specified by Story 2.8; every other story converts errors to `Failure` rather than handling them locally.
