# User Story 2.3: Phase 3 - Hierarchical Task Generation

* **As the** Task Generator Agent,
* **I want to** read the Living Spec and output an executable plan in an Epic -> Story -> Task format,
* **So that** I create a sequential, linear execution plan.
* *Acceptance Criteria:*
    * Tasks are inserted into SQLite using integers for `seq_epic`, `seq_story`, and `seq_task` to ensure strict ordering.
    * The Go scheduler is explicitly programmed to forbid parallel execution of these tasks to prevent Git conflicts.
