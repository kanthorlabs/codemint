# EPIC-02 Appending Tasks

Cross-cutting work that doesn't belong to one user story but applies across EPIC-02. Pick up alongside the story spine in `epic-02.md`.

**Authoritative ACP spec:** `https://agentclientprotocol.com/protocol/schema.md`. Mirrored at `docs/coding/agent-client-protocol.md`. The OpenAPI URL is 404 — do not use it.

---

## Status snapshot (2026-04-28, post-redesign)

| Story | State |
|---|---|
| 2.0 CodeMint project & session bootstrap | **Done** |
| 2.0.1 Workflow File Infrastructure | **Done** |
| 2.0.2 Task Routing & Conditional Execution | **Done** |
| 2.0.3 Workflow Execution State | **Done** |
| 2.0.4 Workflow Command | **Done** |
| 2.0.5 Skill Injection | **Done** |
| 2.1 Project Overview | **Not started** |
| 2.2 Goal Capture | **Not started** |
| 2.3 Goal-scoped Reality | **Not started** |
| 2.4 Options + Confirm Loop | **Not started** |
| 2.5 Plan Generation | **Not started** |
| 2.6 Verification Guardrail | **Not started** |
| 2.7 Confirmation Guardrail | **Not started** |
| 2.8 Error Escalation | **Not started** |
| 2.9 YOLO agent seed | **Done** |
| 2.10 YOLO Delegation | **Not started** |

---

## Cross-cutting concern A — Verbosity respect (P1)

Every story in 2.1–2.8 produces user-visible progress output. All of it must be filtered through a single verbosity level.

**Levels (proposed; concrete enum lives in a future `/verbosity` command story, not in EPIC-02):**

| Level | What surfaces |
|---|---|
| `quiet`   | Errors only. Phase transitions silent. |
| `normal`  | One line per phase entry/exit. Errors. Confirmation prompts. |
| `verbose` | Adds skill-level intermediate messages, tool calls, file reads. |
| `debug`   | Adds raw ACP `session/update` events. |

**What to do per story.** Every render path that today writes directly to `mediator.RenderMessage` must accept a verbosity threshold and skip below-threshold messages. There is no separate verbosity story in the spine — each story owns its own rendering decisions, and all of them honor the level set by `/verbosity` (TBD command).

**Default:** `normal`. Persisted on the session row when the command lands.

**Verification.** Manual: run a workflow at each level; lower levels strictly subset higher levels.

---

## Cross-cutting concern B — Error escalation contract (P0)

Specified in detail by **Story 2.8 Error Escalation**. Summary of the cross-cutting contract every other story must respect:

1. Setting `task.status = Failure` is the **only** failure signal. Don't return errors out of skill handlers; convert to task failure.
2. The scheduler observes `Failure`, reassigns the task to the session's Human Agent, transitions session to `Awaiting`, and halts.
3. Stories 2.6 and 2.7 already produce Failure on their own logic (test exit-code, user reject). Story 2.5 may produce Failure if plan JSON is malformed. Stories 2.1–2.4 may produce Failure on skill output validation errors.
4. Resolution flows back through the `/resolve` command (defined in 2.8). No story should add its own retry / skip / abort UI.

---

## Task A — ACP schema audit & fix (P0, blocks every story below)

Story 2.0 surfaced four wire-level bugs at integration time (`protocolVersion`, `cwd`, `mcpServers`, `[]ContentBlock` prompt). A side-by-side read of `internal/acp/protocol.go` against `protocol/schema.md` shows that more drift remains. Stricter agents (Claude Code, Codex) will reject the handshake; OpenCode tolerates us today by accident.

**What to do.** Walk every type in `internal/acp/protocol.go` against the spec. Reconcile JSON keys, required fields, enum values, and union variants. Known offenders to start from (re-derive the full list from the spec — do not trust this list to be complete):

