# User Story 4.11: Blocked Command Warning Alert (Supports EPIC-03)

* **As a** Developer,
* **I want** the UI to distinctly highlight when a command is blocked by the Auto-Approval Interceptor,
* **So that** I can immediately recognize a potential security risk vs. a standard human input request.
* *Acceptance Criteria:*
    * When a task shifts to `awaiting` due to a denylisted command, the `UIAdapter` renders it as a high-priority warning (e.g., Red text in TUI, ⚠️ emoji in CUI).
    * The prompt provides explicit `[Approve Command]` or `[Block & Revert]` options.
