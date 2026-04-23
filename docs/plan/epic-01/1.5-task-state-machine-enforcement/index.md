# User Story 1.5: Task State Machine Enforcement

* **As the** Go Orchestrator,
* **I want to** strictly enforce mutable active states (`pending`, `processing`, `awaiting`) and immutable terminal states (`success`, `failure`, `completed`, `reverted`, `cancelled`),
* **So that** the execution logic is perfectly predictable and auditable.
* *Acceptance Criteria:*
    * Task states are defined as a custom Go type (e.g., `type TaskStatus int`).
    * Once a task transitions to any state >= 3 (Terminal), its `output` and `status` fields are locked and cannot be updated.