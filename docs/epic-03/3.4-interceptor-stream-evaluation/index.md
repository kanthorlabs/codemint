# User Story 3.4: Interceptor Stream Evaluation

* **As the** Go Orchestrator,
* **I want to** intercept all `tool-call` JSON-RPC events from the ACP Agent before they reach the UI or database,
* **So that** I can evaluate them against the project's permission whitelist.
* *Acceptance Criteria:*
    * A middleware intercepts `stdout` messages matching `{"type": "tool-call"}`.
