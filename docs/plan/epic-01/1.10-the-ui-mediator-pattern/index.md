# User Story 1.10: The UI Mediator Pattern

* **As the** Go Orchestrator,
* **I want** a `UIMediator` to handle all communication between the core execution loop and the registered `UIAdapter` instances,
* **So that** the core scheduler remains completely decoupled from UI race conditions and broadcast logic.
* *Acceptance Criteria:*
    * The `UIMediator` implements a registration pattern (e.g., `RegisterAdapter(adapter UIAdapter)`).
    * When `PromptDecision` is called, the Mediator broadcasts the request to all registered adapters concurrently.
    * The Mediator uses a Go `select` block to capture the *first* response received.
    * Upon receiving the first response, the Mediator sends a cancellation/sync signal to all other adapters so they dismiss their pending prompts.