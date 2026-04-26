# Tasks: 3.3 Graceful Teardown

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/acp/`, `internal/orchestrator/`
**Tech Stack:** Go, `os/exec`, signal handling
**Priority:** P1

---

## Task 3.3.1: `Worker.Stop` Two-Phase Shutdown

* **Action:** Implement a SIGTERM-then-SIGKILL stop on the worker process.
* **Details:**
  * In `internal/acp/worker.go` add:
    ```go
    func (w *Worker) Stop(ctx context.Context, grace time.Duration) error
    ```
  * Steps:
    1. If the agent advertises a graceful exit method (e.g., `shutdown`), send it and wait up to `grace` for the process to exit naturally.
    2. Otherwise (or after grace expires) send SIGTERM via `cmd.Process.Signal(syscall.SIGTERM)`.
    3. Wait up to `grace` again. If still running, send SIGKILL.
    4. Close the `out` channel and the `stdin` writer; drain `Wait()`.
  * Default `grace = 3 * time.Second` when callers pass 0.
  * Idempotent: a second `Stop` returns `nil`.
* **Verification:**
  * Unit test with a stub binary that ignores SIGTERM confirms SIGKILL is sent within `2 * grace`.
  * `pgrep -f "opencode acp"` returns empty after Stop.

---

## Task 3.3.2: Hook Session Archive → Worker Stop

* **Action:** When a session transitions from `Active` to `Archived`, terminate its worker.
* **Details:**
  * Identify the single archive-emitting code path (likely `SessionRepo.Archive` or a new `SessionService.Archive`).
  * Inject the `acp.Registry` into that service and call `registry.Stop(ctx, sessionID)` after the DB row update commits.
  * On stop failure, log `slog.Error` but do **not** roll back the archive — the DB is the source of truth.
  * Add `/session-archive <id>` REPL command that drives this flow end-to-end so it can be exercised manually.
* **Verification:**
  * `/acp say hi` → `/session-archive <current>` → process is gone (`pgrep` empty), session row shows `status=1`.
  * Archive of a session with no live worker succeeds without error.

---

## Task 3.3.3: Graceful Stop on REPL Exit

* **Action:** Ensure shutdown signals do not leave zombie ACP processes.
* **Details:**
  * In `cmd/codemint/main.go`, after the REPL loop returns, run `acpRegistry.StopAll(shutdownCtx)` with a 5s deadline derived from `context.WithTimeout(context.Background(), 5*time.Second)` (do **not** reuse the canceled signal context — children must be reapable).
  * Guarantee `StopAll` runs on `ErrShutdownGracefully`, SIGINT/SIGTERM, and panic recovery.
* **Verification:**
  * `./build/codemint` → `/acp ...` → Ctrl+C → confirm exit message AND `pgrep -f "opencode acp"` is empty.
  * `kill -TERM <pid>` of the codemint process produces the same outcome.

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.3.1 | 3.1.2 |
| 3.3.2 | 3.3.1, 3.1.3 |
| 3.3.3 | 3.3.1, 3.1.4 |

---

## Out of Scope

* Pause-without-archive semantics (no schema field for it yet).
* Crash-restart loops — handled per-task by Story 1.9 fallback.
