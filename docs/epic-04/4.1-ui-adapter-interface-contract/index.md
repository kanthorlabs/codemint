# User Story 4.1: The `UIAdapter` Interface Contract

* **As a** Developer,
* **I want** both the Terminal UI and Chat UI to implement a strict `UIAdapter` Go interface,
* **So that** the core orchestration engine is completely decoupled from rendering logic.
* *Acceptance Criteria:*
    * The interface defines `NotifyEvent(event UIEvent)` for non-blocking pushes.
    * The interface defines `PromptDecision(prompt PromptData) (Response, error)` for blocking requests.
    * The interface defines `CancelPrompt(promptID string)` to instruct the UI to instantly dismiss a pending prompt.
