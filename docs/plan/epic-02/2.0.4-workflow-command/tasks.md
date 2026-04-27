# Tasks for 2.0.4: Workflow Command

## Task 2.0.4.1: Task Generator

**Description:** Create TaskGenerator that converts WorkflowFile into Task records.

**Files to create:**
- `internal/workflow/task_generator.go` (NEW)
- `internal/workflow/task_generator_test.go` (NEW)

**Implementation:**
```go
type TaskGenerator struct {
    humanAgentID     string
    assistantAgentID string
    yoloAgentID      string
}

func NewTaskGenerator(humanID, assistantID, yoloID string) *TaskGenerator

func (g *TaskGenerator) GenerateTasks(
    wf *domain.WorkflowFile, 
    projectID, sessionID, workflowID string,
) ([]*domain.Task, error)

// Internal helpers
func (g *TaskGenerator) createStoryTask(...) *domain.Task
func (g *TaskGenerator) createVerificationTask(parent *domain.Task) *domain.Task
func (g *TaskGenerator) createConfirmationTask(parent *domain.Task) *domain.Task
func (g *TaskGenerator) createRetrospectiveTask(...) *domain.Task
func (g *TaskGenerator) assignSeqNumbers(tasks []*domain.Task)
```

**Verification:**
- Unit tests with sample WorkflowFile
- Generated tasks have correct seq_epic/seq_story/seq_task
- Guardrails are injected based on settings
- `go test ./internal/workflow/...` passes

**Estimated effort:** 2 days

---

## Task 2.0.4.2: Workflow Command Registration

**Description:** Register /workflow command in REPL.

**Files to create/modify:**
- `internal/repl/workflow_commands.go` (NEW)
- `internal/repl/core_commands.go` (register the command)

**Implementation:**
```go
type WorkflowDeps struct {
    FileRegistry   *workflow.FileRegistry
    TaskGenerator  *workflow.TaskGenerator
    TaskRepo       repository.TaskRepository
    WorkflowRepo   repository.WorkflowRepository
    ActiveSession  *orchestrator.ActiveSession
}

func RegisterWorkflowCommands(reg *registry.CommandRegistry, deps WorkflowDeps) {
    reg.Register(registry.Command{
        Name:        "workflow",
        Description: "Start or manage workflows",
        Usage:       "/workflow [name]",
        Completer:   workflowCompleter(deps.FileRegistry),
        Handler:     workflowHandler(deps),
    })
}
```

**Verification:**
- `/workflow` lists available workflows
- `/workflow <name>` starts workflow
- Unknown workflow name produces error
- `go test ./internal/repl/...` passes

**Estimated effort:** 1 day

---

## Task 2.0.4.3: Autocomplete for Workflow Names

**Description:** Implement tab completion for /workflow command.

**Files to modify:**
- `internal/repl/workflow_commands.go`
- REPL line editor integration (if needed)

**Implementation:**
```go
func workflowCompleter(reg *workflow.FileRegistry) func(prefix string) []string {
    return func(prefix string) []string {
        var completions []string
        for _, name := range reg.Names() {
            if strings.HasPrefix(name, prefix) {
                completions = append(completions, name)
            }
        }
        return completions
    }
}
```

**Verification:**
- Tab after `/workflow ` shows available workflows
- Tab after `/workflow br` shows `brainstorming`
- No completions for unknown prefix

**Estimated effort:** 0.5 day

---

## Task 2.0.4.4: Workflow Execution Initiation

**Description:** Create workflow execution record and insert generated tasks.

**Files to modify:**
- `internal/repl/workflow_commands.go`

**Implementation:**
```go
func workflowHandler(deps WorkflowDeps) registry.HandlerFunc {
    return func(ctx context.Context, args []string) error {
        if len(args) == 0 {
            return listWorkflows(deps)
        }
        
        name := args[0]
        wf, ok := deps.FileRegistry.Get(name)
        if !ok {
            return fmt.Errorf("workflow %q not found", name)
        }
        
        // Create workflow execution record
        workflow := domain.NewWorkflow(deps.ActiveSession.SessionID(), wf.Type)
        workflow.FilePath = sql.NullString{String: wf.SourcePath, Valid: true}
        workflow.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
        
        if err := deps.WorkflowRepo.Create(ctx, workflow); err != nil {
            return err
        }
        
        // Generate and insert tasks
        tasks, err := deps.TaskGenerator.GenerateTasks(
            wf, 
            deps.ActiveSession.ProjectID(),
            deps.ActiveSession.SessionID(),
            workflow.ID,
        )
        if err != nil {
            return err
        }
        
        for _, task := range tasks {
            if err := deps.TaskRepo.Create(ctx, task); err != nil {
                return err
            }
        }
        
        fmt.Printf("Starting workflow: %s (v%s)\n", wf.Name, wf.Version)
        return nil
    }
}
```

**Verification:**
- Workflow record created with correct file_path, started_at, status
- All tasks inserted with correct workflow_id
- Scheduler picks up first task
- Integration test end-to-end

**Estimated effort:** 1 day

---

## Task 2.0.4.5: Wire Workflow Command into main.go

**Description:** Initialize WorkflowDeps and register command at startup.

**Files to modify:**
- `cmd/codemint/main.go`

**Implementation:**
- Create TaskGenerator with agent IDs
- Create WorkflowDeps struct
- Call RegisterWorkflowCommands

**Verification:**
- Fresh launch has `/workflow` command available
- `/help` shows workflow command
- End-to-end test with embedded brainstorming workflow

**Estimated effort:** 0.5 day

---

## Dependency Order

```
2.0.4.1 (TaskGenerator)
    │
    ▼
2.0.4.2 (Command Registration)
    │
    ├──► 2.0.4.3 (Autocomplete)
    │
    ▼
2.0.4.4 (Execution Initiation)
    │
    ▼
2.0.4.5 (Wire to main.go)
```

**Note:** 2.0.4.1 depends on 2.0.1 (domain types) and 2.0.2 (routing) being complete.

## Total Estimated Effort: 5 days
