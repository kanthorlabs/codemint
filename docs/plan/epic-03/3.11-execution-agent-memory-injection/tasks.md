# Tasks: 3.11 Execution Agent Memory Injection (Supports EPIC-05)

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/acp/`, `internal/xdg/`
**Tech Stack:** Go, XDG paths, Markdown
**Priority:** P2

---

## Task 3.11.1: Hot Memory Loader

* **Action:** Read the "Hot" wiki files from the per-project memory dir.
* **Details:**
  * Create `internal/acp/memory_loader.go`:
    ```go
    type HotMemory struct {
        Preferences string // patterns/preferences.md
        Decisions   string // architecture/decisions.md
        BugsIndex   string // patterns/bugs/index.md
    }
    func LoadHotMemory(projectID string) (HotMemory, error)
    ```
  * Resolve paths via existing `xdg` helpers: `~/.local/share/codemint/memory/<project_id>/...`.
  * Missing files are not errors — return empty strings.
  * Cap each file at 32 KiB; truncate with a `... [truncated]` marker if larger (cheap guardrail until Story 5.3 ships scoped injection).
* **Verification:**
  * Manual: drop a small `preferences.md`, run `LoadHotMemory(projectID)`, observe content.
  * Missing dir returns `(HotMemory{}, nil)`.

---

## Task 3.11.2: Compose System Prompt

* **Action:** Build the agent system prompt that enforces the Hierarchy of Authority.
* **Details:**
  * Create `internal/acp/system_prompt.go`:
    ```go
    func BuildSystemPrompt(mem HotMemory) string
    ```
  * Template:
    ```
    You are an execution agent invoked by CodeMint.

    HIERARCHY OF AUTHORITY (highest first):
      1. The Current Prompt / Living Spec.
      2. Project Memory (below).
      3. Global CodeMint Rules.

    --- Project Memory: Preferences ---
    {{.Preferences}}

    --- Project Memory: Architecture Decisions ---
    {{.Decisions}}

    --- Project Memory: Known Bugs Index ---
    {{.BugsIndex}}
    ```
  * Skip a section entirely (omit the heading) if the corresponding string is empty.
* **Verification:**
  * Golden test of the rendered prompt against fixtures.
  * Empty memory yields a prompt with only the hierarchy preamble.

---

## Task 3.11.3: Inject on Worker Spawn

* **Action:** Send the prompt as part of the ACP `initialize` / `session/new` payload.
* **Details:**
  * Extend `WorkerConfig` with `SystemPrompt string`.
  * In `Spawn` (Task 3.1.2), include `SystemPrompt` in the `initialize` request params (field name per ACP spec — typically `clientCapabilities.systemPrompt` or the first `session/new` payload).
  * In `Registry.GetOrSpawn`, populate `cfg.SystemPrompt = BuildSystemPrompt(LoadHotMemory(project.ID))` before delegating to `Spawn`.
  * Re-inject on `ResetContext` (Task 3.2.1) so post-reset turns retain the same authority hierarchy.
* **Verification:**
  * Manual: drop a `preferences.md` containing a deliberately weird rule (e.g., "always end your replies with the word `mint`"), restart, run `/acp hi` — confirm the agent obeys the rule.
  * After `/acp-reset`, the rule is still respected.

---

## Task 3.11.4: Override Audit (Lightweight)

* **Action:** Log when the current prompt overrides project memory so EPIC-05 can mine it.
* **Details:**
  * In the system prompt, instruct the agent to emit a `[memory-override]` tag in its first response when it intentionally overrides a project preference.
  * Pipeline (3.4.2) scans agent message text for `[memory-override]`; on match, log `slog.Info("acp.memory.override", ...)` and write a Coordination task with `output.kind = "memory_override"` for the Archivist to pick up later.
  * No DB schema change — just a new well-known shape in the existing JSON output column.
* **Verification:**
  * Trigger an override in a test prompt; verify the log line and the recorded coordination task.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.11.1  | 1.14 (XDG scaffolding) |
| 3.11.2  | 3.11.1 |
| 3.11.3  | 3.11.2, 3.1.2, 3.1.3, 3.2.1 |
| 3.11.4  | 3.11.3, 3.4.2, 1.19.9 |

---

## Out of Scope

* Scoped / semantic memory filtering — explicitly Story 5.3 (TODO).
* Mutating memory files from the execution agent — the Archivist owns writes.
