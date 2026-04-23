# CodeMint User Stories: EPIC-02 (The Brainstorming Workflow)

## Group 1: The 5-Phase Brainstorming Pipeline

**User Story 2.1: Phase 1 - Context Intake (The Gatherer)**
* **As the** Go Orchestrator,
* **I want** the Gatherer Agent to read directory trees and key files before brainstorming starts,
* **So that** the AI is grounded in the reality of the codebase.
* *Acceptance Criteria:*
    * The system reads the project directory tree and passes it to the LLM.
    * Logic is structured to allow future iteration into a "Targeted Grep Gatherer" to save context tokens.

**User Story 2.2: Phase 2 - The Living Spec (Clarification)**
* **As the** Coordinator Agent,
* **I want to** maintain a `specification.md` document in memory during the chat phase,
* **So that** I prevent infinite chat loops and hallucinations.
* *Acceptance Criteria:*
    * The system silently updates the `specification.md` document as decisions are made during the chat.
    * The chat loop continues until the user explicitly triggers the `[Generate Plan]` action.

**User Story 2.3: Phase 3 - Hierarchical Task Generation**
* **As the** Task Generator Agent,
* **I want to** read the Living Spec and output an executable plan in an Epic -> Story -> Task format,
* **So that** I create a sequential, linear execution plan.
* *Acceptance Criteria:*
    * Tasks are inserted into SQLite using integers for `seq_epic`, `seq_story`, and `seq_task` to ensure strict ordering.
    * The Go scheduler is explicitly programmed to forbid parallel execution of these tasks to prevent Git conflicts.

**User Story 2.4: Phase 4 - Verification Guardrail**
* **As the** Task Generator Agent,
* **I want to** strictly append a Verification Task (Type 1) at the end of every User Story,
* **So that** the syntax and logic are automatically checked (e.g., via `go test ./...`) before human review.
* *Acceptance Criteria:*
    * The AI's generated task array always includes a task with `type = 1` right before the end of a `seq_story` block.
    * This task is assigned to the Assistant agent.

**User Story 2.5: Phase 4 - Confirmation Guardrail**
* **As the** Task Generator Agent,
* **I want to** append a Confirmation Task (Type 2) as the final step of every User Story,
* **So that** the execution safely pauses for manual approval.
* *Acceptance Criteria:*
    * A task with `type = 2` is inserted as the absolute final task of a `seq_story` block.
    * This task is assigned to the seeded `human` agent, which naturally shifts the session to `awaiting`.

**User Story 2.6: Phase 4 - Epic Retrospective Guardrail**
* **As the** Task Generator Agent,
* **I want to** append a Retrospective Task (Type 4) as the absolute final step of every Epic,
* **So that** the human can be politely asked for overarching feedback after a large batch of work is done.
* *Acceptance Criteria:*
    * A task with `type = 4` is inserted as the final task of a `seq_epic` block.
    * This task is assigned to the seeded `human` agent.
    * The UI presents a skippable, conversational prompt to the user (e.g., "Did I do anything annoying?").
    * User input (if provided) is saved to the `output` column of the task to be processed by the Archivist.

**User Story 2.7: Phase 5 - Human Review & Activation**
* **As a** Developer,
* **I want to** review the generated list of tasks before they are queued for execution,
* **So that** I have final approval over the AI's plan.
* *Acceptance Criteria:*
    * The UI (TUI/CUI) renders the generated hierarchical list.
    * The user can approve, remove, or refine tasks.
    * Once activated, tasks are committed to SQLite with `status = 0` (pending).

---

## Group 2: Advanced Workflow Features

**User Story 2.8: Mid-Flight Pivots (Dynamic Replanning)**
* **As a** Developer,
* **I want to** select specific pending tasks and instruct the AI to rewrite them mid-execution,
* **So that** I can change my mind without restarting the entire session.
* *Acceptance Criteria:*
    * The UI displays pending tasks for either the current User Story or the Entire Backlog.
    * The user can multi-select tasks to edit.
    * The user provides a natural language prompt, and the AI rewrites the selected tasks, directly updating the `pending` rows in SQLite.

**User Story 2.9: The YOLO Agent Seed**
* **As the** Go Orchestrator,
* **I want to** seed a special `sys-auto-approve` agent into the `agent` table on bootstrap,
* **So that** CodeMint can support fully autonomous execution without changing the database schema.
* *Acceptance Criteria:*
    * The `agent` table initializes with an entry where `name` = "sys-auto-approve".

**User Story 2.10: YOLO Mode Delegation**
* **As a** Developer,
* **I want to** delegate specific Epics or Stories to the YOLO Agent,
* **So that** execution continues autonomously without pausing for my confirmation.
* *Acceptance Criteria:*
    * The UI exposes a `[Delegate to Auto Approval]` action for Epics or Stories.
    * CodeMint updates the `assignee_id` of the target Confirmation tasks to the `sys-auto-approve` UUID.
    * When the Go scheduler reaches a task assigned to this agent, it instantly marks it as `success` with a note bypassing the human pause.

---

## Group 3: Cross-Epic Support Integrations

**User Story 2.11: LLM Wiki Context Injection (Supports EPIC-05)**
* **As the** Go Orchestrator,
* **I want to** inject the "Hot" Wiki files (`preferences.md`, `decisions.md`, `bugs/index.md`) into the Brainstormer Agent's system prompt during Phase 1,
* **So that** the Task Generator respects the project's historical decisions and doesn't repeat past mistakes.
* *Acceptance Criteria:*
    * Before the LLM generates the task list, it reads `~/.local/share/codemint/memory/<project_id>/`.
    * The System Prompt strictly enforces the Hierarchy of Authority: Current Prompt > Project Memory > Global Rules.

**User Story 2.12: The Clarifier Agent Handoff (Supports EPIC-04)**
* **As the** Go Orchestrator,
* **I want to** explicitly route Mid-Flight Pivots to the dedicated `Clarifier Agent`,
* **So that** natural language revisions are correctly translated into database `UPDATE` queries for existing tasks without hallucinating new scopes.
* *Acceptance Criteria:*
    * When a user clicks `[ ✏️ Revise ]`, the UI prompts: "What should I change?".
    * The user's reply is sent specifically to the Clarifier Agent, which directly overwrites the target SQLite rows and generates a new draft for the UI.

**User Story 2.13: ACP-Compliant Payload Formatting (Supports EPIC-03)**
* **As the** Task Generator Agent,
* **I want to** format the generated tasks' `input` field as structured JSON,
* **So that** the Persistent ACP Worker (OpenCode) can seamlessly parse the request over standard I/O.
* *Acceptance Criteria:*
    * The `input` column in the database is populated with a JSON blob containing the context and prompt, not just raw text strings.