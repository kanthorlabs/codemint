# User Story 2.9: YOLO Mode Delegation

* **As a** Developer,
* **I want to** delegate specific Epics or Stories to the YOLO Agent,
* **So that** execution continues autonomously without pausing for my confirmation.
* *Acceptance Criteria:*
    * The UI exposes a `[Delegate to Auto Approval]` action for Epics or Stories.
    * CodeMint updates the `assignee_id` of the target Confirmation tasks to the `sys-auto-approve` UUID.
    * When the Go scheduler reaches a task assigned to this agent, it instantly marks it as `success` with a note bypassing the human pause.
