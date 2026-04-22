# User Story 1.1: SQLite Schema Initialization

* **As the** Go Orchestrator,
* **I want to** automatically initialize a local SQLite database with singular table names (`project`, `agent`, `session`, `workflow`, `task`) upon first run,
* **So that** CodeMint has a robust, local, single-file persistence layer ready immediately.
* *Acceptance Criteria:*
    * Schema creation uses `database/sql` or `sqlx`.
    * `task` table stores `type` and `status` strictly as `INTEGER` columns (mapped to Go enums in the backend code).
    * `task` table includes JSON text columns named concisely: `input` and `output`.