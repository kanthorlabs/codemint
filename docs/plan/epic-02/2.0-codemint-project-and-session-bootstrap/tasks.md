# Tasks: 2.0 CodeMint Project & Session Bootstrap

**Epic:** EPIC-02 (Brainstorming Workflow) — runs first, before any other 2.x story.
**Target Directories:** `internal/db/migrations/`, `internal/domain/`, `internal/repository/`, `internal/repository/sqlite/`, `internal/orchestrator/`, `internal/repl/`, `internal/agent/`, `internal/config/`, `internal/xdg/`, `cmd/codemint/`, `docs/plan/`.
**Tech Stack:** Go, SQLite (goose), modernc.org/sqlite, sqlx.
**Priority:** P0 — blocks EPIC-02 implementation; also closes the EPIC-01 `/project-open` gap.

---

## Task 2.0.1: Schema Migration & Domain Enum

* **Action:** Add the columns the bootstrap, dispatcher routing, and per-project assistant override depend on. One migration, three columns.
* **Details:**
  * Create `internal/db/migrations/000006_add_project_kind_and_assistant.sql`:
    ```sql
    -- +goose Up
    ALTER TABLE project ADD COLUMN kind TEXT NOT NULL DEFAULT 'coding';
    ALTER TABLE project ADD COLUMN assistant_provider TEXT;
    ALTER TABLE project ADD COLUMN assistant_model TEXT;

    -- +goose Down
    -- SQLite does not support DROP COLUMN cleanly; columns remain on downgrade.
    ```
  * In `internal/domain/core.go`, add:
    ```go
    type ProjectKind string

    const (
        ProjectKindCoding   ProjectKind = "coding"
        ProjectKindCodeMint ProjectKind = "codemint"
    )
    ```
  * Extend the `Project` struct with `Kind ProjectKind \`db:"kind"\``, `AssistantProvider sql.NullString \`db:"assistant_provider"\``, `AssistantModel sql.NullString \`db:"assistant_model"\``.
  * Update `domain.NewProject(name, workingDir string)` → `domain.NewProject(name, workingDir string, kind ProjectKind)`. Default callers in tests and existing code pass `ProjectKindCoding`.
  * Update `internal/repository/sqlite/project_repo.go` `Create`, `FindByID`, `FindByName` SELECT/INSERT statements to include the three new columns.
* **Verification:**
  * `make test ./internal/repository/sqlite/...` passes — existing project repo tests updated to assert `kind="coding"` round-trips.
  * `TestProjectRepo_KindRoundtrip` — insert a `codemint`-kind project, fetch back, assert kind preserved.
  * `TestProjectRepo_AssistantOverrideRoundtrip` — insert with `assistant_provider="codex"`, `assistant_model="gpt-5"`; fetch returns matching `sql.NullString` values.
  * Goose up/down cycles cleanly on a fresh test DB.

---

## Task 2.0.2: `EnsureCodeMintProject` Bootstrap Helper

* **Action:** Make project + workspace dir + permission row + active session bootstrap a single idempotent call. Lives in `internal/orchestrator/` because it orchestrates three repositories — `project_repo` should not reach across siblings.
* **Details:**
  * Create `internal/orchestrator/bootstrap.go`:
    ```go
    package orchestrator

    // CodeMintProjectName is the canonical display + lookup name for the
    // CodeMint sentinel project. Other code MUST locate the project via this
    // constant rather than hardcoding the string.
    const CodeMintProjectName = "codemint"

    // EnsureCodeMintProject idempotently ensures the CodeMint sentinel project
    // exists (kind=ProjectKindCodeMint, name=CodeMintProjectName, working_dir
    // pointing at workspaceDir), the workspace directory exists on disk, a
    // project_permission row exists (all-NULL columns), and at least one
    // session under the project has status=Active. Safe to call on every
    // launch.
    func EnsureCodeMintProject(
        ctx context.Context,
        workspaceDir string,
        projectRepo repository.ProjectRepository,
        sessionRepo repository.SessionRepository,
        permRepo repository.ProjectPermissionRepository,
    ) error { ... }
    ```
  * Implementation order (each step early-returns on error):
    1. `projectRepo.FindByName(ctx, CodeMintProjectName)`. If nil:
       `domain.NewProject(CodeMintProjectName, workspaceDir, domain.ProjectKindCodeMint)` → `projectRepo.Create`.
    2. `os.MkdirAll(workspaceDir, 0o755)`. No git init.
    3. Ensure a `project_permission` row exists for the project. If absent: `domain.NewProjectPermission(project.ID)` → `permRepo.Create`. All command/directory columns stay NULL.
    4. `sessionRepo.CountActiveByProjectID(ctx, project.ID)`. If zero: `domain.NewSession(project.ID)` → `sessionRepo.Create`. (Add `CountActiveByProjectID` to `SessionRepository` if absent — small wrapper over `SELECT count(*) FROM session WHERE project_id=? AND status=0`.)
  * Helper does **not** mutate `ActiveSession`. `SessionLoader.LoadMostRecentSession` (called later in `main.go`) picks up the seeded session naturally.
