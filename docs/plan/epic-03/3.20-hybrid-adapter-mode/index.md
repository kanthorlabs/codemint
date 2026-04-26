# User Story 3.20: Hybrid TUI + CUI Adapter Mode

* **As a** Developer running an end-to-end smoke test of CodeMint,
* **I want to** launch the binary with **both** the local TUI and the daemon CUI registered against the same `UIMediator`,
* **So that** I can ask a question on my terminal, see the same conversation thread on Telegram (or whatever backend the CUI is bound to), and reply from either side.
* *Acceptance Criteria:*
    * A new client mode `hybrid` joins `cli` and `daemon` in the `--mode` flag.
    * In hybrid mode, `BuildAdapters` (Story 3.17) registers **both** TUI and CUI against the mediator. They share the same chat thread, the same prompt fan-out, and the same scheduler.
    * Output broadcasts (`EventChatChunk`, `EventTaskStatusChanged`, etc.) reach both adapters; deduplication is the mediator's job, not the adapters'.
    * The TUI continues to own stdin in hybrid mode (so the local terminal stays interactive); inbound CUI messages arrive via Story 3.21's input multiplexer rather than stdin.
    * `mediator.PromptDecision` (Story 3.18) returns the first response from either adapter and cancels the loser, exactly as in single-adapter mode.
    * Verbosity (`/verbosity`) applies per-adapter — the CUI defaults to Level 1 (User Story); the TUI defaults to Level 0 (Task) as today.
