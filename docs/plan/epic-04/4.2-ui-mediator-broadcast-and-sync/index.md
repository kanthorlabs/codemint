# User Story 4.2: The `UIMediator` Broadcast & Sync

* **As the** Go Orchestrator,
* **I want** the `UIMediator` to manage concurrent prompts across multiple active UIs,
* **So that** I achieve seamless "First-In-Wins" synchronization without race conditions.
* *Acceptance Criteria:*
    * The Mediator broadcasts `PromptDecision` requests to all registered adapters concurrently.
    * It uses a Go `select` block to capture the first response received.
    * Immediately after capturing the response, it calls `CancelPrompt` on all other adapters.
