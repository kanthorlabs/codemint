# Tasks for 2.0.2: Task Routing & Conditional Execution

## Task 2.0.2.1: Database Migration for Task Routing

**Description:** Add `depends_on` and `condition` columns to task table.

**Files to create:**
- `internal/db/migrations/000007_add_task_routing.sql` (NEW)

**Implementation:**
```sql
-- +goose Up
ALTER TABLE task ADD COLUMN depends_on TEXT;
ALTER TABLE task ADD COLUMN condition INTEGER;

-- +goose Down
-- Note: SQLite < 3.35.0 doesn't support DROP COLUMN
-- For compatibility, we document this as a forward-only migration
-- or use table recreation approach if rollback is required
```

**Verification:**
- `make test` passes (goose runs migrations)
- New columns exist in task table
- Existing data is unaffected (columns are nullable)

**Estimated effort:** 0.5 day

---

## Task 2.0.2.2: Update Domain Task Struct

**Description:** Add routing fields to Task domain struct and repository.

**Files to modify:**
- `internal/domain/core.go`
- `internal/repository/sqlite/task_repo.go`

**Implementation:**

Domain:
```go
type Task struct {
    // ... existing fields ...
    DependsOn sql.NullString `db:"depends_on"`
    Condition sql.NullInt64  `db:"condition"`
}
```

Repository - update all queries that select/insert Task to include new columns.

**Verification:**
- `go build ./...` succeeds
- Existing task repository tests pass
- New fields round-trip through database

**Estimated effort:** 0.5 day

---

## Task 2.0.2.3: Terminal Status Helper

**Description:** Add helper function to check if a TaskStatus is terminal.

**Files to modify:**
- `internal/domain/core.go`

**Implementation:**
```go
// IsTerminal returns true if the status represents a completed state.
func (s TaskStatus) IsTerminal() bool {
    switch s {
    case TaskStatusSuccess,
         TaskStatusFailure,
         TaskStatusCompleted,
         TaskStatusReverted,
         TaskStatusCancelled:
        return true
    }
    return false
}
```

**Verification:**
- Unit tests for each status value
- `go test ./internal/domain/...` passes

**Estimated effort:** 0.25 day

---

## Task 2.0.2.4: Scheduler Eligibility Check

**Description:** Update Scheduler to evaluate depends_on and condition when selecting tasks.

**Files to modify:**
- `internal/orchestrator/scheduler.go`
- `internal/orchestrator/scheduler_test.go`

**Implementation:**
```go
func (s *Scheduler) isTaskEligible(ctx context.Context, task *domain.Task) bool {
    if !task.DependsOn.Valid {
        return true
    }
    
    predecessor, err := s.taskRepo.Get(ctx, task.DependsOn.String)
    if err != nil {
        return false
    }
    
    if !predecessor.Status.IsTerminal() {
        return false
    }
    
    if !task.Condition.Valid {
        return true
    }
    
    requiredStatus := domain.TaskStatus(task.Condition.Int64)
    return predecessor.Status == requiredStatus
}
```

Update `selectNextTask()` to call `isTaskEligible()` for each pending task.

**Verification:**
- Unit tests for eligibility logic:
  - Task with no depends_on is always eligible
  - Task with depends_on waits for predecessor terminal state
  - Task with condition only eligible when predecessor matches
  - Task with unmatched condition is skipped
- Integration test with branching workflow
- `go test ./internal/orchestrator/...` passes

**Estimated effort:** 1 day

---

## Task 2.0.2.5: Route Generation from WORKFLOW.yaml

**Description:** When generating tasks from WorkflowFile, set depends_on and condition based on routes.

**Files to create/modify:**
- `internal/workflow/task_generator.go` (NEW)
- `internal/workflow/task_generator_test.go` (NEW)

**Implementation:**
```go
func (g *TaskGenerator) GenerateTasks(wf *domain.WorkflowFile, sessionID string) ([]*domain.Task, error) {
    var tasks []*domain.Task
    storyTaskMap := make(map[string]*domain.Task) // story_id → confirmation task
    
    for _, epic := range wf.Epics {
        for _, story := range epic.Stories {
            // Create story tasks...
            
            // If story has routes, create conditional successor tasks
            if len(story.Routes) > 0 {
                for status, nextStoryID := range story.Routes {
                    // Find or create next story's first task
                    // Set depends_on and condition
                }
            }
        }
    }
    
    return tasks, nil
}
```

**Verification:**
- Unit tests with workflow containing routes
- Generated tasks have correct depends_on and condition values
- `go test ./internal/workflow/...` passes

**Estimated effort:** 1.5 days

---

## Dependency Order

```
2.0.2.1 (Migration)
    │
    ▼
2.0.2.2 (Domain Update)
    │
    ├──► 2.0.2.3 (Terminal Helper)
    │         │
    ▼         ▼
2.0.2.4 (Scheduler) ◄── depends on both
    │
    ▼
2.0.2.5 (Route Generation) ◄── depends on 2.0.1 (parser)
```

## Total Estimated Effort: 3.75 days
