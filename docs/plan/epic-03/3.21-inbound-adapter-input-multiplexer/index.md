# User Story 3.21: Inbound Adapter Input Multiplexer (CUI Reply Channel)

* **As the** Go Orchestrator running in hybrid or daemon mode,
* **I want** every adapter to be able to inject inbound messages into the same dispatcher pipeline that the TUI's stdin uses,
* **So that** a reply typed on Telegram (CUI) reaches the System Assistant exactly as if it had been typed on the local terminal — closing the cross-interface conversational loop.
* *Acceptance Criteria:*
    * A new `InputMultiplexer` accepts `InboundMessage` values from any source (TUI stdin, CUI Telegram bot, future Slack adapter, integration tests).
    * The REPL's main loop reads from the multiplexer instead of `os.Stdin` directly.
    * Inbound messages carry a `Source` ("tui", "cui-telegram", etc.) and a logical `UserID` so audit logs can attribute the input.
    * The multiplexer is **back-pressure aware**: each source has a bounded channel; if a source overruns, the multiplexer drops oldest with a warn log rather than blocking the dispatcher.
    * A **stub `inMemoryInbound` backend** is available for tests and local dev without standing up a real Telegram bot. EPIC-04 §4.5 will deliver the production Telegram transport against the same interface.
    * Inbound message ordering is per-source FIFO; cross-source ordering is mediator-defined (first-arrived wins for the dispatcher's serial loop).
