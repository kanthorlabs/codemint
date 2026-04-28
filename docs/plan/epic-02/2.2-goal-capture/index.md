# User Story 2.2: Goal Capture

* **As the** Coding Workflow,
* **I want to** elicit and lock a one-sentence goal plus 1–5 testable success criteria,
* **So that** the rest of the workflow (Reality, Options, Plan, Verification) traces back to a fixed, machine-readable target.

> **Spec slot:** Step 2 (Clarify Goals → GOAL). Re-entered when the user runs `/modify` in 2.4.

## Acceptance Criteria

1. Built-in `@codemint/goal-capture` skill is embedded in the binary.
2. Skill runs in two passes:
   - **Pass 1 — Goal statement:** elicit one sentence with a verb, an object, and a visible outcome. Reject vague ("make it better"), process-only ("refactor for cleanliness"), unbounded ("add features").
   - **Pass 2 — Success criteria:** propose 1–5 testable criteria. Reject criteria that aren't observable / measurable.
3. Story exits when the user types `/lock-goal`.
4. Output handler `lock_workflow_goal` writes `workflow.goal_text` (TEXT) and `workflow.success_criteria` (JSON array). Columns exist from 2.0.3.
5. Goal is **immutable** for the rest of the workflow run. The only way to change it is `/modify` from 2.4, which clears Goal + criteria + chosen_option and re-routes to this story.
6. Skill output that fails JSON validation (missing `goal_text`, empty criteria array) → task `Failure` → 2.8.

## Workflow definition

```yaml
- id: capture_goal
  name: "Goal Capture"
  skill: "@codemint/goal-capture"
  depends_on: gather
  exit_on:
    command: "/lock-goal"
  output:
    handler: "lock_workflow_goal"
```

## Dependencies

- 2.0.1 Workflow File Infrastructure (skill embedding, output handler registration)
- 2.0.3 Workflow Execution State (`goal_text`, `success_criteria` columns)
- 2.0.5 Skill Injection
- 2.1 Project Overview (predecessor)

## Out of Scope

- Goal versioning / history. Only the current locked Goal is stored.
- Multi-goal workflows. One workflow run = one Goal.
- A standalone `/revise-goal` command. The 2.4 `/modify` edge is the only revision path in v1.
