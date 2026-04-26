# Tasks: 3.7 ACP Event to Database Status Translation

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`
**Tech Stack:** Go, state-machine validator (Story 1.5)
**Priority:** P0

---

## Task 3.7.1: Status Mapper

* **Action:** Centralize the rule that turns ACP events into `domain.TaskStatus` transitions.
* **Details:**
  * Create `internal/orchestrator/status_mapper.go`:
    ```go
    type StatusMapper struct{ taskRepo repository.TaskRepository }
    func (m *StatusMapper) Apply(ctx context.Context, taskID string, ev acp.Event) error
    ```
  * Mapping table:
    | Event                          | New Status         |
    |--------------------------------|--------------------|
    | `EventTurnStart`               | `Processing`       |
    | `EventPermissionRequest` (no auto-allow) | `Awaiting` |
    | `EventTurnEnd` (success)       | `Success`          |
    | `EventTurnEnd` (with error)    | `Failure`          |
  * Use the existing state-machine validator from Story 1.5; on a rejected transition, log and skip — never panic.
  * Idempotent: applying the same event twice is a no-op.
* **Verification:**
  * Unit tests for every mapping row, including the failure branch.
  * Invalid transition (e.g., `Pending → Success`) is rejected by the validator and logged.

---

## Task 3.7.2: Drive the Mapper From the Pipeline

* **Action:** Subscribe `StatusMapper` to both event channels.
* **Details:**
  * In `main.go`, after constructing the pipeline (3.4.2) and the interceptor (3.5/3.6):
    * For `Pipeline.Events`: pass turn-start/turn-end events through the mapper.
    * For `Pipeline.Halted`: only call the mapper when the interceptor decided `Block` / `Unknown` (Story 3.6 path); for auto-allow (3.5) status stays `Processing`.
  * The mapper needs the **current task ID** for the worker. Add `Worker.SetCurrentTask(taskID string)` and call it from the scheduler (3.2.2) right before dispatching a task to the agent. Use empty string for ad-hoc `/acp` prompts (mapper short-circuits when ID is empty).
* **Verification:**
  * Manual: drive a real OpenCode prompt; observe DB transitions `pending → processing → success` (or `awaiting` if a tool prompt fires).
  * `/acp` ad-hoc prompts emit no DB transitions.

---

## Task 3.7.3: Schedule Next Task on `Success`

* **Action:** When a task completes successfully, pull the next pending task without manual intervention.
* **Details:**
  * After `StatusMapper.Apply` writes `Success`, signal the scheduler (3.2.2) via a `chan struct{}` to advance.
  * The scheduler loop: select-on `nextTaskCh`, on receive call `taskRepo.NextPending`, hand off, send to worker, repeat.
  * Stop advancing if no pending tasks remain — log `"acp scheduler idle"` and wait.
* **Verification:**
  * Seed a session with three Coding tasks; without user input the scheduler walks through all three (assuming auto-approve for any tool calls or no tool calls at all).
  * `/session-archive` interrupts the loop cleanly.

---

## Task 3.7.4: Surface Transitions to the UI

* **Action:** Emit a `EventTaskStatusChanged` UI event on every transition the mapper performs.
* **Details:**
  * Reuse / extend `registry.UIEvent`:
    ```go
    EventTaskStatusChanged UIEventType = "task_status_changed"
    ```
    Payload: `{TaskID, From, To, Reason}`.
  * Mapper publishes via the mediator after a successful DB write.
* **Verification:**
  * Recording adapter in tests sees one event per transition.
  * No duplicate events on idempotent re-application.

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.7.1 | 1.5 (state machine), 3.4.1 |
| 3.7.2 | 3.7.1, 3.4.2, 3.5/3.6 |
| 3.7.3 | 3.7.1, 3.2.2 |
| 3.7.4 | 3.7.1, 1.10 |

---

## Out of Scope

* Retry/backoff for failed tasks — Story 1.9 handles single-task fallback; richer policies come later.
