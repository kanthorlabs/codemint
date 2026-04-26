# Tasks: 3.19 System Assistant Conversational Pipeline

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/orchestrator/`, `internal/agent/`, `internal/repl/`, `cmd/codemint/`
**Tech Stack:** Go, ACP worker, JSON-RPC
**Priority:** P0 for end-to-end testing affordance; precondition for 3.20 + 3.21.

---

## Task 3.19.1: `SystemAssistant` Interface and Provider-backed Implementation

* **Action:** Define the assistant contract and a Provider-resolved default.
* **Details:**
  * Create `internal/agent/system_assistant.go`:
    ```go
    type SystemAssistant interface {
        Ask(ctx context.Context, sess *orchestrator.ActiveSession, text string) (<-chan ChatChunk, error)
        AgentID() string
        Provider() *Provider // expose the resolved Provider for /acp-status and audit
    }
    type ChatChunk struct {
        Text string
        Done bool
        Err  error
    }
    ```
  * Default implementation: `acpAssistant` consumes the per-session ACP worker. It does **not** hardcode OpenCode — instead it accepts:
    ```go
    func NewACPAssistant(runtime *orchestrator.Runtime, providers *ProviderRegistry, bindingName string) (SystemAssistant, error)
    ```
    `bindingName` is `"system"` here; future stories pass `"brainstormer"`, `"clarifier"`, etc. The constructor calls `providers.Resolve(cfg.Assistants[bindingName].Provider)` and stores the resolved `*Provider`.
  * On first `Ask`:
    1. `runtime.AttachWorkerWithProvider(ctx, sess, project, provider)` — works with `project == nil`; spawn falls back to `os.UserHomeDir()` and an empty system prompt.
    2. Sends `session/prompt` with the raw text and a system preamble: `"You are CodeMint's System Assistant. Answer general questions about CodeMint, this conversation, or programming. You have no project context. Be concise."`
    3. Streams `agent_message_chunk` updates onto the returned channel; closes on `turn-end`.
  * Add a `nullAssistant` for tests that returns a canned reply without spawning any binary.
* **Verification:**
  * `TestACPAssistant_Ask_StreamsChunks` — stub Provider points at a stub worker that emits 2 chunks + turn-end; assistant returns 2 `ChatChunk` values then a `Done=true` close.
  * `TestACPAssistant_NoProject_StillWorks` — `sess.Project == nil` → no panic, assistant operates in `os.UserHomeDir()`.
  * `TestACPAssistant_UsesConfiguredProvider` — config binds System Assistant to `codex`; assistant exposes `Provider().Name == "codex"`.

---

## Task 3.19.2: Dispatcher Routing for Freeform Text

* **Action:** Stop treating non-slash input as an error; route it to the assistant.
* **Details:**
  * In `internal/orchestrator/dispatcher.go`, find the existing parser branch that returns `ErrNotACommand` (or similar) for input not starting with `/`. Replace with:
    ```go
    if !strings.HasPrefix(strings.TrimSpace(input), "/") {
        return d.dispatchToSystemAssistant(ctx, sess, input)
    }
    ```
  * `dispatchToSystemAssistant`:
    1. If `d.systemAssistant == nil`, return a friendly error mentioning `--with-assistant` flag (see 3.19.5).
    2. Open a Coordination task row `status=processing` for audit.
    3. Call `assistant.Ask(...)`, drain chunks, broadcast each chunk to the mediator via `mediator.NotifyEvent(EventChatChunk{Text: chunk.Text})`.
    4. On `Done`, set task `status=success`, persist concatenated `output.text`.
    5. On `Err`, set task `status=failure`, surface a one-line error to the mediator.
* **Verification:**
  * `TestDispatcher_FreeformText_RoutesToAssistant` — input `"What's a goroutine?"` results in `assistant.Ask` being called once.
  * `TestDispatcher_FreeformText_NilAssistant_FriendlyError` — assistant absent, dispatcher returns a non-fatal error string the REPL prints verbatim.

---

## Task 3.19.3: Mediator Broadcast Hook for Chat

* **Action:** Generalize the existing `RenderMessage` so chat events fan out, not just direct-stream.
* **Details:**
  * Extend `registry.UIEvent` with a new kind:
    ```go
    EventChatChunk struct { Source string; Text string; Final bool }
    ```
  * `Source` is `"system-assistant"` for now; reserved for `"agent-coding"`, `"user"` later.
  * Mediator's `NotifyEvent` already broadcasts to all adapters (Story 3.4 fanout). Verify both `TUIAdapter` and `CUIAdapter` render this kind:
    * TUI: append to chat pane.
    * CUI: append to current chat thread (Telegram message group, Slack thread).
  * Add a flush rule: when `Final=true`, emit a delimiter line so adapters can close out the streaming display.
* **Verification:**
  * `TestMediator_ChatChunk_BroadcastsToAllAdapters` — register two stub adapters, fire one chunk, both record receipt.
  * `TestTUIAdapter_ChatChunk_RendersStreaming` — five chunks coalesce into a single visible line, no flicker.

---

## Task 3.19.4: Persist Conversation as Coordination Tasks

* **Action:** Hook the existing `interactionRecorder` so `/activity` shows the chat history.
* **Details:**
  * In `dispatchToSystemAssistant`, after the assistant completes, call:
    ```go
    interactionRecorder.RecordChat(ctx, sess, RecordChatInput{
        UserText:      input,
        AssistantText: collectedReply,
        Source:        clientMode, // cli | daemon
    })
    ```
  * Add `RecordChat` to `interaction_recorder.go`. It writes `task.input = {"command":"/chat","text":<user>}` and `task.output = {"text":<assistant>}` with `type=3` and `status=5`.
* **Verification:**
  * `TestInteractionRecorder_RecordChat_Persists` — Coordination row exists with both fields populated.
  * Manual: `/activity` lists the chat round-trip after the test.

---

## Task 3.19.5: Wire Into `main.go`

* **Action:** Construct the assistant from config and inject it into the dispatcher.
* **Details:**
  * Add a flag: `--with-assistant=true` (default `true`). If false, dispatcher uses `nil` assistant — preserves the current "non-slash text is error" behavior for users who don't want the LLM round-trip.
  * In `cmd/codemint/main.go` after Step 9 (`appCfg`) construct the Provider Registry (Story 3.22.5 owns this block; reuse it). Then after Step 12 (Runtime + Scheduler):
    ```go
    var systemAssistant agent.SystemAssistant
    if cfg.withAssistant {
        sa, err := agent.NewACPAssistant(runtime, providerReg, "system")
        switch {
        case errors.Is(err, agent.ErrProviderBinaryMissing):
            log.Printf("Warning: %v — System Assistant disabled", err)
        case err != nil:
            return fmt.Errorf("system assistant: %w", err)
        default:
            systemAssistant = sa
            log.Printf("System Assistant ready (provider=%s)", sa.Provider().Name)
        }
    }
    dispatcher := orchestrator.NewDispatcher(cmdRegistry, mediator, systemAssistant, workflowReg)
    ```
  * Replace the existing `nil` argument at line 152.
* **Verification:**
  * Boot with default config → System Assistant uses OpenCode; type `What's a goroutine?` → streamed response in TUI.
  * Edit `config.yaml` → `assistants.system.provider: codex`; restart → log line `System Assistant ready (provider=codex)`.
  * Boot when the chosen Provider's binary is absent → warning printed, REPL still boots; freeform input shows "System Assistant disabled (provider %q not installed)".
  * Boot with `--with-assistant=false`, type freeform text → see "freeform input not supported (run with --with-assistant)".

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.19.1  | 3.1 (worker), 3.12.1 (Runtime), 3.22.3 (Provider Registry) |
| 3.19.2  | 3.19.1, existing `Dispatcher` |
| 3.19.3  | 3.19.2, 3.4 (fanout), 3.8, 3.9 |
| 3.19.4  | existing `InteractionRecorder` |
| 3.19.5  | 3.19.1–3.19.4, 3.22.5 (provider wiring in main.go) |

---

## Out of Scope

* Tool calls from the System Assistant — keep it Q&A-only for now (no `tool_call` interception path).
* Long-term memory for the assistant — each session starts fresh.
* RAG / docs grounding — explicitly punted to a future iteration.
