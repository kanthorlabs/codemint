# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Working styles

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

## Build / Test / Run

- `make build` — builds `build/codemint` (LDFLAGS inject `version`/`commit`/`buildDate` into `cmd/codemint/main.go`).
- `make build-race` — race-enabled build for development.
- `make test` — `go test ./... -timeout 120s`. Use `make test-race` for race-checked runs, `make test-coverage` for HTML report at `build/coverage.html`.
- Single package: `go test ./internal/orchestrator -run TestRuntimeAttach -v`.
- `make lint` requires `golangci-lint` (no config in repo — uses defaults).
- `make run` — builds and launches the REPL.
- Runtime flags: `-mode cli|daemon|hybrid` (default `cli`), `-with-assistant=false` to disable the System Assistant, `-config <path>` (default `$XDG_CONFIG_HOME/codemint/config.yaml`), `-db <path>` (default `$XDG_DATA_HOME/codemint/codemint.db`).
- Env override: `CODEMINT_ACP_CMD` forces the System Assistant binary path, bypassing the provider registry resolution. Useful in tests.

## Architecture

CodeMint is a Go REPL that dispatches user input either to local slash-command handlers or to external AI assistants spoken to over **ACP (Agent Communication Protocol, JSON-RPC 2.0 over stdio)**. Most complexity lives in the wiring between domain state (SQLite), the ACP transport, and the UI broadcast layer.

### Boot sequence (`cmd/codemint/main.go`)

The single `run()` function is the source of truth for startup order. Touch it carefully — later steps depend on earlier ones:

1. `xdg.EnsureDirs` → 2. `db.OpenDB` + `db.RunMigrations` (goose, embedded `internal/db/migrations/*.sql`) → 3. repositories (`internal/repository/sqlite`) → 4. `agentRepo.EnsureSystemAgents` (seeds `sys-auto-approve` etc.) → 5. command registry + `repl.RegisterCoreCommands` → 6. `ui.NewUIMediator` → 7. `config.Load` + `workflow.LoadFromConfig` + `agent.NewProviderRegistry` → 8. `agent.ResolveSystemAssistantProvider` (honours `CODEMINT_ACP_CMD`) → 9. `acp.NewRegistry` + `orchestrator.NewRuntime` (wires Pipeline, Interceptor, StatusMapper, Fanout, BufferRegistry, PipelineConsumer) → 10. optional `agent.NewACPAssistant` → 11. `orchestrator.NewDispatcher` + `InteractionRecorder` → 12. `SessionLoader.LoadMostRecentSession` + `ActiveSession` (mode-aware ownership via `active_client = "<mode>:<uuid>"`) → 13. `ui.BuildAdapters(clientMode)` registers `TUIAdapter` (CLI) or `CUIAdapter` (daemon) or both (hybrid) on the mediator → 14. mode/verbosity/daemon/ACP/provider commands → 15. `Heartbeat` + `Scheduler` goroutines → 16. `repl.Loop`.

Graceful shutdown: `signal.NotifyContext` cancels on SIGINT/SIGTERM; the deferred `acpRuntime.Shutdown(5s)` reaps ACP workers. `registry.ErrShutdownGracefully` is the sentinel for `/exit`.

### Domain model (`internal/domain/core.go`)

Five enums drive the entire system — when adding logic, check existing switches before adding parallel paths:

- `TaskType`: `Coding(0) | Verification(1) | Confirmation(2) | Coordination(3) | Retrospective(4)`. Coordination tasks record user commands for audit (see `InteractionRecorder`). YOLO auto-approval only changes Confirmation/Retrospective gates — Coding/Verification still execute the same way.
- `TaskStatus`: `Pending | Processing | Awaiting | Success | Failure | Completed | Reverted | Cancelled` — the StatusMapper translates ACP events into these transitions.
- `AgentType`: `Human | Assistant | System`.
- `SessionStatus`: `Active | Archived`. Sessions own a project and a single `active_client` (mode:uuid).
- `YoloMode`: `Off(0) | On(1)` per project. The `sys-auto-approve` agent ID (cached on `Runtime.YoloAgentID`) is the marker that tells the Executor to skip approval prompts.

ID convention (see `docs/plan/appendings.md`): `<entity>-<uuid-v7>`, generated via `internal/util/idgen`.

### ACP layer (`internal/acp`)

