# Tasks: 3.13 Pending Task Scheduler Loop

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`, `internal/repository/sqlite/`, `cmd/codemint/`
**Tech Stack:** Go, SQLite, `context`, channels
**Priority:** P0 (without it EPIC-02 outputs sit in DB forever)

---

## Task 3.13.1: `NextPending` Repository Query

* **Action:** Add an ordered query that returns the next executable task for a session.
* **Details:**
  * In `internal/repository/sqlite/task_repo.go`:
    ```sql
    SELECT * FROM task
    WHERE session_id = ? AND status = 0
    ORDER BY seq_epic ASC, seq_story ASC, seq_task ASC
    LIMIT 1;
    ```
  * Method signature:
    ```go
    func (r *TaskRepo) NextPending(ctx context.Context, sessionID string) (*domain.Task, error)
    ```
  * Returns `nil, nil` when nothing is pending (let the scheduler decide whether to idle).
* **Verification:**
  * `TestTaskRepo_NextPending_Order` — seed three pending tasks with mixed `seq_*`, assert the lowest tuple wins.
  * `TestTaskRepo_NextPending_SkipsNonPending` — only `status=0` rows are returned.

---

## Task 3.13.2: Scheduler Goroutine

* **Action:** Replace the current dormant `scheduler.go` with a session-bound loop.
* **Details:**
  * `Scheduler.Run(ctx context.Context, sess *ActiveSession)`:
    1. Wait for `runtime.AttachWorker` to return (lazy spawn).
    2. Loop:
       * `task := taskRepo.NextPending(ctx, sess.ID)`.
       * If `task == nil`, sleep 500ms (or block on `sess.WakeupCh()`) and continue.
       * Call `executor.Execute(ctx, sess, task)` (Story 3.14 implements the routing).
       * On `awaiting`, block on `sess.AwaitingCh()` until the prompt resolves; `executor` writes to that channel after `/approve` or `/deny`.
       * Detect story boundary: if `task.SeqStory != lastSeqStory`, call `worker.ResetContext` (Story 3.2) before sending the prompt.
    3. Exit on `ctx.Done()`.
  * Backoff: exponential up to 5s when consecutive errors hit `task_repo` (DB lock); reset on success.
* **Verification:**
  * `TestScheduler_RunsTasksInOrder` — seed 3 pending tasks, scheduler dispatches them in order, finishes when none remain.
  * `TestScheduler_RespectsAwaitingPause` — stub executor flips task to `awaiting`; loop blocks; release via channel; loop resumes.

---

## Task 3.13.3: Wakeup Signaling

* **Action:** Avoid busy-polling — let the scheduler block until something happens.
* **Details:**
  * Extend `ActiveSession`:
    ```go
    func (s *ActiveSession) Wakeup()
    func (s *ActiveSession) WakeupCh() <-chan struct{}
    ```
  * Fire `Wakeup()` from:
    * `Phase 5 - Activation` (Story 2.7) when newly generated tasks are committed.
    * `/approve`, `/deny`, `/yolo` commands.
    * Mid-flight pivots (Story 2.8) that update pending tasks.
  * The scheduler `select`s on `WakeupCh()` and a 1-minute fallback ticker.
* **Verification:**
  * `TestActiveSession_Wakeup_Coalesces` — multiple `Wakeup` calls before the loop reads the channel collapse to a single notification (use `len == 1` capacity).
  * Manual: brainstorm → activate plan → scheduler picks up the first task within 1s.

---

## Task 3.13.4: Wire Scheduler Into `main.go`

* **Action:** Start the loop after the runtime is built.
* **Details:**
  * In `cmd/codemint/main.go` after Step 12 (heartbeat):
    ```go
    if activeSession.Session != nil && activeSession.Project != nil {
        scheduler := orchestrator.NewScheduler(orchestrator.SchedulerConfig{
            Runtime:    runtime,
            TaskRepo:   taskRepo,
            Executor:   orchestrator.NewExecutor(...), // Story 3.14
            Mediator:   mediator,
        })
        go scheduler.Run(ctx, activeSession)
    }
    ```
  * On `/project-open` switching projects, call `scheduler.Restart(newSession)` so the loop targets the new session.
* **Verification:**
  * Boot with a session containing pending tasks → scheduler logs `scheduler: dispatching task=task-...` for each.
  * SIGINT → scheduler logs `scheduler: shutting down` within 5s.

---

## Task 3.13.5: Sequential Execution Guardrail

* **Action:** Enforce the EPIC-02 promise that tasks within a session never run in parallel.
* **Details:**
  * `Scheduler` holds an internal `sync.Mutex` around the dispatch step.
  * Reject concurrent `Run` calls — second invocation returns `ErrSchedulerAlreadyRunning`.
  * Add `Scheduler.IsRunning()` for `/acp-status` to display.
* **Verification:**
  * `TestScheduler_RejectsConcurrentRun` — two `go scheduler.Run(...)` calls; second returns the sentinel error.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.13.1  | Schema with `seq_epic`, `seq_story`, `seq_task` (EPIC-02 2.3) |
| 3.13.2  | 3.13.1, 3.12.1 (Runtime), 3.14.x (Executor) |
| 3.13.3  | 3.13.2, 2.7 (activation pipeline), 2.8 (pivots) |
| 3.13.4  | 3.13.2, 3.12.2 |
| 3.13.5  | 3.13.2 |

---

## Out of Scope

* Branching on `task.type` — that lives in 3.14.
* YOLO bypass — that lives in 3.16.
* Mid-flight pivot DB updates — those are EPIC-02.
