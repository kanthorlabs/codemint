# CodeMint Engineering Specification: EPIC-01 (Foundation & Routing)

## 1. Project Overview & Philosophy
**Codename:** CodeMint
**Language:** Go (Golang)
**Database:** SQLite (Local, single-file per installation)
**Core Intent:** A modular AI orchestrator that translates high-level requirements into verified code via Agent Communication Protocol (ACP).

### The "Human-as-an-Agent" Pattern
In CodeMint, the user is treated as a high-latency, asynchronous API. Do not write separate orchestration loops for AI execution vs. Human input. Instead, implement a unified `Agent` interface. When a task requires human review or input, assign it to the seeded `Human` agent. The system simply pushes the task payload and waits/blocks on a channel until the Unified UI (TUI/CUI) resolves it.

---

## 2. Persistence Layer (Database Contract)

### 2.1 Identifiers: UUID v7
**Mandatory:** All primary keys (`id`) and foreign keys must use **UUID v7**.
* **Why:** UUID v7 includes a Unix timestamp prefix. This ensures lexicographical sorting, meaning SQLite B-Tree insertions remain highly performant (like auto-incrementing integers) while allowing decentralised ID generation in Go before DB commits.

### 2.2 Schema Definitions (Singular Naming)
Use Go `database/sql` or `sqlx`. All table and column names **must be singular**.

| Table | Column | Type | Description / Constraint |
| :--- | :--- | :--- | :--- |
| **`project`** | `id` | TEXT (UUIDv7) | Primary Key |
| | `name` | TEXT | Unique project name |
| | `working_dir` | TEXT | Absolute path to project root |
| **`agent`** | `id` | TEXT (UUIDv7) | Primary Key |
| | `name` | TEXT | e.g., "Human", "OpenCode-Primary" |
| | `type` | INTEGER | Enum: `0` (Human), `1` (Assistant) |
| | `assistant` | TEXT | Config key mapping to `config.yaml` |
| **`session`** | `id` | TEXT (UUIDv7) | Primary Key |
| | `project_id` | TEXT (UUIDv7) | FK to `project.id` |
| | `status` | INTEGER | Enum: `0` (Active), `1` (Archived) |
| **`workflow`** | `id` | TEXT (UUIDv7) | Primary Key |
| | `session_id` | TEXT (UUIDv7) | FK to `session.id` |
| | `type` | INTEGER | Enum mapping to internal Go registry |
| **`task`** | `id` | TEXT (UUIDv7) | Primary Key |
| | `project_id` | TEXT (UUIDv7) | FK to `project.id` |
| | `session_id` | TEXT (UUIDv7) | FK to `session.id` |
| | `workflow_id` | TEXT (UUIDv7) | Optional FK to `workflow.id` |
| | `assignee_id` | TEXT (UUIDv7) | FK to `agent.id` |
| | `type` | INTEGER | Enum: `0`:Coding, `1`:Verification, `2`:Confirmation, `3`:Coordination |
| | `status` | INTEGER | Task status enum (See Section 3) |
| | `input_data` | TEXT | JSON blob of context/prompt |
| | `output_data` | TEXT | JSON blob of result/diff/logs |

### 2.3 The "Single Active Session" Constraint
**CRITICAL:** To prevent file system corruption in the `working_dir`, a project can only have **one** active session at a time.
* **Implementation:** Enforce via SQLite partial index: `CREATE UNIQUE INDEX idx_active_session ON session (project_id) WHERE status = 0;`
* **Go Logic:** If a user runs `/project-open` and an active session exists, prompt them to resume or archive it. Do not instantiate a second active session.

---

## 3. The Task State Machine

Tasks are the atomic units of work. Define these as a custom type in Go (e.g., `type TaskStatus int` with an `iota` block).

### Active States (Mutable)
* **`pending` (0):** Task is in the queue, waiting for the scheduler to allocate an agent.
* **`processing` (1):** Agent is actively executing (sub-process running).
* **`awaiting` (2):** **Blocking state.** The task is paused for Human input or a dependent resource. The scheduler must not advance dependent tasks.

### Terminal States (Immutable)
Once a task hits one of these states, its `output_data` and `status` are locked.
* **`success` (3):** Goal met, changes committed to the `working_dir`.
* **`failure` (4):** Execution finished, but logic failed (e.g., failed tests) or an unrecoverable system issue occurred.
* **`completed` (5):** Process finished with a neutral/informational outcome (e.g., a status report).
* **`reverted` (6):** User rejected the output; changes rolled back (See Section 5).
* **`cancelled` (7):** Manual abort by the user.

---

## 4. Routing & Orchestration Components

### 4.1 Hybrid Router
Incoming requests (via TUI or CUI) pass through a dual-path router:
1.  **Command Parser (Deterministic):** Uses Regex/Argparse to catch slash commands (`/project-new`, `/task-status`, `/approve <id>`). Executes immediately, bypassing AI.
2.  **Coordinator AI (Probabilistic):** Uses a lightweight LLM. Evaluates natural language through a **Binary Context Gateway**:
    * *Context-Aware:* Requires `working_dir` knowledge. Routes to **Project Coding** workflow.
    * *Non-Context:* General inquiries. Routes to **Communication** or **Daily Checking** workflows.

### 4.2 Workflow Registry
Workflows are not hardcoded if/else blocks.
* Load definitions from `config.yaml` at startup.
* Register them in a Go map (e.g., `map[int]Workflow`).
* Core workflows to scaffold: `Project Coding` (Primary), `Communication`, `Daily Checking`.

### 4.3 Unified UI ("First-In-Wins")
* **TUI (Bubble Tea):** Renders interactive lists.
* **CUI (Telegram/Slack):** Sends inline buttons or expects slash commands.
* **State Sync:** Both UI drivers listen to the same Go channel for `awaiting` Human Agent tasks. Whichever interface receives the user's action first processes it and updates the other view.

---

## 5. The Atomic Revert & ACP Layer

### 5.1 ACP Integration Priority
When integrating CLI agents via `os/exec` standard I/O (JSON-RPC), prioritize support in this order based on ACP compliance quality:
1.  **OpenCode** (Primary target)
2.  **Codex**
3.  **Cursor**

### 5.2 The "Undo" Strategy
When a Coding task finishes `processing`, it **always** transitions to `awaiting` (Human Review).
* **Approve:** Go backend sends a commit signal to the ACP Agent -> Task becomes `success`.
* **Revert:** Go backend sends an undo/rollback signal to the ACP Agent -> Task becomes `reverted`. **Reverts are atomic** (all changes in the task are rolled back, no partials).

### 5.3 Agent Crash Fallback
If the `os/exec` process for the ACP agent panics, crashes, or times out during `processing` or while staged:
1.  Reassign the `task.assignee_id` to the **Human Agent**'s UUID.
2.  Keep/Move status to `awaiting`.
3.  Notify the user via UI: *"Agent crashed. Please manually reconcile the working directory and resolve the task status."*

---
**End of Spec - EPIC-01**
