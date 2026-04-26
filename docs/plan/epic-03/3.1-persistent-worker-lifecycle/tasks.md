# Tasks: 3.1 Persistent Worker Lifecycle

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/acp/`, `internal/orchestrator/`
**Tech Stack:** Go, `os/exec`, JSON-RPC over stdio, `sync`, `context`
**Priority:** P0 (foundation — every other 3.x story depends on it)

---

## Task 3.1.1: ACP Wire Protocol Types

* **Action:** Define Go structs for the JSON-RPC 2.0 envelope and the ACP message subset CodeMint cares about.
* **Details:**
  * Create `internal/acp/protocol.go`.
  * Top-level envelope:
    ```go
    type Message struct {
        JSONRPC string          `json:"jsonrpc"` // always "2.0"
        ID      json.RawMessage `json:"id,omitempty"`
        Method  string          `json:"method,omitempty"`
        Params  json.RawMessage `json:"params,omitempty"`
        Result  json.RawMessage `json:"result,omitempty"`
        Error   *RPCError       `json:"error,omitempty"`
    }
    ```
  * Method constants used by EPIC-03:
    * `initialize`, `session/new`, `session/prompt`, `session/cancel`
    * `session/update` (server → client streaming notifications)
    * `session/request_permission` (tool-call gate)
  * Update payload tagged union:
    ```go
    type SessionUpdate struct {
        SessionID  string          `json:"sessionId"`
        Update     UpdateBody      `json:"update"`
    }
    type UpdateBody struct {
        Kind string          `json:"sessionUpdate"` // user_message_chunk | agent_message_chunk | agent_thought_chunk | tool_call | tool_call_update | plan
        Raw  json.RawMessage `json:"-"`             // keep original for circular buffer (3.10)
    }
    ```
  * Provide marshal helpers `NewRequest(id, method, params)`, `NewResponse(id, result)`, `NewError(id, code, msg)`.
* **Verification:**
  * `go test ./internal/acp -run TestProtocol_RoundTrip` round-trips every method through `json.Marshal` / `json.Unmarshal`.
  * Unknown `sessionUpdate.kind` parses without error and preserves `Raw`.

---

## Task 3.1.2: ACP Worker Process Wrapper

* **Action:** Build `acp.Worker` that spawns one ACP-compatible CLI (default: `opencode acp`) and exposes Send / Recv channels.
* **Details:**
  * Create `internal/acp/worker.go`.
  * Constructor:
    ```go
    type WorkerConfig struct {
        Command string   // default: "opencode"
        Args    []string // default: ["acp"]
        Cwd     string   // project workingDir
        Env     []string // inherited + appended
    }
    func Spawn(ctx context.Context, cfg WorkerConfig) (*Worker, error)
    ```
  * Internals:
    * `cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)`; pipe `Stdin`, `Stdout`, `Stderr`.
    * Goroutine A: read stdout line-delimited, parse `acp.Message`, push to `out chan Message` (buffered 256).
    * Goroutine B: read stderr, route to `slog.Debug("acp.stderr", "line", line)`.
    * `Send(msg Message) error` writes `json.Marshal(msg) + "\n"` under a `sync.Mutex` (single writer to stdin).
  * Lifecycle method `Wait()` returns process exit error; `Pid()` returns os PID.
  * Auto-perform `initialize` handshake inside `Spawn` (send `initialize` request, await `result`, store agent capabilities on the worker).
  * Add `Capabilities() InitializeResult` getter so callers can branch on what the agent supports.
* **Verification:**
  * `go test ./internal/acp -run TestWorker_Echo` spawns `cat` (stub) and confirms request/response loop works without hanging.
  * Killing the process closes `out` channel within 1s.
  * Initialize handshake fails with a typed error if no `initialize` response within `cfg.HandshakeTimeout` (default 10s).

---

## Task 3.1.3: Worker Registry (1:1 Session → Worker)

* **Action:** Maintain a process-per-session map guarded by a mutex.
* **Details:**
  * Create `internal/acp/registry.go`:
    ```go
    type Registry struct {
        mu      sync.RWMutex
        workers map[string]*Worker // key = session.ID
        cfg     WorkerConfig
    }
    func NewRegistry(cfg WorkerConfig) *Registry
    func (r *Registry) GetOrSpawn(ctx context.Context, sess *domain.Session, project *domain.Project) (*Worker, error)
    func (r *Registry) Get(sessionID string) (*Worker, bool)
    func (r *Registry) Stop(ctx context.Context, sessionID string) error
    func (r *Registry) StopAll(ctx context.Context) error
    ```
  * `GetOrSpawn` is idempotent: returns the existing worker if one is alive, otherwise spawns and stores.
  * Override the worker command via env var `CODEMINT_ACP_CMD` (default `opencode acp`) so tests can swap in a stub.
* **Verification:**
  * `TestRegistry_GetOrSpawn_Idempotent` — two concurrent calls return the same `*Worker`.
  * `TestRegistry_Stop_RemovesEntry` — after Stop, `Get` returns `false`.

---

## Task 3.1.4: Wire Registry Into `main.go`

* **Action:** Construct the registry at startup and bind it to the active session.
* **Details:**
  * In `cmd/codemint/main.go` after Step 11 (session load), create `acpRegistry := acp.NewRegistry(acp.WorkerConfig{...})`.
  * If the active session has a project, call `acpRegistry.GetOrSpawn(ctx, session, project)` lazily — i.e., wrap it behind a getter `func() (*acp.Worker, error)` passed into the dispatcher. Do not block startup if OpenCode is missing on PATH; surface a friendly warning and continue.
  * On REPL exit, `defer acpRegistry.StopAll(shutdownCtx)` with a 5s deadline.
  * Expose the registry through `ActiveSession.ACP` (add field) so commands can reach it.
* **Verification:**
  * Startup logs `acp: worker not started: opencode binary not found on PATH` when the binary is absent and the REPL still boots.
  * With `opencode` installed, first command that needs the worker spawns exactly one process (verify via `pgrep -f "opencode acp"`).

---

## Task 3.1.5: `/acp` REPL Command (Manual Test Hook)

* **Action:** Add a developer-facing command that pipes a freeform prompt straight into the worker so EPIC-03 can be exercised without EPIC-02 being ready.
* **Details:**
  * Create `internal/repl/acp_commands.go`:
    * `/acp <prompt>` → ensures worker, calls `session/new` if no ACP session yet, sends `session/prompt` with the user text, streams `session/update` notifications back to the UI mediator (one `RenderMessage` per chunk for now — refined in 3.8/3.9).
    * `/acp-status` → prints `pid`, `cwd`, capabilities, current ACP session ID.
    * `/acp-stop` → calls `Registry.Stop(sessionID)`.
  * Register with `SupportedModes: []ClientMode{ClientModeCLI, ClientModeDaemon}`.
  * Persist the round-trip as a Coordination task (`type=3`, status=5) like other commands so `/activity` shows it.
* **Verification:**
  * `./build/codemint` → `/acp say hi` prints OpenCode's streamed response.
  * `/acp-status` shows non-zero pid.
  * `/acp-stop` kills the process; subsequent `/acp` re-spawns.

---

## Dependencies

| Task   | Depends On |
|--------|------------|
| 3.1.2  | 3.1.1 |
| 3.1.3  | 3.1.2 |
| 3.1.4  | 3.1.3 |
| 3.1.5  | 3.1.4 |

---

## Out of Scope

* Tool-call interception (3.4–3.6).
* Status mapping (3.7).
* UI bandwidth split (3.8/3.9).
* Circular buffer / `/summary` (3.10).
