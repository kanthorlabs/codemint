# User Story 1.9: Agent Crash Fallback

* **As the** Go Orchestrator,
* **I want to** catch process panics or crashes from the underlying CLI agent,
* **So that** the system fails gracefully instead of freezing.
* *Acceptance Criteria:*
    * If the `os/exec` process panics or times out, the task's `assignee_id` is automatically reassigned to the human Agent.
    * The task status is forced to `awaiting`.
    * The UI displays: *"Agent crashed. Please manually reconcile the working directory and resolve the task status."*.