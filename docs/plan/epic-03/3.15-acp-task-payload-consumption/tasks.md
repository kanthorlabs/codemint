# Tasks: 3.15 ACP-Compliant Task Payload Consumption

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/domain/`, `internal/orchestrator/`, `internal/acp/`
**Tech Stack:** Go, `encoding/json`, `filepath`
**Priority:** P0 (3.14.2 cannot send a real prompt without this)

---

## Task 3.15.1: `domain.TaskInput` Schema

* **Action:** Codify the JSON schema EPIC-02 promised in §2.13.
* **Details:**
  * Create `internal/domain/task_input.go`:
    ```go
    type TaskInput struct {
        Prompt       string            `json:"prompt"`
        ContextFiles []string          `json:"context_files,omitempty"`
        Tools        []string          `json:"tools,omitempty"`
        Command      string            `json:"command,omitempty"` // verification only
        Cwd          string            `json:"cwd,omitempty"`     // verification only
        Metadata     map[string]string `json:"metadata,omitempty"`
    }
    func ParseTaskInput(raw string) (*TaskInput, error)
    ```
  * `ParseTaskInput`:
    * Empty string → return `nil, ErrEmptyInput`.
    * Valid JSON object → unmarshal.
    * Anything else (legacy raw text) → return `&TaskInput{Prompt: raw}, ErrLegacyText` (the executor decides whether to log).
* **Verification:**
  * `TestParseTaskInput_RoundTrip` — marshal/unmarshal preserves all fields.
  * `TestParseTaskInput_LegacyFallback` — plain string returns the wrapped struct and the legacy sentinel.

---

## Task 3.15.2: Path Resolution for `context_files`

* **Action:** Resolve relative paths under the project working directory and reject escapes.
* **Details:**
  * Helper in `internal/orchestrator/executor.go`:
    ```go
    func resolveContextFiles(project *domain.Project, files []string) ([]string, error)
    ```
  * For each entry:
    * `filepath.Clean` → reject if resulting path starts with `..` or is absolute.
    * Prefix with `project.WorkingDir`.
    * `os.Stat` to confirm it exists; missing files fail the task with `reason=context_file_missing` (per-file).
* **Verification:**
  * `TestResolveContextFiles_RejectsEscape` — `["../../../etc/passwd"]` returns error.
  * `TestResolveContextFiles_AbsolutePathRejected` — `/etc/hosts` returns error.

---

## Task 3.15.3: Build `SessionPromptParams`

* **Action:** Translate `TaskInput` into the wire format.
* **Details:**
  * Extend `internal/acp/protocol.go`:
    ```go
    type SessionPromptParams struct {
        SessionID string             `json:"sessionId"`
        Prompt    string             `json:"prompt"`
        Context   []PromptContextRef `json:"context,omitempty"`
        Tools     []string           `json:"tools,omitempty"`
    }
    type PromptContextRef struct {
        Path string `json:"path"`
        Kind string `json:"kind"` // "file" for now
    }
    ```
  * In `executor.executeCoding`:
    1. `input, err := domain.ParseTaskInput(task.Input.String)`.
    2. On `ErrLegacyText`, `slog.Warn("executor: legacy plain-text task.input", "task", task.ID)` and continue.
    3. `paths, err := resolveContextFiles(project, input.ContextFiles)`.
    4. Construct `SessionPromptParams` with the resolved paths.
* **Verification:**
  * `TestExecutor_Coding_BuildsTypedParams` — fake task with two context files; assert outgoing JSON contains both with absolute paths.

---

## Task 3.15.4: Failure Mapping for Invalid Input

* **Action:** Mark the task `failure` with a structured reason instead of crashing the scheduler.
* **Details:**
  * On any error from `ParseTaskInput` or `resolveContextFiles`:
    * Set `task.Status = TaskStatusFailed`.
    * Write `task.Output = {"error": "<sentinel>", "detail": "<message>"}`.
    * Return nil to the scheduler (this is a per-task failure, not a loop-killer).
  * Sentinels: `invalid_input`, `context_file_missing`, `path_escape`.
* **Verification:**
  * `TestExecutor_InvalidInput_DoesNotKillScheduler` — feed three tasks (good, bad, good); scheduler dispatches all three; bad task ends `failure` with sentinel.

---

## Task 3.15.5: Task Generator Contract Test

* **Action:** Lock the EPIC-02 ↔ EPIC-03 contract with a fixture.
* **Details:**
  * Add `internal/domain/testdata/task_input_v1.json` containing the exact shape the Task Generator agent emits (use the prompt template from EPIC-02 §2.13).
  * `TestTaskInput_Fixture_Compatible` deserializes the fixture and confirms every field is recognized — guards against silent schema drift.
* **Verification:**
  * Fixture parses cleanly; if EPIC-02 changes the template, the test fails loudly.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.15.1  | (none) |
| 3.15.2  | 3.15.1 |
| 3.15.3  | 3.15.1, 3.15.2, 3.1.1 (protocol types) |
| 3.15.4  | 3.15.1, 3.14.1 |
| 3.15.5  | 3.15.1 + EPIC-02 §2.13 prompt finalization |

---

## Out of Scope

* The Task Generator's prompt engineering (lives in EPIC-02 §2.13).
* Streaming context (chunking large files) — future iteration.
