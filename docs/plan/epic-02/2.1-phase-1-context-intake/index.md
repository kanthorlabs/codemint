# User Story 2.1: Phase 1 - Context Intake (The Gatherer)

* **As the** Go Orchestrator,
* **I want** the Gatherer Agent to read directory trees and key files before brainstorming starts,
* **So that** the AI is grounded in the reality of the codebase.
* *Acceptance Criteria:*
    * The system reads the project directory tree and passes it to the LLM.
    * Logic is structured to allow future iteration into a "Targeted Grep Gatherer" to save context tokens.
