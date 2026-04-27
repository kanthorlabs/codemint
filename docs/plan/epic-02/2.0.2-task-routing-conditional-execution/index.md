# User Story 2.0.2: Task Routing & Conditional Execution

* **As the** Go Scheduler,
* **I want to** evaluate `depends_on` and `condition` fields when selecting the next task,
* **So that** workflows can branch based on predecessor task outcomes.

## Acceptance Criteria

1. Database migration adds `depends_on TEXT` and `condition INTEGER` columns to `task` table.
2. `condition` stores `TaskStatus` values (3=Success, 4=Failure, 7=Cancelled).
3. Scheduler skips tasks whose predecessor hasn't reached required status.
4. Tasks with `depends_on=NULL` are eligible based on seq order alone.
5. Confirmation tasks (type=2) can route to different successors based on outcome.

## Technical Design

### Database Schema

```sql
-- Migration 000007_add_task_routing.sql
-- +goose Up
ALTER TABLE task ADD COLUMN depends_on TEXT;   -- Task ID (FK)
ALTER TABLE task ADD COLUMN condition INTEGER; -- TaskStatus value (nullable)

-- +goose Down
-- SQLite doesn't support DROP COLUMN before 3.35.0
-- For older versions, recreate table without columns
```

### Domain Changes

```go
// In domain/core.go
type Task struct {
    // ... existing fields ...
    
    // DependsOn is the task ID this task waits for before becoming eligible.
    DependsOn sql.NullString `db:"depends_on"`
    
    // Condition is the required TaskStatus of the DependsOn task.
    // NULL means any terminal status. If set, only proceeds when predecessor matches.
    Condition sql.NullInt64 `db:"condition"`
}
```

### Scheduler Logic

```go
func (s *Scheduler) isTaskEligible(ctx context.Context, task *domain.Task) bool {
    // No dependency - eligible based on seq order
    if !task.DependsOn.Valid {
        return true
    }
    
    predecessor, err := s.taskRepo.Get(ctx, task.DependsOn.String)
    if err != nil {
        return false
    }
    
    // Check if predecessor is in terminal state
    if !isTerminalStatus(predecessor.Status) {
        return false
    }
    
    // No specific condition - any terminal state is OK
    if !task.Condition.Valid {
        return true
    }
    
    // Check if predecessor status matches required condition
    requiredStatus := domain.TaskStatus(task.Condition.Int64)
    return predecessor.Status == requiredStatus
}

func isTerminalStatus(status domain.TaskStatus) bool {
    switch status {
    case domain.TaskStatusSuccess,
         domain.TaskStatusFailure,
         domain.TaskStatusCompleted,
         domain.TaskStatusReverted,
         domain.TaskStatusCancelled:
        return true
    }
    return false
}
```

### Routing Example

```yaml
# WORKFLOW.yaml
- id: review
  type: confirmation
  routes:
    3: execute      # TaskStatusSuccess → proceed to execute
    4: clarify      # TaskStatusFailure → loop back to clarify
    7: null         # TaskStatusCancelled → end workflow
    
- id: execute
  depends_on: review
  condition: 3      # Only eligible if review was Success
  
- id: clarify_again
  depends_on: review
  condition: 4      # Only eligible if review was Failure
```

## Dependencies

- 2.0.1 Workflow File Infrastructure (for route definitions in WORKFLOW.yaml)

## Blocks

- 2.5 Confirmation Guardrail (uses routing for branching)
- 2.8 Mid-Flight Pivots (needs to re-evaluate dependencies)
