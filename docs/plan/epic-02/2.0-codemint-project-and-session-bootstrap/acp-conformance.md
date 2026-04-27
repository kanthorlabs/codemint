# Story 2.0 Follow-up: ACP Conformance Tasks

Distilled from `docs/coding/acp-coverage.md` (the spec-vs-impl audit) and the four wire-bugs surfaced in `hotfix.md` / `retro.md`. Each task lists **Action**, **Implementation**, **Verification**. Pick up in priority order — P0 blocks every later EPIC-02 story.

**Authoritative spec source:** `https://agentclientprotocol.com/protocol/schema.md`. Spec index mirrored at `docs/coding/agent-client-protocol.md`.

**Constant-naming convention:** No hardcoded protocol strings in handlers. Define string constants and enum types in `internal/acp/protocol.go` (e.g. `ProtocolVersionV1`, `ToolCallStatusPending`, `PermissionOptionKindAllowOnce`) and reference them from call sites.

---

## P0 — Wire conformance fixes (block all EPIC-02 work)

### T1. Reconcile `Initialize*` types with spec keys

- **Action.** Make `InitializeParams` and `InitializeResult` 1:1 with `InitializeRequest` / `InitializeResponse` in the schema.
- **Implementation.**
  - Rename `InitializeParams.Capabilities` JSON key from `capabilities` → `clientCapabilities`.
  - Drop `InitializeParams.WorkingDir` (not in spec).
  - Rename `InitializeResult.ServerInfo` → `agentInfo`, `Capabilities` → `agentCapabilities`.
  - Add required `InitializeResult.ProtocolVersion uint16` and `AuthMethods []AuthMethod`.
  - Update `Worker.initialize` to send/parse the new shapes.
- **Verification.**
  1. Round-trip test: marshal a hand-authored frame copied verbatim from the spec, decode through the new types, re-encode, assert byte-equal modulo key order.
  2. `make test ./internal/acp/... ./internal/orchestrator/...` green.

### T2. Replace flat `Caps` / `ServerCaps` with nested capability structs

- **Action.** Match `ClientCapabilities` and `AgentCapabilities` from the schema exactly.
- **Implementation.**
  - `ClientCapabilities { fs FileSystemCapabilities; terminal bool }`.
  - `FileSystemCapabilities { readTextFile bool; writeTextFile bool }`.
  - `AgentCapabilities { loadSession bool; mcpCapabilities McpCapabilities; promptCapabilities PromptCapabilities; sessionCapabilities SessionCapabilities }`.
  - Mirror sub-structs (`McpCapabilities`, `PromptCapabilities`, `SessionCapabilities`).
  - Default zero-values must serialize as the schema's defaults, not as `null`.
- **Verification.**
  - Spec-fixture decode test for `InitializeResponse` with the example default object.
  - Verify our outbound `InitializeRequest` declares only what we actually support: `fs.readTextFile=false`, `fs.writeTextFile=false`, `terminal=false`.

### T3. Fix `SessionNewParams` to spec shape

- **Action.** Drop fields not in spec; deliver system prompt elsewhere.
- **Implementation.**
  - Remove `SessionNewParams.SessionID` and `SystemPrompt`.
  - Keep required `Cwd` and `McpServers []McpServer`. Always serialize `McpServers` as `[]`, never `null`.
  - Move system-prompt injection to the first `ContentBlock` of `session/prompt`, or via an MCP server (out-of-scope for this task — file follow-up).
- **Verification.**
  - Decode the spec example for `NewSessionRequest`. Re-encode. Byte-equal.
  - Manually run `make run` against OpenCode and confirm `session/new` is accepted (no `-32602`).

### T4. Fix `SessionPromptParams` to spec shape

- **Action.** Drop invented fields. Encode context as ContentBlocks.
- **Implementation.**
  - Remove `SessionPromptParams.Context` and `Tools`.
  - Existing `[]ContentBlock` prompt stays.
  - Add helper to build `resource_link` ContentBlocks for context-file references (used by `Executor.executeCoding` and the future Brainstormer).
- **Verification.**
  - Existing executor test `TestExecutor_Coding_BuildsTypedParams` migrated to assert ContentBlock array contains a `resource_link` per context file.
  - Spec-fixture round-trip.

### T5. Fix `SessionPromptResult` to spec shape

