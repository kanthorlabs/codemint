# Manual Test: Cross-Interface Input Multiplexer

This document describes how to manually test the input multiplexer with the CUI stub backend.

## Prerequisites

- Built CodeMint binary: `go build -o build/codemint ./cmd/codemint`
- A terminal that supports standard input

## Test Procedure

### Step 1: Start CodeMint in Hybrid Mode with Stub Inbound

```bash
./build/codemint --mode=hybrid --cui-stub-inbound
```

> **Note:** The `--cui-stub-inbound` flag enables the stub inbound backend for testing.
> This flag is hidden and intended for testing only.

### Step 2: Verify TUI Input Works

Type a question in the terminal:

```
> What's a goroutine?
```

**Expected Result:**
- The System Assistant responds with an explanation
- The response appears in the terminal
- The response is also logged to `~/.local/state/codemint/cui.log`

### Step 3: Test CUI Stub Input (Future)

> **Note:** Full CUI stub injection requires the `--inject` REPL command or
> a FIFO bridge, which will be implemented in EPIC-04.

For now, the stub can be tested programmatically via the test suite:

```bash
go test -v ./internal/orchestrator/... -run "InputMultiplexer|StubInbound|MultiSource"
```

### Step 4: Verify Activity Recording

After interacting with CodeMint, check that the `/activity` command shows
the source of each input:

```
> /activity
```

**Expected Result:**
- Command interactions show `source=tui`
- When CUI inbound is enabled, messages from that source show `source=cui-stub`

## Automated E2E Test

The automated test (`internal/orchestrator/system_assistant_e2e_test.go`) verifies:

1. **Cross-Interface Loop** - Messages from both TUI and CUI sources reach the dispatcher
2. **Source Attribution** - Input source and user ID are properly tracked
3. **Per-Source FIFO** - Message ordering is preserved per-source
4. **Stub Integration** - The StubInbound correctly injects messages

Run the tests:

```bash
go test -v ./internal/orchestrator/... -run "InputMultiplexer|StubInbound|MultiSource" -count=10
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        InputMultiplexer                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │ TUI Source  │  │ CUI Source  │  │ Future Src  │             │
│  │ (stdin)     │  │ (Telegram)  │  │ (Slack)     │             │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘             │
│         │                │                │                     │
│         v                v                v                     │
│  ┌────────────────────────────────────────────────┐            │
│  │              Shared Output Channel              │            │
│  │         (back-pressure aware, FIFO)            │            │
│  └──────────────────────┬─────────────────────────┘            │
└─────────────────────────┼───────────────────────────────────────┘
                          │
                          v
┌─────────────────────────────────────────────────────────────────┐
│                         Dispatcher                              │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  DispatchInbound(ctx, msg)                               │  │
│  │    1. Set input source in ActiveSession                  │  │
│  │    2. Dispatch(ctx, msg.Text)                            │  │
│  │    3. Clear input source                                 │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                          │
                          v
┌─────────────────────────────────────────────────────────────────┐
│                    InteractionRecorder                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  RecordWithSource(ctx, ..., source, userID, ...)         │  │
│  │    - Stores source in task.input JSON                    │  │
│  │    - Enables audit trails per-source                     │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Files Involved

| File | Purpose |
|------|---------|
| `internal/input/message.go` | `InboundMessage` type definition |
| `internal/input/multiplexer.go` | `Multiplexer` implementation |
| `internal/ui/cui_inbound.go` | `InboundBackend` interface |
| `internal/ui/cui_inbound_stub.go` | `StubInbound` test backend |
| `internal/repl/loop.go` | `LoopWithMux` function |
| `internal/orchestrator/dispatcher.go` | `DispatchInbound` method |
| `internal/orchestrator/active_session.go` | Input source tracking |
| `internal/orchestrator/interaction_recorder.go` | Source-aware recording |
