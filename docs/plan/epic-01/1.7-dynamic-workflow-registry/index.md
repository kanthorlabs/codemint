# User Story 1.7: Dynamic Workflow Registry

* **As the** Go Orchestrator,
* **I want to** load available workflows from a configuration file rather than hardcoding them,
* **So that** the system can be easily expanded with new behaviors.
* *Acceptance Criteria:*
    * Workflows are loaded from `config.yaml` at startup.
    * Workflows are registered into a Go map (`map[int]Workflow`).
    * At minimum, `Project Coding`, `Communication`, and `Daily Checking` workflows are scaffolded.