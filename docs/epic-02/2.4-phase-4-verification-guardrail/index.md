# User Story 2.4: Phase 4 - Verification Guardrail

* **As the** Task Generator Agent,
* **I want to** strictly append a Verification Task (Type 1) at the end of every User Story,
* **So that** the syntax and logic are automatically checked (e.g., via `go test ./...`) before human review.
* *Acceptance Criteria:*
    * The AI's generated task array always includes a task with `type = 1` right before the end of a `seq_story` block.
    * This task is assigned to the Assistant agent.
