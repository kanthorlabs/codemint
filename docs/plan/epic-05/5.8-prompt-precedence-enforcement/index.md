# User Story 5.8: Prompt Precedence Enforcement

* **As the** Go Orchestrator,
* **I want to** format the System Prompt to strictly order the injected memory,
* **So that** the AI doesn't argue with the user when a temporary prompt contradicts an old preference.
* *Acceptance Criteria:*
    * The injected system prompt enforces the Hierarchy of Authority: 1. Current Prompt -> 2. Project Memory -> 3. Global Rules.
