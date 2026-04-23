# User Story 1.14: XDG Base Directory Scaffolding

* **As the** Go Orchestrator,
* **I want to** initialize local file system directories following the XDG Base Directory Specification on startup,
* **So that** the Adaptive Learning System (LLM Wiki) has a proper place to store knowledge files.
* *Acceptance Criteria:*
    * System automatically creates `~/.local/share/codemint/memory` and `~/.config/codemint/` if they do not exist.