# User Story 3.17: CUI Adapter Activation in Daemon Mode (Supports EPIC-04 §4.5–§4.7)

* **As the** Go Orchestrator,
* **I want to** create and register the existing `CUIAdapter` against the `UIMediator` whenever the binary is launched in `daemon` mode,
* **So that** the low-bandwidth pulse defined by Story 3.9 and the chat primitives defined by EPIC-04 §4.5–§4.7 actually receive events. Today the daemon path constructs `daemon_commands.CUIAdapter` only as a struct field — no instance is wired in `main.go`, so the daemon REPL has zero registered adapters.
* *Acceptance Criteria:*
    * Launching with `--mode=daemon` constructs exactly one `CUIAdapter`, registers it with the mediator, and writes its log file to the XDG state path.
    * The TUI adapter is **not** registered in daemon mode (avoids duplicate output to the same stdout).
    * `/approve`, `/deny`, `/tasks`, `/status` (already implemented in `daemon_commands.go`) reach a non-nil `CUIAdapter` instance — current code path returns "CUI adapter not initialized".
    * Graceful shutdown closes the adapter's log file with `defer adapter.Close()`.
    * Both adapters share the same `UIMediator` so multi-interface (TUI + Telegram bridge) future work needs no refactor.
