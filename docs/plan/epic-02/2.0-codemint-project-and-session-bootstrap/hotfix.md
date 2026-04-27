# Hotfix: Story 2.0 — CodeMint Project & Session Bootstrap

Summary of changes shipped during the implementation of Story 2.0, grouped by `internal/` subdirectory. Items marked **(planned)** matched the task spec; items marked **(unplanned)** were discovered mid-implementation and added to make the plan actually work.

---

## `internal/db/migrations/`

- **(planned)** Added `000006_add_project_kind_and_assistant.sql` — adds `kind` (default `'coding'`), `assistant_provider`, `assistant_model` columns to `project`. Down migration is a no-op (SQLite limitation).

## `internal/domain/`

- **(planned)** Added `ProjectKind` string enum (`coding`, `codemint`).
- **(planned)** Extended `Project` with `Kind`, `AssistantProvider sql.NullString`, `AssistantModel sql.NullString`.
- **(planned)** Changed `NewProject(name, workingDir)` → `NewProject(name, workingDir, kind)`. All call sites updated.

## `internal/xdg/`

- **(planned)** Added `WorkspaceDir()` returning `$XDG_DATA_HOME/codemint/workspace`.

## `internal/repository/`

- **(planned)** `ProjectRepository.UpdateAssistantBinding(ctx, id, provider, model)` — empty strings clear (NULL) the binding.
- **(planned)** `SessionRepository.CountActiveByProjectID(ctx, projectID)` — used by the bootstrap idempotency check.
- **(planned)** SQLite implementations updated to read/write the three new project columns; tests now assert `Kind` round-trips and added `KindRoundtrip`, `AssistantOverrideRoundtrip`, `UpdateAssistantBinding_NotFound` cases.
- **(unplanned)** All existing test fixtures rewritten to pass `domain.ProjectKindCoding` to `NewProject` (signature ripple).

## `internal/orchestrator/`

- **(planned)** New `bootstrap.go` exporting `EnsureCodeMintProject` — idempotent: creates project, workspace dir, permission row (via `Upsert`, not `Create`), and an active session if absent.
- **(planned)** `bootstrap_test.go` covers fresh-DB, idempotent re-run, archive-then-restore, foreign-project preservation, pre-existing workspace dir.
- **(planned)** `ActiveSession.IsGlobal` removed; `IsCodeMintSession()` / `GetIsCodeMint()` added; `GetProjectID()` added to satisfy the new interface contract used by `/project-assistant`.
- **(planned)** `Dispatcher.Dispatch` switches on `Project.Kind`: CodeMint → SystemAssistant; Coding → workflow registry → `ErrNoBrainstormer`.
- **(planned)** `Interceptor` gains `ProjectKind` field; CodeMint sessions short-circuit to auto-allow in both `handleToolCall` and `handlePermissionRequest` before any whitelist/permission lookup.
- **(planned)** `interceptor_test.go` adds `CodeMintSession_BypassesPermissionChecks`, `CodeMintSession_PermissionRequest`, `CodingSession_StillRequiresPermissions`.
- **(planned)** `interaction_recorder.go` no longer early-returns on global; CodeMint chats now persist as Coordination tasks.
- **(planned)** `session_loader.go` drops the `IsGlobal: true` legs.
- **(unplanned)** `Runtime.AttachWorkerRaw` — new method that returns a worker without spawning Pipeline/StatusMapper/Consumer. Required because SystemAssistant consumes `worker.Out()` directly; the existing `AttachWorker` started a Pipeline that contended for the same channel and dropped the assistant's events.
- **(unplanned)** `Interceptor.evaluateCommand` gained a `permRepo == nil` guard to avoid nil-deref when CodeMint runtime has no permission lookup wired.
- **(unplanned)** `Executor.executeCoding` updated to wrap its prompt in `[]acp.ContentBlock{acp.TextContent(...)}` after the ACP protocol fix below.
- **(unplanned)** Added `slog.Info` traces in `Dispatch`, `dispatchToSystemAssistant`, `AttachWorker(Raw)` for routing diagnosis. Should be downgraded/removed once stable.

