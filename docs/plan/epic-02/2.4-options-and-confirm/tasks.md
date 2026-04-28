# Tasks for 2.4: Options + Confirm Loop

> First story that needs **multi-command** `exit_on` (`/pick-option` + `/modify`) — the dispatcher from 2.2.4 is single-command. Extend it here.
> First story that needs **loop-back task creation** — `/modify` cancels Goal/Reality/Options tasks and re-creates them with incremented seq.

---

## Task 2.4.1: Embed `options-proposer` SKILL.md

**Description:** Author the new skill per index AC: 2–3 distinct candidates with `id`/`name`/`summary`/`files_touched_estimate`/`pros[]`/`cons[]`/`risk_level`; OR exactly 1 with `reason_for_single` when the Goal is trivial. Skill MUST stay neutral — no recommendation. Anti-patterns called out (variant-only options, "same as A but cleaner", etc.).

**Files to modify:**
- `internal/skills/embedded/options-proposer/SKILL.md` (NEW)
- `internal/skills/embedded/options-proposer/references/options-schema.json` (NEW)

**Schema:**
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["options"],
  "properties": {
    "options": {
      "type": "array",
      "minItems": 1,
      "maxItems": 3,
      "items": {
        "type": "object",
        "required": ["id","name","summary","files_touched_estimate","pros","cons","risk_level"],
        "properties": {
          "id": { "type": "string", "pattern": "^[A-C]$" },
          "name": { "type": "string" },
          "summary": { "type": "string" },
          "files_touched_estimate": { "type": "integer", "minimum": 0 },
          "pros": { "type": "array", "items": { "type": "string" } },
          "cons": { "type": "array", "items": { "type": "string" } },
          "risk_level": { "enum": ["low","medium","high"] }
        }
      }
    },
    "reason_for_single": { "type": ["string","null"] }
  }
}
```

**Verification:**
- Skill discovered + parsed by registry tests.
- `go test ./internal/skills/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.4.2: Extend `exit_on` to accept a list of commands

**Description:** Today `Story.ExitOn.Command` is a single string. Allow either a single command (existing) OR a `commands: [list]` form. The dispatcher (2.2.4) must accept any of the listed commands and pass the matched command name to the handler via `HandlerArgs.ExitCmd`.

**Files to modify:**
- `internal/workflow/parser.go` (parse new `commands` field; keep singular `command` for back-compat)
- `internal/workflow/parser_test.go`
- `internal/domain/core.go` (`ExitCondition.Commands []string` alongside existing `Command string`)
- `internal/orchestrator/exit_on_dispatcher.go` (match against any registered command; pass matched name to handler invocation)
- `internal/orchestrator/exit_on_dispatcher_test.go`

**YAML form:**
```yaml
exit_on:
  commands:
    - "/pick-option"
    - "/modify"
```

(Validation rule: exactly one of `command` / `commands` must be set; both → parse error.)

