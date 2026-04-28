# Tasks for 2.5: Plan Generation

> Final pre-execution stage. Produces the executable backlog (Epic→Story→Task on the flat `task` table) and **auto-injects the SYSTEM-TASKs** (Verification 2.6, Confirmation 2.7) per coding Story.
> No new generic infra needed — handler registry + dispatcher already exist (2.2.2 / 2.2.4).

---

## Task 2.5.1: Embed `task-generator` SKILL.md + `task-schema.json`

**Description:** Author the skill at `internal/skills/embedded/task-generator/SKILL.md` per index AC.
Inputs: `goal_text`, `success_criteria`, `chosen_option`, appended context. Output: Epic → Story → Task JSON; **never** include verification/confirmation tasks (Go-side injects those). On incoherent input, emit `{"error": "..."}` instead of fabricating tasks.

**Files to modify:**
- `internal/skills/embedded/task-generator/SKILL.md` (NEW)
- `internal/skills/embedded/task-generator/references/task-schema.json` (NEW)

**Schema (concrete, validates the Coding plan only — guardrails injected by Go):**
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "oneOf": [
    {
      "required": ["epics"],
      "properties": {
        "epics": {
          "type": "array", "minItems": 1,
          "items": {
            "type": "object",
            "required": ["id","name","stories"],
            "properties": {
              "id": { "type": "string" },
              "name": { "type": "string" },
              "description": { "type": "string" },
              "stories": {
                "type": "array", "minItems": 1,
                "items": {
                  "type": "object",
                  "required": ["id","name","tasks"],
                  "properties": {
                    "id": { "type": "string" },
                    "name": { "type": "string" },
                    "description": { "type": "string" },
                    "verification": { "type": "string" },
                    "tasks": {
                      "type": "array", "minItems": 1,
                      "items": {
                        "type": "object",
                        "required": ["id","name"],
                        "properties": {
                          "id": { "type": "string" },
                          "name": { "type": "string" },
                          "description": { "type": "string" },
                          "files": { "type": "array", "items": { "type": "string" } }
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    { "required": ["error"], "properties": { "error": { "type": "string" } } }
  ]
}
```

**Verification:**
- Skill registered without error.
- Schema is syntactically valid (round-trip through a Go JSON-Schema validator like `github.com/santhosh-tekuri/jsonschema`).
- `go test ./internal/skills/...` passes.

**Estimated effort:** 0.75 day

---

## Task 2.5.2: `create_implementation_tasks` handler — core insertion

**Description:** Parse the skill output, validate against schema, then insert one row per leaf Task into the `task` table inside a single transaction. Use `seq_epic` / `seq_story` / `seq_task` indexes (0-based). All inserted tasks: `type = TaskTypeCoding (0)`, assignee = the session's Assistant agent, `status = Pending`. Schema variant `{"error": "..."}` → handler returns error → task `Failure` → 2.8.

**Files to modify:**
- `internal/workflow/handlers_plan.go` (NEW)
- `internal/workflow/handlers_plan_test.go` (NEW)
- `internal/repository/sqlite/task_repo.go` (add `BulkInsert(ctx, []*domain.Task) error` if not present — single-tx insert)
- `internal/repository/sqlite/task_repo_test.go`

**Implementation sketch:**
```go
func CreateImplementationTasksHandler(
    workflowRepo repository.WorkflowRepository,
    taskRepo repository.TaskRepository,
    sessionRepo repository.SessionRepository,
    schemaPath string, // path or embed key for task-schema.json
) HandlerFunc {
    return func(ctx context.Context, args HandlerArgs) error {
        if err := validateAgainstSchema(args.Output, schemaPath); err != nil {
            return fmt.Errorf("create_implementation_tasks: schema: %w", err)
        }
        var plan struct {
            Error string `json:"error"`
            Epics []planEpic `json:"epics"`
        }
        if err := json.Unmarshal([]byte(args.Output), &plan); err != nil {
            return fmt.Errorf("create_implementation_tasks: invalid JSON: %w", err)
        }
        if plan.Error != "" {
            return fmt.Errorf("create_implementation_tasks: skill aborted: %s", plan.Error)
        }

        wf, err := workflowRepo.GetByID(ctx, args.WorkflowID)
        if err != nil { return err }
        sess, err := sessionRepo.GetByID(ctx, wf.SessionID)
        if err != nil { return err }
        assistantID := sess.AssistantAgentID // already cached on the session

        var rows []*domain.Task
        for ei, epic := range plan.Epics {
            for si, story := range epic.Stories {
                for ti, t := range story.Tasks {
                    rows = append(rows, buildCodingTask(args.WorkflowID, sess.ID, assistantID, ei, si, ti, t))
                }
                // 2.5.3 will append the Verification + Confirmation rows here.
            }
        }
        return taskRepo.BulkInsert(ctx, rows)
    }
}
```

**Verification:**
- Unit test (happy path, 1 epic / 2 stories / 3 tasks each): rows inserted with correct seq triples, all `TaskTypeCoding`.
- Unit test (`{"error": "..."}` variant): handler returns error; transaction rolls back; no rows inserted.
- Unit test (schema-invalid JSON): handler returns error; no rows inserted.
- Unit test (DB error mid-insert): transaction rolls back fully.
- `go test ./internal/workflow/... ./internal/repository/sqlite/...` passes.

**Estimated effort:** 1 day

---

## Task 2.5.3: Auto-inject Verification + Confirmation tasks

**Description:** Extend the handler from 2.5.2 — after each Story's coding rows, append:
- one Verification row (`TaskTypeVerification`, assignee = assistant) unless story-level or workflow-level `guardrails.verification: false`. `Input` JSON: `{"command": "<resolved>"}` where the command is `story.verification` || `workflow.settings.verification` || `"go test ./..."`.
- one Confirmation row (`TaskTypeConfirmation`, assignee = the session's Human Agent) unless `guardrails.confirmation: false`. `Input` JSON: `{"prompt": "Approve <story_name>?"}`.

Both rows participate in the same single transaction as 2.5.2.

`depends_on` wiring:
- Verification → last coding task of the story.
- Confirmation → Verification (or last coding task if verification opted out).

**Files to modify:**
- `internal/workflow/handlers_plan.go`
- `internal/workflow/handlers_plan_test.go`

**Implementation sketch:**
```go
guardrails := wf.Settings.Guardrails // resolve workflow-level
if story.Guardrails != nil { guardrails = mergeStoryOver(guardrails, *story.Guardrails) }

var lastCodingTaskID string = rows[len(rows)-1].ID

if guardrails.Verification {
    cmd := firstNonEmpty(story.Verification, wf.Settings.Verification, "go test ./...")
    verifyTask := buildVerificationTask(args.WorkflowID, sess.ID, assistantID, ei, si, ti+1, cmd)
    verifyTask.DependsOn = sql.NullString{String: lastCodingTaskID, Valid: true}
    rows = append(rows, verifyTask)
    lastCodingTaskID = verifyTask.ID
}
if guardrails.Confirmation {
    confirmTask := buildConfirmationTask(args.WorkflowID, sess.ID, sess.HumanAgentID, ei, si, ti+2, story.Name)
    confirmTask.DependsOn = sql.NullString{String: lastCodingTaskID, Valid: true}
    rows = append(rows, confirmTask)
}
```

**Verification:**
- Unit test (defaults): verification + confirmation injected per story; `depends_on` chain correct.
- Unit test (`guardrails.verification: false` at story level): skipped for that story; confirmation now depends on last coding task.
- Unit test (`guardrails.confirmation: false` at workflow level): no confirmation rows anywhere.
- Unit test (custom `verification` command at story level): the resolved command lands in `task.Input`.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 1 day

---

## Task 2.5.4: Boot wiring + WORKFLOW.yaml `generate` step

**Description:** Register the handler. Append the final `generate` story to `brainstorming/WORKFLOW.yaml`. After this lands, the workflow is end-to-end runnable up to plan creation (execution of the generated tasks is the scheduler's existing job).

**Files to modify:**
- `cmd/codemint/main.go` (register handler)
- `internal/workflow/embedded/brainstorming/WORKFLOW.yaml`

**YAML diff:**
```yaml
      - id: generate
        name: "Plan Generation"
        skill: "@codemint/task-generator"
        type: coding
        depends_on: propose_options
        output:
          schema: "skills/task-generator/references/task-schema.json"
          handler: "create_implementation_tasks"
```

**Verification:**
- Boot succeeds with all five generate-stage handlers registered.
- `make build` + `./build/codemint -mode cli` lists `brainstorming` workflow.
- `go test ./...` passes.

**Estimated effort:** 0.25 day

---

## Task 2.5.5: E2E — full Coding Workflow run

**Description:** End-to-end test that runs every step from 2.1 through 2.7 with fake ACP responses, asserting the final task table contains the expected coding/verification/confirmation rows in the right order.

**Files to modify:**
- `internal/orchestrator/coding_workflow_e2e_test.go` (NEW)

**Test outline:**
1. Boot orchestrator with embedded brainstorming workflow.
2. Inject fake worker that returns canned outputs for each skill in turn:
   - `gatherer`: stub project summary JSON.
   - `goal-capture`: `{"goal_text":"Add login","success_criteria":["…","…"]}`.
   - `targeted-gatherer`: `{"skipped": false, "keyword_hits": [...]}`.
   - `options-proposer`: 2-option JSON.
   - `task-generator`: 1 epic / 1 story / 2 coding tasks JSON.
3. Drive lock/pick commands at the right moments: `/lock-goal`, `/pick-option A`.
4. Assert intermediate state at each step (workflow row columns; task rows in the table).
5. Final state: 2 Coding rows + 1 Verification row + 1 Confirmation row, all with the same epic/story seq, contiguous task seq, correct `depends_on` chain, all `Pending`.
6. The test does NOT execute the coding tasks — that's the scheduler/Executor path covered by other suites.

**Verification:**
- Test passes.
- Negative case: `task-generator` returns `{"error":"option B impossible"}` → generate task ends `Failure`; no plan rows inserted.
- `go test ./internal/orchestrator/... -run CodingWorkflow -v` passes.

**Estimated effort:** 1.5 days

---

## Dependency Order

```
2.5.1 (skill + schema)
   └─► 2.5.4 (YAML)
2.5.2 (handler core)
   └─► 2.5.3 (guardrail injection)
            └─► 2.5.4 (register)
                       │
                       ▼
                  2.5.5 (E2E)
```

## Total Estimated Effort: ~4.5 days
