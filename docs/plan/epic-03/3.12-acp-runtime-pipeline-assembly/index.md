# User Story 3.12: ACP Runtime Pipeline Assembly (Supports EPIC-02)

* **As the** Go Orchestrator,
* **I want to** wire the ACP `Pipeline`, `Interceptor`, `StatusMapper`, `Fanout`, `BufferRegistry`, and `PipelineConsumer` into a single goroutine started at session boot,
* **So that** events emitted by the ACP worker actually flow through the interceptor → status translation → UI fanout → ring buffer chain that Stories 3.4, 3.5, 3.6, 3.7, and 3.10 already implemented in isolation.
* *Acceptance Criteria:*
    * On startup, when an active session has a project, the orchestrator builds one `Pipeline` per worker and starts a single consumer goroutine.
    * `tool_call` events emitted by the worker are intercepted and gated by the project permission whitelist before reaching the UI mediator.
    * `session/update` chunks reach the registered UI adapter (TUI/CUI) via the `Fanout`, not directly from `worker.Out()`.
    * `BufferRegistry` is non-nil in `ACPCommandDeps` so `/summary` reads real events instead of returning "Event buffer not available."
    * Project permissions are loaded from `project_permission` once at session boot and refreshed whenever the project switches.
