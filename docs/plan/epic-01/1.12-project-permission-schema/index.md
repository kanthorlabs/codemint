# User Story 1.12: Project Permission Schema

* **As the** Go Orchestrator,
* **I want** a `project_permission` table initialized in the database,
* **So that** the Execution Layer has a central source of truth for the Auto-Approval Interceptor.
* *Acceptance Criteria:*
    * Table includes `id` (UUIDv7), `project_id` (UUIDv7), and `allowed_commands`, `allowed_directories`, `blocked_commands` (all stored as JSON text).