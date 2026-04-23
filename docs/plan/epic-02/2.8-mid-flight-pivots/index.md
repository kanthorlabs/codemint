# User Story 2.7: Mid-Flight Pivots (Dynamic Replanning)

* **As a** Developer,
* **I want to** select specific pending tasks and instruct the AI to rewrite them mid-execution,
* **So that** I can change my mind without restarting the entire session.
* *Acceptance Criteria:*
    * The UI displays pending tasks for either the current User Story or the Entire Backlog.
    * The user can multi-select tasks to edit.
    * The user provides a natural language prompt, and the AI rewrites the selected tasks, directly updating the `pending` rows in SQLite.
