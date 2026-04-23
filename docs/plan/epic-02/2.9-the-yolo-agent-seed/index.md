# User Story 2.8: The YOLO Agent Seed

* **As the** Go Orchestrator,
* **I want to** seed a special `sys-auto-approve` agent into the `agent` table on bootstrap,
* **So that** CodeMint can support fully autonomous execution without changing the database schema.
* *Acceptance Criteria:*
    * The `agent` table initializes with an entry where `name` = "sys-auto-approve".