**Verification:**
- Parser test for both single and list forms.
- Parser test rejects YAML that sets both forms.
- Dispatcher test: each command in the list closes the task; the handler receives the matched command name.
- Existing 2.2 single-command path still works.
- `go test ./internal/workflow/... ./internal/orchestrator/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.4.3: `lock_chosen_option` handler

**Description:** Handler invoked when the matched exit command is `/pick-option`. Parses the args (`/pick-option B` → `B`), finds the corresponding option in the skill's JSON output, validates against `options-schema.json`, then calls `WorkflowRepo.LockChosenOption` with the picked option's full JSON.

**Files to modify:**
- `internal/workflow/handlers_options.go` (NEW)
- `internal/workflow/handlers_options_test.go` (NEW)

**Implementation sketch:**
```go
func LockChosenOptionHandler(repo repository.WorkflowRepository) HandlerFunc {
    return func(ctx context.Context, args HandlerArgs) error {
        // The dispatcher passes "/pick-option B" → split out the picked id.
        parts := strings.Fields(args.ExitCmd)
        if len(parts) != 2 || parts[0] != "/pick-option" {
            return fmt.Errorf("lock_chosen_option: expected '/pick-option <id>', got %q", args.ExitCmd)
        }
        pickedID := parts[1]

        var proposed struct {
            Options []json.RawMessage `json:"options"`
        }
        if err := json.Unmarshal([]byte(args.Output), &proposed); err != nil {
            return fmt.Errorf("lock_chosen_option: invalid options JSON: %w", err)
        }
        for _, raw := range proposed.Options {
            var meta struct{ ID string `json:"id"` }
            _ = json.Unmarshal(raw, &meta)
            if meta.ID == pickedID {
                return repo.LockChosenOption(ctx, args.WorkflowID, string(raw))
            }
        }
        return fmt.Errorf("lock_chosen_option: option %q not in proposed set", pickedID)
    }
}
```

**Verification:**
- Unit test (happy path): picked option JSON is written to repo.
- Unit test (unknown id): error.
- Unit test (malformed JSON): error.
- Unit test (already-locked, repo error): bubbles up.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.4.4: `reset_workflow_to_goal` handler (the `/modify` loop)

**Description:** Handler invoked when the matched exit command is `/modify`. Steps:
1. Clear `workflow.goal_text`, `workflow.success_criteria`, `workflow.chosen_option` (new repo method `ResetGOROW`).
2. Mark the current Goal/Reality/Options task chain `Cancelled` (incl. completed Goal+Reality and the still-active Options task — the dispatcher already moved the Options task to Success; revert that to Cancelled in the same transaction OR mark a new equivalent triple after).
3. Re-insert three new tasks (capture_goal → gather_targeted → propose_options) with `seq_task` incremented past the existing maximum.
4. Scheduler picks them up via the standard pending-task path.

**Files to modify:**
- `internal/repository/sqlite/workflow_repo.go` (`ResetGOROW(ctx, workflowID) error` — sets the three columns to NULL)
- `internal/repository/sqlite/workflow_repo_test.go`
- `internal/workflow/handlers_options.go` (the `ResetWorkflowToGoalHandler`)
- `internal/workflow/handlers_options_test.go`

**Implementation sketch:**
```go
func ResetWorkflowToGoalHandler(
    workflowRepo repository.WorkflowRepository,
    taskRepo repository.TaskRepository,
    fileRegistry *FileRegistry,
) HandlerFunc {
    return func(ctx context.Context, args HandlerArgs) error {
        // 1. clear the three columns
        if err := workflowRepo.ResetGOROW(ctx, args.WorkflowID); err != nil { return err }

        // 2. cancel still-pending Goal/Reality/Options tasks for this workflow
        //    (Options task is the current one; the dispatcher already set it Success
        //    so we re-mark it Cancelled to tag the iteration as discarded.)
        if err := taskRepo.CancelByWorkflowAndStoryIDs(
            ctx, args.WorkflowID,
            []string{"capture_goal", "gather_targeted", "propose_options"},
        ); err != nil { return err }

        // 3. re-insert the three stories with bumped seq_task.
        wf, err := workflowRepo.GetByID(ctx, args.WorkflowID)
        if err != nil { return err }
        spec, err := fileRegistry.Get(wf.FilePath.String)
        if err != nil { return err }
        return regenerateGoalLoopTasks(ctx, taskRepo, wf, spec)
    }
}
```

`regenerateGoalLoopTasks` is the small helper that walks the WORKFLOW.yaml's first epic, picks the three story IDs above, and creates new task rows with `seq_task = max(existing)+1` per story.

**Verification:**
- Unit test on `ResetGOROW`: sets all three columns to NULL; `LockGoal`/`LockChosenOption` succeed again afterwards (they require NULL).
- Unit test on `CancelByWorkflowAndStoryIDs`: only the listed story IDs flip to `Cancelled`.
- Unit test on the handler: full happy path inserts three new tasks with bumped seq.
- E2E: covered by 2.4.6.
- `go test ./internal/repository/sqlite/... ./internal/workflow/...` passes.

**Estimated effort:** 1.5 days

---

## Task 2.4.5: Register `/pick-option`, `/modify` REPL commands + handlers + extend YAML

**Description:** Boot wiring.
- Register `/pick-option` and `/modify` as known REPL commands (no-op bodies, dispatched by 2.2.4).
- Register `lock_chosen_option` + `reset_workflow_to_goal` in `HandlerRegistry`.
- Extend `brainstorming/WORKFLOW.yaml` with the `propose_options` story.

**Files to modify:**
- `internal/repl/core_commands.go` (or sibling)
- `cmd/codemint/main.go`
- `internal/workflow/embedded/brainstorming/WORKFLOW.yaml`

**YAML diff:**
```yaml
      - id: propose_options
        name: "Options + Confirm Loop"
        skill: "@codemint/options-proposer"
        type: coding
        depends_on: gather_targeted
        exit_on:
          commands:
            - "/pick-option"
            - "/modify"
        output:
          schema: "skills/options-proposer/references/options-schema.json"
          handler: "lock_chosen_option"   # /pick-option path
        # /modify routes to a different handler; the dispatcher picks based on
        # ExitCmd. Implementation: when the matched command is "/modify", the
        # dispatcher invokes "reset_workflow_to_goal" instead of the default.
```

> Decision needed during implementation: either (a) make `output.handler` a map keyed by exit-command, OR (b) always invoke a single handler and dispatch on `args.ExitCmd` inside it. (b) is simpler — the single registered handler is `lock_chosen_option` and it delegates to `ResetWorkflowToGoalHandler` when the exit cmd is `/modify`. Pick (b) for v1.

**Verification:**
- Boot succeeds with new commands + handlers registered.
- `/help` lists `/pick-option` and `/modify`.
- `go test ./...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.4.6: E2E — full options loop including `/modify` round-trip

**Description:** Drive the workflow to the propose_options task, then exercise both exit paths.

**Files to modify:**
- `internal/orchestrator/options_loop_e2e_test.go` (NEW)

**Test outline (single test with sub-cases):**

Subcase A — `/pick-option`:
1. Run the workflow up to and including the propose_options task. Fake worker emits two-option JSON.
2. Inject `/pick-option B`.
3. Assert: propose_options task is `Success`; `WorkflowRepo.GetByID` returns `chosen_option` containing the B option.

Subcase B — `/modify`:
1. Same setup as A but inject `/modify` instead.
2. Assert:
   - `chosen_option`, `goal_text`, `success_criteria` are all NULL again on the workflow row.
   - Original capture_goal/gather_targeted/propose_options tasks are `Cancelled`.
   - Three new pending tasks exist with the same story IDs and bumped seq.
3. Re-run goal-capture with new content; lock; reach options again. Now `/pick-option A`. Assert lock succeeds.

**Verification:**
- Both subcases pass.
- `go test ./internal/orchestrator/... -run OptionsLoop -v` passes.

**Estimated effort:** 1.5 days

---

## Dependency Order

```
2.4.1 (skill body)
2.4.2 (multi-command exit_on) ──┐
2.4.3 (lock_chosen_option)      ├─► 2.4.5 (register + YAML)
2.4.4 (reset_workflow_to_goal) ─┘            │
                                              ▼
                                         2.4.6 (E2E)
```

## Total Estimated Effort: ~5 days
