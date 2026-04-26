# Tasks: 3.16 YOLO Auto-Approval Bypass

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`, `internal/repository/sqlite/`
**Tech Stack:** Go, SQLite, agent registry
**Priority:** P1 (functional only when at least one Story has been delegated; depends on EPIC-04 §4.13 toggle)

---

## Task 3.16.1: Cache `sys-auto-approve` Agent ID

* **Action:** Look up the YOLO agent once at startup.
* **Details:**
  * In `Runtime.NewRuntime`:
    ```go
    yolo, err := agentRepo.FindByName(ctx, "sys-auto-approve")
    if err != nil || yolo == nil {
        return nil, fmt.Errorf("runtime: sys-auto-approve agent missing — re-run agent seeding")
    }
    rt.YoloAgentID = yolo.ID
    ```
  * Hard-fail at startup if the agent isn't seeded — this catches a corrupted DB before tasks start running and silently mis-routing.
* **Verification:**
  * `TestRuntime_New_LoadsYoloID` — agent seeded → `runtime.YoloAgentID` is non-empty.
  * `TestRuntime_New_FailsWithoutYoloAgent` — empty `agent` table → constructor returns the sentinel error.

---

## Task 3.16.2: Bypass in `executeConfirmation` and `executeRetrospective`

* **Action:** Short-circuit before calling `mediator.PromptDecision`.
* **Details:**
  * Helper:
    ```go
    func (e *Executor) isAutoApproved(task *domain.Task) bool {
        return task.AssigneeID.Valid && task.AssigneeID.String == e.runtime.YoloAgentID
    }
    ```
  * In `executeConfirmation`:
    ```go
    if e.isAutoApproved(task) {
        task.Status = domain.TaskStatusCompleted
        task.Output = sqlNullString(`{"auto_approved":true,"agent":"sys-auto-approve"}`)
        return e.taskRepo.Update(ctx, task)
    }
    ```
  * Same pattern in `executeRetrospective` — auto-approved retrospectives skip the freeform follow-up entirely.
* **Verification:**
  * `TestExecutor_Confirmation_AutoApproved` — task with YOLO assignee bypasses the mediator (mock recorded zero `PromptDecision` calls); status `success`, output JSON matches.
  * `TestExecutor_Retrospective_AutoApprovedSkipsFreeform` — freeform prompt never opens.

---

## Task 3.16.3: UI Audit Notification

* **Action:** Tell the user when a Story or Epic auto-approves so the YOLO toggle isn't invisible.
* **Details:**
  * Emit `registry.UIEvent{Kind: EventYoloAutoApproved, Payload: {"task_id": task.ID, "seq_story": task.SeqStory}}` from the bypass branch.
  * TUI adapter renders a single-line `⚡ auto-approved: Story <N>`; CUI adapter sends a low-priority chat line on Story boundaries only (skip per-task to avoid spam).
* **Verification:**
  * `TestExecutor_AutoApproved_EmitsEvent` — assert one `EventYoloAutoApproved` per bypass.
  * Manual: delegate a Story → console shows the auto-approval line.

---

## Task 3.16.4: Reject YOLO on Non-Approval Types

* **Action:** Refuse to interpret `assignee_id == sys-auto-approve` on a Coding or Verification task — those should always run on their own merit.
* **Details:**
  * In `Execute` dispatch, before branching, log and clear the YOLO assignee for `type=0` / `type=1`:
    ```go
    if e.isAutoApproved(task) && (task.Type == TaskTypeCoding || task.Type == TaskTypeVerification) {
        slog.Warn("executor: yolo assignee on executable task — ignoring", "task", task.ID, "type", task.Type)
    }
    ```
  * Document the constraint in `internal/domain/task.go` doc comment.
* **Verification:**
  * `TestExecutor_Coding_IgnoresYoloAssignee` — Coding task with YOLO assignee still runs the ACP path; warn log captured.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.16.1  | EPIC-02 §2.9 (agent seeding); 3.12.1 (Runtime) |
| 3.16.2  | 3.16.1, 3.14.4, 3.14.5 |
| 3.16.3  | 3.16.2, 3.18 (mediator broadcast) |
| 3.16.4  | 3.14.1 |

---

## Out of Scope

* `[Delegate to Auto Approval]` UI button — EPIC-04 §4.10/§4.13.
* Storing per-Epic delegation history — EPIC-05.
