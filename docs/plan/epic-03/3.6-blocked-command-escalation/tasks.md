# Tasks: 3.6 Blocked Command Escalation

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`, `internal/ui/`
**Tech Stack:** Go, UIMediator, JSON-RPC
**Priority:** P0

---

## Task 3.6.1: Halt + Mark Task Awaiting

* **Action:** When the matcher returns `Block` or `Unknown`, freeze the worker turn and flip the active task to `TaskStatusAwaiting`.
* **Details:**
  * In `Interceptor.Handle`, after matcher decision in {Block, Unknown}:
    1. Do **not** send any response to the worker yet — its `session/request_permission` is left pending, naturally pausing the agent turn.
    2. `taskRepo.UpdateTaskStatus(ctx, currentTaskID, domain.TaskStatusAwaiting)`.
    3. Cache the pending permission request (`acpSessionID`, JSON-RPC `id`, command) in an `Interceptor.pending` map keyed by task ID.
  * If `currentTaskID` is empty (e.g., a `/acp` ad-hoc prompt), still pause and stash the pending request keyed by ACP session ID.
* **Verification:**
  * Trigger a non-whitelisted `tool_call`; DB shows task in `awaiting`, no response written to worker stdin yet (assert via stub worker).

---

## Task 3.6.2: Approval Prompt via UIMediator

* **Action:** Build the structured prompt the user sees and route it to all UIs.
* **Details:**
  * Define a `registry.PromptRequest` payload:
    ```go
    PromptKind: "acp_command_approval"
    Title:      "Allow command?"
    Body:       "<cmd>\nin <cwd>"
    Options:    [
        {ID: "allow_once",   Label: "Allow once"},
        {ID: "allow_session", Label: "Allow for this session"},
        {ID: "deny",         Label: "Deny"},
    ]
    ```
  * Call `mediator.PromptDecision(ctx, req)`; on first response, call `CancelPrompt` on remaining adapters (existing mediator behavior).
  * `allow_session` adds the command to an in-memory per-session whitelist used by the matcher (does **not** mutate `project_permission` — durable changes are user-driven).
* **Verification:**
  * TUI shows the prompt; choosing `Allow once` releases the agent.
  * Choosing `Allow for this session` prevents future prompts for the same command in the same process lifetime.

---

## Task 3.6.3: Reply to the Worker Based on User Choice

* **Action:** Translate the user's choice into the right ACP response and unblock the agent.
* **Details:**
  * On `allow_once` / `allow_session`: run the command via `LocalRunner` (3.5.2), then send the same allow response as 3.5.3.
  * On `deny`: respond to the pending `session/request_permission` with `outcome = "selected", optionId = "reject_once"` (or whatever the agent advertises in the request). The agent should then either choose another path or end the turn.
  * Always clear the entry from `Interceptor.pending`.
  * Transition the task back to `processing` once the worker is unblocked. If the agent ends the turn with an error after a deny, Story 3.7 will move it to `failure`.
* **Verification:**
  * Approve flow: status goes `processing → awaiting → processing` and the worker resumes.
  * Deny flow: agent receives the rejection and emits a follow-up message visible in the UI.

---

## Task 3.6.4: Timeout / Cancel Safety Net

* **Action:** Don't leave a worker hung forever if the user walks away.
* **Details:**
  * Add a configurable approval timeout (default 30 minutes, env `CODEMINT_ACP_APPROVAL_TIMEOUT`).
  * When the timeout fires:
    * Send `deny` response to the worker.
    * Update task status to `awaiting` → `failure` only if the agent itself errors out; otherwise keep `awaiting`.
    * Render a UI message: `"Approval timed out — denied automatically."`
  * Hook session shutdown / archive: cancel any pending approvals via `mediator.CancelPrompt`.
* **Verification:**
  * Manual: trigger prompt, wait past timeout, observe auto-deny.
  * `/session-archive` while a prompt is pending — prompt disappears, worker is killed.

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.6.1 | 3.4.3, 3.5.1 |
| 3.6.2 | 3.6.1, 1.10 (UIMediator), 1.13 (UIAdapter contract) |
| 3.6.3 | 3.6.2, 3.5.2 |
| 3.6.4 | 3.6.2, 3.3.1 |

---

## Out of Scope

* Persisting `allow_session` choices into `project_permission` (durable changes happen via a future settings UI).
* Cryptographic command signing.
