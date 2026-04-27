# ACP Feature Coverage Map

This document maps every ACP (Agent Communication Protocol) spec feature to its implementation status in CodeMint. Use this to quickly answer "does CodeMint support X?" without re-reading the spec.

**Legend:**
- **Have** — Fully implemented and tested
- **Partial** — Types/structure defined but not fully wired or used
- **Missing** — Not implemented
- **Out-of-scope** — Intentionally not supported (documented reason)

**Spec Reference:** https://agentclientprotocol.com/protocol/

---

## Initialization

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `initialize` method | **Have** | Full handshake with version negotiation | [`internal/acp/worker.go:525-571`](../../internal/acp/worker.go) |
| Protocol version negotiation | **Have** | Sends version 1, validates response | [`internal/acp/worker.go:528`](../../internal/acp/worker.go) |
| Client capabilities declaration | **Have** | Truthfully declares `fs.*=false`, `terminal=false` | [`internal/acp/worker.go:533-539`](../../internal/acp/worker.go) |
| Agent capabilities parsing | **Have** | Stores `InitializeResult` with all agent caps | [`internal/acp/worker.go:563`](../../internal/acp/worker.go) |
| `clientInfo` / `agentInfo` | **Have** | Sends name/version, parses agent info | [`internal/acp/worker.go:529-532`](../../internal/acp/worker.go) |
| Custom capabilities (`_meta`) | **Have** | `_meta` passthrough on all top-level types | [`internal/acp/protocol.go`](../../internal/acp/protocol.go) |

---

## Session Setup

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `session/new` | **Have** | Creates session with `cwd` | [`internal/acp/worker.go:464-493`](../../internal/acp/worker.go) |
| Working directory (`cwd`) | **Have** | Set from project root path | [`internal/acp/worker.go:469`](../../internal/acp/worker.go) |
| System prompt injection | **Have** | Memory files injected via `BuildSystemPrompt` | [`internal/acp/system_prompt.go`](../../internal/acp/system_prompt.go) |
| MCP servers config | **Partial** | Types defined in `SessionNewParams.McpServers` | [`internal/acp/protocol.go:380-385`](../../internal/acp/protocol.go) |
| `session/load` (replay history) | **Missing** | Capability-gated, not implemented | [Story 2.7](../plan/epic-02/2.7-phase-5-human-review-and-activation/index.md) |

---

## Session List / Resume / Close

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `session/list` | **Missing** | Type `SessionCapabilities.List` defined but unused | No story yet |
| `session/resume` | **Missing** | Type `SessionCapabilities.Resume` defined but unused | No story yet |
| `session/close` | **Missing** | Type `SessionCapabilities.Close` defined but unused | No story yet |
| Session metadata (`session_info_update`) | **Have** | Type `SessionInfoUpdate` defined, event classified | [`internal/acp/protocol.go:165-176`](../../internal/acp/protocol.go) |

---

## Prompt Turn

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `session/prompt` | **Have** | Sends user prompt with `ContentBlock` array | [`internal/acp/worker.go:382-420`](../../internal/acp/worker.go) |
| Content blocks (text) | **Have** | `TextContent()` helper used | [`internal/acp/protocol.go:240`](../../internal/acp/protocol.go) |
| Content blocks (image) | **Partial** | Type defined, not sent | [`internal/acp/protocol.go:245`](../../internal/acp/protocol.go) |
| Content blocks (audio) | **Partial** | Type defined, not sent | [`internal/acp/protocol.go:250`](../../internal/acp/protocol.go) |
| Embedded resource content | **Partial** | `EmbeddedResourceContent()` helper exists, not used | [`internal/acp/protocol.go:260`](../../internal/acp/protocol.go) |
| Resource link content | **Have** | `ResourceLinkContent()` used for context files | [`internal/acp/protocol.go:255`](../../internal/acp/protocol.go) |
| `session/cancel` | **Have** | Sent on task timeout | [`internal/orchestrator/executor.go:520`](../../internal/orchestrator/executor.go) |
| `session/update` (inbound) | **Have** | Full event stream parsing | [`internal/acp/events.go`](../../internal/acp/events.go) |
| Stop reasons parsing | **Have** | `end_turn`, `cancelled`, etc. handled | [`internal/acp/protocol.go:95-100`](../../internal/acp/protocol.go) |

---

