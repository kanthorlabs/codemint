# User Story 2.8: Error Escalation

* **As the** Coding Workflow,
* **I want** every task `Failure` to be reassigned to the session's Human Agent and the workflow paused until the human resolves it,
* **So that** errors never silently abort a run — and the human is always the entity that decides retry / skip / abort.

> **Spec slot:** Cross-cutting requirement from `appendings.md`. Every other story (2.1–2.7) hands off to this one on Failure.

## Acceptance Criteria

### Detection

1. The scheduler observes `task.status` transitioning to `Failure` (set by any caller — skill handler, verification command exit, confirmation reject, JSON-validation error in 2.4 / 2.5, etc.).
2. On observation, the scheduler:
   - Reassigns the failed task to the session's **Human Agent** (`task.assignee_id = humanAgentID`).
   - Records the failure context in `task.output` as `{"sentinel": "<sentinel>", "reason": "<text>", "stage": "<story-id>"}`. Existing failure-sentinel constants (`FailureSentinelSkillNotFound`, `FailureSentinelInvalidInput`, etc.) are reused.
   - Transitions the session to `SessionStatusAwaiting`.
   - Halts. No subsequent tasks in the same workflow execution are picked up.

### Resolution

3. New REPL command `/resolve <action> [notes]` with three actions:
   - `retry` — clones the failed task into a new `Pending` task (new `task_id`, same `type` and `input`, incremented `seq_task`), sets the original to `Cancelled`, resumes the scheduler. The retry runs the same skill / command; if it fails again it re-enters this story.
   - `skip`  — marks the failed task `Cancelled`, advances to the next eligible task in `seq` order. (Use case: a flaky verification the user knows is wrong.)
   - `abort` — marks the workflow `Cancelled`, all remaining `Pending` tasks `Cancelled`, session back to `Active` (new prompts allowed but this workflow is over).
4. `notes` (optional) is appended to the resolved task's `output` as `human_resolution_notes`. Useful when the human Reject'd in 2.7 with feedback that the next iteration should consider.
5. `/resolve` with no arguments prints the current escalation context (failed task summary + available actions) and exits.

### Audit

6. `/resolve` itself produces a Coordination task (`TaskTypeCoordination (3)`) recording who resolved, which action, and the notes. Existing `InteractionRecorder` already covers this for slash commands.
7. Verbosity respect: the escalation prompt itself always surfaces (it's a blocker). Below-threshold side messaging (e.g., "scheduler halted") is suppressed at `quiet`.

### Lifecycle invariants

8. At most one task per workflow execution is in escalation at any time. (The scheduler is halted, so no others are picked up.) If a parallel external trigger somehow Failures another task while one is escalated, the second one is queued behind the first; `/resolve` only acts on the head of the queue.
9. The Human Agent assignee referenced here is the same one used by 2.7 Confirmation. There is exactly one Human Agent per session.

## Failure sources covered

| Story | Source of Failure |
|---|---|
| 2.1 | Skill output JSON invalid; project unreadable. |
| 2.2 | Skill output JSON invalid; missing `goal_text` or empty criteria. |
| 2.3 | Skill output JSON invalid (greenfield `skipped:true` is *not* failure). |
| 2.4 | Skill output JSON invalid; schema violation. |
| 2.5 | Plan JSON invalid; insert transaction rolled back. |
| 2.6 | Verification command non-zero exit. |
| 2.7 | User selected `Reject`. |
| 2.8 | (none — this story IS the handler) |

## Dependencies

- 2.0.2 Task Routing (clone-and-replace for `retry`)
- 2.0.3 Workflow Execution State (the workflow's `status` field)
- 2.0.4 Workflow Command (`/resolve` registered alongside `/workflow`)

## Out of Scope

- Per-task retry budgets. v1 lets the human retry indefinitely.
- Auto-recovery heuristics (e.g., "if skill output failed JSON 3x, ask a different skill"). All recovery is human-driven in v1.
- Branching to a different skill on `retry`. The retry is identical to the original task.
- Nested escalations (two halted tasks at once). Constrained out by AC #8.
