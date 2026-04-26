# User Story 3.18: Mediator First-In-Wins Broadcast (Supports EPIC-04 §2.2)

* **As the** Go Orchestrator,
* **I want** the `UIMediator.PromptDecision` call to broadcast to every registered adapter concurrently and accept whichever response lands first,
* **So that** EPIC-04 §2.2's "First-In-Wins" contract holds when both TUI and CUI (or future Telegram/Slack) adapters answer the same prompt.
* *Acceptance Criteria:*
    * `PromptDecision` fans out to N adapters via N goroutines.
    * The first non-error response wins; the mediator immediately calls `CancelPrompt(promptID)` on every other adapter so their UIs dismiss the dialog.
    * If all adapters return errors, the mediator returns the first error and a sentinel `ErrAllAdaptersFailed` wrap so callers can distinguish "user said no" from "all UIs broken".
    * Cancellation respects `ctx`: a cancelled parent context cancels all in-flight adapter calls within 250ms.
    * Adapters must support a `freeform` prompt kind alongside the existing single-select options (needed by Story 3.14.5 retrospectives).
