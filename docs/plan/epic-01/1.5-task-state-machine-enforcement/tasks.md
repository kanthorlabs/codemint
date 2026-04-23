# Tasks: 1.5 Task State Machine Enforcement

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.5-task-state-machine/`
**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), `jmoiron/sqlx`

---

## State Machine Specification

### 1. State Glossary
| State | ID | Description |
| :--- | :--- | :--- |
| **Pending** | 0 | Task is in the backlog, waiting for the Orchestrator to "Claim" it. |
| **Processing** | 1 | Task is currently being executed by an Agent (Human, Assistant, or System). |
| **Awaiting** | 2 | Execution is paused. Requires human input (Confirmation) or Security Interceptor approval. |
| **Success** | 3 | Agent finished the task and produced output/diff, but it is not yet "finalized." |
| **Failure** | 4 | Agent was unable to complete the task or returned an error. |
| **Completed** | 5 | **Terminal State.** Verification passed and the task is moved to project history. |
| **Reverted** | 6 | The task's changes were undone by a user or an automated rollback. |
| **Cancelled** | 7 | The task was manually terminated before execution finished. |

### 2. State Machine Logic (Valid Transitions)
To maintain data integrity, the system must enforce these transitions:
* **Active Loop:** `Pending (0)` ➔ `Processing (1)` ➔ `Success (3)` OR `Failure (4)`.
* **Human Intervention:** `Processing (1)` ➔ `Awaiting (2)` ➔ `Processing (1)`.
* **Finalization:** `Success (3)` ➔ `Completed (5)` (Triggered by a successful Type 1 Verification task).
* **Revision Loop:** `Success (3)` ➔ `Processing (1)` (Triggered by a user clicking `[ ✏️ Revise ]`).
* **Abnormal Ends:** `Pending` / `Processing` / `Awaiting` ➔ `Cancelled` / `Reverted`.

### 3. The "Buffer Zone": Success vs. Completed
CodeMint distinguishes between **Success** and **Completed** to enable a robust recovery and revision workflow:
* **Success (The Agent's Goal):** When an agent finishes writing code or running a command, it is successful. However, the code hasn't been "vouched for" by the system yet. Keeping it in `Success` allows the user to revise the output or the system to run automated tests against it.
* **Completed (The Orchestrator's Goal):** A task is only `Completed` when the chain of logic is verified. For example, a "Write Code" task is only marked `Completed` once a subsequent "Run Tests" task passes. This ensures we never lose the ability to "roll back" to a known good state if a later verification fails.

---

## Technical Reference: Data Integrity & Safe Recovery

### Why We Need Atomic System Writes
While external agents (`OpenCode`, `ClaudeCode`) manage their own file I/O for source code, CodeMint's Go backend is the **sole bookkeeper** for "System Memory" (EPIC-05). This includes `preferences.md`, `decisions.md`, and the `index.md` insight ledger.

A standard `os.Create` followed by `f.Write` is **not an atomic operation**. If the system crashes, the power fails, or the disk fills up during the write:
1. The file is truncated to 0 bytes or left with mangled, partial content.
2. CodeMint loses its historical context and preferences.
3. The next boot will likely fail to parse the corrupted Markdown files.

**The Atomic Solution:** We write the new content to a temporary file (`path.tmp`), ensure it is flushed to physical storage (`f.Sync`), and then use the OS-level `os.Rename`. This ensures the file is either in the **Old State** or the **New State**, but never a **Corrupted State**.

### Why "Blind Recovery" is Dangerous
If CodeMint crashes while a task is `Processing (1)`, the file system might be in a "Dirty" state (e.g., half-applied code changes from an agent).
* **Blind Reset:** If we simply change the status back to `Pending (0)`, the next agent execution will attempt to apply its changes on top of a broken/dirty workspace, causing a cascade of syntax errors or logic bugs.
* **Safe Recovery:** We use Git as our "Physical Sanity Check." We only reset to `Pending` if `git status` is clean. If dirty, we force a `RecoveryIntent` to the user to handle the conflict manually.

---

## Task 1.5.1: Task Repository Interface (Effective Go)
* **Action:** Update `internal/repository/task_repo.go`.
* **Details:**
    * Define the `TaskRepository` interface with atomic methods (avoiding stuttering):
        ```go
        type TaskRepository interface {
            Next(ctx context.Context, sessionID string) (*domain.Task, error)
            Claim(ctx context.Context, taskID string) error
            UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error
            FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error)
        }
        ```

## Task 1.5.2: Implement the "Next Task" Selector
* **Action:** Implement `Next` in `internal/repository/sqlite/task_repo.go`.
* **Details:**
    * Write a query to find the first task where `session_id = ?` AND `status` is `Pending (0)` or `Awaiting (2)`.
    * **Crucial:** It MUST `ORDER BY seq_epic, seq_story, seq_task ASC` to respect the hierarchy.

## Task 1.5.3: Atomic "Claim" Operation
* **Action:** Implement `Claim` in `internal/repository/sqlite/task_repo.go`.
* **Details:**
    * Use a transaction with `BEGIN IMMEDIATE`.
    * Check if the task is still `Pending`.
    * If yes, `UPDATE task SET status = 1 WHERE id = ?`.

## Task 1.5.4: Interrupted Task Detection (Safe Recovery)
* **Action:** Implement `FindInterrupted` in the repository.
* **Details:**
    * Returns all tasks where `status = 1` (Processing) for the current session.
    * This method does **not** perform updates; it only gathers data for the Orchestrator to evaluate.

## Task 1.5.5: The Workspace Verifier & Safe Reset
* **Action:** Create `internal/workspace/verifier.go` and `internal/orchestrator/recovery.go`.
* **Details:**
    * Implement `IsDirty()` via `git status --porcelain`.
    * **Orchestrator Logic:**
        1. Call `taskRepo.FindInterrupted()`.
        2. If tasks found: Check `IsDirty()`.
        3. If clean: Execute `UpdateStatus(id, Pending)`.
        4. If dirty: Emit a `RecoveryIntent` to the `UIMediator`.

## Task 1.5.6: Atomic System Writer (Utility)
* **Action:** Create `internal/util/atomicio/writer.go`.
* **Details:**
    * Implement `WriteAtomic(path string, data []byte) error`.
    * Use the pattern: Create `path.tmp` ➔ Write ➔ `f.Sync()` ➔ `os.Rename(path.tmp, path)`.

## Task 1.5.7: Task Repository Unit Tests
* **Action:** Create `internal/repository/sqlite/task_repo_test.go`.
* **Details:**
    * Assert hierarchical ordering.
    * Assert `Claim` atomicity (simulating concurrent access).
    * Verify `FindInterrupted` correctly identifies tasks stuck in the processing state.