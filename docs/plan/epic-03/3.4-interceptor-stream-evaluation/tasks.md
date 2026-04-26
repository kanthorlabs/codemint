# Tasks: 3.4 Interceptor Stream Evaluation

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/acp/`, `internal/orchestrator/`
**Tech Stack:** Go, channels, JSON-RPC
**Priority:** P0 (security-critical pre-req for 3.5/3.6)

---

## Task 3.4.1: Event Type Catalog

* **Action:** Enumerate the ACP event subset CodeMint must classify.
* **Details:**
  * Add `internal/acp/events.go`:
    ```go
    type EventKind int
    const (
        EventUnknown EventKind = iota
        EventThinking          // agent_thought_chunk
        EventMessage           // agent_message_chunk / user_message_chunk
        EventPlan              // plan
        EventToolCall          // tool_call (pre-execution announcement)
        EventToolUpdate        // tool_call_update
        EventPermissionRequest // session/request_permission (this is the "intercept me" signal)
        EventTurnStart
        EventTurnEnd
    )
    type Event struct {
        Kind     EventKind
        ACPSessionID string
        Raw      json.RawMessage
        // For tool calls / permission requests:
        ToolName string
        ToolArgs json.RawMessage
        Command  string  // shell command extracted from args, when applicable
        Cwd      string
    }
    func Classify(msg Message) Event
    ```
  * `Classify` parses `session/update` notifications and `session/request_permission` requests; everything else maps to `EventUnknown`.
* **Verification:**
  * Table-driven test with golden fixtures for each `EventKind`.
  * Unknown payloads do not panic; `Raw` is preserved.

---

## Task 3.4.2: Event Pipeline (Worker → Interceptor → Fanout)

* **Action:** Wrap the worker's raw output channel with a classifying pipeline.
* **Details:**
  * Create `internal/acp/pipeline.go`:
    ```go
    type Pipeline struct {
        in       <-chan Message      // from Worker.out
        Events   chan Event          // classified, non-intercepted
        Halted   chan Event          // tool_call / permission events for the interceptor
    }
    func NewPipeline(in <-chan Message) *Pipeline
    func (p *Pipeline) Run(ctx context.Context)
    ```
  * For each incoming `Message`, call `Classify`. If `EventToolCall` or `EventPermissionRequest`, push to `Halted` and **do not** forward to `Events`. Everything else → `Events`.
  * Both channels buffered 256; on full buffer, drop oldest and log a counter (avoid blocking the worker reader).
* **Verification:**
  * `TestPipeline_Splits` feeds a mixed stream and asserts tool events end up only in `Halted`.
  * Pipeline shuts down cleanly when the input channel closes.

---

## Task 3.4.3: Interceptor Skeleton

* **Action:** Build the empty-shell interceptor that consumes `Halted` and (for now) re-emits everything as awaiting human approval.
* **Details:**
  * Create `internal/orchestrator/interceptor.go`:
    ```go
    type Interceptor struct {
        permRepo  repository.ProjectPermissionRepository
        taskRepo  repository.TaskRepository
        ui        registry.UIMediator
        worker    *acp.Worker // for sending tool results / errors back
    }
    func (i *Interceptor) Handle(ctx context.Context, ev acp.Event, currentTaskID string)
    ```
  * In this story, `Handle` simply forwards to a "no-match" path that pings the UI with `RenderMessage("[ACP] Tool call halted: <name> <command>")`. Story 3.5 adds whitelist execution; Story 3.6 adds the formal awaiting-approval flow.
  * Wire the interceptor in `main.go`: subscribe to `pipeline.Halted` via a goroutine bound to `ctx`.
* **Verification:**
  * Drive the worker with a stub that emits a `tool_call` event; confirm the UI receives the halt message and the event never reaches the verbosity/streaming UI path.

---

## Task 3.4.4: Non-Tool Fanout to UI Mediator

* **Action:** Route classified non-tool events to a single broadcast point so 3.8/3.9 can subscribe.
* **Details:**
  * In the same goroutine that consumes `Pipeline.Events`, call `mediator.NotifyEvent(...)` with a typed `registry.UIEvent` carrying `Kind` and `Raw` payload.
  * Add a new event type to `registry.UIEventType`:
    ```go
    EventACPStream UIEventType = "acp_stream"
    ```
  * Keep the conversion lossless — adapters decide what to render.
* **Verification:**
  * Mediator receives `EventACPStream` events for non-tool messages.
  * Tool events are absent from the mediator stream (assert via a recording adapter in tests).

---

## Dependencies

| Task  | Depends On |
|-------|------------|
| 3.4.1 | 3.1.1 |
| 3.4.2 | 3.4.1 |
| 3.4.3 | 3.4.2, 1.12 (project_permission schema) |
| 3.4.4 | 3.4.2, 1.10 (UIMediator) |

---

## Out of Scope

* Whitelist matching logic (3.5).
* Awaiting-approval prompt flow (3.6).
* DB status transitions (3.7).
