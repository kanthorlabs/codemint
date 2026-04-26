# Tasks: 3.10 Circular Event Buffer & `/summary` Command

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/acp/`, `internal/repl/`
**Tech Stack:** Go, ring buffer, Markdown
**Priority:** P2

---

## Task 3.10.1: Per-Task Ring Buffer

* **Action:** Capture the last N raw ACP events for every active task.
* **Details:**
  * Create `internal/acp/buffer.go`:
    ```go
    type RingBuffer struct {
        mu   sync.Mutex
        cap  int
        data []Event
        head int
        full bool
    }
    func NewRingBuffer(cap int) *RingBuffer
    func (r *RingBuffer) Push(ev Event)
    func (r *RingBuffer) Snapshot() []Event
    ```
  * `cap` default 256 events.
  * Wrap in a registry keyed by `(sessionID, taskID)`; create on first event, drop when the task hits a terminal status (Story 3.7).
  * Also retain a "session default" buffer keyed by `(sessionID, "")` for ad-hoc `/acp` prompts that have no task ID.
* **Verification:**
  * `TestRingBuffer_WrapAround` confirms snapshot returns events in chronological order even after wrap.
  * `TestRegistry_DropsOnTerminal` removes the task entry once a `Success` transition lands.

---

## Task 3.10.2: Buffer Hookup

* **Action:** Push every classified event into the right buffer.
* **Details:**
  * In the pipeline consumer goroutine (3.4.4), call `bufferRegistry.Push(sessionID, currentTaskID, ev)` before fanning out to the mediator.
  * Include both `Pipeline.Events` and `Pipeline.Halted` so the buffer captures tool calls and approvals — the user wants the full thinking trail when debugging.
* **Verification:**
  * After an `/acp` prompt run, `bufferRegistry.Snapshot(sessID, "")` returns a non-empty slice.

---

## Task 3.10.3: `/summary` Command

* **Action:** Render a Markdown `<thinking>` block from the buffer.
* **Details:**
  * Add to `internal/repl/acp_commands.go`:
    * `/summary` (no arg): pick the active session's most recent `Processing` or `Awaiting` task via `taskRepo.MostRecentActive(ctx, sessionID)`. Fallback to the session-default buffer if no such task exists.
    * `/summary <task_id>`: load buffer for that task. If empty (process restarted), print `"No buffered events for task <id> — buffer is in-memory and resets on restart."`.
  * Output format:
    ```
    <thinking task="abc" session="xyz">
    [12:01:03] thought: ...
    [12:01:04] tool: bash `go test ./...`
    [12:01:09] tool_update: success
    [12:01:10] message: All tests passed.
    </thinking>
    ```
  * Truncate individual entries to 500 chars with `…` to keep the block readable.
* **Verification:**
  * Manual: run `/acp explain channels`, then `/summary` — see the streamed reasoning consolidated.
  * `/summary <bogus-id>` returns the empty-buffer message, no panic.

---

## Task 3.10.4: Persist Summary as a Coordination Task (Optional but Recommended)

* **Action:** Save the rendered summary so the Archivist (EPIC-05) can pick it up later.
* **Details:**
  * After rendering, insert a `type=3` Coordination task with:
    * `input` = `{"command":"/summary","arg":"<task_id>"}`
    * `output` = `{"markdown":"...rendered..."}`
    * `status = 5`
  * Reuse `interactionRecorder`.
  * Skip persistence on empty buffer.
* **Verification:**
  * `/activity` lists the `/summary` invocation with a non-empty output payload.

---

## Dependencies

| Task   | Depends On |
|--------|------------|
| 3.10.1 | 3.4.1 |
| 3.10.2 | 3.10.1, 3.4.2 |
| 3.10.3 | 3.10.2, 1.5 (status filter on most-recent) |
| 3.10.4 | 3.10.3, 1.19.9 (interaction recorder) |

---

## Out of Scope

* Persistent on-disk event logs — buffer is in-memory only by design.
* Cross-session aggregation.
