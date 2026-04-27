# Tasks: 3.23 `assistants:` Config Refactor & Per-Provider Model Override

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/config/`, `internal/agent/`, `internal/acp/`, `configs/`
**Tech Stack:** Go, YAML
**Priority:** P0 — clears legacy schema and unlocks per-Assistant model selection (precondition for shipping 3.22).

---

## Task 3.23.1: Remove Legacy `agents:` Schema

* **Action:** Delete the unused `AgentConfig` type and the `Agents` field from `Config`.
* **Details:**
  * In `internal/config/config.go`:
    * Remove the `Agents []AgentConfig` field.
    * Remove the entire `AgentConfig` struct.
  * In `internal/config/validate.go`: drop any rule that referenced `Agents` (a quick `grep` shows none today, but verify after the field is removed).
  * In `internal/config/loader_test.go` and `validate_test.go`: remove fixtures that reference `agents:`.
* **Verification:**
  * `go build ./...` and `go test ./internal/config/...` pass.
  * `grep -rn "AgentConfig\|cfg.Agents" internal cmd` returns zero matches.

---

## Task 3.23.2: Deprecation Warning for Legacy YAML Key

* **Action:** When a user's `config.yaml` still has `agents:` at the top level, log a friendly deprecation warning instead of silently ignoring it.
* **Details:**
  * In `internal/config/loader.go`, before strict-decode:
    1. Decode the file once into `map[string]any` to peek at top-level keys.
    2. If `m["agents"]` exists, `slog.Warn("config: 'agents:' key is deprecated; use 'assistants:' instead", "path", path)`.
    3. Remove the key from the map (or use a tolerant decoder) so the strict-decode pass that follows doesn't fail on the unknown field.
  * Keep the strict-decoder for everything else — typos in `assistants:` should still fail loud.
* **Verification:**
  * `TestLoader_LegacyAgentsKey_LogsDeprecation` — a fixture with `agents: [{name: foo}]` loads cleanly and the test captures the warning via a `slog` test handler.
  * `TestLoader_TyposStillFailStrict` — a config with `assistantz:` (note the `z`) still errors.

---

## Task 3.23.3: `Provider.ModelFlag` and Catalog Defaults

* **Action:** Teach the Provider type how to translate a `model` string into spawn args.
* **Details:**
  * Extend `internal/agent/provider.go`:
    ```go
    type Provider struct {
        // ... existing fields ...
        ModelFlag string // e.g. "--model"; empty disables model injection
    }
    ```
  * Built-in catalog defaults (`provider_catalog.go`):
    * `opencode`: `ModelFlag: "--model"`.
    * `codex`: `ModelFlag: "--model"`.
    * `claude-code`: `ModelFlag: "--model"`.
  * Override via config: `providers[].model_flag` lets users adjust if a Provider changes its CLI surface.
  * Document in the doc comment: an empty `ModelFlag` means the Provider doesn't accept a CLI model selector — the binding's `model` is then ignored with a `slog.Debug` line.
* **Verification:**
  * `TestProvider_ModelFlag_DefaultsForCatalog` — table test asserts each built-in's flag.
  * `TestProvider_ConfigOverridesModelFlag` — config sets `model_flag: "-m"`; resolved Provider reflects it.

---

## Task 3.23.4: Spawn-Args Composition With Model

* **Action:** Inject the Assistant's model into the worker spawn args at the right place.
* **Details:**
  * Update `acp.WorkerConfigFromProvider` (introduced in 3.22.4) to accept the binding's model:
    ```go
    func WorkerConfigFromProvider(p *agent.Provider, binding agent.AssistantBinding, cwd string) WorkerConfig {
        args := append([]string{}, p.Args...)
        if binding.Model != "" && p.ModelFlag != "" {
            args = append(args, p.ModelFlag, binding.Model)
        } else if binding.Model != "" && p.ModelFlag == "" {
            slog.Debug("provider does not support CLI model selector; ignoring",
                "provider", p.Name, "model", binding.Model)
        }
        return WorkerConfig{
            Command: p.Command,
            Args:    args,
            Cwd:     cwd,
            Env:     mergeEnv(os.Environ(), p.Env),
        }
    }
    ```
  * The model lands at the end of the args slice, after the Provider's standard args (`["acp"]` for OpenCode). Verify against OpenCode's actual CLI (it accepts `opencode acp --model <name>`); if a Provider needs the flag in a different position, add a `ModelArgPosition` enum.
  * In Story 3.19's `NewACPAssistant`, pass the resolved binding through to the worker config — not just the Provider name.
* **Verification:**
  * `TestWorkerConfig_AppendsModelArg` — Provider with `ModelFlag="--model"` + binding `{Model: "gpt-5"}` → args end with `--model gpt-5`.
  * `TestWorkerConfig_NoModel_NoFlag` — empty model leaves args untouched.
  * Manual: `assistants.system.model: github-copilot/claude-sonnet-4.6` → `pgrep -a opencode` shows `opencode acp --model github-copilot/claude-sonnet-4.6`.

---

## Task 3.23.5: Update `configs/config.yaml.example`

* **Action:** Reflect the final shape with a working OpenCode + custom model example.
* **Details:**
  * Replace the existing commented `agents:` block with:
    ```yaml
    # Available ACP providers. Names matching the built-in catalog (opencode,
    # codex, claude-code) inherit defaults — only override fields you need.
    # providers:
    #   - name: opencode
    #     # command: opencode    # default; override only if not on PATH
    #     # args: ["acp"]
    #   - name: codex
    #     env:
    #       OPENAI_API_KEY: "${OPENAI_API_KEY}"
    #   - name: claude-code
    #     command: claude

    # Bind logical assistants to a provider and (optionally) a model.
    # Omit the entire block to use the OpenCode default with the Provider's
    # default model.
    # assistants:
    #   system:
    #     provider: opencode
    #     model: "github-copilot/claude-sonnet-4.6"
    #   brainstormer:        # EPIC-02
    #     provider: opencode
    #     model: "github-copilot/claude-sonnet-4.6"
    #   clarifier:           # EPIC-02 §2.12
    #     provider: opencode
    #   archivist:           # EPIC-05
    #     provider: opencode
    ```
  * The Coding/Verification tasks (EPIC-02 outputs) continue to flow through the per-session ACP worker; that worker's Provider is currently shared with the System Assistant. A future story may add a separate `coding` binding — note this in a TODO comment.
* **Verification:**
  * `TestExampleConfig_Loads` — `config.Load("configs/config.yaml.example")` returns no error.
  * Manual review: example file is uncommented-friendly (no syntax landmines if a user removes leading `#`).

