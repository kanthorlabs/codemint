# User Story 3.6: Blocked Command Escalation

* **As the** Go Orchestrator,
* **I want to** halt the stream and escalate non-whitelisted commands to the user,
* **So that** the human can explicitly approve or block dangerous CLI commands.
* *Acceptance Criteria:*
    * If the command does not match the whitelist or hits the `blocked_commands` list, the backend halts execution.
    * The task status is updated to `awaiting`.
    * The UI Mediator is pinged for manual human approval.
