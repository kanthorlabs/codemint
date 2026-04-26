# Tasks: 1.13 UI Adapter Interface Contract

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.13-ui-adapter-interface-contract/`
**Tech Stack:** Go, interfaces, context-based cancellation

---

## Task 1.13.1: Define UIEvent Type for Event Notifications
* **Action:** Create or update `internal/ui/adapter.go` or `internal/registry/commands.go`.
* **Details:**
  * Define `UIEvent` struct with fields:
    * `Type string` - Event category (e.g., "task_started", "task_completed", "agent_crashed")
    * `TaskID string` - Optional, the task this event relates to
    * `Message string` - Human-readable description
    * `Payload any` - Optional structured data for UI rendering
  * Events are fire-and-forget notifications, not blocking prompts.

## Task 1.13.2: Define UIAdapter Interface with Both Methods
* **Action:** Update `internal/ui/adapter.go`.
* **Details:**
  * Interface must include:
    * `NotifyEvent(event UIEvent)` - Non-blocking event broadcast
    * `PromptDecision(ctx context.Context, req PromptRequest) PromptResponse` - Blocking decision prompt
  * `NotifyEvent` enables real-time UI updates (progress bars, status changes).
  * `PromptDecision` blocks until user responds or context canceled.
* **Status:** ⚠️ Partial - `PromptDecision` exists (adapter.go line 17), `NotifyEvent` missing

## Task 1.13.3: Define PromptRequest and PromptResponse Types
* **Action:** Update `internal/registry/commands.go`.
* **Details:**
  * `PromptRequest` struct with:
    * `TaskID string` - The task requiring decision
    * `Message string` - Prompt text
    * `Options []string` - Available choices (e.g., ["approve", "revert", "cancel"])
  * `PromptResponse` struct with:
    * `Selected string` - User's chosen option
    * `Cancelled bool` - True if context was canceled before selection
* **Status:** ✅ Implemented (commands.go lines 105-112)

## Task 1.13.4: Update UIMediator to Use NotifyEvent
* **Action:** Update `internal/ui/mediator.go`.
* **Details:**
  * Add `NotifyAll(event UIEvent)` method to broadcast events to all registered adapters.
  * Iterate over registered adapters and call `NotifyEvent` on each.
  * Fire-and-forget: do not wait for adapter acknowledgment.

## Task 1.13.5: Create Mock UIAdapter for Testing
* **Action:** Create `internal/ui/mock_adapter.go` or use in test files.
* **Details:**
  * Implement `MockUIAdapter` satisfying `UIAdapter` interface.
  * Store received events in slice for test assertions.
  * Allow pre-configured `PromptResponse` return values.
  * Support simulating context cancellation.

## Task 1.13.6: Write Interface Contract Tests
* **Action:** Update `internal/ui/mediator_test.go`.
* **Details:**
  * Test `NotifyEvent` broadcasts to multiple adapters.
  * Test `PromptDecision` returns first responder's selection.
  * Test context cancellation dismisses prompt on all adapters.
  * Verify interface compile-time satisfaction with `var _ UIAdapter = (*ConcreteType)(nil)`.
