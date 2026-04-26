# Tasks: 3.2 Context Boundary Clearing

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/acp/`, `internal/orchestrator/`
**Tech Stack:** Go, JSON-RPC over stdio
**Priority:** P1

---

## Task 3.2.1: Worker `ResetContext` Method

* **Action:** Add `Worker.ResetContext(ctx) error` that flushes the agent's working memory without killing the process.
* **Details:**
  * In `internal/acp/worker.go`, add a method that issues a fresh `session/new` request, awaits the response, and replaces the worker's stored ACP session ID with the new value.
  * Reuse the same Cwd / system prompt the worker booted with — only the conversational context is recycled.
  * If the agent advertises `capabilities.session.cancel`, send `session/cancel` for the previous ACP session before opening the new one to avoid orphaned turns.
  * Emit a debug log: `slog.Debug("acp: context reset", "session_id", sessID, "old_acp", old, "new_acp", new)`.
* **Verification:**
  * Stub worker test confirms a `session/new` request is sent and the new ID is stored.
  * Calling `ResetContext` on a closed worker returns `acp.ErrWorkerClosed` (no panic).

---

## Task 3.2.2: Story-Boundary Detection in the Scheduler

* **Action:** Track `last_seq_story` per session in the scheduler and call `ResetContext` when it changes.
* **Details:**
  * Add `internal/orchestrator/scheduler.go` (new) with a thin loop:
    1. `next, err := taskRepo.NextPending(ctx, sessionID)` — pulls the lowest `(seq_epic, seq_story, seq_task)` row in `Pending` status.
    2. If `next.SeqStory != lastSeqStory`, call `worker.ResetContext`. Update `lastSeqStory` only after the reset succeeds.
    3. Hand the task off to the executor.
  * Persist nothing for the boundary itself — the trigger is purely in-memory; on restart the scheduler always resets on the first task it pulls (cheap and safe).
  * Skip the reset for `TaskTypeCoordination` and `TaskTypeConfirmation` tasks since they do not consume agent context.
* **Verification:**
  * Unit test: feed a sequence `(1,1,1) → (1,1,2) → (1,2,1)`; assert `ResetContext` is called once between task 2 and task 3.
  * Reset within a story does not happen.

---

## Task 3.2.3: `/acp-reset` Manual Command

* **Action:** Expose context reset as a REPL command for manual testing.
* **Details:**
  * Extend `internal/repl/acp_commands.go` (3.1.5) with `/acp-reset`.
  * Calls `Worker.ResetContext`; prints `"ACP context reset (new session: <id>)"` on success.
  * Records as a Coordination task.
* **Verification:**
  * Long conversation via `/acp`, then `/acp-reset`, then a follow-up `/acp` proves the model has lost prior turns (e.g., it re-asks for context that was previously provided).

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.2.1 | 3.1.2 |
| 3.2.2 | 3.2.1, 1.5 (state machine), 1.11 (seq cols) |
| 3.2.3 | 3.2.1, 3.1.5 |

---

## Out of Scope

* Persisting reset events in the DB (Archivist territory — EPIC-05).
* Mid-story resets triggered by token-usage heuristics — future iteration.
