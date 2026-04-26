# Tasks: 3.8 TUI High-Bandwidth Streaming

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/ui/`, `internal/repl/`
**Tech Stack:** Go, line-based TTY rendering
**Priority:** P1

---

## Task 3.8.1: TUI Adapter ACP Subscription

* **Action:** Make the existing CLI/TUI adapter consume `EventACPStream` events.
* **Details:**
  * Locate the current TUI adapter (`internal/ui/cli_adapter.go` if it exists, else extend the `RenderMessage` writer wired in `mediator.go`).
  * Add a handler that switches on `acp.EventKind` and writes:
    * `EventThinking` → dim grey prefix `· thinking ·` + delta text.
    * `EventMessage` → plain text streamed character-by-character.
    * `EventToolUpdate` → `→ tool: <name> [<status>]`.
    * `EventPlan` → indented bullet list.
  * Coalesce consecutive thinking deltas into a single line until the kind changes (avoid one TTY write per token).
  * Respect verbosity (Task 3.8.3) — at level 1+ skip thinking.
* **Verification:**
  * `/acp say a long story about cats` produces visibly streaming output, not one big block at the end.
  * Reasoning chunks appear inline under a single `· thinking ·` marker.

---

## Task 3.8.2: Backpressure-Safe Renderer

* **Action:** Don't let a chatty agent freeze the REPL prompt.
* **Details:**
  * The adapter consumes events from a per-adapter buffered channel (cap 256). On overflow, drop the oldest non-state event and increment a `metrics.acp_dropped_events` counter (slog).
  * Render writes go through a single goroutine that owns `os.Stdout` to avoid interleaving with REPL prompt redraws.
  * When the user is mid-input (REPL has stdin focus), buffer renders until the next newline boundary so the prompt does not shred.
* **Verification:**
  * Stress test: stub agent emits 10k thinking chunks; REPL stays responsive and counter reflects any drops.

---

## Task 3.8.3: Verbosity Filter Hookup

* **Action:** Honor the global `/verbosity <level>` setting when streaming.
* **Details:**
  * Add `/verbosity` REPL command (Story 4.x scope, but implement the minimum here so tests work):
    * `level=0 (Task)`: emit everything (default for TUI).
    * `level=1 (Story)`: drop thinking, drop tool_update, keep tool_call announcements + final messages.
    * `level=2 (Epic)`: drop everything except `EventTurnEnd` and `EventTaskStatusChanged`.
  * Persist to `ActiveSession.Verbosity` (memory only is fine for this story).
  * Adapter consults the verbosity level on every event; no rebuild required.
* **Verification:**
  * `/verbosity 1` then `/acp explain Go channels` — no `· thinking ·` lines.
  * `/verbosity 2` — only the final summary line + status changes appear.

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.8.1 | 3.4.4 |
| 3.8.2 | 3.8.1 |
| 3.8.3 | 3.8.1 |

---

## Out of Scope

* Bubble Tea / rich layout — plain stdin/stdout is fine for this story.
* CUI behaviour (handled in 3.9).
