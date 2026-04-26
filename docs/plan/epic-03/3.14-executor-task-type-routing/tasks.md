# Tasks: 3.14 Executor Task-Type Routing & Confirmation Pause

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`, `internal/domain/`
**Tech Stack:** Go, type switch, JSON
**Priority:** P0 (scheduler from 3.13 needs an Executor that does more than send prompts)

---

## Task 3.14.1: TaskType Dispatch Table

* **Action:** Define a single dispatch entry point keyed on `domain.TaskType`.
* **Details:**
  * In `internal/orchestrator/executor.go`:
    ```go
    func (e *Executor) Execute(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
        switch task.Type {
        case domain.TaskTypeCoding:        return e.executeCoding(ctx, sess, task)
        case domain.TaskTypeVerification:  return e.executeVerification(ctx, sess, task)
        case domain.TaskTypeConfirmation:  return e.executeConfirmation(ctx, sess, task)
        case domain.TaskTypeCoordination:  return e.skipCoordination(ctx, task)
        case domain.TaskTypeRetrospective: return e.executeRetrospective(ctx, sess, task)
        default:
            return fmt.Errorf("executor: unknown task type %d", task.Type)
        }
    }
    ```
  * `Coordination` is a no-op (already-completed echoes from `/acp`, `/help`, etc.).
* **Verification:**
  * `TestExecutor_Execute_Routes` — table test with one task per type and a stubbed inner method; assert correct branch fires.

---

## Task 3.14.2: Coding Task → ACP Prompt

* **Action:** Forward Coding tasks to the worker.
* **Details:**
  * `executeCoding`:
    1. Mark task `processing` via `taskRepo.UpdateStatus`.
    2. `consumer.SetCurrentTask(task.ID)` so the buffer registry and status mapper attribute incoming events to this task.
    3. Parse `task.Input` JSON into `acp.SessionPromptParams` (Story 3.15 owns the schema).
    4. `worker.Send(NewRequest(MethodSessionPrompt, params))`.
    5. Block on the status mapper signaling `Success` or `Failure` for `task.ID`. Use a per-task `done chan struct{}` registered with the mapper.
  * Timeout: honor `task.timeout` (column added in migration `000002`); cancel via `session/cancel` on overrun.
* **Verification:**
  * `TestExecutor_Coding_SendsPromptAndAwaitsTerminal` — stub mapper signals success after a fake `turn-end`; executor returns nil and task row shows `success`.
  * `TestExecutor_Coding_TimesOut` — `task.timeout = 50ms`; executor sends `session/cancel` and returns `ErrTaskTimeout`.

---

## Task 3.14.3: Verification Task → LocalRunner

* **Action:** Run the verification command without involving the ACP agent.
* **Details:**
  * `executeVerification`:
    1. Parse `task.Input` JSON: `{"command": "go test ./...", "cwd": "."}`. Reject if `command` is missing.
    2. Match the command against `project_permission` — fail closed if it isn't whitelisted (verification commands must be pre-approved at brainstorm time per EPIC-02 §2.4).
    3. Execute via `runtime.Interceptor.Runner.Run(ctx, cmd, args, cwd)`.
    4. Persist exit code, stdout (truncated 64 KiB), stderr in `task.Output` JSON: `{"exit_code": N, "stdout": "...", "stderr": "..."}`.
    5. Map exit code: `0 → Success`, non-zero → `Failure`.
* **Verification:**
  * `TestExecutor_Verification_PassesOnZeroExit` — stub runner returns `exit=0`; task ends `success`.
  * `TestExecutor_Verification_BlocksUnwhitelisted` — runner never invoked; task ends `failure` with reason `command_not_whitelisted`.

---

## Task 3.14.4: Confirmation Task → Human Pause

* **Action:** Drive the EPIC-02 §2.5 manual-approval pause.
* **Details:**
  * `executeConfirmation`:
    1. Set `task.Status = TaskStatusAwaiting`; `session.Status = SessionStatusAwaiting`.
    2. Build a `registry.PromptRequest`:
       * `Title`: derived from the parent User Story (`task.SeqStory`).
       * `Body`: `task.Input.prompt` (Task Generator pre-fills "Please review the changes for Story X").
       * `Options`: `[{ID: "approve", Label: "Approve & Continue"}, {ID: "revise", Label: "Revise"}, {ID: "abort", Label: "Abort Session"}]`.
    3. `mediator.PromptDecision(ctx, req)` blocks; returned `optionID` decides:
       * `approve` → task `success`, scheduler picks next.
       * `revise` → emit `EventReviseRequested` for EPIC-02 §2.8 to consume; task stays `awaiting`.
       * `abort` → task `failure` with `reason=user_abort`; scheduler exits.
* **Verification:**
  * `TestExecutor_Confirmation_BlocksUntilApproval` — stub mediator delays 100ms then returns `approve`; executor returns nil within 200ms; task `success`.
  * `TestExecutor_Confirmation_AbortPropagates` — option `abort` sets task failure and sentinel error.

---

## Task 3.14.5: Retrospective Task → Conversational Prompt

* **Action:** Reuse the confirmation flow with a skippable conversational template.
* **Details:**
  * `executeRetrospective`:
    1. Build `PromptRequest` with body matching EPIC-02 §2.6 (`"Epic <slug> just wrapped. Anything I did annoyingly?"`).
    2. Options: `[{ID: "share", Label: "Share Feedback"}, {ID: "skip", Label: "Skip"}]`.
    3. If `share`: open a follow-up `PromptDecision` of kind `freeform` (Story 3.18 must support this kind). Persist freeform text into `task.Output`.
    4. Always mark task `success` (skipping is allowed; the retrospective never blocks the next epic indefinitely).
* **Verification:**
  * `TestExecutor_Retrospective_StoresFeedback` — freeform reply lands in `task.output`.
  * `TestExecutor_Retrospective_SkipStillSucceeds` — option `skip` marks task `success` with empty output.

---

## Task 3.14.6: Coordination Task No-Op

* **Action:** Make sure user-command echoes (Type 3) don't get sent to the agent.
* **Details:**
  * `skipCoordination(ctx, task)` simply returns nil; the row was already inserted with `status=5` by the interaction recorder.
  * Add a sanity log: `slog.Debug("executor: skipping coordination task", "id", task.ID)`.
* **Verification:**
  * `TestExecutor_Coordination_DoesNothing` — worker `Send` count remains 0 after dispatch.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.14.1  | `domain.TaskType` enum (EPIC-02 §2.4–§2.6) |
| 3.14.2  | 3.12.4 (consumer task tracking), 3.15 (input schema) |
| 3.14.3  | 3.5 (LocalRunner), 3.12.3 (permission load) |
| 3.14.4  | 3.18 (mediator broadcast), 3.6 (approval flow primitives) |
| 3.14.5  | 3.14.4, 3.18 (freeform prompt kind) |
| 3.14.6  | 3.14.1 |

---

## Out of Scope

* `sys-auto-approve` short-circuit — see 3.16.
* Mid-flight pivot DB writes — EPIC-02 §2.8.
