# User Story 1.11: Hierarchical Task Schema Expansion

* **As the** Go Orchestrator,
* **I want** the `task` table schema to include sequential ordering columns,
* **So that** the Brainstormer can map tasks to a strict linear execution graph.
* *Acceptance Criteria:*
    * The `task` table includes `seq_epic`, `seq_story`, and `seq_task` (all `INTEGER` types).
    * The scheduler queries tasks ordered by `seq_epic ASC, seq_story ASC, seq_task ASC`.