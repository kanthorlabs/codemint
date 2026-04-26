# Tasks: 3.18 Mediator First-In-Wins Broadcast

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/ui/`, `internal/registry/`
**Tech Stack:** Go, `context`, `select`, channels
**Priority:** P1 (single-adapter behavior is fine today; needed before multi-UI)

---

## Task 3.18.1: Audit Current Mediator Behavior

* **Action:** Document what `mediator.PromptDecision` does today before changing it.
* **Details:**
  * Read `internal/ui/mediator.go`. If the current implementation forwards to a single adapter and ignores the rest, capture this in `internal/ui/mediator_test.go` as a regression baseline (`TestMediator_Legacy_SingleAdapter`).
  * If it already broadcasts but doesn't cancel losers, only the cancel path needs work — note this in the task PR description so reviewers see the diff scope.
* **Verification:**
  * Baseline test passes against current code (proves the audit is honest).

---

## Task 3.18.2: Concurrent Broadcast With Cancel-the-Losers

* **Action:** Implement true fan-out.
* **Details:**
  * In `mediator.go`:
    ```go
    func (m *UIMediator) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
        adapters := m.snapshotAdapters()
        if len(adapters) == 0 { return errResponse(ErrNoAdapters) }
        promptCtx, cancel := context.WithCancel(ctx)
        defer cancel()

        type result struct{ idx int; resp registry.PromptResponse }
        resultCh := make(chan result, len(adapters))
        for i, a := range adapters {
            go func(i int, a UIAdapter) {
                resultCh <- result{i, a.PromptDecision(promptCtx, req)}
            }(i, a)
        }
        var firstErr error
        for received := 0; received < len(adapters); received++ {
            r := <-resultCh
            if r.resp.Err == nil {
                cancel() // signal others to abort
                m.cancelOthers(adapters, i, req.ID)
                return r.resp
            }
            if firstErr == nil { firstErr = r.resp.Err }
        }
        return errResponse(fmt.Errorf("%w: %v", ErrAllAdaptersFailed, firstErr))
    }
    ```
  * `cancelOthers` calls `adapter.CancelPrompt(req.ID)` on every adapter except the winner.
* **Verification:**
  * `TestMediator_FirstInWins` — two stub adapters, one resolves at 10ms, the other at 100ms; mediator returns the fast result; slow adapter's `CancelPrompt` was called.
  * `TestMediator_AllFail` — both stubs return errors; mediator returns `ErrAllAdaptersFailed` wrapping the first.

---

## Task 3.18.3: Adapter `CancelPrompt` Implementation

* **Action:** Confirm both adapters honor `CancelPrompt`.
* **Details:**
  * `TUIAdapter.CancelPrompt(id string)`: emits a "prompt cancelled" line and unregisters the pending prompt.
  * `CUIAdapter.CancelPrompt(id string)`: deletes the entry from `pendingPrompts` and writes a log line so daemon users see the dismissal.
  * Add `CancelPrompt` to the `UIAdapter` interface if not already present.
* **Verification:**
  * `TestTUIAdapter_CancelPrompt_RemovesPending` — call sets internal map to empty.
  * `TestCUIAdapter_CancelPrompt_LogsAndRemoves` — log file contains the cancel line.

---

## Task 3.18.4: Freeform Prompt Kind

* **Action:** Add a kind that accepts arbitrary text instead of a single option.
* **Details:**
  * Extend `registry.PromptRequest`:
    ```go
    type PromptKind int
    const (
        PromptSingleSelect PromptKind = iota
        PromptFreeform
    )
    type PromptRequest struct {
        ID      string
        Kind    PromptKind
        Title   string
        Body    string
        Options []PromptOption
    }
    type PromptResponse struct {
        OptionID string // populated for SingleSelect
        Text     string // populated for Freeform
        Err      error
    }
    ```
  * TUIAdapter: freeform prompt reads a single line from stdin.
  * CUIAdapter: freeform prompt logs `(awaiting freeform reply for prompt-X — use /reply X <text>)` and waits on `pendingPrompts`. Add `/reply <id> <text>` to `daemon_commands.go`.
* **Verification:**
  * `TestExecutor_Retrospective_FreeformResolves` — full integration: scheduler runs a retrospective; freeform answer captured into `task.output`.
  * `TestCUIAdapter_Reply_ResolvesPending` — `/reply` resolves the prompt and the orchestrator unblocks.

---

## Task 3.18.5: Context-Cancellation Test

* **Action:** Make sure SIGINT during a prompt doesn't leave goroutines hanging.
* **Details:**
  * `TestMediator_ParentContextCancel_AbortsAdapters` — start a prompt with a stub adapter that blocks 5s; cancel the parent context; assert mediator returns within 250ms with `ctx.Err()` and the adapter's goroutine drains.
  * Use `goleak` (or a manual goroutine-count check via `runtime.NumGoroutine`) to catch leaks.
* **Verification:**
  * Test passes; no leaked goroutines.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.18.1  | (none) |
| 3.18.2  | 3.18.1 |
| 3.18.3  | 3.18.2, 3.8, 3.9 |
| 3.18.4  | 3.18.2, 3.17.3 |
| 3.18.5  | 3.18.2 |

---

## Out of Scope

* Inline keyboard rendering (EPIC-04 §4.7).
* Multi-question tab overlay (EPIC-04 §4.4).
* Conversational revision flow (EPIC-04 §5.1).