## `internal/acp/`

- **(unplanned, protocol compliance)** `InitializeParams.ProtocolVersion: 1` — agents reject the handshake without it.
- **(unplanned, protocol compliance)** `SessionPromptParams.Prompt` retyped from `string` to `[]ContentBlock`. Added `ContentBlock` struct + `TextContent` helper.
- **(unplanned, protocol compliance)** `SessionNewParams` gained required `Cwd` and `McpServers []McpServer` (must be `[]`, never `null`). Added `McpServer` and `McpEnvVariable` types.
- **(unplanned)** `Worker.createSession` populates `Cwd: w.cfg.Cwd` and `McpServers: []McpServer{}`.
- **(unplanned)** `pipeline.go` gained verbose `slog.Info` per-message logging for wire-level debugging.
- **(unplanned)** `protocol_test.go` updated for the new `Prompt` shape.

## `internal/agent/`

- **(planned)** `AssistantSession.IsGlobal` → `IsCodeMint`; `system_assistant.go` consumes via `AttachWorkerRaw` (new contract on `WorkerAttacher`).
- **(planned)** `system_assistant.go` builds prompts as `[]ContentBlock` (matches ACP fix).
- **(unplanned)** `provider_catalog.go` — `codex` and `claude-code` builtins **removed**. Only `opencode` remains. The other two providers were not validated end-to-end and were producing broken sessions.
- **(unplanned)** `provider_config.go` — dead `mergeEnv` helper removed.
- **(unplanned)** `system_assistant.go` — module-level `systemPrompt` constant removed (was unused).

## `internal/registry/`

- **(planned)** `ActiveSessionInfo.GetIsGlobal` renamed to `GetIsCodeMint`.
- **(unplanned)** `MutableSessionInfo.GetProjectID()` added — `/project-assistant` needs it; the original plan didn't account for the interface boundary.

## `internal/repl/`

- **(planned)** New `project_commands.go` registers `/project-open`, `/project-list`, `/project-assistant` (CLI + Daemon modes).
- **(planned)** `/project-open <path>` — abs-path resolve, dir check, `git rev-parse --is-inside-work-tree` precondition, name-collision fallback (`name-<hash>`), permission row upsert, session reuse-or-create, then `ActiveSession.SetSession`.
- **(planned)** `/project-list` — prints registered projects with `[codemint]` marker.
- **(planned)** `/project-assistant [provider|reset]` — show, set, or clear per-project override; rejects on CodeMint project (system assistant only); validates against `ProviderRegistry.Names()` when wired.
- **(planned)** Test mocks updated: `daemon_commands_test.go` (`GetIsCodeMint`, `GetProjectID`), `provider_commands_test.go` (`GetIsCodeMint`).
- **(unplanned)** `acp_commands.go` — `/acp prompt` updated for the `[]ContentBlock` prompt shape; `createACPSession` now passes `McpServers: []McpServer{}` to satisfy schema.

---

## Cross-cutting wiring (`cmd/codemint/`, `configs/`)

- `main.go` — `permissionRepo` construction moved earlier (Step 6b) so `EnsureCodeMintProject` can take it; `RegisterProjectCommands` wired in with `ProjectCommandDeps`.
- `main_test.go` — REPL exit/help/cancel tests now build sessions with `Project.Kind = ProjectKindCodeMint` instead of `IsGlobal`.
- `configs/config.yaml.example` — `workflows:` block commented out (workflows aren't ready); `assistants.system` block uncommented and pinned to `opencode` + `github-copilot/claude-sonnet-4.6` so freshly-installed users land on a working assistant.

## Docs

- `docs/plan/appendings.md` — Story 2.0's project-kind item moved from TODO to DONE with implementation note.
