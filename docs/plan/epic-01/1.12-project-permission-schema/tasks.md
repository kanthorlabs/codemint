# Tasks: 1.12 Project Permission Schema

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.12-project-permission-schema/`
**Tech Stack:** Go, SQLite, `sqlx`, `json.RawMessage`

---

## Task 1.12.1: Create Project Permission Table in Schema
* **Action:** Update `internal/db/migrations/000001_init_schema.sql`.
* **Details:**
  * Create `project_permission` table with:
    * `id TEXT PRIMARY KEY` - UUIDv7
    * `project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE`
    * `allowed_commands TEXT` - JSON array of whitelisted commands
    * `allowed_directories TEXT` - JSON array of safe paths
    * `blocked_commands TEXT` - JSON array of strictly forbidden commands
  * CASCADE delete ensures permissions are removed when project is deleted.
* **Status:** ✅ Implemented (000001_init_schema.sql lines 11-17)

## Task 1.12.2: Define ProjectPermission Domain Entity
* **Action:** Update `internal/domain/core.go`.
* **Details:**
  * Create `ProjectPermission` struct with fields:
    * `ID string` with tag `` `db:"id"` ``
    * `ProjectID string` with tag `` `db:"project_id"` ``
    * `AllowedCommands json.RawMessage` with tag `` `db:"allowed_commands"` ``
    * `AllowedDirectories json.RawMessage` with tag `` `db:"allowed_directories"` ``
    * `BlockedCommands json.RawMessage` with tag `` `db:"blocked_commands"` ``
  * Use `json.RawMessage` for deferred JSON parsing by consumers.
* **Status:** ✅ Implemented (core.go lines 68-75)

## Task 1.12.3: Create Factory Constructor for ProjectPermission
* **Action:** Update `internal/domain/core.go`.
* **Details:**
  * Add `NewProjectPermission(projectID string) *ProjectPermission` factory.
  * Generate UUIDv7 for `ID` using `idgen.MustNew()`.
  * Initialize JSON fields as `nil` (empty/unset).
* **Status:** ✅ Implemented (core.go lines 138-143)

## Task 1.12.4: Create ProjectPermissionRepository Interface
* **Action:** Create `internal/repository/project_permission_repo.go`.
* **Details:**
  * Define `ProjectPermissionRepository` interface with methods:
    * `FindByProjectID(ctx, projectID) (*ProjectPermission, error)`
    * `Upsert(ctx, perm *ProjectPermission) error`
  * Upsert enables idempotent permission updates.

## Task 1.12.5: Implement SQLite ProjectPermissionRepository
* **Action:** Create `internal/repository/sqlite/project_permission_repo.go`.
* **Details:**
  * Implement `FindByProjectID` with single-row SELECT.
  * Implement `Upsert` using `INSERT ... ON CONFLICT(project_id) DO UPDATE`.
  * Return `nil, nil` when no permission record exists (permissive default).

## Task 1.12.6: Write Repository Unit Tests
* **Action:** Create `internal/repository/sqlite/project_permission_repo_test.go`.
* **Details:**
  * Test `FindByProjectID` returns nil for non-existent project.
  * Test `Upsert` creates new permission record.
  * Test `Upsert` updates existing record (idempotency).
  * Test CASCADE delete removes permission when project deleted.