## Tool Calls

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `tool_call` update parsing | **Have** | Extracts tool name, args, command, cwd | [`internal/acp/events.go:120-180`](../../internal/acp/events.go) |
| `tool_call_update` parsing | **Have** | Status transitions tracked | [`internal/acp/events.go:182-200`](../../internal/acp/events.go) |
| Tool status (`pending`→`completed`) | **Have** | Full lifecycle tracking | [`internal/acp/protocol.go:105-113`](../../internal/acp/protocol.go) |
| Tool kinds (`read`, `edit`, `execute`, etc.) | **Partial** | Parsed but not used for routing | [`internal/acp/events.go:150`](../../internal/acp/events.go) |
| Diff content (`oldText`/`newText`) | **Partial** | Types defined, not displayed | [`internal/acp/protocol.go:541-545`](../../internal/acp/protocol.go) |
| Terminal output in tool content | **Partial** | Parsed, not specially handled | No story yet |
| Location tracking (file:line) | **Partial** | Extracted but not used for navigation | [`internal/acp/events.go:160`](../../internal/acp/events.go) |
| Raw I/O (`rawInput`/`rawOutput`) | **Have** | Types defined in `ToolCallUpdate` | [`internal/acp/protocol.go:530-532`](../../internal/acp/protocol.go) |

---

## Permissions

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `session/request_permission` (inbound) | **Have** | Routed to `Interceptor.Handle()` | [`internal/orchestrator/interceptor.go:180`](../../internal/orchestrator/interceptor.go) |
| Permission option parsing | **Have** | `allow_once`, `allow_always`, `reject_*` | [`internal/acp/protocol.go:94-103`](../../internal/acp/protocol.go) |
| Permission response (`selected`) | **Have** | `RequestPermissionResult.SelectedOutcome()` | [`internal/acp/protocol.go:584`](../../internal/acp/protocol.go) |
| Permission response (`cancelled`) | **Have** | Sent on timeout/deny | [`internal/acp/protocol.go:579`](../../internal/acp/protocol.go) |
| Auto-approval (client-side) | **Have** | Via `PermissionMatcher` whitelist | [`internal/orchestrator/permission_matcher.go`](../../internal/orchestrator/permission_matcher.go) |
| Session-scoped allow (`allow_session`) | **Have** | In-memory whitelist per session | [`internal/orchestrator/interceptor.go:350`](../../internal/orchestrator/interceptor.go) |
| Persistent permissions (DB) | **Have** | `ProjectPermission` rows checked | [`internal/orchestrator/interceptor.go:220`](../../internal/orchestrator/interceptor.go) |
| YOLO mode bypass | **Have** | Skips confirmation/retrospective gates | [`internal/orchestrator/executor.go:600`](../../internal/orchestrator/executor.go) |

---

## Slash Commands

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `available_commands_update` (inbound) | **Have** | Type `AvailableCommandsUpdate` defined, event classified | [`internal/acp/protocol.go:152-157`](../../internal/acp/protocol.go) |
| Dynamic command discovery | **Partial** | Event classified but not surfaced to REPL | No story yet |
| Command execution via prompt | **Have** | `/command` text routed to local handlers | [`internal/orchestrator/dispatcher.go:80`](../../internal/orchestrator/dispatcher.go) |

---

## Terminals

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `terminal/create` | **Out-of-scope** | CodeMint uses `LocalRunner` for shell commands | [`internal/orchestrator/local_runner.go`](../../internal/orchestrator/local_runner.go) |
| `terminal/output` | **Out-of-scope** | Not needed; output captured synchronously | — |
| `terminal/wait_for_exit` | **Out-of-scope** | `LocalRunner.Run()` blocks until exit | — |
| `terminal/kill` | **Out-of-scope** | Context cancellation used instead | — |
| `terminal/release` | **Out-of-scope** | No persistent terminals | — |
| Client capability `terminal: true` | **Have** | Declared `false` (truthful) | [`internal/acp/worker.go:538`](../../internal/acp/worker.go) |

---

## File System

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `fs/read_text_file` | **Out-of-scope** | Agent reads files directly; no client capability | — |
| `fs/write_text_file` | **Out-of-scope** | Agent writes files directly; no client capability | — |
| Client capability `fs.*` | **Have** | Declared `false` (truthful) | [`internal/acp/worker.go:534-537`](../../internal/acp/worker.go) |

---

## Session Modes

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `modes` in session response | **Have** | Type `SessionModeState` defined and parsed | [`internal/acp/protocol.go:410-413`](../../internal/acp/protocol.go) |
| `session/set_mode` | **Missing** | Method not implemented | No story yet |
| `current_mode_update` (inbound) | **Have** | Type `CurrentModeUpdate` defined, event classified | [`internal/acp/protocol.go:159-163`](../../internal/acp/protocol.go) |
| Exit plan mode transitions | **Missing** | Not implemented | No story yet |

---

## Session Config Options

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `configOptions` in session response | **Have** | Type `SessionConfigOption` defined | [`internal/acp/protocol.go:395-401`](../../internal/acp/protocol.go) |
| `session/set_config_option` | **Missing** | Method not implemented | No story yet |
| `config_option_update` (inbound) | **Have** | Type `ConfigOptionUpdate` defined, event classified | [`internal/acp/protocol.go:165-170`](../../internal/acp/protocol.go) |
| Option categories (`mode`, `model`, etc.) | **Have** | Types support arbitrary option IDs | [`internal/acp/protocol.go:395-407`](../../internal/acp/protocol.go) |

