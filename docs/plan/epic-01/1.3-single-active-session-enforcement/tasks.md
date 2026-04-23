# Tasks: 1.3 Single Active Session Enforcement

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.3-single-active-session-enforcement/`
**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), `jmoiron/sqlx`

---

## Task 1.3.1: Define Domain Error & Repository Interface
* **Action:** Update `internal/domain/models.go` and create `internal/repository/session_repo.go`.
* **Details:**
  * Define a custom Go error: `var ErrActiveSessionExists = errors.New("project already has an active session")`.
  * Following *Effective Go* (avoiding stuttering), define a `SessionRepository` interface with methods:
    * `Create(ctx context.Context, s *domain.Session) error`
    * `FindActive(ctx context.Context, projectID string) (*domain.Session, error)`
    * `Archive(ctx context.Context, id string) error`

## Task 1.3.2: Implement Session Repository Methods
* **Action:** Create `internal/repository/sqlite/session_repo.go`.
* **Details:**
  * Implement `Create`. Catch the database error returned by `sqlx`. Use type assertion to check if the error is a `modernc.org/sqlite` constraint violation (`SQLITE_CONSTRAINT_UNIQUE`) on `idx_active_session`. If it is, return `domain.ErrActiveSessionExists`.
  * Implement `FindActive`. Query the session table `WHERE project_id = ? AND status = 0`.
  * Implement `Archive`. Execute an `UPDATE session SET status = 1 WHERE id = ?`.

## Task 1.3.3: Write Constraint Unit Tests
* **Action:** Create `internal/repository/sqlite/session_repo_test.go`.
* **Details:**
  * Create a mock project in the test database.
  * *Test 1 (Success):* Insert an active session. Assert success.
  * *Test 2 (Rejection):* Attempt to insert a second active session for the same project. Assert that the returned error is exactly `domain.ErrActiveSessionExists`.
  * *Test 3 (Archiving bypass):* Archive the first session (status = 1), then attempt to insert a new active session. Assert success, proving the partial index works perfectly.

## Task 1.3.4: Orchestrator Logic for `/project-open`
* **Action:** Create `internal/orchestrator/session_manager.go`.
* **Details:**
  * Write an `OpenProject(projectID string)` function.
  * Call `FindActive`. If a session is returned, do *not* spawn a new one. Instead, return a structured intent (e.g., `SessionConflictIntent`) that tells the UIMediator to block and ask the user: *"Active session found. [Resume] or [Archive]?"*.
  * *(Note: Actual UI rendering is handled in EPIC-04, this task just returns the routing intent).*