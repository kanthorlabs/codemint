# Tasks: 3.22 Assistant Provider Configuration & Registry

**Epic:** EPIC-03 (ACP Execution Layer)
**Target Directory:** `internal/config/`, `internal/acp/`, `internal/agent/`, `configs/`, `cmd/codemint/`
**Tech Stack:** Go, YAML, `os/exec`, `PATH` lookup
**Priority:** P0 — precondition for shipping 3.19; without it the System Assistant is forever bound to OpenCode.

---

## Task 3.22.1: `Provider` Domain Type and Built-in Catalog

* **Action:** Model an ACP Provider as a first-class type and ship a catalog of known good defaults.
* **Details:**
  * Create `internal/agent/provider.go`:
    ```go
    type Provider struct {
        Name         string            // "opencode" | "codex" | "claude-code" | <custom>
        DisplayName  string            // "OpenCode", "OpenAI Codex CLI", "Claude Code"
        Command      string            // binary on PATH, or absolute path
        Args         []string          // default ACP-mode args, e.g. ["acp"]
        Env          map[string]string // extra env vars (API keys, model overrides)
        Capabilities ProviderCaps      // streaming, toolCalls, planning, sessionReset
        SystemPromptStrategy PromptStrategy // how to inject memory (stdin / flag / env)
    }
    type ProviderCaps struct {
        Streaming   bool
        ToolCalls   bool
        Planning    bool
        ContextReset bool // supports `session/new` for Story 3.2
    }
    ```
  * Built-in catalog in `internal/agent/provider_catalog.go`:
    * `opencode`: `Command: "opencode"`, `Args: ["acp"]`, all caps true. **Default.**
    * `codex`: `Command: "codex"`, `Args: ["acp"]`, planning=false (Codex CLI isn't planning-aware yet). System prompt via `--system-prompt-file` flag.
    * `claude-code`: `Command: "claude"`, `Args: ["--acp"]`, all caps true. System prompt via stdin (Anthropic CLI honors initial message).
  * Catalog lookup: `agent.LookupBuiltinProvider(name string) (*Provider, bool)`.
* **Verification:**
  * `TestLookupBuiltinProvider_Opencode` — returns the canonical OpenCode entry.
  * `TestLookupBuiltinProvider_Unknown` — returns `(nil, false)`.

---

## Task 3.22.2: Config Schema for Providers and Assistants

* **Action:** Extend `config.Config` to carry providers and assistant bindings.
* **Details:**
  * In `internal/config/config.go`:
    ```go
    type Config struct {
        Workflows  []WorkflowConfig  `yaml:"workflows" validate:"dive"`
        Agents     []AgentConfig     `yaml:"agents,omitempty" validate:"dive"`
        Providers  []ProviderConfig  `yaml:"providers,omitempty" validate:"dive"`
        Assistants AssistantsConfig  `yaml:"assistants,omitempty"`
    }
    type ProviderConfig struct {
        Name        string            `yaml:"name" validate:"required"`
        Command     string            `yaml:"command,omitempty"` // override builtin
        Args        []string          `yaml:"args,omitempty"`
        Env         map[string]string `yaml:"env,omitempty"`
        Disabled    bool              `yaml:"disabled,omitempty"`
    }
    type AssistantsConfig struct {
        System      AssistantBinding `yaml:"system"`
        Brainstormer AssistantBinding `yaml:"brainstormer,omitempty"` // EPIC-02
        Clarifier    AssistantBinding `yaml:"clarifier,omitempty"`    // EPIC-02 §2.12
        Archivist    AssistantBinding `yaml:"archivist,omitempty"`    // EPIC-05
    }
    type AssistantBinding struct {
        Provider string `yaml:"provider" validate:"required"` // references ProviderConfig.Name or builtin
        Model    string `yaml:"model,omitempty"`
    }
    ```
  * Validation rule: every `AssistantBinding.Provider` must resolve via the catalog **or** an entry in `Providers`. Unknown name fails config load with `unknown provider %q (known: opencode, codex, claude-code; or declare under providers:)`.
  * Default when `assistants.system` is omitted: `{Provider: "opencode"}`.
* **Verification:**
  * `TestConfig_Validate_UnknownProvider` — config load returns the listed error.
  * `TestConfig_DefaultAssistantSystem` — empty `assistants` resolves to OpenCode.
  * `TestConfig_ProviderOverride_Wins` — config `providers.command` overrides the catalog entry.

---

## Task 3.22.3: `ProviderRegistry`

* **Action:** Resolve final `Provider` instances at runtime.
* **Details:**
  * Create `internal/agent/provider_registry.go`:
    ```go
    type ProviderRegistry struct {
        mu       sync.RWMutex
        entries  map[string]*Provider
    }
    func NewProviderRegistry(cfg *config.Config) (*ProviderRegistry, error)
    func (r *ProviderRegistry) Resolve(name string) (*Provider, error)
    func (r *ProviderRegistry) MustExist(name string) error // PATH-check the binary
    func (r *ProviderRegistry) List() []*Provider           // for /providers REPL command (3.22.5)
    ```
  * Construction merges:
    1. Built-in catalog (3.22.1).
    2. Config overrides (`Providers` slice). Same `Name` overrides catalog fields field-by-field; `Disabled: true` removes the entry from `Resolve` results.
    3. `CODEMINT_ACP_CMD` env override (Story 3.1) — still applies, but only to the Provider used by the System Assistant. Other Providers ignore it.
  * `MustExist` runs `exec.LookPath(p.Command)` and returns a typed error so callers can surface a clean "Codex CLI not installed" message.
* **Verification:**
  * `TestProviderRegistry_BuiltinPlusOverride` — declares `providers: [{name: opencode, args: ["--debug"]}]` and confirms resolution merges.
  * `TestProviderRegistry_DisabledProvider` — `Disabled: true` excludes from `List` and `Resolve`.
  * `TestProviderRegistry_MustExist_MissingBinary` — fake `Command: "definitely-not-installed"` returns the typed error.

---

## Task 3.22.4: Wire Provider Into `acp.Registry` and `SystemAssistant`

* **Action:** Replace `acp.DefaultConfig()` with a Provider-driven config.
* **Details:**
  * Change `acp.WorkerConfig` constructor:
    ```go
    func WorkerConfigFromProvider(p *agent.Provider, cwd string) WorkerConfig {
        return WorkerConfig{
            Command: p.Command,
            Args:    append([]string{}, p.Args...),
            Cwd:     cwd,
            Env:     mergeEnv(os.Environ(), p.Env),
        }
    }
    ```
  * `acp.NewRegistry(cfg)` keeps its current shape, but `main.go` builds the config from `providerRegistry.Resolve("opencode")` (or whatever the system assistant binding says) instead of `acp.DefaultConfig()`.
  * Story 3.19's `acpAssistant` constructor takes a `*Provider` and a `*acp.Registry`; on first `Ask`, it ensures the worker uses the resolved provider.
  * The system prompt strategy field (3.22.1) decides where to inject memory — current code only supports stdin (default for OpenCode/Claude Code). Codex needs a `--system-prompt-file <tempfile>` arg; implement that branch.
* **Verification:**
  * `TestACPRegistry_UsesProvider` — register a stub Provider with `Command: "echo"`, observe the worker is spawned with that command.
  * `TestSystemPrompt_FileStrategy` — for `Provider.SystemPromptStrategy == PromptFile`, the generated file is passed via `--system-prompt-file`.

---

## Task 3.22.5: Config Loading + Wiring in `main.go`

* **Action:** Plumb the registry from config into the runtime.
* **Details:**
  * In `cmd/codemint/main.go` after Step 9 (`appCfg` loaded):
    ```go
    providerReg, err := agent.NewProviderRegistry(appCfg)
    if err != nil { return fmt.Errorf("provider registry: %w", err) }
    sysProvider, err := providerReg.Resolve(appCfg.Assistants.System.Provider)
    if err != nil { return fmt.Errorf("system assistant provider: %w", err) }
    if err := providerReg.MustExist(sysProvider.Name); err != nil {
        log.Printf("Warning: %v — System Assistant disabled", err)
        sysProvider = nil
    }
    ```
  * Pass `sysProvider` into `acp.NewRegistry(...)` as the default WorkerConfig template.
  * Pass `providerReg` into Story 3.19's `agent.NewACPAssistant(runtime, providerReg, "system")` so it can resolve at call time.
  * On `--with-assistant=true` but `sysProvider == nil`, the dispatcher behaves as if the assistant is absent (Story 3.19 already handles this nil case).
* **Verification:**
  * Boot with default config → System Assistant uses OpenCode.
  * Edit `config.yaml` to `assistants.system.provider: codex` → System Assistant tries to spawn `codex acp`.
  * Boot when `opencode` binary is missing → warning printed, `/help` still works, freeform input shows the friendly "assistant disabled" message.

---

## Task 3.22.6: `/providers` REPL Command

* **Action:** Give users a quick way to inspect what's configured and reachable.
* **Details:**
  * New file `internal/repl/provider_commands.go`:
    * `/providers` — lists each provider with status: `opencode (default, ✓ found at /usr/local/bin/opencode)`, `codex (✗ not on PATH)`, `claude-code (disabled)`.
    * `/providers test <name>` — spawns the binary with `--version` (or the catalog's `VersionArgs`) and prints stdout. Useful for verifying ACP installs.
  * Hook into the same `SupportedModes: cli|daemon|hybrid` so it's available everywhere.
* **Verification:**
  * Manual: `/providers` shows the live status; tests pass with `--version` mocked.

---

## Task 3.22.7: Update `config.yaml.example` and Docs

* **Action:** Show users the new shape.
* **Details:**
  * Append to `configs/config.yaml.example`:
    ```yaml
    # Available ACP providers. Names matching the built-in catalog (opencode, codex,
    # claude-code) inherit defaults — only override fields you need.
    # providers:
    #   - name: opencode             # default; override args or env if needed
    #     args: ["acp"]
    #   - name: codex
    #     env:
    #       OPENAI_API_KEY: "${OPENAI_API_KEY}"
    #   - name: claude-code
    #     command: "claude"          # override binary path if not on PATH
    #     args: ["--acp"]

    # Bind logical assistants to a provider. Omit to use the OpenCode default.
    # assistants:
    #   system:
    #     provider: opencode         # OpenCode | codex | claude-code | <custom>
    #     model: "claude-sonnet-4-6"
    ```
  * Add a short section `## Providers` to whatever README documents config (or leave a TODO for EPIC-04 docs work).
* **Verification:**
  * `go vet ./...` and `make test` pass.
  * Example loads cleanly via `config.Load("configs/config.yaml.example")` (an existing test or a new one).

---

## Dependencies

| Task    | Depends On |
|---------|------------|
| 3.22.1  | (none) |
| 3.22.2  | 3.22.1, existing `internal/config` package |
| 3.22.3  | 3.22.1, 3.22.2 |
| 3.22.4  | 3.22.3, 3.1 (`acp.Worker`), 3.11 (system prompt builder) |
| 3.22.5  | 3.22.4 |
| 3.22.6  | 3.22.3 |
| 3.22.7  | 3.22.5 |

---

## Out of Scope

* Hot-reload of provider config — restart required for now.
* Provider-specific tool-call schema differences (Codex tool format vs OpenCode) — handled at the ACP layer; Story 3.4 already treats them as opaque JSON.
* Cost / token accounting per Provider — future EPIC.
* Selecting a Provider per project (today: process-global) — future EPIC.
