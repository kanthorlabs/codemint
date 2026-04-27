# User Story 2.0.3: Workflow Execution State

* **As the** Go Orchestrator,
* **I want to** track workflow execution progress in the database,
* **So that** I can resume, report status, and trace multiple executions of the same workflow.

## Acceptance Criteria

1. Database migration extends `workflow` table with execution tracking columns.
2. `WorkflowStatus` enum: Active(0), Completed(1), Cancelled(2).
3. `/workflow` command creates a new workflow execution record.
4. Scheduler updates `current_epic_id` and `current_story_id` as tasks complete.
5. `/status` command shows current workflow progress.

## Technical Design

### Database Schema

```sql
-- Migration 000008_extend_workflow_execution.sql
-- +goose Up
ALTER TABLE workflow ADD COLUMN file_path TEXT;
ALTER TABLE workflow ADD COLUMN current_epic_id TEXT;
ALTER TABLE workflow ADD COLUMN current_story_id TEXT;
ALTER TABLE workflow ADD COLUMN started_at INTEGER;
ALTER TABLE workflow ADD COLUMN completed_at INTEGER;
ALTER TABLE workflow ADD COLUMN status INTEGER DEFAULT 0;

-- +goose Down
-- Forward-only migration (SQLite column drop limitations)
```

### Domain Changes

```go
// WorkflowStatus represents the lifecycle state of a workflow execution.
type WorkflowStatus int

const (
    WorkflowStatusActive    WorkflowStatus = iota // 0
    WorkflowStatusCompleted                       // 1
    WorkflowStatusCancelled                       // 2
)

// Workflow - extended with execution state
type Workflow struct {
    ID             string         `db:"id"`
    SessionID      string         `db:"session_id"`
    Type           int            `db:"type"`
    FilePath       sql.NullString `db:"file_path"`
    CurrentEpicID  sql.NullString `db:"current_epic_id"`
    CurrentStoryID sql.NullString `db:"current_story_id"`
    StartedAt      sql.NullInt64  `db:"started_at"`
    CompletedAt    sql.NullInt64  `db:"completed_at"`
    Status         WorkflowStatus `db:"status"`
}
```

### Status Queries

```sql
-- Current workflow execution for session
SELECT * FROM workflow 
WHERE session_id = ? AND status = 0
ORDER BY started_at DESC
LIMIT 1;

-- Workflow execution history
SELECT w.*, 
       (SELECT COUNT(*) FROM task t WHERE t.workflow_id = w.id AND t.status IN (3, 5)) as completed_tasks,
       (SELECT COUNT(*) FROM task t WHERE t.workflow_id = w.id) as total_tasks
FROM workflow w
WHERE w.file_path LIKE '%/brainstorming/WORKFLOW.yaml'
ORDER BY w.started_at DESC;
```

## Dependencies

- 2.0.1 Workflow File Infrastructure (file_path from WorkflowFile)

## Blocks

- 2.0.4 Workflow Command (creates execution records)
- 2.7 Human Review (activation tracking)
