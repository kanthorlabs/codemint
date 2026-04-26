# Tasks: 3.5 Whitelisted Command Execution

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`
**Tech Stack:** Go, `os/exec`, shell-safe matching
**Priority:** P0 (paired with 3.6)

---

## Task 3.5.1: Permission Matcher

* **Action:** Decide whether an intercepted command is allowed, denied, or unknown.
* **Details:**
  * Create `internal/orchestrator/permission_matcher.go`:
    ```go
    type Decision int
    const (
        DecisionUnknown Decision = iota
        DecisionAllow
        DecisionBlock
    )
    type Matcher struct{ perm *domain.ProjectPermission }
    func (m *Matcher) Evaluate(command, cwd string) Decision
    ```
  * Logic, in order:
    1. If `command` matches any prefix in `BlockedCommands` → `DecisionBlock`.
    2. If `command` matches any prefix in `AllowedCommands` AND `cwd` is inside any `AllowedDirectories` (filepath.Rel + `..` check) → `DecisionAllow`.
    3. Otherwise → `DecisionUnknown`.
  * Use `github.com/google/shlex` (already in go.mod) to tokenize the command before prefix-matching so `go test ./...` matches `go test`.
  * Unit-test edge cases: relative cwd, command with shell metacharacters, allowed dir but blocked command (block wins).
* **Verification:**
  * Table-driven tests cover allow / block / unknown; precedence verified.

---

## Task 3.5.2: Local Command Executor

* **Action:** Run an allowed command inside the project workingDir and capture its output.
* **Details:**
  * Create `internal/orchestrator/local_runner.go`:
    ```go
    type RunResult struct {
        ExitCode int
        Stdout   string
        Stderr   string
        Duration time.Duration
        Killed   bool
    }
    func (r *LocalRunner) Run(ctx context.Context, cmdline, cwd string, timeout time.Duration) (RunResult, error)
    ```
  * Implementation: tokenize via `shlex`, `exec.CommandContext`, `Dir = cwd`, capture stdout/stderr to bounded buffers (cap 64 KiB each, truncate with a `[truncated]` marker).
  * Default timeout 60s; surface as a `LocalRunner` field so callers can override.
  * Never expand env vars or run via a shell — pass argv directly.
* **Verification:**
  * `TestLocalRunner_Captures` runs `echo hello` and asserts stdout.
  * `TestLocalRunner_Timeout` runs `sleep 5` with 200ms timeout; asserts `Killed=true`.

---

## Task 3.5.3: Wire Allow Path Into Interceptor

* **Action:** Extend `orchestrator.Interceptor.Handle` to execute the command when matcher returns `DecisionAllow`.
* **Details:**
  * Order of operations on `EventToolCall` / `EventPermissionRequest` with `Decision = Allow`:
    1. Run the command via `LocalRunner`.
    2. Build an ACP `session/request_permission` response (or the agent-specific tool result) with `outcome = "selected"` and the chosen option ID `"allow_once"` (per ACP spec). Include captured stdout/stderr in the response payload if the agent supports it; otherwise reply via `tool_call_update` injection on stdin.
    3. `worker.Send(response)`.
    4. Leave the parent CodeMint task in `TaskStatusProcessing` — do not transition.
  * Stream a low-volume notification to the UI (`registry.EventACPAutoApproved`) with command + duration, so users can audit what fired silently.
* **Verification:**
  * Manual test with a project_permission row allowing `go test`: the agent's `go test` calls run automatically and the agent receives the output without the user being prompted.
  * Task status remains `processing` throughout.

---

## Task 3.5.4: Audit Trail

* **Action:** Persist every auto-approved execution for later inspection.
* **Details:**
  * After each successful auto-run, insert a Coordination task (`type=3`, status=5) with:
    * `assignee_id` = `sys-auto-approve` agent (seeded by Story 2.9; if it does not yet exist, fall back to `human` and TODO-tag the row).
    * `input` = `{"intercepted":true,"command":"...","cwd":"..."}`.
    * `output` = `{"exit":0,"stdout":"...","stderr":"...","duration_ms":...}` (truncated).
  * Reuse the existing `interactionRecorder` API where practical.
* **Verification:**
  * `sqlite3 ... "SELECT json_extract(input,'$.command') FROM task WHERE type=3 AND assignee_id=<sys-auto-approve-id>"` lists fired commands.
  * `/activity` shows the entries.

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.5.1 | 1.12 (project_permission schema) |
| 3.5.2 | None |
| 3.5.3 | 3.5.1, 3.5.2, 3.4.3 |
| 3.5.4 | 3.5.3, 2.9 (sys-auto-approve seed) |

---

## Out of Scope

* Streaming command output back to the agent in chunks (current impl waits for completion).
* Per-task command rate limiting.
