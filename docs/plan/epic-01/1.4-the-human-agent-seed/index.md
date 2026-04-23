# User Story 1.4: The `human` Agent Seed

* **As the** Go Orchestrator,
* **I want to** seed a system `human` agent into the `agent` table on initialization,
* **So that** I can assign blocking tasks (like approvals) directly to the user without writing separate orchestration loops.
* *Acceptance Criteria:*
    * The `agent` table contains an entry with `name` = "human" (strictly kebab-case) and `type` = 0.
    * When a task is assigned to this UUID, the scheduler halts and pushes the payload to the Unified UI channel.