- **Action.** Replace ad-hoc `success: bool` with `stopReason: StopReason`.
- **Implementation.**
  - `SessionPromptResult { StopReason StopReason }`.
  - Define `StopReason` enum with constants per spec (`StopReasonEndTurn`, `StopReasonMaxTokens`, `StopReasonMaxTurnRequests`, `StopReasonRefusal`, `StopReasonCancelled`).
  - Update `Worker.SendPrompt` to surface stop reason to the orchestrator. `StatusMapper` translates each StopReason into a `domain.TaskStatus` transition.
- **Verification.**
  - Unit test for each StopReason → TaskStatus mapping.
  - E2E test: cancel a prompt, assert task ends in `Cancelled`.

### T6. Rewrite `ContentBlock` as a discriminated union

- **Action.** Decode and encode all five variants from the spec, not just `text`.
- **Implementation.**
  - Use a discriminator field `type` with constants: `ContentBlockTypeText`, `ContentBlockTypeImage`, `ContentBlockTypeAudio`, `ContentBlockTypeResource`, `ContentBlockTypeResourceLink`.
  - Custom `UnmarshalJSON` dispatches on `type`. Custom `MarshalJSON` per variant.
  - Add helpers: `TextContent`, `ImageContent`, `AudioContent`, `ResourceContent`, `ResourceLinkContent`.
- **Verification.**
  - One round-trip sub-test per variant using spec examples.
  - Decode a `session/update` carrying mixed content; assert no variant lost.

### T7. Rewrite `PermissionRequest` / `PermissionResponse` to spec shape

- **Action.** Match `RequestPermissionRequest` / `RequestPermissionResponse` exactly.
- **Implementation.**
  - `RequestPermissionRequest { SessionID string; ToolCall ToolCallUpdate; Options []PermissionOption }`.
  - `PermissionOption { OptionID string; Name string; Kind PermissionOptionKind }`.
  - `PermissionOptionKind` constants: `PermissionOptionKindAllowOnce`, `PermissionOptionKindAllowAlways`, `PermissionOptionKindRejectOnce`, `PermissionOptionKindRejectAlways`.
  - `RequestPermissionResponse` carries `Outcome`, a union: `{outcome: "cancelled"}` or `{outcome: "selected", optionId: PermissionOptionId}`. Implement via a typed wrapper with helper constructors `OutcomeCancelled()` / `OutcomeSelected(id)`.
  - Refactor `Interceptor.respond` (and the two existing call sites in `interceptor.go`) onto the new types.
- **Verification.**
  - Round-trip the spec examples.
  - Existing `TestInterceptor_*` cases still pass after migration; add `TestInterceptor_PermissionRequest_DeserializesSpecPayload`.
  - Wire-test against OpenCode confirms response is accepted.

### T8. Define ACP-specific error codes

- **Action.** Add typed constants and propagate them through the runtime.
- **Implementation.**
  - In `internal/acp/protocol.go` add: `ErrCodeAuthRequired = -32000`, `ErrCodeResourceNotFound = -32002`.
  - Map ACP error `-32000` to a sentinel `acp.ErrAuthRequired` so the orchestrator can prompt for auth instead of failing the task.
- **Verification.**
  - Unit test: receiving `-32000` yields `acp.ErrAuthRequired` from `Worker.SendRequest`.
  - Unit test: receiving `-32002` yields `acp.ErrResourceNotFound`.

---

## P1 — Inbound update parsing (needed by EPIC-02 Brainstormer)

### T9. Re-derive `UpdateKind` constants from the spec

- **Action.** Confirm every `sessionUpdate` discriminator string in our code matches the spec's `SessionUpdate` union.
- **Implementation.**
  - Walk each constant in `protocol.go`'s update-kind block. Cross-check against the spec `SessionUpdate` variant names.
  - Add missing kinds: `UpdateKindAvailableCommandsUpdate`, `UpdateKindConfigOptionUpdate`, `UpdateKindCurrentModeUpdate`, `UpdateKindSessionInfoUpdate` (if stable per the announcement page).
  - Replace any string literal in handlers with the constant.
- **Verification.**
  - `grep -rn '"agent_message_chunk"\|"tool_call"\|"plan"' internal/` returns hits only in `protocol.go` (the constant definitions), not in handlers.
  - Unit test: feed one fixture per kind; assert `Pipeline.Classify` routes to the right `Event.Kind`.

