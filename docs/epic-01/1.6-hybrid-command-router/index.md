# User Story 1.6: Hybrid Command Router

* **As a** Developer,
* **I want** my input to be parsed by a dual-path router that handles deterministic commands instantly and routes natural language to an AI,
* **So that** I don't waste API tokens on simple slash commands.
* *Acceptance Criteria:*
    * The parser uses strict prefix matching (e.g., `^/`) to catch valid slash commands at the very beginning of the string.
    * Natural language is sent to the Coordinator AI, which uses a Binary Context Gateway to determine if it requires `working_dir` context.