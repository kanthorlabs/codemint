# Tasks: 3.9 CUI Low-Bandwidth Pulse

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/ui/`
**Tech Stack:** Go
**Priority:** P1

---

## Task 3.9.1: Terminal-State Filter

* **Action:** Restrict the CUI adapter to status-change events only.
* **Details:**
  * Open `internal/ui/cui_adapter.go` (already a stub from Story 1.19.7).
  * Implement `NotifyEvent` to switch on `event.Type`:
    * Forward: `EventTaskStatusChanged` (when `To` ∈ {Awaiting, Success, Failure, Reverted}), `EventSessionTakeover`, `EventSessionReclaimed`, approval prompts (which arrive via `PromptDecision`).
    * Drop everything else, including `EventACPStream` micro-events.
  * Until a real Telegram/Slack transport lands, the adapter prints to a dedicated daemon log file `~/.local/state/codemint/cui.log` so the filtering can be exercised in `--mode=daemon`.
* **Verification:**
  * Run `--mode=daemon`, drive a long `/acp` prompt; tail the log — only state-change lines appear, no `thinking`/`tool_update` noise.

---

## Task 3.9.2: Pull Commands (`/tasks`, `/status`)

* **Action:** Provide the CUI's "maximalist pull" surface for daemon clients to query state on demand.
* **Details:**
  * Add `/tasks` REPL command: prints active session's tasks grouped by `(seq_epic, seq_story)` with status indicators (`P`/`R`/`A`/`S`/`F`/`C`).
  * Add `/status` REPL command: prints active task (if any), worker pid, and last-status timestamp.
  * Register with `SupportedModes: []ClientMode{ClientModeCLI, ClientModeDaemon}` so both interfaces benefit.
* **Verification:**
  * From a daemon REPL, `/tasks` returns the hierarchy text-only (no streaming).
  * `/status` reflects the current task accurately during an `/acp` run.

---

## Task 3.9.3: Approval Prompt Adapter Surface

* **Action:** Make the CUI handle `PromptDecision` calls inline so 3.6 prompts can resolve from a daemon.
* **Details:**
  * Update `CUIAdapter.PromptDecision`:
    * Print the prompt body and an enumerated option list to stdout (and the log).
    * Read a single line from a small in-process queue fed by `/approve <id> <choice>` REPL command (e.g., `/approve 1 allow_once`). Use a `sync.Map` keyed by prompt ID.
    * Return `registry.PromptResponse{OptionID: choice}` once a matching `/approve` arrives.
    * Honor `ctx.Done()` for timeout/cancel from 3.6.4.
  * Add the `/approve` and `/deny` REPL commands gated to daemon mode.
* **Verification:**
  * In daemon mode, trigger an unwhitelisted command: prompt prints with prompt ID; `/approve <id> allow_once` resolves the worker; `/deny <id>` triggers the deny path.

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.9.1 | 3.4.4, 3.7.4, 1.19.7 |
| 3.9.2 | 3.7.4, 1.11 (hierarchical schema) |
| 3.9.3 | 3.6.2, 1.13 |

---

## Out of Scope

* Real Telegram/Slack transports — EPIC-04 work; the in-process queue here is the minimum needed to test bandwidth filtering and approval routing.
