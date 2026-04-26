# Tasks: 3.17 CUI Adapter Activation in Daemon Mode

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `cmd/codemint/`, `internal/repl/`, `internal/ui/`
**Tech Stack:** Go, XDG paths
**Priority:** P0 for daemon mode (TUI mode unaffected)

---

## Task 3.17.1: Adapter Selection Helper

* **Action:** Centralize the "which adapters do we register?" decision so it stops being scattered across `main.go`.
* **Details:**
  * Create `internal/ui/registration.go`:
    ```go
    type AdapterSet struct {
        TUI *TUIAdapter
        CUI *CUIAdapter
    }
    func BuildAdapters(mode registry.ClientMode, cfg AdapterConfig) (AdapterSet, error)
    ```
  * `BuildAdapters`:
    * `ClientModeCLI` → returns only `TUIAdapter`.
    * `ClientModeDaemon` → returns only `CUIAdapter`.
    * `cfg` carries `Writer`, `LogPath`, `VerbosityGetter`, `Stdout` for the TUI.
  * `AdapterSet.RegisterAll(mediator)` registers whichever fields are non-nil.
* **Verification:**
  * `TestBuildAdapters_CLI` — only `TUI` set.
  * `TestBuildAdapters_Daemon` — only `CUI` set; log path defaults to `xdg.StateDir()+"/cui.log"`.

---

## Task 3.17.2: Replace Manual Registration in `main.go`

* **Action:** Remove the hard-coded TUI registration (Step 11b) in favor of mode-aware registration.
* **Details:**
  * In `cmd/codemint/main.go`, replace:
    ```go
    tuiAdapter := ui.NewTUIAdapter(...)
    mediator.RegisterAdapter(tuiAdapter)
    defer tuiAdapter.Stop()
    ```
    with:
    ```go
    adapters, err := ui.BuildAdapters(clientMode, ui.AdapterConfig{
        Writer:          os.Stdout,
        LogPath:         filepath.Join(xdg.StateDir(), "cui.log"),
        VerbosityGetter: func() ui.VerbosityLevel { return ui.VerbosityLevel(activeSession.GetVerbosity()) },
    })
    if err != nil { return fmt.Errorf("ui adapters: %w", err) }
    adapters.RegisterAll(mediator)
    defer adapters.Close()
    ```
  * `AdapterSet.Close()` invokes `tui.Stop()` and `cui.Close()` if the respective field is non-nil.
* **Verification:**
  * Boot in `--mode=cli` → only TUI registered (`mediator.AdapterCount() == 1`).
  * Boot in `--mode=daemon` → only CUI registered.

---

## Task 3.17.3: Pass CUIAdapter Into Daemon Commands

* **Action:** Stop the daemon command handlers from looking at a permanently-nil `CUIAdapter` field.
* **Details:**
  * Extend `repl.RegisterDaemonCommands` to accept the `AdapterSet`:
    ```go
    daemonDeps := &repl.DaemonCommandDeps{
        ActiveSession: activeSession,
        TaskRepo:      taskRepo,
        CUIAdapter:    adapters.CUI, // may be nil in CLI mode
    }
    ```
  * In each handler that needs the adapter (`/approve`, `/deny`, `/queue`, etc.), keep the existing nil-check but the message should now say "Run with --mode=daemon to use this command" — the previous "CUI adapter not initialized" was a wiring bug, not a user error.
* **Verification:**
  * `TestDaemonCommands_ApproveResolves` — daemon mode → `/approve <id>` resolves a pending prompt.
  * `TestDaemonCommands_ApproveCLIMode` — CLI mode → command rejects with the new mode-hint message.

---

## Task 3.17.4: Push Notification Hook

* **Action:** Reserve the integration point for EPIC-04 §4.5 push notifications without implementing the transport yet.
* **Details:**
  * Add `CUIAdapter.SetPusher(p ui.NotificationPusher)` where `NotificationPusher` is an interface with a single `Push(ctx, msg)` method. Default pusher is a no-op that writes to the log file (existing behavior).
  * EPIC-04 will plug Telegram/Slack pushers into this seam.
  * Document the contract in `internal/ui/cui_adapter.go` doc comment.
* **Verification:**
  * `TestCUIAdapter_DefaultPusher_LogsOnly` — no pusher set → push events appear in `cui.log`, no panic.
  * Compile-time check: a stub `noopPusher` satisfies the interface.

---

## Task 3.17.5: Smoke Test the Daemon REPL

* **Action:** Confirm the wiring with a manual test recipe in the README of this story (no production code).
* **Details:**
  * Add `docs/plan/epic-03/3.17-cui-adapter-daemon-activation/manual-test.md` with steps:
    1. `./build/codemint --mode=daemon`.
    2. `/acp say hi` → log line in `~/.local/state/codemint/cui.log`.
    3. Trigger a blocked command (e.g., manually insert a permission row that blocks `ls`) → `/queue` shows the pending prompt → `/approve <id>` releases the worker.
* **Verification:**
  * Manual run completes the recipe without nil-pointer panics.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.17.1  | 3.9 (CUIAdapter), 3.8 (TUIAdapter) |
| 3.17.2  | 3.17.1 |
| 3.17.3  | 3.17.2, existing `daemon_commands.go` |
| 3.17.4  | 3.17.3 |
| 3.17.5  | 3.17.2, 3.12.2 |

---

## Out of Scope

* Telegram/Slack transport (EPIC-04 §4.5).
* Inline keyboards (EPIC-04 §4.7).
* TUI 3-panel Bubble Tea rewrite (EPIC-04 §4.3).
