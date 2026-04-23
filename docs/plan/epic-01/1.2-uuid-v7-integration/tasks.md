# Tasks: 1.2 UUID v7 Integration

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.2-uuid-v7-integration/`
**Tech Stack:** Go, `github.com/google/uuid`

---

## Task 1.2.1: Setup UUID Generator Utility
* **Action:** Create `internal/util/idgen/uuid.go`.
* **Details:** * Run `go get github.com/google/uuid`.
  * Create a wrapper function `func New() (string, error)` (or panic-safe `MustNew() string`).
  * The function must explicitly call `uuid.NewV7()` and return its string representation. 
  * *Why a wrapper?* If we ever need to swap the UUID library, change the encoding (e.g., to Base58 for shorter CLI strings), or mock IDs for testing, we only have to change it in this one utility package.

## Task 1.2.2: Write Lexicographical Sorting Tests
* **Action:** Create `internal/util/idgen/uuid_test.go`.
* **Details:**
  * Write a unit test `TestUUIDv7_Version` that parses the generated string and strictly asserts `version == 7`.
  * Write a unit test `TestUUIDv7_Sortable` that generates a UUID, applies a small `time.Sleep()`, generates a second UUID, and asserts that `id1 < id2` via standard string comparison. This proves our SQLite B-Tree insertions will remain sequential.

## Task 1.2.3: Integrate into Domain Constructors (Factories)
* **Action:** Update `internal/domain/models.go` (created in 1.1).
* **Details:**
  * Since we shouldn't rely on the database or the UI to generate IDs, we need factory functions for our entities.
  * Implement constructor functions like `NewProject(name, workingDir string) *Project`, `NewSession(projectID string) *Session`, and `NewTask(...) *Task`.
  * Inside these constructors, automatically invoke `idgen.MustNew()` and assign it to the `ID` field of the struct before returning it.