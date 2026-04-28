# User Story 2.7: Confirmation Guardrail

* **As the** Coding Workflow,
* **I want** an auto-injected Confirmation task at the end of every Story,
* **So that** execution pauses for explicit human approval (or auto-approval under YOLO) before moving on.

> **Spec slot:** Step 8.2 (Confirmation SYSTEM-TASK). Runs after 2.6 Verification (or directly after coding if verification is opted out).

## Acceptance Criteria

1. The 2.5 Plan Generation handler auto-injects exactly one Confirmation task per Story unless `guardrails.confirmation: false`.
2. Confirmation task: `type = TaskTypeConfirmation (2)`, `assignee = Human Agent` for this session. Under YOLO (2.10) the assignee is `sys-auto-approve`.
3. When the scheduler picks up a Confirmation task assigned to a human:
   - Session transitions to `Awaiting`.
   - UIMediator emits a decision prompt summarizing the Story's coding output + the Verification result.
   - Three responses: `Approve`, `Reject`, `Cancel`.
4. Mapping responses to status:
   - `Approve` → `TaskStatusSuccess`. Scheduler moves to the next Story.
   - `Reject` → `TaskStatusFailure` → 2.8 Error Escalation (the rejection becomes the failure context the human resolves immediately).
   - `Cancel` → `TaskStatusCancelled`. Workflow ends with `WorkflowStatus = Cancelled`.
5. Under YOLO, the scheduler short-circuits: it sets the task to `Success` immediately when the assignee is `sys-auto-approve`. No UI prompt. The behavior is identical to the human-Approve path.
6. Verbosity respect: at `quiet`, only the prompt itself surfaces; at `normal`+, the Story summary is rendered above the prompt.

## Workflow integration

Like Verification, Confirmation is injected per-story by 2.5. Per-story / workflow opt-outs:

```yaml
- id: minor-fix
  skill: quick-fix
  guardrails:
    confirmation: false

settings:
  guardrails:
    confirmation: true
```

## Dependencies

- 2.0.2 Task Routing (`depends_on` for sequencing)
- 2.5 Plan Generation (the injector)
- 2.6 Verification Guardrail (predecessor in the per-Story chain)
- 2.8 Error Escalation (Reject path)
- 2.9 / 2.10 YOLO (alternate assignee)

## Out of Scope

- Multi-step Confirmation (e.g., separate Approve-coding vs Approve-tests). One Approve gate per Story.
- Reviewer comments stored as structured data. Free-text feedback can be captured by 2.8 when the user picks `Reject` and supplies notes.
- Auto-loop-back to a specific earlier story on Reject. v1 simply hands the failure to 2.8; the human decides whether to retry the Story, skip it, or abort.
