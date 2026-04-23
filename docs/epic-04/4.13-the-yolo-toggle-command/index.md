# User Story 4.13: The `/yolo` Toggle Command

* **As a** Developer using the TUI or CUI,
* **I want to** use the `/yolo` command to toggle Auto-Approval mode,
* **So that** I can easily switch between supervised and autonomous execution.
* *Acceptance Criteria:*
    * Executing `/yolo` toggles the YOLO state for the current active project context and persists it in the database.
    * Executing `/yolo --project-id <id>` explicitly toggles the state for the specified project.
    * Executing `/yolo --temp` enables YOLO mode strictly in the **volatile memory** of the active session. If the user switches sessions, closes CodeMint, or changes projects, the `--temp` flag is destroyed and the session reverts to standard human-approval mode upon return.
