# User Story 2.11: The Clarifier Agent Handoff (Supports EPIC-04)

* **As the** Go Orchestrator,
* **I want to** explicitly route Mid-Flight Pivots to the dedicated `Clarifier Agent`,
* **So that** natural language revisions are correctly translated into database `UPDATE` queries for existing tasks without hallucinating new scopes.
* *Acceptance Criteria:*
    * When a user clicks `[ ✏️ Revise ]`, the UI prompts: "What should I change?"
    * The user's reply is sent specifically to the Clarifier Agent, which directly overwrites the target SQLite rows and generates a new draft for the UI.