* **Verification:**
  * `TestEnsureCodeMintProject_FreshDB` — empty DB → one project (`kind=codemint`), one permission row (all NULLs), one active session, workspace dir on disk.
  * `TestEnsureCodeMintProject_Idempotent` — second call inserts nothing; row counts unchanged across project/permission/session tables.
  * `TestEnsureCodeMintProject_CreatesNewSessionWhenNoneActive` — pre-archive the existing CodeMint session, re-run; assert exactly one new active session.
  * `TestEnsureCodeMintProject_PreservesUserActiveSessions` — pre-create an active session under a separate user project, run bootstrap; assert the user session is untouched.
  * `TestEnsureCodeMintProject_WorkspaceDirAlreadyExists` — pre-`mkdir` the workspace; bootstrap succeeds (no error on `EEXIST`).

---

## Task 2.0.3: Wire Bootstrap into `main.go`

* **Action:** Run the bootstrap in the boot sequence before `SessionLoader.LoadMostRecentSession`.
* **Details:**
  * In `cmd/codemint/main.go` `run()`, immediately after `agentRepo.EnsureSystemAgents(ctx)` (Step 6):
    ```go
    workspaceDir := filepath.Join(xdg.DataDir(), "workspace")
    if err := orchestrator.EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permissionRepo); err != nil {
        return fmt.Errorf("ensure codemint project: %w", err)
    }
    ```
  * Optionally export a constant `xdg.WorkspaceDir() string` returning `filepath.Join(DataDir(), "workspace")` for symmetry with `DatabasePath()`. Use it from main.go.
  * No change to `SessionLoader` logic — once bootstrap runs, `LoadMostRecentSession` finds the CodeMint session naturally.
* **Verification:**
  * `TestRun_FreshDB_LandsInCodeMintSession` (or extend `cmd/codemint/main_test.go`) — boot with empty DB, assert `ActiveSession.Project.Kind == ProjectKindCodeMint` and `ActiveSession.Session != nil`.
  * Manual: `rm -rf $XDG_DATA_HOME/codemint && ./build/codemint -mode cli` → REPL opens with CodeMint session loaded; freeform `hello` reaches SystemAssistant.

---

## Task 2.0.4: Remove `IsGlobal`, Add `IsCodeMintSession`

* **Action:** Replace the runtime-only `IsGlobal` flag with a kind-based predicate sourced from the persisted project.
* **Details:**
  * In `internal/orchestrator/active_session.go`:
    * Delete `ActiveSession.IsGlobal` field.
    * Delete the `IsGlobal=true` assignment in `SetSession`.
    * Add:
      ```go
      func (a *ActiveSession) IsCodeMintSession() bool {
          return a.Project != nil && a.Project.Kind == domain.ProjectKindCodeMint
      }
      ```
    * Update `GetIsGlobal()` (interface method on `registry.ActiveSessionInfo`) — either rename to `GetIsCodeMint()` or delete from the interface if no consumer needs it. Pick rename to minimise churn; update `registry.ActiveSessionInfo`.
  * Update every call site flagged below — replace `IsGlobal` with `IsCodeMintSession()`:
    * `internal/orchestrator/dispatcher.go` (routing branch)
    * `internal/orchestrator/dispatcher_test.go`
    * `internal/orchestrator/session_loader.go` — drop the `IsGlobal: true` legs; bootstrap guarantees the loader always finds a session.
    * `internal/orchestrator/interaction_recorder.go` — delete the early-return on `IsGlobal`. CodeMint chats persist as Coordination tasks under the CodeMint project.
    * `internal/repl/daemon_commands.go` — `/status` formatter switches "(none - global mode)" to "(codemint session)" when applicable.
    * `internal/repl/daemon_commands_test.go`, `internal/repl/provider_commands_test.go`, `internal/orchestrator/system_assistant_e2e_test.go` — update expectations.
