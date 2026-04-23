# User Story 2.10: LLM Wiki Context Injection (Supports EPIC-05)

* **As the** Go Orchestrator,
* **I want to** inject the "Hot" Wiki files (`preferences.md`, `decisions.md`, `bugs/index.md`) into the Brainstormer Agent's system prompt during Phase 1,
* **So that** the Task Generator respects the project's historical decisions and doesn't repeat past mistakes.
* *Acceptance Criteria:*
    * Before the LLM generates the task list, it reads `~/.local/share/codemint/memory/<project_id>/`.
    * The System Prompt strictly enforces the Hierarchy of Authority: Current Prompt > Project Memory > Global Rules.
