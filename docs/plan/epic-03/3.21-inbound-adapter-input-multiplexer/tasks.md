# Tasks: 3.21 Inbound Adapter Input Multiplexer

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/repl/`, `internal/ui/`, `internal/orchestrator/`, `cmd/codemint/`
**Tech Stack:** Go, channels, `bufio`
**Priority:** P0 for the cross-interface acceptance test (3.19 + 3.20 don't close the loop without it).

---

## Task 3.21.1: `InputMultiplexer` Type

* **Action:** Build the multiplexer.
* **Details:**
  * Create `internal/repl/input_multiplexer.go`:
    ```go
    type InboundMessage struct {
        Source string    // "tui", "cui-telegram", ...
        UserID string    // optional, source-specific
        Text   string
        At     time.Time
    }
    type InputMultiplexer struct {
        out chan InboundMessage
        // per-source bounded buffers
    }
    func NewInputMultiplexer() *InputMultiplexer
    func (m *InputMultiplexer) RegisterSource(source string, capacity int) chan<- InboundMessage
    func (m *InputMultiplexer) Recv() <-chan InboundMessage
    func (m *InputMultiplexer) Close()
    ```
  * Each call to `RegisterSource` returns a write-only channel sized at `capacity` (default 32). A goroutine forwards from the source channel to the shared `out` channel.
  * Drop policy: if `out` is full and a source is stalled, drop the **oldest** queued message from that source (`slog.Warn("input: dropping oldest message", "source", ...)`).
* **Verification:**
  * `TestMultiplexer_FIFO_PerSource` — interleaved sends preserve per-source order.
  * `TestMultiplexer_DropOldest_WhenFull` — fill source channel, send one more, oldest drops, newest stays.
  * `TestMultiplexer_Close_StopsAllForwarders` — no leaked goroutines after `Close`.

---

## Task 3.21.2: TUI Stdin Source

* **Action:** Move the existing `bufio.Scanner` over `os.Stdin` into a multiplexer source.
* **Details:**
  * In `internal/repl/loop.go`, before the read loop:
    ```go
    inbox := mux.RegisterSource("tui", 16)
    go func() {
        scanner := bufio.NewScanner(stdin)
        for scanner.Scan() {
            inbox <- InboundMessage{Source: "tui", Text: scanner.Text(), At: time.Now()}
        }
        close(inbox)
    }()
    for msg := range mux.Recv() { dispatcher.DispatchInput(ctx, msg) }
    ```
  * `Dispatcher.DispatchInput` already exists; extend its signature to accept the full `InboundMessage` so it can stash `Source` and `UserID` into the recorded Coordination task.
* **Verification:**
  * Boot in `--mode=cli` → typing on the terminal still works exactly as before.
  * `/activity` rows now show the source (e.g., `source=tui`).

---

## Task 3.21.3: CUI Inbound Stub Backend

* **Action:** Provide a deterministic backend for tests and local hybrid runs.
* **Details:**
  * Create `internal/ui/cui_inbound_stub.go`:
    ```go
    type StubInbound struct {
        ch chan repl.InboundMessage
    }
    func NewStubInbound(mux *repl.InputMultiplexer) *StubInbound {
        return &StubInbound{ch: mux.RegisterSource("cui-stub", 16)}
    }
    func (s *StubInbound) Inject(text string) {
        s.ch <- repl.InboundMessage{Source: "cui-stub", Text: text, At: time.Now()}
    }
    ```
  * Wired in `main.go` only when `--cui-stub-inbound` flag is set (default off; a hidden testing-only flag).
* **Verification:**
  * `TestStubInbound_Inject_DispatchesViaMux` — inject "hello" → dispatcher sees the message with `Source=cui-stub`.

---

## Task 3.21.4: Telegram Inbound Hook (Interface Only)

* **Action:** Reserve the integration point without writing the Telegram client.
* **Details:**
  * Define `internal/ui/cui_inbound.go`:
    ```go
    type InboundBackend interface {
        Start(ctx context.Context, sink chan<- repl.InboundMessage) error
        Stop(ctx context.Context) error
    }
    ```
  * `CUIAdapter.SetInboundBackend(b InboundBackend)` plus `cuiAdapter.StartInbound(ctx)` — wires the backend to the multiplexer's CUI source.
  * EPIC-04 §4.5 will implement `telegramInboundBackend` against this interface. Keep the file empty of Telegram code; only document the contract.
* **Verification:**
  * Compile-time check: `StubInbound` in 3.21.3 implements `InboundBackend` (or a thin wrapper does). No production code imports `telegram-bot-api` in this story.

---

## Task 3.21.5: End-to-End Test Recipe

* **Action:** Capture the full cross-interface acceptance test the user asked for, even before Telegram lands.
* **Details:**
  * Create `internal/orchestrator/system_assistant_e2e_test.go`:
    1. Boot orchestrator with `ClientModeHybrid`, stub ACP worker that echoes "Goroutines are lightweight threads…" to any prompt.
    2. Register TUI adapter against `bytes.Buffer`, register CUI adapter (stub variant logging to another buffer).
    3. Wire `StubInbound` for the CUI source.
    4. **Step A**: Send "What's a goroutine?" via the TUI source. Assert both buffers contain the assistant reply (broadcast).
    5. **Step B**: `stubInbound.Inject("Explain channels too.")`. Assert assistant produces a follow-up reply, again visible in both buffers.
    6. Assert `taskRepo.ListBySession` returns two Coordination rows with the right `source` values.
  * Document the matching manual recipe in `docs/plan/epic-03/3.21-inbound-adapter-input-multiplexer/manual-test.md`:
    * Run `./build/codemint --mode=hybrid --cui-stub-inbound`.
    * Type a question in TUI → see reply locally and in `~/.local/state/codemint/cui.log`.
    * In a second terminal, send the inject command (a small `nc` / fifo bridge or a `--inject` REPL command) → see the assistant respond again, visible in both surfaces.
* **Verification:**
  * E2E test passes deterministically (run with `-count=10` to catch flakes).
  * Manual recipe completes without modifying any other story.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.21.1  | (none) |
| 3.21.2  | 3.21.1, existing REPL loop |
| 3.21.3  | 3.21.1 |
| 3.21.4  | 3.21.1, 3.17.4 (`NotificationPusher` seam) |
| 3.21.5  | 3.19, 3.20, 3.21.1–3.21.3 |

---

## Out of Scope

* Telegram Bot API client implementation — owned by EPIC-04 §4.5.
* Slack inbound — same.
* Authentication / rate-limiting on inbound messages — EPIC-04.
* Per-user routing across sessions — EPIC-04.