---

## Agent Plan

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `plan` update (inbound) | **Have** | Event classified as `EventPlan` | [`internal/acp/events.go:144`](../../internal/acp/events.go) |
| Plan entry parsing | **Have** | Type `PlanUpdate` with `[]PlanEntry` | [`internal/acp/protocol.go:115-145`](../../internal/acp/protocol.go) |
| Plan status tracking | **Have** | `PlanEntryStatus` enum with pending/in_progress/completed | [`internal/acp/protocol.go:125-130`](../../internal/acp/protocol.go) |
| Dynamic plan updates | **Have** | Each `plan` update replaces previous | [`internal/acp/events.go:144`](../../internal/acp/events.go) |

---

## Content

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| Text content blocks | **Have** | `ContentBlock{Type:"text", Text:...}` | [`internal/acp/protocol.go:468-470`](../../internal/acp/protocol.go) |
| Image content blocks | **Partial** | Type defined, not used in prompts | [`internal/acp/protocol.go:459`](../../internal/acp/protocol.go) |
| Audio content blocks | **Partial** | Type defined, not used in prompts | [`internal/acp/protocol.go`](../../internal/acp/protocol.go) |
| Embedded resource blocks | **Have** | Helper `EmbeddedResourceContent()` exists | [`internal/acp/protocol.go:483-493`](../../internal/acp/protocol.go) |
| Resource link blocks | **Have** | Used for context file references | [`internal/acp/protocol.go:473-481`](../../internal/acp/protocol.go) |
| Annotations on content | **Partial** | Type `Annotations` defined | [`internal/acp/protocol.go:463-467`](../../internal/acp/protocol.go) |

---

## Extensibility

| Feature | Status | Note | File/Story |
|---------|--------|------|------------|
| `_meta` field support | **Have** | Passthrough on all top-level types | [`internal/acp/protocol.go`](../../internal/acp/protocol.go) |
| Extension methods (`_*`) | **Out-of-scope** | No custom methods needed | — |
| W3C trace context headers | **Missing** | `traceparent` etc. not implemented | No story yet |
| Custom capabilities | **Have** | Types support capability extensions | [`internal/acp/protocol.go`](../../internal/acp/protocol.go) |

---

## Summary

| Category | Have | Partial | Missing | Out-of-scope |
|----------|------|---------|---------|--------------|
| Initialization | 6 | 0 | 0 | 0 |
| Session Setup | 4 | 1 | 1 | 0 |
| Session List/Resume/Close | 1 | 0 | 3 | 0 |
| Prompt Turn | 7 | 3 | 0 | 0 |
| Tool Calls | 5 | 3 | 0 | 0 |
| Permissions | 7 | 0 | 0 | 0 |
| Slash Commands | 2 | 1 | 0 | 0 |
| Terminals | 1 | 0 | 0 | 5 |
| File System | 1 | 0 | 0 | 2 |
| Session Modes | 2 | 0 | 2 | 0 |
| Session Config Options | 3 | 0 | 1 | 0 |
| Agent Plan | 4 | 0 | 0 | 0 |
| Content | 3 | 3 | 0 | 0 |
| Extensibility | 2 | 0 | 1 | 1 |
| **Total** | **48** | **11** | **8** | **8** |

---

## EPIC-02 Story Index

Stories that will add missing ACP features:

| Story | Title | ACP Features |
|-------|-------|--------------|
| [2.1](../plan/epic-02/2.1-phase-1-context-intake/index.md) | Context Intake | Context injection via `session/prompt` |
| [2.2](../plan/epic-02/2.2-phase-2-the-living-spec/index.md) | The Living Spec | Structured prompt body |
| [2.7](../plan/epic-02/2.7-phase-5-human-review-and-activation/index.md) | Human Review | `session/load` for history replay |
| [2.8](../plan/epic-02/2.8-mid-flight-pivots/index.md) | Mid-Flight Pivots | Dynamic `session/prompt` revisions |
| [2.10](../plan/epic-02/2.10-yolo-mode-delegation/index.md) | YOLO Delegation | Permission response round-trip |
| [2.13](../plan/epic-02/2.13-acp-compliant-payload-formatting/index.md) | ACP Payload Formatting | Structured `ContentBlock` payloads |
| [Task A](../plan/epic-02/appendings.md) | ACP Schema Audit | Wire-level spec conformance |
| [Task C](../plan/epic-02/appendings.md) | ACP Conformance Harness | Integration test coverage |

---

*Last updated: 2026-04-27*
