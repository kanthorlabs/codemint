# Tasks: 1.1 SQLite Schema Initialization

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.1-sqlite-schema-initialization/`
**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), `jmoiron/sqlx`, `pressly/goose`

---

## Task 1.1.1: Define Go Domain Entities & Enums
* **Action:** Create `internal/domain/models.go`.
* **Details:**
  * Define the 6 core Go structs: `Project`, `ProjectPermission`, `Agent`, `Session`, `Workflow`, `Task`.
  * Apply explicit struct tags for `sqlx` mapping (e.g., `` db:"project_id" ``).
  * Use `json.RawMessage` or `string` for JSON payload columns (`input`, `output`, `allowed_commands`, etc.).
  * Define strongly-typed enums using Go `iota` constants for:
    * `TaskType` (Coding=0, Verification=1, Confirmation=2, Coordination=3)
    * `TaskStatus` (Pending=0, Processing=1, Awaiting=2, Success=3, Failure=4, Completed=5, Reverted=6, Cancelled=7)
    * `AgentType` (Human=0, Assistant=1, System=2)
    * `SessionStatus` (Active=0, Archived=1)

## Task 1.1.2: Build the SQLite Migrator & Initializer
* **Action:** Create `internal/db/database.go`.
* **Details:**
  * Write `InitDB(dbPath string) (*sqlx.DB, error)`.
  * Import and configure `modernc.org/sqlite` as the pure-Go SQLite driver.
  * Use `//go:embed migrations/*.sql` to embed the migration files into an `embed.FS` filesystem variable.
  * Initialize `pressly/goose` using the embedded filesystem (`goose.SetBaseFS`).
  * Inside `InitDB`, ensure the parent directory of `dbPath` exists, open the database connection with `sqlx.Connect()`, and run `goose.Up()` programmatically to apply pending migrations automatically on application startup.

## Task 1.1.3: Write the Migration Unit Test
* **Action:** Create `internal/db/database_test.go`.
* **Details:**
  * Write a unit test invoking `InitDB("file::memory:?cache=shared")`.
  * Assert that the function returns a valid `*sqlx.DB` instance without errors.
  * Assert that running the migrations up executes successfully.
  * Verify the tables exist by querying the `sqlite_master` table.
  * Call `goose.Down()` programmatically to ensure the rollback script executes flawlessly without foreign key errors.