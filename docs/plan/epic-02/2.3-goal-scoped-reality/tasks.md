# Tasks for 2.3: Goal-scoped Reality

> Builds on 2.2's exit_on dispatcher + handler registry. No new generic infra here — only a skill, a YAML extension, and a small append-not-replace handler.

---

## Task 2.3.1: Embed `targeted-gatherer` SKILL.md

**Description:** Author the new skill at `internal/skills/embedded/targeted-gatherer/SKILL.md` per the index AC: takes locked Goal + criteria + cheap-pass output as input; runs grep + targeted reads + one-hop import follow; bounded ~30k-token budget; emits an *append* JSON object; emits `{"skipped": true, "reason": "..."}` on greenfield.

**Files to modify:**
- `internal/skills/embedded/targeted-gatherer/SKILL.md` (NEW)

**Skill body outline:**
- Frontmatter `name: targeted-gatherer`, `compatibility: Requires locked Goal`.
- Section "Inputs": `goal_text`, `success_criteria`, `cheap_context`.
- Section "Procedure": extract keywords, grep, read top hits, follow ≤1 import hop, stop at budget.
- Section "Output format" with the literal JSON shape:
  ```
  { "skipped": false, "keyword_hits": [...], "files_read": {...}, "token_budget_used": <int> }
  ```
- Section "Skip case": greenfield → `{"skipped": true, "reason": "..."}`.

**Verification:**
- `internal/skills/registry_test.go`: skill is discovered and parsed without error.
- `go test ./internal/skills/...` passes.
- Manual: `make build && ./build/codemint -mode cli` followed by `/skills` (or equivalent listing) shows `@codemint/targeted-gatherer`.

**Estimated effort:** 0.5 day

---

## Task 2.3.2: `append_targeted_context` output handler

**Description:** Concrete handler that validates the skill JSON against the documented shape (`skipped` bool + either `keyword_hits`/`files_read` OR `reason`), then writes the raw object to the *task's* `output` column. The cheap-pass output from 2.1 is preserved on its own task; downstream stories read both task outputs from the workflow's task list — no merging in the handler.

**Files to modify:**
- `internal/workflow/handlers_reality.go` (NEW)
- `internal/workflow/handlers_reality_test.go` (NEW)

**Implementation sketch:**
```go
func AppendTargetedContextHandler() HandlerFunc {
    return func(ctx context.Context, args HandlerArgs) error {
        var parsed struct {
            Skipped         bool                   `json:"skipped"`
            Reason          string                 `json:"reason,omitempty"`
            KeywordHits     []json.RawMessage      `json:"keyword_hits,omitempty"`
            FilesRead       map[string]string      `json:"files_read,omitempty"`
            TokenBudgetUsed int                    `json:"token_budget_used,omitempty"`
        }
        if err := json.Unmarshal([]byte(args.Output), &parsed); err != nil {
            return fmt.Errorf("append_targeted_context: invalid JSON: %w", err)
        }
        if parsed.Skipped && strings.TrimSpace(parsed.Reason) == "" {
            return errors.New("append_targeted_context: skipped=true requires reason")
        }
        // Output stays on the task row; no merge step needed in v1.
        return nil
    }
}
```

(The handler is mostly a *validator* — the skill output already lives in `task.Output`. The handler exists so malformed JSON converts cleanly to a task `Failure` per 2.8.)

**Verification:**
- Unit test (well-formed `skipped: false`): no error.
- Unit test (well-formed `skipped: true` with reason): no error.
- Unit test (`skipped: true` with empty reason): error.
- Unit test (malformed JSON): error.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 0.25 day

---

## Task 2.3.3: Register `append_targeted_context` in `HandlerRegistry`

**Description:** Wire the handler into the registry built in 2.2.2. One-line registration in the boot path.

**Files to modify:**
- `cmd/codemint/main.go`

**Implementation:**
```go
handlers := workflow.NewHandlerRegistry()
_ = handlers.Register("lock_workflow_goal", workflow.LockWorkflowGoalHandler(workflowRepo))
_ = handlers.Register("append_targeted_context", workflow.AppendTargetedContextHandler())
// ... future handlers register here.
```

**Verification:**
- Boot succeeds with both handlers registered.
- `go test ./cmd/codemint/...` (if any) passes; otherwise `make test`.

**Estimated effort:** 0.1 day

---

## Task 2.3.4: Extend `brainstorming/WORKFLOW.yaml` with `gather_targeted` step

**Description:** Append the new story between `capture_goal` and (future) `propose_options`.

**Files to modify:**
- `internal/workflow/embedded/brainstorming/WORKFLOW.yaml`

**Diff:**
```yaml
      - id: gather_targeted
        name: "Goal-scoped Reality"
        skill: "@codemint/targeted-gatherer"
        type: coding
        depends_on: capture_goal
        output:
          handler: "append_targeted_context"
```

**Verification:**
- `make build` succeeds.
- `go test ./internal/workflow/...` passes (registry parses the extended YAML; 2.0.5 L1 validation finds the new skill).
- Boot the REPL → `/workflow brainstorming` runs the three-step sequence.

**Estimated effort:** 0.1 day

---

## Task 2.3.5: E2E — gather → capture_goal → gather_targeted

**Description:** Extend the 2.2.7 e2e to cover the new step. Fake worker responds to the targeted-gatherer prompt with a canned `{"skipped": false, ...}`. Assert the task ends `Success` and `task.Output` contains the canned JSON. Add a second variant for the greenfield case (`skipped: true`).

**Files to modify:**
- `internal/orchestrator/goal_capture_e2e_test.go` (extend; or add `targeted_gather_e2e_test.go`)

**Verification:**
- Both variants pass.
- Negative case: malformed JSON ends task in `Failure` with the handler's error wrapped as the failure reason.
- `go test ./internal/orchestrator/... -run TargetedGather -v` passes.

**Estimated effort:** 0.5 day

---

## Dependency Order

```
2.3.1 (skill body)
   └─► 2.3.4 (YAML)
2.3.2 (handler) ─► 2.3.3 (register)
                              ⇣
                          2.3.5 (E2E)
```

## Total Estimated Effort: ~1.45 days
