# User Story 1.17: Workflow Registry from Config

* **As the** Go Orchestrator,
* **I want** workflow definitions loaded from `config.yaml` at startup and registered in a Go map,
* **So that** the Coordinator AI can route requests to the appropriate workflow handler dynamically.
* *Acceptance Criteria:*
    * `config.yaml` supports a `workflows` section defining workflow type, name, and handler mapping.
    * Core workflows scaffolded: `ProjectCoding` (type=0), `Communication` (type=1), `DailyChecking` (type=2).
    * `WorkflowRegistry` struct with `Register`, `Lookup`, `All` methods.
    * Unknown workflow types return a descriptive error.
    * Workflow definitions are validated at startup (no duplicate types, required fields present).