Speaks JSON-RPC 2.0 to external CLI agents (OpenCode, Codex, Claude Code) over stdin/stdout. Upstream spec index: [`docs/coding/agent-client-protocol.md`](docs/coding/agent-client-protocol.md) (mirrors authoritative docs at https://agentclientprotocol.com); canonical schema lives at https://agentclientprotocol.com/protocol/schema.md. Key files:

- `protocol.go` — message envelope, method names (`session/new`, `session/prompt`, `session/cancel`, `session/update`, `session/request_permission`), update kinds, error codes.
- `worker.go` — process lifecycle for a single ACP child.
- `registry.go` — pool of workers keyed by config (one per provider configuration).
- `pipeline.go` — translates ACP `session/update` events into a typed event stream consumed by `orchestrator.PipelineConsumer`.
- `buffer.go` + `BufferRegistry` — per-task ring buffer of events, used by `/summary`.

Provider catalog and config overrides live in `internal/agent` (`provider_catalog.go`, `provider_registry.go`). `WorkerConfigFromProvider` is the bridge from a logical provider into an `acp.Config`.

### Orchestrator (`internal/orchestrator`)

Central nervous system. The `Runtime` (`runtime.go`) owns per-session maps of `Pipeline`, `Interceptor`, `StatusMapper`, and `PipelineConsumer` cancel funcs — attach/detach happens here when a session activates.

- `Dispatcher` (`dispatcher.go`) routes input: slash command → registry handler; freeform text → workflow registry → System Assistant. Returns `ErrSystemAssistantDisabled` / `ErrNoBrainstormer` when the relevant assistant isn't wired.
- `Scheduler` (`scheduler.go`) polls pending tasks and feeds them to the `Executor`. Driven by `advanceCh` signals from `StatusMapper`.
- `Interceptor` is the policy boundary — it gates ACP `session/request_permission` against `ProjectPermission` rows.
- `Fanout` is how Runtime multicasts pipeline events to multiple consumers (mediator + buffer + status mapper).
- `Heartbeat` updates `sessions.last_activity_at` so other modes can see liveness.

### UI layer (`internal/ui`)

`UIMediator` is the single broadcast point for both rendered output (`RenderMessage`) and approval prompts (`PromptDecision` — first-response-wins across registered adapters). Adapters:

- `TUIAdapter` — CLI mode, high-bandwidth streaming of agent thoughts/tool calls/messages.
- `CUIAdapter` — daemon mode, low-bandwidth pulse notifications surfaced via `/tasks`, `/status`.
- `BuildAdapters(mode, ...)` (`registration.go`) selects which set to register based on `-mode`.

The `registry.UIMediator` interface (in `internal/registry`, not `internal/ui`) is the contract — most consumers depend on it, not the concrete `*UIMediator`.

### REPL (`internal/repl`)

`Loop` reads stdin and calls a `Dispatcher`. Commands are registered into `registry.CommandRegistry` in groups, each with its own `Deps` struct (see `core_commands.go`, `session_commands.go`, `mode_commands.go`, `verbosity_commands.go`, `daemon_commands.go`, `acp_commands.go`, `provider_commands.go`). Adding a command means: define a handler, add it to one of the `Register*Commands` functions, and pass the dep struct from `main.go`.

`InputMultiplexer` (`input_multiplexer.go`) is the inbound side for non-stdin sources (e.g., daemon-injected messages); it carries `Source` + `UserID` for Coordination-task audit metadata.

### Skills, workflows, providers

- **Skills** (`internal/skills`) — embedded markdown personas in `embedded/seniorgodev/`, plus external `~/.agents/skills` scanned via `GeneralParser`. Embedded entries always win on ID collision.
- **Workflows** (`internal/workflow`) — type/name/triggers from `config.yaml`; the dispatcher uses triggers to route freeform text. Currently three types: Project Coding (0), Communication (1), Daily Checking (2).
- **Providers** (`internal/agent/provider_*.go`) — built-in catalog (`opencode`, `codex`, `claude-code`); config can override `command`/`args`. `Assistants.System.Provider` in config picks which one the System Assistant runs on top of.

## Conventions

- **Coding-project precondition**: any project of type Coding must have git initialized — verify with `git rev-parse --is-inside-work-tree` in the project root.
- **Data layout**: SQLite at `$XDG_DATA_HOME/codemint/codemint.db` (Linux/macOS) or `%LOCALAPPDATA%\codemint` (Windows). Migrations are embedded — never hand-edit a committed migration; add `00000N_*.sql`.
- **`NullableJSON`** (in `domain/core.go`) is the standard wrapper for nullable JSON columns; `NewNullString` for nullable text. Always use them rather than raw `sql.Null*` for new fields with JSON payloads.
- **Effective Go 2026** house style lives at `docs/coding/effective_go_2026.md` — consult it for naming, error handling, and concurrency patterns when extending the codebase.
- **ACP spec**: see `docs/coding/agent-client-protocol.md` for upstream spec index (protocol methods, schema, RFDs, OpenAPI link).
- **Run the tests for the code you touch.** Most packages have a sibling `_test.go` and the orchestrator has e2e tests (`runtime_e2e_test.go`, `system_assistant_e2e_test.go`) that exercise the full pipeline.
