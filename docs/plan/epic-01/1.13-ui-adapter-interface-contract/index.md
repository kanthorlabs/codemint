# User Story 1.13: UI Adapter Interface Contract

* **As a** Developer,
* **I want** the core orchestrator to use a strict `UIAdapter` Go interface,
* **So that** the backend is entirely decoupled from the Bubble Tea TUI or Telegram CUI logic.
* *Acceptance Criteria:*
    * Go interface defined with `NotifyEvent(event UIEvent)` and `PromptDecision(prompt PromptData) (Response, error)`.