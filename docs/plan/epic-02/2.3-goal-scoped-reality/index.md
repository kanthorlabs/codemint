# User Story 2.3: Goal-scoped Reality

* **As the** Coding Workflow,
* **I want to** run a second, Goal-scoped Gatherer pass after the Goal is locked,
* **So that** Options proposal sees dense, relevant context — not the whole tree.

> **Spec slot:** Step 3 (Goal-scoped context → REALITY). Inserts between 2.2 Goal Capture and 2.4 Options.

## Acceptance Criteria

1. Built-in `@codemint/targeted-gatherer` skill is embedded in the binary.
2. Story is gated on `workflow.goal_text` + `workflow.success_criteria` being non-null.
3. Skill receives, as input: the locked Goal text, the success criteria, and the cheap-pass output from 2.1.
4. Skill performs targeted operations only:
   - Grep for keywords drawn from `goal_text` and each entry in `success_criteria`.
   - Read files matching the grep hits.
   - Follow at most one hop of imports / references from the highest-signal hit.
5. Skill is bounded by an approximate ~30k-token budget on read content. The budget is advisory (LLM self-tracks); overshoot is allowed but logged.
6. Output is **appended** to the cheap-pass context from 2.1 (not a replacement). Downstream stories see both layers in the task's `output`.
7. Skill skips cleanly if the Goal contains no codebase keywords (greenfield request) — emits `{"skipped": true, "reason": "..."}` so 2.4 can adapt.
8. Skill output that fails JSON validation → task `Failure` → 2.8.

## Workflow definition

```yaml
- id: gather_targeted
  name: "Goal-scoped Reality"
  skill: "@codemint/targeted-gatherer"
  depends_on: capture_goal
```

## Dependencies

- 2.0.1 Workflow File Infrastructure
- 2.0.3 Workflow Execution State (`goal_text`, `success_criteria`)
- 2.0.5 Skill Injection
- 2.2 Goal Capture (predecessor)

## Out of Scope

- Hard token-budget enforcement (LLM self-tracks; v1 trusts it).
- AST-level analysis. Grep + read is enough for v1.
- Caching of targeted-gather results between runs.
