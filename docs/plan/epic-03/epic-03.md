# CodeMint PRD & Engineering Spec: EPIC-03 (ACP Execution Layer)

## 1. Overview
EPIC-03 defines how CodeMint executes the tasks generated in the Brainstorming Workflow. It acts as the bridge between the Go orchestration backend and the underlying AI CLI agents (e.g., OpenCode, Codex) via standard standard input/output (JSON-RPC over standard I/O).

---

## 2. Core Architecture: The Persistent ACP Worker

### 2.1 Lifecycle Management
- **Single Process:** To prevent high token costs and cold-start latency, CodeMint spawns **one** background `os/exec` process (the Persistent Worker) per active CodeMint Session.
- **Context Clearing:** The Go backend sends a `clear` or `reset` command via the JSON-RPC stream at the boundary of every User Story to flush the agent's context window, keeping token usage lean without killing the process.
- **Teardown:** The `os/exec` process is gracefully terminated only when the user archives or pauses the CodeMint Session.

---

## 3. Security & Autonomy: The Auto-Approval Interceptor

To prevent the "Agent Babysitter" problem while maintaining security, CodeMint intercepts all CLI execution requests from the Agent.

### 3.1 Schema Update: `project_permission`
A new table manages the whitelist/denylist for agent operations.

| Column | Type | Description / Logic |
| :--- | :--- | :--- |
| `id` | TEXT (UUIDv7) | Primary Key |
| `project_id` | TEXT (UUIDv7) | FK to `project.id` |
| `allowed_commands` | TEXT (JSON) | e.g., `["go test", "npm run lint"]` |
| `allowed_directories` | TEXT (JSON) | e.g., `["./src", "./tests"]` |
| `blocked_commands` | TEXT (JSON) | Explicit denylist (e.g., `["rm", "curl"]`) |

### 3.2 Interceptor Logic
When the Agent emits a tool call (e.g., `{"type": "tool-call", "tool": "terminal", "command": "go test"}`):
1. **The Intercept:** CodeMint halts the event from reaching the UI.
2. **Evaluation:** It checks the command against the `project_permission` table.
3. **Match (Whitelist):** CodeMint automatically executes the command locally, captures the output, and sends it back to the Agent via `stdin`. The task status remains `processing`. The user is not disturbed.
4. **No Match / Blocked:** CodeMint halts the agent stream, updates the task status to `awaiting`, and pings the UI for manual human approval.

---

## 4. State Mapping & Event Routing

### 4.1 JSON-RPC to DB Status Translation
CodeMint translates standard ACP JSON-RPC events into our immutable database states:

* **`session.start` / `turn-start`:** CodeMint Task -> `processing`.
* **`tool-call` (Not Whitelisted):** CodeMint Task -> `awaiting` (Requires human input).
* **`human-input-request`:** CodeMint Task -> `awaiting`.
* **`turn-end` / `task-complete`:** CodeMint Task -> `success` (Agent finished, scheduler pulls next task).

### 4.2 Noise Reduction & Event Streaming
Agent execution produces high-frequency "noise" (e.g., `thinking`, `file-edit` micro-events).

* **TUI (Terminal UI):** Subscribes to the raw, high-bandwidth event stream to show real-time "thinking" logs in a dedicated pane.
* **CUI (Chat UI):** Subscribes *only* to terminal/blocking state changes (`awaiting`, `success`, `failure`) to prevent chat spam.
* **The `/summary` Command:** * **Usage:** `/summary [task_id]`
    * **Logic:** CodeMint stores the last N raw events in a circular memory buffer. When called, it aggregates these events into a clean `<thinking>` block.
    * **Fallback:** If no `task_id` is provided, the Go backend queries the database for the active session's most recent `processing` or `awaiting` task and summarizes that context.