---

## Task 3.23.6: Validation for Unknown Assistant Roles

* **Action:** Reject typos in the `assistants:` block early.
* **Details:**
  * Strict-decode `AssistantsConfig` so an unknown role like `assistants.systen:` (typo) fails config load with `unknown assistant role %q (known: system, brainstormer, clarifier, archivist)`.
  * Use YAML strict mode (`KnownFields(true)`) on the `AssistantsConfig` struct decoding pass. If the loader decodes the whole `Config` strict already, only the deprecation peek (3.23.2) needs the tolerant pass — keep that scoped.
* **Verification:**
  * `TestConfig_UnknownAssistantRole_Fails` — fixture with `assistants.systen:` returns the listed error.
  * `TestConfig_KnownAssistantRoles_Pass` — all four roles load.

---

## Task 3.23.7: End-to-End Smoke Test

* **Action:** Prove the full path: config edit → spawn arg.
* **Details:**
  * `TestAssistant_E2E_ModelInSpawnArgs`:
    1. Write a temp `config.yaml` with `assistants.system: {provider: opencode, model: "github-copilot/claude-sonnet-4.6"}`.
    2. Override the OpenCode catalog entry to point `Command` at a stub script that records its argv to a temp file.
    3. Boot orchestrator far enough to spawn the System Assistant.
    4. Assert the recorded argv contains `["acp", "--model", "github-copilot/claude-sonnet-4.6"]`.
  * Manual recipe in `manual-test.md` for the same path against the real OpenCode binary.
* **Verification:**
  * Test passes deterministically (`-count=20`).

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.23.1  | (none) |
| 3.23.2  | 3.23.1 |
| 3.23.3  | 3.22.1 (Provider type) |
| 3.23.4  | 3.23.3, 3.22.4 (`WorkerConfigFromProvider`) |
| 3.23.5  | 3.23.1, 3.23.4 |
| 3.23.6  | 3.22.2 (`AssistantsConfig`) |
| 3.23.7  | 3.23.4, 3.19 (assistant spawn path) |

---

## Out of Scope

* Per-Assistant API key isolation — currently shared via Provider env. A future story may move keys onto the binding.
* Hot-reload of model changes — restart required.
* Codex / Claude Code model name validation — left to the upstream CLI.
* A separate `coding` Assistant binding distinct from `system` — tracked as a future enhancement; for now Coding tasks (EPIC-02) reuse the per-session ACP worker, which inherits the System Assistant's Provider/model.
