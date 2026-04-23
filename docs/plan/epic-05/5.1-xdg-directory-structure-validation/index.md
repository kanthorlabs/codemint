# User Story 5.1: XDG Directory Structure Validation

* **As the** Go Orchestrator,
* **I want to** validate and create the complete LLM Wiki directory tree inside `~/.local/share/codemint/memory/<project_id>/` upon session start,
* **So that** The Archivist has the exact folder structure required for staging and archiving.
* *Acceptance Criteria:*
    * The system ensures the existence of `inbox/insights/unverified/`, `inbox/insights/archive/`, `history/`, `architecture/archive/`, and `patterns/bugs/`.
    * If `index.md`, `preferences.md`, or `decisions.md` do not exist, they are initialized as empty files.