* **Verification:**
  * `make test ./...` passes after substitution.
  * `TestDispatch_FreeformInCodeMintSession_RoutesToSystemAssistant`.
  * `TestInteractionRecorder_PersistsCodeMintChat` — chat in a CodeMint session creates a `Coordination` task (was previously skipped).
  * Grep gate: `git grep IsGlobal internal/` returns zero matches.

---

## Task 2.0.5: Dispatcher Kind-Based Routing

* **Action:** Lock the freeform routing rule before EPIC-02 ships the Brainstormer.
* **Details:**
  * In `internal/orchestrator/dispatcher.go` `Dispatch` (or whichever method handles non-slash input):
    ```go
    switch active.Project.Kind {
    case domain.ProjectKindCodeMint:
        return d.routeToSystemAssistant(ctx, active, input)
    case domain.ProjectKindCoding:
        return ErrNoBrainstormer // EPIC-02 swaps in Brainstormer
    default:
        return fmt.Errorf("dispatcher: unsupported project kind %q", active.Project.Kind)
    }
    ```
  * Keep `ErrSystemAssistantDisabled` behaviour: if `--with-assistant=false` and we're in a CodeMint session, return that error instead of routing.
  * Update doc comments on `Dispatcher` to describe the new contract.
* **Verification:**
  * `TestDispatch_CodeMintSession_RoutesToSystemAssistant` (rename of existing global-session test).
  * `TestDispatch_CodingSession_ReturnsErrNoBrainstormer` — placeholder until 2.1+ wires Brainstormer.
  * `TestDispatch_CodeMintSession_AssistantDisabled_ReturnsFriendlyError`.

---

## Task 2.0.6: Interceptor Bypass for CodeMint Kind

* **Action:** Allow tool calls inside CodeMint sessions without consulting the empty permission row.
* **Details:**
  * In `internal/orchestrator/interceptor.go`, before the existing whitelist/blocklist evaluation:
    ```go
    if session.Project != nil && session.Project.Kind == domain.ProjectKindCodeMint {
        return DecisionAllow, nil
    }
    ```
  * Confirm the Interceptor receives the project (via session lookup or pre-resolved struct). If it currently fetches `project_permission` only, extend it to also know the project kind — pull from `Runtime.RefreshPermissions` or pass the project through the same call.
  * Comment the bypass with a `TODO(epic-02 appendings): replace with directory-scoped enforcement once Interceptor learns to gate on cwd/path arguments`.
* **Verification:**
  * `TestInterceptor_CodeMintSession_AutoAllows` — tool call with no matching whitelist/blocklist resolves to `DecisionAllow`.
  * `TestInterceptor_CodingSession_StillRespectsLists` — non-codemint project unchanged behaviour (whitelist hit → allow, blocklist → block, unmatched → escalate).

---

## Task 2.0.7: `/project-open <path>` Command

* **Action:** Close the EPIC-01 hole. Lets users register a Coding project and start a session against any git-initialised directory.
* **Details:**
  * In `internal/repl/session_commands.go` (or a new `project_commands.go`), register:
    ```go
    {
        Name:           "project-open",
        Description:    "Open a project at the given path. Creates the project if it does not exist; opens an active session.",
        Usage:          "/project-open <path>",
        SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
        Handler:        projectOpenHandler(deps),
    }
    ```
  * Handler logic:
    1. Resolve `path` to absolute via `filepath.Abs`.
    2. Run `git rev-parse --is-inside-work-tree` in the path; reject with a clear error if not a git repo (per `appendings.md` precondition for Coding kind).
    3. `FindByName(filepath.Base(path))` — if a project already exists with that name, validate the working dir matches; otherwise create with `ProjectKindCoding` (collision: append suffix or error, decide via test — prefer error to avoid silent dupes).
    4. Ensure a `project_permission` row (all-NULL whitelist; user can `/permission` later — out of scope here).
    5. If the project has no active session, create one. If an active session exists, prompt the user whether to resume or archive (per EPIC-01 §2.3 single-active-session rule).
    6. Set the new project + session on `ActiveSession` via `SetSession`.
  * Error messages reference the actual hole-fill: replace any `"Use /project-open to start"` strings (`session_loader.go`, `acp_commands.go`, etc.) — they are no longer vacuously broken.
* **Verification:**
  * `TestProjectOpen_GitRepo_CreatesProjectAndSession`.
  * `TestProjectOpen_NotGitRepo_ReturnsError`.
  * `TestProjectOpen_ExistingProjectWithActiveSession_PromptsUser`.
  * `TestProjectOpen_DuplicateName_DifferentPath_ReturnsError`.

