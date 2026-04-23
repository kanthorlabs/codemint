# User Story 4.12: The Knowledge Review Interface (Supports EPIC-05)

* **As a** Developer,
* **I want** the UI to render the Archivist's pending insights via the `/review` command,
* **So that** I can easily Accept, Edit, or Dismiss new learnings.
* *Acceptance Criteria:*
    * The UI displays a non-intrusive alert (e.g., `[ 🔔 Review Project Memory ]` in the TUI status bar) when the backend scheduler triggers.
    * Executing `/review` opens a Level 1 List View of pending items.
    * Selecting an item opens a Level 2 Detail View with `[Accept]`, `[Dismiss]`, and `[Edit]` actions.
