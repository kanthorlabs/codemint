# User Story 2.5: Plan Generation

* **As the** Coding Workflow,
* **I want to** expand the chosen option into an Epic → Story → Task plan and persist it to SQLite,
* **So that** the scheduler has a sequential, executable backlog to drive workers against.

> **Spec slot:** Step 6 (Generate plan → WAY FORWARD). Auto-injects the SYSTEM-TASK guardrails (2.6 Verification + 2.7 Confirmation) per story.

## Acceptance Criteria

### Skill output

1. Built-in `@codemint/task-generator` skill is embedded in the binary.
2. Skill input (immutable): `workflow.goal_text`, `workflow.success_criteria`, `workflow.chosen_option`, the appended context from 2.1 + 2.3.
3. Skill output: JSON matching `task-schema.json` — a list of Epics, each with Stories, each with Tasks. Each Task carries `id`, `name`, `description`, `files[]`, optional `verification` (command override).
4. Skill must NOT include Verification or Confirmation tasks. The Go-side handler injects those.
5. If the chosen option is genuinely incoherent and cannot be expanded, skill emits `{"error": "..."}` instead of fabricating tasks. Handler converts that to task `Failure` → 2.8.

### DB hierarchy (flat schema)

6. Plan is stored in the **single `task` table** with sequence integers — no Epic / Story tables.
   - `seq_epic`  = epic index in the plan (0-based).
   - `seq_story` = story index within the epic (0-based).
   - `seq_task`  = task index within the story (0-based, leaving room for guardrail injection).
7. Coding tasks: `type = TaskTypeCoding (0)`, `assignee_id = <Assistant agent for this session>`.

### Guardrail auto-injection

8. After each Story's coding tasks, the handler appends:
   - One **Verification** task — `type = TaskTypeVerification (1)`, `assignee = Assistant`. Owns the story's `verification` command (or workflow default `go test ./...`). See 2.6.
   - One **Confirmation** task — `type = TaskTypeConfirmation (2)`, `assignee = Human`. See 2.7.
9. Per-story opt-out via `guardrails: { verification: false, confirmation: false }` in WORKFLOW.yaml.
10. Workflow-level default lives under `settings.guardrails`.

### Sequencing

11. Scheduler executes strictly in `(seq_epic, seq_story, seq_task)` order. No parallelism.
12. Each Verification task has `depends_on = <last coding task in story>`; Confirmation has `depends_on = <verification task>` (or last coding task if verification is opted out).

### Failure surface

13. Malformed plan JSON → task `Failure` → 2.8. The plan is NOT partially inserted; insertion is a single transaction.

## Workflow definition

```yaml
- id: generate
  name: "Plan Generation"
  skill: "@codemint/task-generator"
  depends_on: propose_options
  output:
    schema: "skills/task-generator/references/task-schema.json"
    handler: "create_implementation_tasks"
```

## Dependencies

- 2.0.1 Workflow File Infrastructure
- 2.0.2 Task Routing (`depends_on` column)
- 2.0.3 Workflow Execution State (`chosen_option`)
- 2.0.5 Skill Injection
- 2.4 Options + Confirm Loop (predecessor)

## Out of Scope

- A separate Human Review & Activation gate. Confirmation happened in 2.4 (the user picked an option); Plan Generation runs straight through.
- Mid-flight task editing. The only revision path in v1 is `/modify` from 2.4 (which runs before 2.5).
- Parallel task execution.
- Sub-task decomposition by the scheduler. The skill emits the leaves; we don't refine further.
