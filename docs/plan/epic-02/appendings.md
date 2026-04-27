# EPIC-02 Appending Tasks

Cross-cutting work that doesn't belong to one user story but blocks or de-risks EPIC-02 as a whole. Pick up in priority order.

**Authoritative spec source:** `https://agentclientprotocol.com/protocol/schema.md`. Spec index mirrored at `docs/coding/agent-client-protocol.md`. The OpenAPI URL is 404 — do not use it.

---

## Status snapshot (2026-04-27)

| Story | State |
|---|---|
| 2.0 CodeMint project & session bootstrap | **Done** (see `2.0-.../tasks.md`, `hotfix.md`, `retro.md`) |
| 2.9 YOLO agent seed | **Done** (`agentRepo.EnsureSystemAgents` seeds `sys-auto-approve`) |
| 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.10, 2.11, 2.12, 2.13 | **Not started** (only `index.md`, no `tasks.md`) |

The Brainstormer pipeline (Phase 1–5) is the long pole. None of those stories should start until Task A below lands — the ACP wire is too non-conformant to build on safely.

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
- `PermissionRequest` / `PermissionResponse`: rewrite to spec shapes (`{sessionId, toolCall: ToolCallUpdate, options: PermissionOption[]}` and `{outcome: cancelled | {outcome:"selected", optionId}}`). Today's shape (`requestId`, `granted`, `tool`, `parameters`) is invented.
- `PermissionOption.kind` enum: `allow_once | allow_always | reject_once | reject_always`.
- `UpdateKind` constants: re-derive from the spec union (`ContentChunk`, `ToolCallUpdate`, `Plan`, `AvailableCommandsUpdate`, `ConfigOptionUpdate`, `CurrentModeUpdate`). Today's `agent_message_chunk` etc. need verifying against actual spec discriminators.
- ACP error codes: `-32000 auth_required`, `-32002 resource not found` — define and surface.

**How to verify.**
1. Add round-trip unit tests in `internal/acp/protocol_test.go`: marshal → unmarshal → assert field-by-field equality for every typed payload. One sub-test per type.
2. Add golden fixtures: hand-author one canonical JSON-RPC frame per method (`initialize`, `session/new`, `session/prompt`, `session/cancel`, `session/update`, `session/request_permission`) using snippets copied verbatim from `protocol/schema.md`. Decode through our types, re-encode, assert byte-equal modulo key ordering.
3. `make test ./...` green after every call site is updated.

**Success state.** Every JSON key, required field, and enum value in `internal/acp/protocol.go` has a 1:1 match in `protocol/schema.md`. No invented fields remain. Golden fixtures pass.

---

## Task B — ACP feature coverage map (P1)

The team needs a single page that says "yes / partial / no" for every protocol feature so future EPIC-02 story authors don't have to re-read the spec.

**What to do.** Produce `docs/coding/acp-coverage.md` with one row per spec page (Initialization, Session Setup, Prompt Turn, Tool Calls, Permissions, Slash Commands, Terminals, File System, Session List/Resume/Close, Session Modes, Session Config Options, Agent Plan, Content, Extensibility). Mark each **Have / Partial / Missing / Out-of-scope** with a one-line note and a link to the implementing file (or to the EPIC-02 story that will add it).

This is documentation, not code.

**How to verify.** A teammate not on this PR can answer "does CodeMint support X?" for any X in the spec by `grep`-ing one file. PR review spot-checks three random "Have" rows against the cited code.

**Success state.** `acp-coverage.md` exists. Every spec page has a row. Every Have/Partial row links to a file path. Every Missing row links to an epic-02 story (or notes "no story yet").

---

## Task C — ACP wire conformance harness (P1, depends on A)

Schema fixes (Task A) protect against type drift. They do not protect against semantic regressions — wrong state machine, missing handshake step, wrong notification ordering. We need a harness that pokes a real agent.

**What to do.** Add `make acp-conform`. It boots a real ACP agent (start with OpenCode — the provider we ship), runs the happy path `initialize → session/new → session/prompt → session/cancel`, and asserts:

1. `initialize` is accepted (no `-32600`, no `-32602`).
2. `session/new` returns a `sessionId`.
3. `session/prompt` produces at least one `session/update` notification.
4. `session/cancel` is accepted as a notification (no `id`, no response expected).
5. Final `stopReason` is one of the spec's enumerated values.

Skip cleanly if the OpenCode binary isn't on `PATH` — `t.Skip` with a clear message. Optionally gate on `CODEMINT_ACP_CONFORM=1` so the harness doesn't run on every `go test` by default.

**How to verify.** `CODEMINT_ACP_CONFORM=1 make acp-conform` passes locally with OpenCode installed. Without the env var, the test is skipped and `make test` is unaffected.

**Success state.** Any change to `internal/acp/` triggers this harness in pre-merge or local dev and gives a yes/no wire signal. The harness is parameterizable to add Codex and Claude Code later (not required for v1).

---

## Task D — Planning-template guardrail (P2, process)

Story 2.0's retro identified the root cause of the integration bugs as planning-side, not coding-side. Lock it in.

**What to do.** Add a clause to the planning template (or `CLAUDE.md` Working Styles) requiring that any task touching `internal/acp/` must:

1. Cite the relevant spec section by URL in the task description.
2. Include in its verification clause both: "schema diff is empty for the touched types" and "ACP wire conformance harness passes."

**How to verify.** First post-2.0 ACP-touching task after this lands has both clauses present at PR review.

**Success state.** No future EPIC-02 story discovers an ACP wire bug at integration time. (That is the actual goal; A/B/C are how we get there. D keeps us there.)

---

## Priority

A → unblocks 2.1, 2.4, 2.7, 2.8, 2.13 (anything that sends task payloads to ACP).
A → unblocks 2.2 (Living Spec → ACP `session/prompt` body shape).
A → unblocks 2.10 (YOLO Confirmation auto-approve must round-trip through correct PermissionResponse).

B and C can run in parallel with A or right after. D is one-line policy.
