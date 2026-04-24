# Tasks: 1.9 Agent Crash Fallback

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.9-agent-crash-fallback/`
**Tech Stack:** Go (os/exec, context timeouts), SQLite

---

## Task 1.9.1: Add Timeout to Task Entity
* **Action:** Update `internal/domain/task.go` and SQLite migrations.
* **Details:**
    * Add a `Timeout` field (representing milliseconds) to the `Task` struct and a `timeout` column (INTEGER) to the SQLite schema.
    * **Logic:** Update the Task instantiation logic (e.g., `NewTask()`). If a timeout is not explicitly provided during creation, it must default to `3600000` (1 hour in milliseconds) to ensure processes never hang infinitely.

## Task 1.9.2: Process Monitor & Timeout Handling
* **Action:** Update the Agent execution wrapper (`internal/agent/interface.go` implementation).
* **Details:** * Convert `task.Timeout` into a `time.Duration` (e.g., `time.Duration(task.Timeout) * time.Millisecond`).
    * Wrap the `os/exec` call with `context.WithTimeout` using the resulting duration.
    * Monitor the process for unexpected exits (e.g., context deadline exceeded, or non-zero exit codes indicating a panic/segfault).

## Task 1.9.3: State Reassignment
* **Action:** Update the database logic on crash detection.
* **Details:** If the agent crashes or times out:
  1. Reassign the `assignee_id` from the Coding Agent back to the Human User.
  2. Transition the task status to `TaskStatusFailure` (to trigger the crash flow) and subsequently to `TaskStatusAwaiting` for human review.

## Task 1.9.4: UI Notification
* **Action:** Trigger an alert via the UI Mediator.
* **Details:** Render the exact message to the user: *"⚠️ Agent crashed or timed out. Please manually reconcile the working directory and resolve the task status."*

## Task 1.9.5: Discard Command Placeholder (Change Request)
* **Action:** Register a new command in the Dispatcher.
* **Details:** * **Command:** `/task discard <task_id>`
  * **Constraint:** Verify the task belongs to a Coding Agent workflow.
  * **Implementation:** For now, leave this as a stub with a `TODO: Implement OS-level git restore/clean`. 
  * **UI Integration:** Ensure the UI presents a `[ Discard ]` button alongside the crash notification that triggers this new command.