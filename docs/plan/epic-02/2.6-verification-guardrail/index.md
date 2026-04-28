# User Story 2.6: Verification Guardrail

* **As the** Coding Workflow,
* **I want** an auto-injected Verification task at the end of every Story,
* **So that** the per-story coding output is checked against a deterministic command (compile, lint, tests) before it's gated for human approval.

> **Spec slot:** Step 8.1 (Verification SYSTEM-TASK). Runs after the Story's coding tasks, before its 2.7 Confirmation.

## Acceptance Criteria

1. The 2.5 Plan Generation handler auto-injects exactly one Verification task per coding Story unless `guardrails.verification: false` is set on the Story or in workflow `settings.guardrails`.
2. Verification task: `type = TaskTypeVerification (1)`, `assignee = Assistant`.
3. Command resolution order:
   1. Story-local `verification:` field in WORKFLOW.yaml.
   2. Workflow-level `settings.verification`.
   3. Hard default: `go test ./...`.
4. Scheduler executes the command via the existing Bash / ACP tool path (whichever the Assistant chooses; the worker is unconstrained).
5. Exit code 0 → task `Success`. Scheduler advances to the Story's Confirmation task.
6. Non-zero exit code (or any worker error) → task `Failure` → 2.8 Error Escalation. The Confirmation task does NOT run automatically; it waits for the human resolution.
7. Verbosity respect: at `quiet`, only failures surface; at `normal`, a one-liner per verification result; at `verbose`+, full command stdout/stderr.

## Workflow integration

Verification is not a top-level WORKFLOW.yaml step — it's injected per-story by 2.5's `create_implementation_tasks` handler. Per-story override:

```yaml
- id: hotfix
  skill: quick-fix
  guardrails:
    verification: false        # skip the test gate for this story

- id: implement-foo
  skill: foo-coder
  verification: "make test-unit"
```

Workflow-level default:

```yaml
settings:
  guardrails:
    verification: true
  verification: "go test ./..."
```

## Dependencies

- 2.0.2 Task Routing (`depends_on` for sequencing)
- 2.5 Plan Generation (the injector)
- 2.8 Error Escalation (failure handler)

## Out of Scope

- Goal-level verification against `success_criteria`. v1 only checks story-level correctness; Goal achievement is a manual review the user does at retrospective time (not in EPIC-02).
- Auto-retry on flaky tests. A failure goes to 2.8; the human picks `retry` if appropriate.
- Coverage thresholds, custom result parsers. Exit code is the only signal in v1.
