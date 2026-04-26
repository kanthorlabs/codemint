# Tasks: 1.16 Core Repository Layer Completion

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.16-core-repository-completion/`
**Tech Stack:** Go, SQLite, `sqlx`
**Priority:** P1 (Important - Blocks project/session lifecycle)

---

## Task 1.16.1: Define ProjectRepository Interface
* **Action:** Create `internal/repository/project_repo.go`.
* **Details:**
  * Define `ProjectRepository` interface:
    ```go
    type ProjectRepository interface {
        Create(ctx context.Context, p *domain.Project) error
        FindByID(ctx context.Context, id string) (*domain.Project, error)
        FindByName(ctx context.Context, name string) (*domain.Project, error)
        Update(ctx context.Context, p *domain.Project) error
        Delete(ctx context.Context, id string) error
        List(ctx context.Context) ([]*domain.Project, error)
    }
    ```
  * Document that `Delete` cascades to sessions, tasks, and permissions.
* **Verification:**
  * Interface compiles without errors.
  * Godoc renders method documentation.

## Task 1.16.2: Implement SQLite ProjectRepository
* **Action:** Create `internal/repository/sqlite/project_repo.go`.
* **Details:**
  * Implement all `ProjectRepository` methods.
  * `Create`: INSERT with conflict check on `name` unique constraint.
  * `FindByName`: SELECT WHERE name = ? (case-sensitive).
  * `Update`: UPDATE SET name = ?, working_dir = ?, yolo_mode = ? WHERE id = ?.
  * `Delete`: DELETE WHERE id = ? (CASCADE handles children).
  * Return wrapped errors with context (e.g., `sqlite: project %q not found`).
* **Verification:**
  * Unit test: Create → FindByID → returns same data.
  * Unit test: Create duplicate name → returns error.
  * Unit test: Delete → FindByID → returns not found.

## Task 1.16.3: Define SessionRepository Interface
* **Action:** Create `internal/repository/session_repo.go`.
* **Details:**
  * Define `SessionRepository` interface:
    ```go
    type SessionRepository interface {
        Create(ctx context.Context, s *domain.Session) error
        FindByID(ctx context.Context, id string) (*domain.Session, error)
        FindActiveByProjectID(ctx context.Context, projectID string) (*domain.Session, error)
        Archive(ctx context.Context, id string) error
        ListByProjectID(ctx context.Context, projectID string) ([]*domain.Session, error)
    }
    ```
  * `FindActiveByProjectID` returns nil, nil if no active session exists.
  * `Archive` sets status = 1 (Archived).
* **Verification:**
  * Interface compiles without errors.

## Task 1.16.4: Implement SQLite SessionRepository
* **Action:** Create `internal/repository/sqlite/session_repo.go`.
* **Details:**
  * `Create`: INSERT; if active session exists for project, partial index rejects.
  * `FindActiveByProjectID`: SELECT WHERE project_id = ? AND status = 0.
  * `Archive`: UPDATE SET status = 1 WHERE id = ? AND status = 0.
  * Handle unique constraint violation with descriptive error.
* **Edge case:** `Create` when active session exists should return `ErrActiveSessionExists`.
* **Verification:**
  * Unit test: Create active session → Create another → returns constraint error.
  * Unit test: Archive → Create new → succeeds.
  * Unit test: FindActiveByProjectID with no active → returns nil, nil.

## Task 1.16.5: Implement Agent Seeding on Database Init
* **Action:** Update `internal/db/database.go` or create `internal/db/seed.go`.
* **Details:**
  * After migrations, check if `human` agent exists.
  * If not, insert seed agents:
    ```go
    seeds := []*domain.Agent{
        domain.NewAgent("human", domain.AgentTypeHuman, ""),
        domain.NewAgent("sys-auto-approve", domain.AgentTypeSystem, ""),
    }
    ```
  * Use INSERT OR IGNORE to make seeding idempotent.
  * Log which agents were seeded at INFO level.
* **Verification:**
  * Fresh database: `SELECT * FROM agent` returns 2 rows.
  * Re-run init: no duplicate rows, no errors.
  * `human` agent has type = 0, `sys-auto-approve` has type = 2.

## Task 1.16.6: Update AgentRepository with FindByName
* **Action:** Update `internal/repository/agent_repo.go` and SQLite implementation.
* **Details:**
  * Add `FindByName(ctx context.Context, name string) (*domain.Agent, error)` if missing.
  * Executor.handleCrash uses this to find `human` agent.
  * Return nil, nil if agent not found (not an error).
* **Status:** ⚠️ May already exist - verify current implementation.
* **Verification:**
  * `FindByName("human")` returns seeded human agent.
  * `FindByName("nonexistent")` returns nil, nil.

## Task 1.16.7: Add ErrActiveSessionExists Sentinel
* **Action:** Update `internal/repository/session_repo.go` or SQLite impl.
* **Details:**
  * Define `var ErrActiveSessionExists = errors.New("repository: active session already exists")`.
  * SQLite impl detects UNIQUE constraint violation on partial index.
  * Wrap raw SQLite error with sentinel for clean handling upstream.
* **Verification:**
  * Create active session twice → `errors.Is(err, ErrActiveSessionExists)` is true.

## Task 1.16.8: Write Comprehensive Repository Tests
* **Action:** Create/update test files for each repository.
* **Details:**
  * `internal/repository/sqlite/project_repo_test.go`:
    * TestCreate, TestFindByID, TestFindByName, TestUpdate, TestDelete, TestList
    * TestCreateDuplicateName
  * `internal/repository/sqlite/session_repo_test.go`:
    * TestCreate, TestFindByID, TestFindActiveByProjectID, TestArchive
    * TestCreateWhileActiveExists
    * TestArchiveThenCreateNew
  * Use `t.Parallel()` where safe.
  * Use `t.TempDir()` for isolated in-memory or file DBs.
* **Verification:**
  * `go test ./internal/repository/sqlite/... -v` all pass.
  * Coverage > 80% for new code.

## Task 1.16.9: Document Repository Error Handling Conventions
* **Action:** Add comments to `internal/repository/` package doc.
* **Details:**
  * Document that `Find*` methods return `(nil, nil)` when not found (not an error).
  * Document that `Create` methods return wrapped errors with entity context.
  * Document that sentinel errors (`ErrActiveSessionExists`, `ErrInvalidTransition`) should be checked with `errors.Is`.
* **Verification:**
  * `go doc codemint.kanthorlabs.com/internal/repository` shows conventions.
