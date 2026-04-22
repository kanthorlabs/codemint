# User Story 3.5: Whitelisted Command Execution

* **As the** Go Orchestrator,
* **I want to** automatically execute intercepted commands that match the `project_permission` whitelist,
* **So that** the agent can operate autonomously and the human is not disturbed.
* *Acceptance Criteria:*
    * If the command matches `allowed_commands` and targets `allowed_directories`, the backend executes it locally.
    * The Go backend injects the success result back into the agent's `stdin`.
    * The database task status remains `processing`.