### T10. Parse `Plan` updates as typed entries

- **Action.** Promote Plan entries from raw JSON to a typed `PlanEntry` slice.
- **Implementation.**
  - `Plan { Entries []PlanEntry }`, `PlanEntry { Content string; Priority PlanEntryPriority; Status PlanEntryStatus }`.
  - Constants for `PlanEntryPriority` (`High|Medium|Low`) and `PlanEntryStatus` (`Pending|InProgress|Completed`).
  - Pipeline emits a typed `EventPlan` with the parsed slice (not raw).
  - UI mediator `RenderPlan` consumes the typed shape (replace any `json.RawMessage` consumer).
- **Verification.**
  - Unit test: decode spec example for a multi-entry plan, assert all entries.
  - TUI snapshot test renders plan correctly from typed input.

### T11. Handle `available_commands_update` as a typed update

- **Action.** Capture and surface dynamic slash commands advertised by the agent.
- **Implementation.**
  - Decode into `AvailableCommandsUpdate { Commands []SlashCommand }`.
  - Store the latest set on the per-session state owned by `Runtime`.
  - Expose via `/help` / command discovery in the REPL (fold agent-advertised commands into the existing registry view, marked as agent-sourced).
- **Verification.**
  - Unit test: feed an `available_commands_update` payload; assert the runtime's exposed list matches.
  - Integration: with OpenCode, log advertised commands and confirm at least one shows up.

---

## P2 — Capability honesty

### T12. Declare client capabilities truthfully in `initialize`

- **Action.** Stop sending `{}` capabilities — declare what we actually do or do not support.
- **Implementation.**
  - Today CodeMint does NOT implement `fs/read_text_file` or `fs/write_text_file`, and does NOT host terminals. Declare `clientCapabilities.fs.readTextFile = false`, `writeTextFile = false`, `terminal = false`. Out-of-scope rows in `acp-coverage.md` confirm.
  - When future stories add these (none planned), flip the flags.
- **Verification.**
  - Capture an `initialize` frame on the wire and confirm declared caps match what we actually serve.

### T13. Plumb `_meta` passthrough on top-level types

- **Action.** Preserve agent-side `_meta` blobs without forcing schema knowledge.
- **Implementation.**
  - Add `Meta json.RawMessage \`json:"_meta,omitempty"\`` to `InitializeResult`, `SessionNewResult`, `SessionUpdate`, `ToolCallUpdate`, `PermissionRequest`. Forward as opaque.
  - Do not drop on re-encode (custom marshal must round-trip).
- **Verification.**
  - Round-trip test: decode → encode → byte-equal for a frame containing `_meta`.

---

## Out of scope (decided, document the reason)

| Feature | Reason |
|---|---|
| `fs/read_text_file`, `fs/write_text_file` | Agents read/write files directly via OS — CodeMint is not a remote-FS client. |
| `terminal/*` family | CodeMint runs shell via `internal/orchestrator/local_runner.go`. ACP terminals are a hosted-IDE pattern we don't need. |
| `session/list`, `session/resume`, `session/close` | Not needed before Story 2.7 (Human Review). Re-evaluate when 2.7 starts. |
| `session/set_mode`, mode updates | No mode use case in the Brainstormer pipeline. Re-evaluate per concrete need. |
| `session/set_config_option` | Same — defer until a story names a config option we want to control. |
| W3C trace context | No distributed tracing requirement today. |

---

## Dependencies and order

```
T1, T2 (initialize)         ─┐
T3, T4, T5 (session/new+prompt) ┤
T6 (ContentBlock union)      ├─→ T7 (permissions)
T8 (error codes)             ┘

T9 (update kinds) ─→ T10, T11

T12, T13 (independent)
```

## Definition of done for the whole list

1. `make test ./...` green.
2. `CODEMINT_ACP_CONFORM=1 make acp-conform` (Task C in `epic-02/appendings.md`) passes against OpenCode end-to-end.
3. `docs/coding/acp-coverage.md` summary table shows zero **Partial** rows in scope; **Missing** rows are limited to the Out-of-Scope set above with linked rationale.
4. No string-literal protocol values remain in handlers; all reference constants per the new convention in `docs/plan/appendings.md`.