---

## Task 2.0.8: `/project-assistant` Command + Per-Project Override Resolution

* **Action:** Allow users to set `assistant_provider` / `assistant_model` on the active project; teach SystemAssistant resolution to honour the per-project override.
* **Details:**
  * Register in `internal/repl/provider_commands.go` (it already owns provider-related commands):
    ```go
    {
        Name:           "project-assistant",
        Description:    "Show or set the assistant provider/model for the active project. Persists to the project row.",
        Usage:          "/project-assistant [provider] [model]",
        SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
        Handler:        projectAssistantHandler(deps),
    }
    ```
  * Handler:
    * No args → print effective binding: project columns if set, else `config.yaml` `assistants.codemint.*`, else built-in default.
    * One arg → set provider; clear model.
    * Two args → set provider + model.
    * Validate provider name against `ProviderRegistry`; reject unknown names.
    * Persist via `projectRepo.UpdateAssistantBinding(ctx, projectID, provider, model)` (new repo method — small wrapper over `UPDATE project SET ... WHERE id=?`).
  * In `internal/agent/system_assistant.go` (or wherever `ResolveSystemAssistantProvider` lives), when constructing the SA for a session, prefer:
    1. `project.AssistantProvider` (if non-NULL) → resolve via registry.
    2. Else `config.Assistants.CodeMint.Provider` (new config block, defaults to today's `assistants.system`).
    3. Else `CODEMINT_ACP_CMD` env override (preserves existing behaviour).
    4. Else builtin default.
  * Add `assistants.codemint` to `internal/config/config.go` and `configs/config.yaml.example`. Keep `assistants.system` as a deprecated alias for one release, mapped onto `assistants.codemint`.
* **Verification:**
  * `TestProjectAssistantHandler_NoArgs_PrintsEffective`.
  * `TestProjectAssistantHandler_SetProvider_PersistsToDB`.
  * `TestProjectAssistantHandler_UnknownProvider_Rejects`.
  * `TestResolveSystemAssistantProvider_PrefersProjectOverrideOverConfig`.
  * `TestConfigLoad_AcceptsAssistantsCodemintBlock`.

---

## Task 2.0.9: Update `epic-03/mvp.md` and `appendings.md`

* **Action:** Reflect the bootstrap in the existing test plan and stash deferred work.
* **Details:**
  * `docs/plan/epic-03/mvp.md` Suite 0: drop the SQL seed block. Replace with: "Fresh DB launches into a CodeMint session automatically. Use `/project-open <path>` to register a user project before running Coding-task suites."
  * `docs/plan/epic-03/mvp.md` Suite 14 (System Assistant): clarify that chats now persist as Coordination tasks under the CodeMint project, queryable via `/tasks`.
  * `docs/plan/appendings.md` — append a new section:
    ```markdown
    ## Deferred from Story 2.0

    1. Interceptor directory-scoped enforcement: today CodeMint sessions get a kind-based bypass; replace with `cwd`/path-arg gating against `allowed_directories`.
    2. Per-session assistant override (today per-project only).
    3. Configurable workspace path via env or config (today hardcoded `xdg.DataDir() + "/workspace"`).
    4. Concurrent multiple-active sessions for the CodeMint project (today single-active rule holds).
    ```
* **Verification:**
  * Manual review — no broken cross-refs after rewrites.
  * `grep "Use /project-open to start" internal/` zero hits or only in defensive fallback code paths (matches Task 2.0.7 cleanup).

---

## Dependencies

| Task   | Depends On |
|--------|------------|
| 2.0.1  | None — foundational migration. |
| 2.0.2  | 2.0.1 |
| 2.0.3  | 2.0.2 |
| 2.0.4  | 2.0.1 (uses `Project.Kind`) |
| 2.0.5  | 2.0.4 |
| 2.0.6  | 2.0.4 |
| 2.0.7  | 2.0.1, 2.0.4 |
| 2.0.8  | 2.0.1, 2.0.7 (provider command sibling), `internal/agent` provider registry from Story 3.22 |
| 2.0.9  | 2.0.3, 2.0.7 |

---

## Out of Scope

* Brainstormer implementation for Coding-kind projects (Stories 2.1+).
* Directory-scoped Interceptor enforcement (deferred to `appendings.md`).
* Per-session assistant override (`appendings.md`).
* Configurable workspace path (`appendings.md`).
* Concurrent multi-active CodeMint sessions (`appendings.md`).
* Telegram/Slack inbound transports (EPIC-04 §4.5).
