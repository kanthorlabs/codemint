# Tasks: 3.20 Hybrid TUI + CUI Adapter Mode

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/registry/`, `internal/ui/`, `cmd/codemint/`
**Tech Stack:** Go, flags
**Priority:** P1 — purely a testing/dev affordance, but blocks the cross-interface acceptance test.

---

## Task 3.20.1: New `ClientMode` Variant

* **Action:** Add `ClientModeHybrid`.
* **Details:**
  * In `internal/registry/client_mode.go`:
    ```go
    const ClientModeHybrid ClientMode = "hybrid"
    ```
  * Update `parseClientMode` in `cmd/codemint/main.go` to recognize the new value.
  * Update `Command.SupportedModes` declarations: every command that currently lists `[ClientModeCLI, ClientModeDaemon]` must include `ClientModeHybrid`. Run a quick grep to catch them all.
* **Verification:**
  * `TestParseClientMode_Hybrid` — `--mode=hybrid` returns the new constant; invalid modes still error.
  * `TestCommandRegistry_Hybrid_AllCommandsAvailable` — every registered command resolves under hybrid.

---

## Task 3.20.2: `BuildAdapters` Hybrid Branch

* **Action:** Register both adapters when mode is hybrid.
* **Details:**
  * In `internal/ui/registration.go` (introduced in 3.17.1):
    ```go
    case registry.ClientModeHybrid:
        return AdapterSet{TUI: tui, CUI: cui}, nil
    ```
  * Default verbosity:
    * TUI → Level 0 (Task) — stays the same.
    * CUI → Level 1 (User Story) — quieter for chat clients.
    * Allow override via env: `CODEMINT_CUI_VERBOSITY=0|1|2`.
* **Verification:**
  * `TestBuildAdapters_Hybrid_RegistersBoth` — set has both fields non-nil.
  * `TestBuildAdapters_Hybrid_VerbosityDefaults` — TUI=0, CUI=1.

---

## Task 3.20.3: Mediator Deduplication

* **Action:** Make sure a single domain event reaches each adapter exactly once.
* **Details:**
  * Audit `mediator.go`. If `NotifyEvent` already iterates the adapter list, no work; if it currently dispatches based on `clientMode`, remove that filter (the adapter itself decides whether to render via `shouldForwardEvent`).
  * Add `TestMediator_Hybrid_NoDuplicateDelivery` — fire one event, observe each adapter's recorder shows count==1.
* **Verification:**
  * Test passes.
  * Manual: type `What's a goroutine?` in TUI; both terminal and CUI log show one rendering of the assistant reply.

---

## Task 3.20.4: TUI Owns Stdin

* **Action:** Prevent the CUI from racing on stdin in hybrid mode.
* **Details:**
  * In `repl.Loop`, leave the existing stdin reader unchanged — TUI is the stdin owner regardless of mode.
  * The CUI's only inbound surface is the Story 3.21 multiplexer.
  * Document the contract in a doc comment on `AdapterSet`.
* **Verification:**
  * Boot in hybrid → TUI receives keyboard input as before; no contention errors logged.

---

## Task 3.20.5: Update `--help` and Docs

* **Action:** Reflect the new mode in user-facing docs.
* **Details:**
  * Update `flag.StringVar(&cfg.mode, ...)` description to: `"Client mode: cli, daemon, or hybrid (cli + daemon adapters together)"`.
  * Add a one-paragraph note to the README (or `docs/README.md`) describing hybrid mode as a testing affordance.
* **Verification:**
  * `./build/codemint --help` lists hybrid; help text mentions it's for cross-interface testing.

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.20.1  | (none) |
| 3.20.2  | 3.20.1, 3.17.1 |
| 3.20.3  | 3.18 (broadcast contract) |
| 3.20.4  | 3.20.2 |
| 3.20.5  | 3.20.1 |

---

## Out of Scope

* Production deployment of hybrid mode — it's a dev/test convenience.
* Per-user adapter routing (e.g., "Alice gets TUI, Bob gets Telegram"). EPIC-04 territory.
