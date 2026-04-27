# User Story 2.0.3: Workflow Execution State

* **As the** Go Orchestrator,
* **I want to** track workflow execution progress in the database,
* **So that** I can resume, report status, and trace multiple executions of the same workflow.

## Acceptance Criteria

1. Database migration extends `workflow` table with execution tracking columns *and GROW Goal/Options columns* (see Schema below).
2. `WorkflowStatus` enum: Active(0), Completed(1), Cancelled(2).
3. `/workflow` command creates a new workflow execution record.
4. Scheduler updates `current_epic_id` and `current_story_id` as tasks complete.
5. `/status` command shows current workflow progress, including the Goal banner and `criteria_met / total_criteria` (once 2.4.1 has run).
6. Repository exposes `LockGoal(workflowID, goalText, criteriaJSON)` and `LockChosenOption(workflowID, optionJSON)` setters; both refuse to overwrite a non-null value (one-shot lock semantics — overwrite path is `/revise-goal`).

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

-- GROW alignment (2026-04-27): first-class Goal + Options storage.
ALTER TABLE workflow ADD COLUMN goal_text TEXT;             -- one-sentence goal from 2.2.1
ALTER TABLE workflow ADD COLUMN success_criteria TEXT;      -- JSON array of testable strings, from 2.2.1
ALTER TABLE workflow ADD COLUMN chosen_option TEXT;         -- JSON of the single option picked in 2.3.1

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

// Workflow - extended with execution state and GROW Goal/Options
type Workflow struct {
    ID              string         `db:"id"`
    SessionID       string         `db:"session_id"`
    Type            int            `db:"type"`
    FilePath        sql.NullString `db:"file_path"`
    CurrentEpicID   sql.NullString `db:"current_epic_id"`
    CurrentStoryID  sql.NullString `db:"current_story_id"`
    StartedAt       sql.NullInt64  `db:"started_at"`
    CompletedAt     sql.NullInt64  `db:"completed_at"`
    Status          WorkflowStatus `db:"status"`

    // GROW alignment fields (locked once each, mutated only by /revise-goal)
    GoalText        sql.NullString `db:"goal_text"`         // 2.2.1
    SuccessCriteria sql.NullString `db:"success_criteria"`  // 2.2.1, JSON array
    ChosenOption    sql.NullString `db:"chosen_option"`     // 2.3.1, JSON
}
```

### Repository Setters (one-shot lock)

```go
// LockGoal writes goal_text + success_criteria; errors if already non-null.
func (r *workflowRepo) LockGoal(ctx context.Context, workflowID, goalText, criteriaJSON string) error {
    res, err := r.db.ExecContext(ctx,
        `UPDATE workflow SET goal_text = ?, success_criteria = ?
         WHERE id = ? AND goal_text IS NULL`,
        goalText, criteriaJSON, workflowID,
    )
    if err != nil { return err }
    n, _ := res.RowsAffected()
    if n == 0 {
        return errors.New("goal already locked; use /revise-goal to change")
    }
    return nil
}

// LockChosenOption writes chosen_option; errors if already non-null.
func (r *workflowRepo) LockChosenOption(ctx context.Context, workflowID, optionJSON string) error {
    res, err := r.db.ExecContext(ctx,
        `UPDATE workflow SET chosen_option = ?
         WHERE id = ? AND chosen_option IS NULL`,
        optionJSON, workflowID,
    )
    if err != nil { return err }
    n, _ := res.RowsAffected()
    if n == 0 {
        return errors.New("option already chosen for this workflow")
    }
    return nil
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
- 2.2.1 Goal Capture (writes `goal_text` + `success_criteria`)
- 2.3.1 Options Proposer (writes `chosen_option`)
- 2.4.1 Goal Verification (reads `success_criteria`)
- 2.6 Retrospective split (Outcome retro reads `goal_text` + `success_criteria`)
- 2.7 Human Review (activation tracking + Goal banner)
