# Tasks: 1.4 The `human` Agent Seed

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.4-the-human-agent-seed/`
**Tech Stack:** Go, `jmoiron/sqlx`

---

## Task 1.4.1: Define Agent Repository Interface
* **Action:** Create `internal/repository/agent_repo.go`.
* **Details:**
  * Following *Effective Go*, define the interface:
    ```go
    type AgentRepository interface {
        EnsureSystemAgents(ctx context.Context) error
        FindByName(ctx context.Context, name string) (*domain.Agent, error)
    }
    ```

## Task 1.4.2: Implement Idempotent Seeding
* **Action:** Create `internal/repository/sqlite/agent_repo.go`.
* **Details:**
  * Implement `EnsureSystemAgents`. 
  * It should perform an `INSERT OR IGNORE` into the `agent` table for the name `human`.
  * Use `idgen.MustNew()` for the ID and set `type` to `0` (Human).
  * *Bonus:* Also seed the `sys-auto-approve` agent (Type 2) here to satisfy the requirement in EPIC-02 early.

## Task 1.4.3: Hook Seeding into Database Initialization
* **Action:** Update `internal/db/database.go`.
* **Details:**
  * Update `InitDB` to accept the `AgentRepository`.
  * After `goose.Up()` completes successfully, call `agentRepo.EnsureSystemAgents(ctx)`.
  * This ensures that by the time `InitDB` returns, the database is not just structured, but also populated with the necessary actors.

## Task 1.4.4: Write Seeding Unit Test
* **Action:** Create `internal/repository/sqlite/agent_repo_test.go`.
* **Details:**
  * Initialize an in-memory database.
  * Call `EnsureSystemAgents`. 
  * Query the table and assert that two agents (`human` and `sys-auto-approve`) exist with the correct types.
  * Call `EnsureSystemAgents` a second time and assert that no duplicate key errors occur and the count remains at 2.