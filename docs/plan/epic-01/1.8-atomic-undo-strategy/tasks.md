# Tasks: 1.8 Atomic Undo Strategy

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.8-atomic-undo/`
**Tech Stack:** Go, SQLite

---

## 🛠 Architectural Concept: The Orchestrator Proxy & Defensive Reverts
CodeMint does not manually diff or revert files. Instead, it delegates the actual file system rollback to the underlying Coding Agent (e.g., via the Agent Control Protocol - ACP, or native CLI commands). 

When a coding task finishes processing, CodeMint pauses the state machine at `Awaiting`. The UI presents "Accept" or "Revert" buttons. Clicking these sends a command to the Orchestrator. 
**The Guardrail:** If the underlying Agent returns an error during a Revert action, CodeMint assumes the Agent's internal state is corrupted. It immediately transitions the task to `TaskStatusFailure` and triggers the Agent Crash Fallback Flow (Story 1.9).

---

## Task 1.8.1: Enforce the Standard Task State Machine
* **Action:** Verify/Update `internal/domain/task.go`.
* **Details:**
    * Ensure the `TaskStatus` enum matches our agreed-upon `iota` structure:
      ```go
      type TaskStatus int

      const (
          TaskStatusPending    TaskStatus = iota // 0
          TaskStatusProcessing                   // 1
          TaskStatusAwaiting                     // 2: Waiting for human review
          TaskStatusSuccess                      // 3
          TaskStatusFailure                      // 4: Used for Agent Crash / Revert Failures
          TaskStatusCompleted                    // 5
          TaskStatusReverted                     // 6: Human rejected, rollback complete
          TaskStatusCancelled                    // 7
      )
      ```

## Task 1.8.2: Expand the Coding Agent Interface
* **Action:** Update `internal/agent/interface.go`.
* **Details:**
    * Ensure the interface that wraps the underlying ACP/Coding Agent CLI enforces the Accept/Revert contract:
      ```go
      type CodingAgent interface {
        // Transitions to Awaiting
        ExecuteTask(ctx context.Context, task *domain.Task) error 
        
        // Finalizes the change (can use task.Name for commit messages)
        Accept(ctx context.Context, task *domain.Task) error      
        
        // Triggers agent's native undo (can use task data for error logging)
        Revert(ctx context.Context, task *domain.Task) error
      }
      ```

## Task 1.8.3: Implement the Review Command Handlers
* **Action:** Create `internal/project/review_commands.go`.
* **Details:**
    * Register two new commands in the Dispatcher to act as the backend targets for the UI/CUI:
        * **`/task accept <task_id>`:** 1. Verifies the task is `TaskStatusAwaiting`.
            2. Calls `CodingAgent.Accept(taskID)`.
            3. Updates task status to `TaskStatusSuccess` (or `Completed`) in SQLite.
            4. Returns a `CommandResult` to notify the UI.
        * **`/task revert <task_id>`:**
            1. Verifies the task is `TaskStatusAwaiting`.
            2. Calls `CodingAgent.Revert(taskID)`.
            3. **Crash Fallback Trigger:** If `Revert` returns an error, log the failure, update status to `TaskStatusFailure`, and trigger the fallback sequence (Story 1.9).
            4. If successful, updates task status to `TaskStatusReverted` in SQLite.
            5. Returns a `CommandResult` reflecting the outcome.

## Task 1.8.4: Database Update Methods
* **Action:** Update `internal/db/task_store.go`.
* **Details:**
    * Implement `UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus) error`.
    * Ensure this operates within a transaction to safely commit the integer state change alongside any error logs.

## Task 1.8.5: Review Flow Unit Tests
* **Action:** Create `internal/project/review_commands_test.go`.
* **Details:**
    * *Test A (Happy Path Accept):* Mock a task in `TaskStatusAwaiting`. Call `/task accept <task_id>`, assert the mock agent's `Accept` method was called, and the DB status is updated properly.
    * *Test B (Happy Path Revert):* Mock a task in `TaskStatusAwaiting`. Call `/task revert <task_id>`, assert the mock agent's `Revert` method was called, and the DB status is `TaskStatusReverted`.
    * *Test C (Agent Crash on Revert):* Mock an agent that returns an error on `Revert()`. Assert the DB status becomes `TaskStatusFailure` to trigger the fallback flow.