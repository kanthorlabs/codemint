# User Story 3.8: TUI High-Bandwidth Streaming

* **As a** Developer using the TUI,
* **I want** my interface to receive raw, high-frequency micro-events (e.g., `thinking`, `file-edit`),
* **So that** I can watch the agent's real-time reasoning and progress.
* *Acceptance Criteria:*
    * The `UIAdapter` implementation for the TUI subscribes to the raw, unthrottled event stream from the ACP worker.
