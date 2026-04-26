# Tasks: 3.12 ACP Runtime Pipeline Assembly

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `cmd/codemint/`, `internal/orchestrator/`, `internal/acp/`, `internal/repl/`
**Tech Stack:** Go, `context`, goroutines
**Priority:** P0 (unlocks 3.13–3.16; precondition for any EPIC-02 task auto-execution)

---

## Task 3.12.1: Runtime Wiring Helper

* **Action:** Extract the pipeline assembly into a single constructor so `main.go` stays readable and tests can exercise the full chain.
* **Details:**
  * Create `internal/orchestrator/runtime.go`:
    ```go
    type Runtime struct {
        Registry        *acp.Registry
        BufferRegistry  *acp.BufferRegistry
        StatusMapper    *StatusMapper
        Fanout          *Fanout
        Interceptor     *Interceptor
        PermissionRepo  repository.ProjectPermissionRepository
        Mediator        registry.UIMediator
        TaskRepo        repository.TaskRepository
        SessionRepo     repository.SessionRepository
    }
    func NewRuntime(cfg RuntimeConfig) *Runtime
    func (rt *Runtime) AttachWorker(ctx context.Context, sess *domain.Session, project *domain.Project) (*acp.Worker, error)
    ```
  * `AttachWorker` calls `Registry.GetOrSpawn`, builds the per-session `Pipeline`, starts the `PipelineConsumer.Run` goroutine, loads `project_permission` rows into the `Interceptor`, and returns the worker.
  * Stop hook: `Runtime.DetachSession(sessionID)` cancels the consumer context and lets `Registry.Stop` reap the worker.
* **Verification:**
  * `TestRuntime_AttachWorker_StartsConsumer` — spawn stub worker, push a `tool_call` event onto its stdout, observe the interceptor evaluating against a permission row.
  * `TestRuntime_DetachSession_CancelsConsumer` — consumer goroutine exits within 1s of detach.

---

## Task 3.12.2: Wire Runtime Into `main.go`

* **Action:** Replace the lone `acp.NewRegistry(...)` call (Step 11c) with the full runtime.
* **Details:**
  * In `cmd/codemint/main.go` after Step 11b:
    ```go
    permissionRepo := sqlite.NewProjectPermissionRepo(dbConn)
    bufferRegistry := acp.NewBufferRegistry(256)
    runtime := orchestrator.NewRuntime(orchestrator.RuntimeConfig{
        Registry:       acp.NewRegistry(acp.DefaultConfig()),
        BufferRegistry: bufferRegistry,
        Mediator:       mediator,
        TaskRepo:       taskRepo,
        SessionRepo:    sessionRepo,
        PermissionRepo: permissionRepo,
    })
    activeSession.SetACPRegistry(runtime.Registry)
    activeSession.SetACPRuntime(runtime) // new field, used by /acp commands
    ```
  * Replace the existing `defer acpRegistry.StopAll(...)` with `defer runtime.Shutdown(shutdownCtx)` (forwards to `Registry.StopAll` plus consumer cancellation).
  * Pass `bufferRegistry` into `ACPCommandDeps.BufferRegistry` so `/summary` works.
  * If `loadResult.Project != nil`, call `runtime.AttachWorker(ctx, session, project)` lazily via the existing dispatcher hook — do **not** spawn at startup; keep cold-start UX intact.
* **Verification:**
  * Boot CodeMint with `opencode` on PATH and a project open. First `/acp say hi` triggers exactly one consumer goroutine (verify via `runtime.ConsumerCount() == 1`).
  * `/summary` after `/acp` returns a non-empty `<thinking>` block (no longer "Event buffer not available").

---

## Task 3.12.3: Permission Repo Load at Session Boot

* **Action:** Load `project_permission` rows into the `Interceptor` once per project switch.
* **Details:**
  * Extend `ActiveSession` with `OnProjectSwitch(fn func(*domain.Project))`. Fire when `/project-open` or session reload changes the active project.
  * In `Runtime.AttachWorker`, fetch `permissionRepo.FindByProjectID(ctx, project.ID)` and pass into `Interceptor.SetPermissions(perm)`.
  * Cache invalidation: when the user runs `/permission-allow` or `/permission-block` (future EPIC-03.x), call `Runtime.RefreshPermissions(projectID)`.
* **Verification:**
  * `TestRuntime_PermissionLoad_OnAttach` — seed a permission row with `allowed_commands = ["go test"]`, attach worker, assert `Interceptor.Matcher().Allows("go test")`.
  * `TestRuntime_PermissionRefresh` — update DB row, call `RefreshPermissions`, observe matcher reflects new allowlist.

---

## Task 3.12.4: Active Task Tracking for Status Mapper

* **Action:** Make sure the `StatusMapper` knows which task each `turn-start` / `turn-end` belongs to.
* **Details:**
  * `PipelineConsumer.Run` already takes `sessionID`. Add `currentTaskID` setter:
    ```go
    func (c *PipelineConsumer) SetCurrentTask(taskID string)
    ```
  * The scheduler (Story 3.13) calls `SetCurrentTask` before sending `session/prompt`. The mapper writes `task.id = currentTaskID` when transitioning DB status.
  * `BufferRegistry.Push` is keyed on `(sessionID, currentTaskID)` per Task 3.10.2 — verify the consumer reads the task ID from the same setter, not a separate field.
* **Verification:**
  * `TestConsumer_SetCurrentTask_RoutesEvents` — set task A, push events, assert buffer for `(sess, A)` is non-empty and buffer for `(sess, "")` is empty.

---

## Task 3.12.5: Telemetry Smoke Test

* **Action:** Confirm the assembled chain end-to-end with a stub ACP server.
* **Details:**
  * New file `internal/orchestrator/runtime_e2e_test.go`.
  * Stub worker: `cat`-style binary emits a canned `session/update` (agent_message_chunk + turn-end) and a `tool_call` for `go vet`.
  * Permission row allows `go vet`. Test asserts:
    1. UI mediator received exactly the message chunk (tool_call was halted by interceptor).
    2. `local_runner` was invoked with `go vet`.
    3. `BufferRegistry.Snapshot(sess, taskID)` contains 3 events (chunk, tool_call, turn-end).
    4. `taskRepo.Get(taskID).Status == TaskStatusCompleted`.
* **Verification:**
  * `go test ./internal/orchestrator -run TestRuntime_E2E` passes without flakes (run with `-count=20`).

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.12.1  | 3.4.4 (PipelineConsumer), 3.5.x (LocalRunner), 3.7.x (StatusMapper) |
| 3.12.2  | 3.12.1, 3.10.1 (BufferRegistry) |
| 3.12.3  | 3.12.1, schema for `project_permission` |
| 3.12.4  | 3.12.1, 3.10.2 |
| 3.12.5  | 3.12.2, 3.12.3, 3.12.4 |

---

## Out of Scope

* Pulling pending tasks from DB (3.13).
* Branching on task type (3.14).
* CUI registration (3.17).
