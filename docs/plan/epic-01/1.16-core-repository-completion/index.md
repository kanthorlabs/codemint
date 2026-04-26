# User Story 1.16: Core Repository Layer Completion

* **As the** Go Orchestrator,
* **I want** complete CRUD repositories for Project, Session, and Agent entities with human agent seeding,
* **So that** the `/project-new`, `/project-open`, and session lifecycle commands have a persistence layer to operate on.
* *Acceptance Criteria:*
    * `ProjectRepository` interface with `Create`, `FindByID`, `FindByName`, `Update`, `Delete` methods.
    * `SessionRepository` interface with `Create`, `FindByID`, `FindActiveByProjectID`, `Archive` methods.
    * Human agent (`name="human"`, `type=0`) is automatically seeded on first database initialization.
    * System agent (`name="sys-auto-approve"`, `type=2`) is seeded for future Auto-Approval Interceptor.
    * SQLite implementations enforce the single-active-session constraint via the partial index.
