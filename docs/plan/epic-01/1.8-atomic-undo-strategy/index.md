# User Story 1.8: Atomic Undo Strategy

* **As a** Developer,
* **I want** coding tasks to pause for my review before changes are finalized,
* **So that** I can easily revert AI mistakes.
* *Acceptance Criteria:*
    * When an OpenCode task finishes `processing`, its status shifts to `awaiting` (assigned to human).
    * If I click "Revert", the Go backend sends a rollback signal to the ACP Agent, and the task becomes `reverted`.

---

## Change Request

- If the revert action is failed, we should treat it as agent crash and need to process the agent crash fallback flow, what we will do in the next user story.