- `InitializeParams`: rename `capabilities` → `clientCapabilities`; drop `workingDir` (not in spec).
- `InitializeResult`: rename `serverInfo` → `agentInfo`, `capabilities` → `agentCapabilities`; add required `protocolVersion`; add `authMethods`.
- `Caps`/`ServerCaps`: replace flat booleans with the nested `fs / terminal / mcpCapabilities / promptCapabilities / sessionCapabilities` structures.
- `SessionNewParams`: drop `sessionId` and `systemPrompt` (not in spec — system prompt must travel as the first `ContentBlock` of `session/prompt`, or via MCP).
- `SessionPromptParams`: drop `context` and `tools` (not in spec); context goes through `resource_link` / `resource` ContentBlocks or via MCP.
- `SessionPromptResult`: replace `success` with `stopReason: StopReason`.
- `ContentBlock`: typed discriminated union for `text|image|audio|resource|resource_link` (today only `text` round-trips).
- `PermissionRequest` / `PermissionResponse`: rewrite to spec shapes (`{sessionId, toolCall: ToolCallUpdate, options: PermissionOption[]}` and `{outcome: cancelled | {outcome:"selected", optionId}}`).
- `PermissionOption.kind` enum: `allow_once | allow_always | reject_once | reject_always`.
- `UpdateKind` constants: re-derive from the spec union (`ContentChunk`, `ToolCallUpdate`, `Plan`, `AvailableCommandsUpdate`, `ConfigOptionUpdate`, `CurrentModeUpdate`).
- ACP error codes: `-32000 auth_required`, `-32002 resource not found` — define and surface.

**How to verify.**
1. Add round-trip unit tests in `internal/acp/protocol_test.go`.
2. Add golden fixtures: hand-author one canonical JSON-RPC frame per method using snippets copied verbatim from `protocol/schema.md`.
3. `make test ./...` green after every call site is updated.

**Success state.** Every JSON key, required field, and enum value in `internal/acp/protocol.go` has a 1:1 match in `protocol/schema.md`. No invented fields remain. Golden fixtures pass.

---

## Task B — ACP feature coverage map (P1)

The team needs a single page that says "yes / partial / no" for every protocol feature so future story authors don't have to re-read the spec.

**What to do.** Produce `docs/coding/acp-coverage.md` with one row per spec page (Initialization, Session Setup, Prompt Turn, Tool Calls, Permissions, Slash Commands, Terminals, File System, Session List/Resume/Close, Session Modes, Session Config Options, Agent Plan, Content, Extensibility). Mark each **Have / Partial / Missing / Out-of-scope** with a one-line note and a link to the implementing file (or to the EPIC-02 story that will add it).

**Success state.** `acp-coverage.md` exists. Every spec page has a row. Every Have/Partial row links to a file path. Every Missing row links to a story (or notes "no story yet").

---

## Task C — ACP wire conformance harness (P1, depends on A)

Schema fixes (Task A) protect against type drift. They do not protect against semantic regressions. We need a harness that pokes a real agent.

**What to do.** Add `make acp-conform`. It boots a real ACP agent (start with OpenCode), runs the happy path `initialize → session/new → session/prompt → session/cancel`, and asserts:

1. `initialize` is accepted (no `-32600`, no `-32602`).
2. `session/new` returns a `sessionId`.
3. `session/prompt` produces at least one `session/update` notification.
4. `session/cancel` is accepted as a notification.
5. Final `stopReason` is one of the spec's enumerated values.

Skip cleanly if the OpenCode binary isn't on `PATH`. Gate on `CODEMINT_ACP_CONFORM=1` so it doesn't run on every `go test`.

**Success state.** Any change to `internal/acp/` triggers this harness in pre-merge or local dev.

---

## Task D — Planning-template guardrail (P2, process)

Story 2.0's retro identified the root cause of the integration bugs as planning-side. Lock it in.

**What to do.** Add a clause to the planning template (or `CLAUDE.md` Working Styles): any task touching `internal/acp/` must:

1. Cite the relevant spec section by URL in the task description.
2. Include in its verification clause both: "schema diff is empty for the touched types" and "ACP wire conformance harness passes."

**Success state.** No future EPIC-02 story discovers an ACP wire bug at integration time.

---

## Priority & Dependencies

```
                    Task A (ACP Schema)
                          │
        ┌─────────────────┼─────────────────┐
        ▼                 ▼                 ▼
   Task B (Coverage)  Task C (Harness)  Task D (Process)
        │                 │
        └────────┬────────┘
                 │
                 ▼
   ┌───────── Coding Workflow spine ──────────┐
   │ 2.1 → 2.2 → 2.3 → 2.4 ─/modify→ 2.2     │
   │                       └/pick→ 2.5        │
   │ 2.5 → [scheduler] → 2.6 + 2.7 per story │
   │ any Failure → 2.8                        │
   └──────────────────────────────────────────┘
```

**Critical path:** Task A → 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6 → 2.7 → 2.8.
