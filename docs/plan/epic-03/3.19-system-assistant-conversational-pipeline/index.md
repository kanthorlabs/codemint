# User Story 3.19: System Assistant Conversational Pipeline

* **As a** Developer testing CodeMint before any project workflow exists,
* **I want** freeform (non-slash) text typed into any registered UI to be routed to the **System Assistant** — a session-scoped agent that handles non-project inquiries — and have its response broadcast back to every registered adapter,
* **So that** I can validate the cross-interface plumbing (e.g., ask a question via TUI, see the same conversation thread on Telegram CUI) without waiting for EPIC-02 brainstorming or any project-specific feature to ship.
* *Acceptance Criteria:*
    * `Dispatcher` no longer accepts `nil` for the system assistant; a default `SystemAssistant` is constructed in `main.go` and injected.
    * Any input that does not start with `/` is routed to the System Assistant instead of being treated as a parse error.
    * The assistant's reply is broadcast to **all** registered adapters via `mediator.RenderMessage` (or the equivalent broadcast hook), not only to the adapter that originated the input.
    * Each conversational exchange is persisted as a Coordination task (`type=3`) with `input.text = <user prompt>` and `output.text = <assistant reply>` so it shows up under `/activity`.
    * The assistant works in sessions that have **no** active project — exactly the "non-project inquiry" case in `appendings.md`.
    * The assistant runs through the existing ACP worker plumbing (Stories 3.1–3.12) so we exercise that pipeline with real traffic before EPIC-02 lands.
    * The Provider that backs the assistant (OpenCode, Codex, Claude Code, …) is resolved from the **Provider Registry** introduced in Story 3.22 — never hardcoded. Default is OpenCode; switching to another Provider is a `config.yaml` edit.
