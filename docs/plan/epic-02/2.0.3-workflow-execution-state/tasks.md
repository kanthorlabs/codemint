# Tasks for 2.0.3: Workflow Execution State

## Task 2.0.3.1: Database Migration for Workflow Execution

**Description:** Extend workflow table with execution tracking columns.

**Files to create:**
- `internal/db/migrations/000008_extend_workflow_execution.sql` (NEW)

**Implementation:**
```sql
-- +goose Up
ALTER TABLE workflow ADD COLUMN file_path TEXT;
ALTER TABLE workflow ADD COLUMN current_epic_id TEXT;
ALTER TABLE workflow ADD COLUMN current_story_id TEXT;
ALTER TABLE workflow ADD COLUMN started_at INTEGER;
ALTER TABLE workflow ADD COLUMN completed_at INTEGER;
ALTER TABLE workflow ADD COLUMN status INTEGER DEFAULT 0;

-- +goose Down
-- Forward-only migration
```

**Verification:**
- `make test` passes
- New columns exist in workflow table
- Existing workflows have `status = 0` (active) by default

**Estimated effort:** 0.5 day

---

## Task 2.0.3.2: WorkflowStatus Enum

**Description:** Add WorkflowStatus enum to domain.

**Files to modify:**
- `internal/domain/core.go`

**Implementation:**
```go
type WorkflowStatus int

const (
    WorkflowStatusActive    WorkflowStatus = iota // 0
    WorkflowStatusCompleted                       // 1
    WorkflowStatusCancelled                       // 2
)

func (s WorkflowStatus) String() string {
    switch s {
    case WorkflowStatusActive:
        return "Active"
    case WorkflowStatusCompleted:
        return "Completed"
    case WorkflowStatusCancelled:
        return "Cancelled"
    default:
        return "Unknown"
    }
}
```

**Verification:**
- Unit tests for String() method
- `go build ./...` succeeds

**Estimated effort:** 0.25 day

---

## Task 2.0.3.3: Update Workflow Domain Struct

**Description:** Add execution state fields to Workflow struct.

**Files to modify:**
- `internal/domain/core.go`
- `internal/repository/sqlite/workflow_repo.go`

**Implementation:**

Domain:
```go
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

Repository - update queries to include new columns.

**Verification:**
- `go build ./...` succeeds
- Existing workflow repository tests pass
- New fields round-trip through database

**Estimated effort:** 0.5 day

---

## Task 2.0.3.4: Workflow Repository Methods

**Description:** Add repository methods for workflow execution management.

**Files to modify:**
- `internal/repository/sqlite/workflow_repo.go`
- `internal/repository/sqlite/workflow_repo_test.go`

**Implementation:**
```go
// GetActiveForSession returns the currently active workflow execution.
func (r *WorkflowRepo) GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error)

// UpdateProgress updates the current epic/story position.
func (r *WorkflowRepo) UpdateProgress(ctx context.Context, id, epicID, storyID string) error

// MarkCompleted sets status to Completed and records completed_at timestamp.
func (r *WorkflowRepo) MarkCompleted(ctx context.Context, id string) error

// MarkCancelled sets status to Cancelled.
func (r *WorkflowRepo) MarkCancelled(ctx context.Context, id string) error

// ListByFilePath returns all executions of a specific workflow file.
func (r *WorkflowRepo) ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error)
```

**Verification:**
- Unit tests for each method
- `go test ./internal/repository/sqlite/...` passes

**Estimated effort:** 1 day

---

## Task 2.0.3.5: Scheduler Progress Updates

**Description:** Update Scheduler to track workflow progress as tasks complete.

**Files to modify:**
- `internal/orchestrator/scheduler.go`

**Implementation:**
- After task completes, determine which epic/story it belongs to
- Call `workflowRepo.UpdateProgress()`
- When all tasks in workflow complete, call `workflowRepo.MarkCompleted()`

**Verification:**
- Integration test: workflow progress updates as tasks run
- Workflow marked completed when last task finishes
- `go test ./internal/orchestrator/...` passes

**Estimated effort:** 1 day

---

## Task 2.0.3.6: Update /status Command

**Description:** Show workflow execution progress in /status output.

**Files to modify:**
- `internal/repl/core_commands.go` (or wherever /status is)

**Implementation:**
```
> /status

Session: sess-abc123 (Active)
Project: my-project (coding)

Workflow: brainstorming (Active)
  Progress: Epic 1/2, Story 3/5
  Tasks: 12/20 completed
  Started: 2026-04-27 10:30:00
```

**Verification:**
- `/status` shows workflow info when active workflow exists
- No workflow info shown when no active workflow
- Progress numbers are accurate

**Estimated effort:** 0.5 day

---

## Dependency Order

```
2.0.3.1 (Migration)
    │
    ├──► 2.0.3.2 (WorkflowStatus Enum)
    │         │
    ▼         ▼
2.0.3.3 (Domain Update)
    │
    ▼
2.0.3.4 (Repository Methods)
    │
    ├──► 2.0.3.5 (Scheduler Updates)
    │
    └──► 2.0.3.6 (/status Command)
```

## Total Estimated Effort: 3.75 days
