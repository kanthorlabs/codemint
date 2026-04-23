# User Story 2.6: Phase 5 - Human Review & Activation

* **As a** Developer,
* **I want to** review the generated list of tasks before they are queued for execution,
* **So that** I have final approval over the AI's plan.
* *Acceptance Criteria:*
    * The UI (TUI/CUI) renders the generated hierarchical list.
    * The user can approve, remove, or refine tasks.
    * Once activated, tasks are committed to SQLite with `status = 0` (pending).
