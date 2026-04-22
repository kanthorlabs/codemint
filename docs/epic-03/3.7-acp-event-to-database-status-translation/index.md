# User Story 3.7: ACP Event to Database Status Translation

* **As the** Go Orchestrator,
* **I want to** translate standard ACP lifecycle events into CodeMint's immutable database states,
* **So that** the UI accurately reflects the agent's progress.
* *Acceptance Criteria:*
    * `session.start` or `turn-start` shifts task to `processing`.
    * `human-input-request` shifts task to `awaiting`.
    * `task-complete` or `turn-end` shifts task to `success` (and pulls the next task).
