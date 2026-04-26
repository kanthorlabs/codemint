# Tasks: 1.18 Nullable Column Convention Enforcement

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.18-nullable-column-convention/`
**Tech Stack:** Go, `database/sql`, `sqlx`
**Priority:** P1 (Technical Debt - Prevents subtle bugs)

---

## Task 1.18.1: Update Agent Struct for Nullable Assistant
* **Action:** Update `internal/domain/core.go`.
* **Details:**
  * Change `Assistant string` to `Assistant sql.NullString`.
  * Import `database/sql` package.
  * Update `NewAgent()` factory to set `Assistant: sql.NullString{String: assistant, Valid: assistant != ""}`.
* **Affected columns:**
  * `agent.assistant` - nullable TEXT in schema
* **Verification:**
  * Unit test: Create agent with empty assistant → `Assistant.Valid == false`.
  * Unit test: Create agent with "opencode" → `Assistant.Valid == true`.
  * Existing tests pass.

## Task 1.18.2: Update Task Struct for Nullable Fields
* **Action:** Update `internal/domain/core.go`.
* **Details:**
  * Change `WorkflowID string` to `WorkflowID sql.NullString`.
  * Change `Input string` to `Input sql.NullString`.
  * Change `Output string` to `Output sql.NullString`.
  * Update `NewTask()` factory accordingly.
* **Affected columns:**
  * `task.workflow_id` - nullable FK to workflow
  * `task.input` - nullable JSON payload
  * `task.output` - nullable JSON payload
* **Verification:**
  * Unit test: Task with no workflow → `WorkflowID.Valid == false`.
  * Unit test: Task with workflow → `WorkflowID.Valid == true`.

## Task 1.18.3: Remove COALESCE Workarounds from TaskRepository
* **Action:** Update `internal/repository/sqlite/task_repo.go`.
* **Details:**
  * Remove `COALESCE(workflow_id, '') as workflow_id` from all queries.
  * Use direct column selection: `workflow_id`.
  * sqlx handles `sql.NullString` scanning automatically.
* **Lines to fix:**
  * Line 64: `Next()` query
  * Line 146: `FindByID()` query
  * Line 199: `FindInterrupted()` query
* **Verification:**
  * Unit test: Task with NULL workflow_id scans correctly.
  * Unit test: Task with valid workflow_id scans correctly.
  * All existing task_repo_test.go tests pass.

## Task 1.18.4: Update UpdateStatus to Handle NullString Output
* **Action:** Update `internal/repository/sqlite/task_repo.go`.
* **Details:**
  * `UpdateStatus(ctx, taskID, status, output string)` signature may need adjustment.
  * Option A: Keep string param, convert to NullString internally.
  * Option B: Change to `output sql.NullString` for explicit null handling.
  * Recommend Option A for backward compatibility.
* **Verification:**
  * Unit test: UpdateStatus with empty output → stores NULL.
  * Unit test: UpdateStatus with content → stores value.

## Task 1.18.5: Update Callers of Task Fields
* **Action:** Grep and update all callers accessing nullable fields.
* **Details:**
  * Search for `task.WorkflowID`, `task.Input`, `task.Output`, `agent.Assistant`.
  * Update access patterns:
    ```go
    // Before
    if task.WorkflowID != "" { ... }
    
    // After
    if task.WorkflowID.Valid { ... }
    ```
  * Update string extraction:
    ```go
    // Before
    workflowID := task.WorkflowID
    
    // After
    workflowID := task.WorkflowID.String // or check .Valid first
    ```
* **Files to check:**
  * `internal/orchestrator/*.go`
  * `internal/project/*.go`
* **Verification:**
  * `go build ./...` succeeds.
  * All tests pass.

## Task 1.18.6: Add Helper Methods for NullString
* **Action:** Update `internal/domain/core.go` or create `internal/domain/helpers.go`.
* **Details:**
  * Add helper for creating NullString:
    ```go
    func NewNullString(s string) sql.NullString {
        return sql.NullString{String: s, Valid: s != ""}
    }
    ```
  * Add helper for extracting with default:
    ```go
    func (t *Task) GetWorkflowID() string {
        if t.WorkflowID.Valid {
            return t.WorkflowID.String
        }
        return ""
    }
    ```
* **Verification:**
  * Helpers reduce boilerplate in callers.
  * Unit tests cover edge cases.

## Task 1.18.7: Update AgentRepository Queries
* **Action:** Update `internal/repository/sqlite/agent_repo.go`.
* **Details:**
  * Ensure SELECT queries don't use COALESCE for `assistant`.
  * sqlx should handle `sql.NullString` automatically.
* **Verification:**
  * Unit test: Agent with NULL assistant scans correctly.
  * `FindByName("human")` returns agent with `Assistant.Valid == false`.

## Task 1.18.8: Update Coding Guidelines
* **Action:** Update `docs/coding/effective_go_2026.md`.
* **Details:**
  * Add new section "## Database" after "## Testing".
  * Document nullable column conventions.
  * Include examples of sql.NullString usage.
  * Reference existing `NullableJSON` pattern.
* **Verification:**
  * Section renders correctly in Markdown viewer.
  * Examples compile.

## Task 1.18.9: Write Migration Test for Null Handling
* **Action:** Update `internal/db/database_test.go`.
* **Details:**
  * Insert task with NULL workflow_id directly via SQL.
  * Query via TaskRepository.
  * Assert `WorkflowID.Valid == false`.
  * Insert task with valid workflow_id.
  * Assert `WorkflowID.Valid == true` and `WorkflowID.String` matches.
* **Verification:**
  * Test passes with both NULL and non-NULL values.
  * No COALESCE needed in queries.
