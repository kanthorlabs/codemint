# User Story 2.4: Options + Confirm Loop

* **As the** Coding Workflow,
* **I want to** surface 2–3 candidate approaches with tradeoffs and either lock one or loop back to Goal Capture,
* **So that** the user makes the architectural call explicitly — instead of one approach being silently chosen by the Plan Generator.

> **Spec slot:** Step 4 (Options) + Step 5 (modify-or-confirm). The `/modify` edge is the **only mid-workflow pivot v1 supports**.

## Acceptance Criteria

### Proposal

1. Built-in `@codemint/options-proposer` skill is embedded in the binary.
2. Skill input: `workflow.goal_text`, `workflow.success_criteria`, the appended context from 2.1 + 2.3.
3. Skill output: 2–3 distinct candidate approaches, OR exactly 1 with explicit `reason_for_single` when the Goal is genuinely trivial.
4. Each option carries: `id` (single uppercase letter), `name`, `summary`, `files_touched_estimate` (int), `pros[]`, `cons[]`, `risk_level` (`low|medium|high`).
5. Output is rendered in the UI side-by-side (or stacked on narrow terminals).

### Confirm

6. User runs `/pick-option <id>`. Output handler `lock_chosen_option` writes the selected option's full JSON to `workflow.chosen_option` (column from 2.0.3) and advances to 2.5.
7. `/pick-option <id>` with an unknown id → command-level error message; story does not exit.

### Modify (the loop)

8. User runs `/modify`. Handler:
   - Clears `workflow.goal_text`, `workflow.success_criteria`, `workflow.chosen_option`.
   - Cancels any pending Plan Generation (2.5) tasks if already inserted. (For v1 this is impossible because 2.5 hasn't run yet — but the handler must be defensive.)
   - Re-creates 2.2, 2.3, 2.4 tasks with incremented seq, marking the original three as `Cancelled`. Scheduler then runs them again.
9. The user may `/modify` any number of times.

> **Why /modify routes all the way back to Goal Capture (not just back to Options):** if the Goal changes, Reality and Options must re-derive from it. A shorter `/modify-options` shortcut is intentionally deferred until v1 shows it's worth the complexity.

### Validation

10. Skill output that fails the `options-schema.json` validator → task `Failure` → 2.8.
11. `risk_level` is recorded but does not block the user. (2.10 YOLO Delegation may later refuse to auto-execute `high`-risk options; out of scope here.)

## Workflow definition

```yaml
- id: propose_options
  name: "Options + Confirm Loop"
  skill: "@codemint/options-proposer"
  depends_on: gather_targeted
  exit_on:
    commands:
      - "/pick-option"   # → handler: lock_chosen_option, advance
      - "/modify"        # → handler: reset_workflow_to_goal, loop back to 2.2
  output:
    schema: "skills/options-proposer/references/options-schema.json"
    handler: "lock_chosen_option"
```

## Dependencies

- 2.0.1 Workflow File Infrastructure (multi-command `exit_on`)
- 2.0.2 Task Routing (loop-back via task creation, not graph rewiring)
- 2.0.3 Workflow Execution State (`chosen_option` column)
- 2.0.5 Skill Injection
- 2.3 Goal-scoped Reality (predecessor)

## Out of Scope

- `/more-options` re-prompt without unlocking Goal. v1 ships one round per loop iteration.
- Hybrid options (combine A + B). Pick one, or `/modify`.
- Cost / hour estimation.
- A shortcut `/modify-options` that loops back only to 2.4. Deferred until needed.
