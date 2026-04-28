# Tasks for 2.2: Goal Capture

> 2.2 carries the **shared wiring** every later story (2.3, 2.4, 2.5) reuses:
> - generic `exit_on` slash-command dispatcher (closes the active workflow task on a registered command),
> - generic output-handler registry (invokes a named Go func with the task's `output` after it succeeds).
>
> Build them once here; later stories just register more commands and handlers.

---

## Task 2.2.1: Audit and update `goal-capture` SKILL.md to match index AC

**Description:** The skill body already exists at `internal/skills/embedded/goal-capture/SKILL.md` (seeded under the old GROW plan). Reconcile it against the AC in `index.md`: two-pass flow (goal sentence + 1–5 testable criteria), reject vague/process-only/unbounded goals, reject untestable criteria, emit ONLY the JSON `{"goal_text", "success_criteria"}` on `/lock-goal`.

**Files to modify:**
- `internal/skills/embedded/goal-capture/SKILL.md`

**Verification:**
- Skill body opens with a single-line description matching AC #1.
- Pass-1 and Pass-2 sections present with explicit rejection rules.
- "Output format" section instructs the model to emit raw JSON (no markdown fences) on `/lock-goal`.
- `go test ./internal/skills/...` passes (parser still loads it).

**Estimated effort:** 0.25 day

---

## Task 2.2.2: Generic output-handler registry

**Description:** Add `internal/workflow/handlers` (or sibling under `internal/workflow`) that maps a handler name (string) to a `func(ctx, runtime, task, args) error`. Story 2.2 registers `lock_workflow_goal`; later stories register their own. The orchestrator invokes the named handler when a story task with `Output.Handler` set transitions to `Success`.

**Files to modify:**
- `internal/workflow/handlers.go` (NEW)
- `internal/workflow/handlers_test.go` (NEW)

**Implementation sketch:**
```go
package workflow

import (
    "context"
    "fmt"
    "sync"

    "github.com/kanthorlabs/codemint/internal/domain"
)

// HandlerArgs is the payload a handler sees when invoked.
type HandlerArgs struct {
    WorkflowID string
    Task       *domain.Task
    Output     string  // raw skill output (task.Output.String)
    ExitCmd    string  // the slash command that triggered exit, if any
}

type HandlerFunc func(ctx context.Context, args HandlerArgs) error

type HandlerRegistry struct {
    mu sync.RWMutex
    m  map[string]HandlerFunc
}

func NewHandlerRegistry() *HandlerRegistry { return &HandlerRegistry{m: map[string]HandlerFunc{}} }

func (r *HandlerRegistry) Register(name string, fn HandlerFunc) error { /* err if dupe */ }
func (r *HandlerRegistry) Invoke(ctx context.Context, name string, args HandlerArgs) error {
    r.mu.RLock(); fn, ok := r.m[name]; r.mu.RUnlock()
    if !ok { return fmt.Errorf("workflow: handler %q not registered", name) }
    return fn(ctx, args)
}
```

**Verification:**
- Unit test: register + invoke round-trips correctly.
- Unit test: duplicate `Register` returns error.
- Unit test: `Invoke` for unknown name returns the expected error.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.2.3: `lock_workflow_goal` handler

**Description:** Concrete handler that parses skill output as `{"goal_text": string, "success_criteria": []string}`, validates non-empty, then calls `WorkflowRepo.LockGoal`. Returns error on bad JSON / empty goal / empty criteria — error becomes task `Failure` per the cross-cutting contract (2.8).

**Files to modify:**
- `internal/workflow/handlers_goal.go` (NEW)
- `internal/workflow/handlers_goal_test.go` (NEW)

**Implementation sketch:**
```go
func LockWorkflowGoalHandler(repo repository.WorkflowRepository) HandlerFunc {
    return func(ctx context.Context, args HandlerArgs) error {
        var parsed struct {
            GoalText        string   `json:"goal_text"`
            SuccessCriteria []string `json:"success_criteria"`
        }
        if err := json.Unmarshal([]byte(args.Output), &parsed); err != nil {
            return fmt.Errorf("lock_workflow_goal: invalid JSON: %w", err)
        }
        if strings.TrimSpace(parsed.GoalText) == "" {
            return errors.New("lock_workflow_goal: goal_text is required")
        }
        if len(parsed.SuccessCriteria) == 0 {
            return errors.New("lock_workflow_goal: at least one success criterion required")
        }
        criteriaJSON, err := json.Marshal(parsed.SuccessCriteria)
        if err != nil { return err }
        return repo.LockGoal(ctx, args.WorkflowID, parsed.GoalText, string(criteriaJSON))
    }
}
```

**Verification:**
- Unit test (happy path): valid JSON → `repo.LockGoal` called with parsed values; row updated.
- Unit test (bad JSON): returns error; repo not touched.
- Unit test (missing goal_text): returns error.
- Unit test (empty criteria array): returns error.
- Unit test (already-locked, `repo.LockGoal` returns its own error): bubbles up.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.2.4: Generic `exit_on` dispatcher in REPL ↔ orchestrator

**Description:** When a workflow task is `Processing` and its story has `ExitOn.Command` set (or a list — see 2.4), incoming REPL slash commands matching that command must close the task with `Success` and trigger the output handler before the scheduler advances. Add an `ExitOnDispatcher` registered with the REPL command-registry that the workflow runtime hooks per active task.

**Files to modify:**
- `internal/orchestrator/exit_on_dispatcher.go` (NEW)
- `internal/orchestrator/exit_on_dispatcher_test.go` (NEW)
- `internal/orchestrator/runtime.go` (subscribe per active workflow task)
- `internal/repl/core_commands.go` (route unknown slash to dispatcher BEFORE returning "unknown command")

**Behavior:**
1. Runtime, on task `Processing`, looks up the task's owning Story → `Story.ExitOn`. If set, registers `(taskID, commandName)` with the dispatcher.
2. REPL receives a slash command. Dispatcher looks up active `(*, command)` registrations. If matched:
   - mark task `Success`,
   - if `Story.Output.Handler` is set, invoke it through `HandlerRegistry`,
   - on handler error: mark task `Failure` instead, record reason via `failTaskWithReason`,
   - advance scheduler.
3. On task transition out of `Processing` (any reason), dispatcher entry is dropped.

**Verification:**
- Unit test: dispatcher with one registration matches the right slash command and ignores others.
- Unit test: handler error converts `Success` to `Failure` with the right sentinel.
- Unit test: deregistration on task transition (no orphan entries).
- E2E: a fake-skill task with `exit_on: /test-cmd` and a no-op handler closes when `/test-cmd` is typed; `/other` is forwarded to the normal command path.
- `go test ./internal/orchestrator/...` passes.

**Estimated effort:** 1 day

---

## Task 2.2.5: Wire `/lock-goal` REPL command

**Description:** Register `/lock-goal` as a known command so the REPL doesn't reject it as unknown. The command itself is *handled by the dispatcher*, so its core-command body is a no-op — but it must be in the registry to suppress "unknown command" output and to participate in autocomplete.

**Files to modify:**
- `internal/repl/core_commands.go` (or sibling — wherever `RegisterCoreCommands` lives)

**Implementation:**
```go
registry.Register(&Command{
    Name: "/lock-goal",
    Description: "Lock the captured goal and advance the workflow",
    Handler: func(ctx context.Context, args ...string) error {
        // No-op: ExitOnDispatcher in the orchestrator handles closure + handler invocation.
        // We register the command only so the REPL recognizes it and tab-completion finds it.
        return nil
    },
})
```

**Verification:**
- REPL boot: `/lock-goal` appears in `/help` output.
- Manual: typing `/lock-goal` outside an active goal-capture task does nothing visible (no error).
- Manual: typing `/lock-goal` during a goal-capture task closes it (covered by 2.2.7 e2e).

**Estimated effort:** 0.25 day

---

## Task 2.2.6: Update `brainstorming/WORKFLOW.yaml` — replace old spec with new spine (partial)

**Description:** Strip the obsolete `clarify` + `generate` stories (which referenced the dropped Living Spec / old task generator). Add the `capture_goal` story per the index. Subsequent stories (2.3, 2.4, 2.5) extend the YAML further.

**Files to modify:**
- `internal/workflow/embedded/brainstorming/WORKFLOW.yaml`

**Implementation:**
```yaml
name: brainstorming
version: "1.0"
description: |
  Coding Workflow: Project Overview → Goal → Reality → Options → Plan → Verify → Confirm.

settings:
  default_timeout: 3600000
  guardrails:
    verification: true
    confirmation: true

epics:
  - id: planning
    name: "Coding Workflow"
    stories:
      - id: gather
        name: "Project Overview"
        skill: "@codemint/gatherer"
        type: coding

      - id: capture_goal
        name: "Goal Capture"
        skill: "@codemint/goal-capture"
        type: coding
        depends_on: gather
        exit_on:
          command: "/lock-goal"
        output:
          handler: "lock_workflow_goal"
```

**Verification:**
- `make build` succeeds.
- `make test ./internal/workflow/...` passes (registry tests still load the embedded workflow without error).
- Boot a CLI session: `/workflow` lists `brainstorming`; `/workflow brainstorming` enters the gather → capture_goal sequence (covered by e2e in 2.2.7).

**Estimated effort:** 0.25 day

---

## Task 2.2.7: E2E — gather → capture_goal → /lock-goal writes goal+criteria

**Description:** Drive the workflow through both stages and assert the workflow row's `goal_text` and `success_criteria` reflect the parsed JSON.

**Files to modify:**
- `internal/orchestrator/runtime_e2e_test.go` (extend) or sibling `goal_capture_e2e_test.go` (NEW)

**Test outline:**
1. Start with a fresh fake project + session.
2. Run `/workflow brainstorming`.
3. Fake ACP worker: respond to the gather prompt with a stub JSON; respond to the goal-capture prompt with `{"goal_text":"…","success_criteria":["…","…"]}`.
4. Inject `/lock-goal` via the REPL multiplexer.
5. Assert:
   - capture_goal task is `Success`.
   - `WorkflowRepo.GetByID` returns the workflow with `GoalText` non-null and matching the stub.
   - `SuccessCriteria` parses to the stub array.
6. Negative case: skill emits `{"goal_text":""}` → handler returns error → task ends `Failure` with sentinel from the handler error.

**Verification:**
- E2E passes locally and in CI.
- `go test ./internal/orchestrator/... -run GoalCapture -v` passes.

**Estimated effort:** 1 day

---

## Dependency Order

```
2.2.1 (skill body audit)          [parallel]
2.2.2 (handler registry)
   └─► 2.2.3 (lock_workflow_goal handler)
2.2.4 (exit_on dispatcher)
   └─► 2.2.5 (/lock-goal REPL registration)
2.2.6 (WORKFLOW.yaml — gather + capture_goal)
   ⇣
2.2.7 (E2E)  ← gates story acceptance
```

## Total Estimated Effort: ~3.75 days
