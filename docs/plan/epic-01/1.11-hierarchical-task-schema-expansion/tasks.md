# Tasks: 1.11 Hierarchical Task Schema Expansion

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.11-hierarchical-task-schema-expansion/`
**Tech Stack:** Go, SQLite, `sqlx`

---

## Task 1.11.1: Add Sequential Ordering Columns to Task Schema
* **Action:** Update `internal/db/migrations/000001_init_schema.sql`.
* **Details:**
  * Add `seq_epic INTEGER NOT NULL DEFAULT 0` to the `task` table.
  * Add `seq_story INTEGER NOT NULL DEFAULT 0` to the `task` table.
  * Add `seq_task INTEGER NOT NULL DEFAULT 0` to the `task` table.
  * These columns enable Brainstormer to map tasks to a strict linear execution graph.
* **Status:** ✅ Implemented (lines 52-55)

## Task 1.11.2: Extend Domain Task Struct with Sequence Fields
* **Action:** Update `internal/domain/core.go`.
* **Details:**
  * Add `SeqEpic int` with tag `` `db:"seq_epic"` `` to `Task` struct.
  * Add `SeqStory int` with tag `` `db:"seq_story"` `` to `Task` struct.
  * Add `SeqTask int` with tag `` `db:"seq_task"` `` to `Task` struct.
* **Status:** ✅ Implemented (lines 112-114)

## Task 1.11.3: Update Repository Queries to Order by Sequence
* **Action:** Update `internal/repository/sqlite/task_repo.go`.
* **Details:**
  * `Next()` query must include `ORDER BY seq_epic ASC, seq_story ASC, seq_task ASC`.
  * `FindInterrupted()` query must include same ordering for consistent traversal.
  * All SELECT queries returning tasks must include the three sequence columns.
* **Status:** ✅ Implemented
  * `Next()` - line 69
  * `FindInterrupted()` - line 204

## Task 1.11.4: Update TaskRepository Interface Documentation
* **Action:** Update `internal/repository/task_repo.go`.
* **Details:**
  * Document that `Next()` returns tasks ordered by `(seq_epic, seq_story, seq_task) ASC`.
  * Clarify that only Pending (0) or Awaiting (2) tasks are considered actionable.
* **Status:** ✅ Implemented (lines 13-17)

## Task 1.11.5: Verify Ordering in Unit Tests
* **Action:** Update `internal/repository/sqlite/task_repo_test.go`.
* **Details:**
  * Add test case inserting multiple tasks with varying sequence values.
  * Assert `Next()` returns task with lowest `(seq_epic, seq_story, seq_task)` tuple.
  * Assert ordering holds when tasks span different epics/stories.
