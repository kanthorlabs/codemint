# CodeMint PRD & Engineering Spec: EPIC-02 (The Brainstorming Workflow)

## 1. Overview
The Brainstorming Workflow is the core intelligence layer of CodeMint. It transforms a user's natural language requirement into a structured, executable, and reversible list of tasks grouped by Epics and User Stories.

---

## 2. The 5-Phase Brainstorming Pipeline

### Phase 1: Context Intake (The Gatherer)
* **Goal:** Ground the AI in the reality of the codebase.
* **Implementation:** A placeholder for a "Targeted Grep Gatherer." Currently, it will read directory trees and key files. Future iterations will use custom `grep` logic to pull only the exact files related to the user's prompt to save LLM context tokens.

### Phase 2: The Living Spec (Clarification)
* **Goal:** Prevent infinite chat loops and hallucination.
* **Implementation:** The Coordinator Agent chats with the user to clarify requirements. Instead of relying on raw chat history, the system maintains a `specification.md` (Living Spec) in memory. This document is silently updated as decisions are made. The chat continues until the user explicitly triggers `[Generate Plan]`.

### Phase 3: Hierarchical Generation (The Planner)
* **Goal:** Create a sequential, linear execution plan.
* **Implementation:** The Task Generator Agent reads the Living Spec and outputs tasks in an Epic -> Story -> Task format.
* **Database Mapping:** Tasks are inserted into SQLite using three integers: `seq_epic`, `seq_story`, and `seq_task`. 
* **Execution Constraint:** The Go scheduler strictly executes tasks ordered by `seq_epic ASC, seq_story ASC, seq_task ASC`. Parallel execution is explicitly forbidden to prevent Git conflicts.

### Phase 4: Implicit Guardrails (Verification & Confirmation)
* **Goal:** Prevent the "Agent Babysitter" problem while maintaining safety.
* **Implementation:** The Task Generator is strictly prompted to append two final tasks to the end of *every* User Story:
    1.  **Verification Task (Type 1):** Assigned to the Assistant. Runs the test suite or linter (e.g., `go test ./...`) to ensure syntax/logic is correct.
    2.  **Confirmation Task (Type 2):** Assigned to the `Human Agent`. This automatically shifts the session to `awaiting` and prompts the TUI/CUI for human review of the completed User Story.

### Phase 5: Human Review & Activation
* **Goal:** Final approval before execution.
* **Implementation:** The TUI/CUI renders the generated list. The user can approve, remove, or refine tasks. Once activated, tasks are inserted into SQLite with `status = 0` (pending).

---

## 3. Advanced Workflow Features

### 3.1 Mid-Flight Pivots (Dynamic Replanning)
If a user rejects a Confirmation task or changes their mind mid-execution:
1.  **Scope Selection:** The UI displays the remaining pending tasks for either the current User Story or the Entire Backlog.
2.  **User Action:** The user selects specific tasks to edit, or chooses "Edit All".
3.  **Update:** The user manually updates the tasks or provides a prompt to have the AI rewrite the selected tasks. The SQLite `pending` tasks are then updated.

### 3.2 YOLO Mode (The `Auto Approval` Agent)
To allow fully autonomous execution without changing the database schema or execution logic:
1.  **System Agent:** CodeMint bootstraps with a special `sys-auto-approve` agent.
2.  **Invocation:** The user selects Epics or Stories in the UI and clicks `[Delegate to Auto Approval]`.
3.  **Logic:** CodeMint updates the `assignee_id` of the Confirmation tasks to `sys-auto-approve`. When the Go scheduler reaches these tasks, it instantly marks them as `success` with the note "Auto-approved via YOLO mode", completely bypassing the human pause.
