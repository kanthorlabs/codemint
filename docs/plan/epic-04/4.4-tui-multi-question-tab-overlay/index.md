# User Story 4.4: TUI Multi-Question Tab Overlay

* **As a** Developer using the terminal,
* **I want** batched agent questions to render as a tabbed overlay,
* **So that** I can cleanly navigate and answer multiple questions at once.
* *Acceptance Criteria:*
    * When an agent sends an array of questions, the TUI intercepts and renders a Tabbed Overlay.
    * `Tab` key cycles through the question tabs.
    * The final tab is hardcoded as `[Confirmation]`, which compiles the answers into a single JSON-RPC response when `[ OK ]` is selected.
