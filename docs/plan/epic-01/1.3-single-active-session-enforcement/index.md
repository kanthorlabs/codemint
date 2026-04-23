# User Story 1.3: Single Active Session Enforcement

* **As a** Developer,
* **I want** the database to strictly prevent multiple active sessions per project,
* **So that** my local `working_dir` file system doesn't get corrupted by parallel AI edits.
* *Acceptance Criteria:*
    * The SQLite schema includes a unique partial index: `CREATE UNIQUE INDEX idx_active_session ON session (project_id) WHERE status = 0;`.
    * If I execute `/project-open` and a session with `status = 0` exists, the Go backend blocks creation and prompts me to resume or archive the active one.