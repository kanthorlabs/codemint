# User Story 3.11: Execution Agent Memory Injection (Supports EPIC-05)

* **As the** Go Orchestrator,
* **I want to** inject the project's LLM Wiki (e.g., `preferences.md`) into the ACP Agent's context upon startup,
* **So that** OpenCode respects the same coding standards and decisions established during the Brainstorming phase.
* *Acceptance Criteria:*
    * The `stdin` initialization payload sent to the ACP Agent includes the concatenated text of the "Hot" memory files.
