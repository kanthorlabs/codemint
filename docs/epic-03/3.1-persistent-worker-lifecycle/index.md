# User Story 3.1: Persistent Worker Lifecycle

* **As the** Go Orchestrator,
* **I want to** spawn exactly one background `os/exec` process for the ACP agent (e.g., OpenCode) per active session,
* **So that** I prevent high token costs and cold-start latency associated with spawning a new process per task.
* *Acceptance Criteria:*
    * The system maintains a 1:1 mapping between an active CodeMint Session and an `os/exec` process.
    * Tasks within the session are continuously piped into this single process's `stdin`.
