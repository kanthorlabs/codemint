# User Story 3.2: Context Boundary Clearing

* **As the** Go Orchestrator,
* **I want to** send a `clear` or `reset` command via the JSON-RPC stream at the boundary of every User Story,
* **So that** the agent's context window is flushed, keeping token usage lean without killing the binary.
* *Acceptance Criteria:*
    * When the scheduler detects a transition to a new `seq_story` integer, it injects a reset command to the ACP agent before sending the next task.
