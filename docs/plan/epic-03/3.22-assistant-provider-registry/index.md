# User Story 3.22: Assistant Provider Configuration & Registry

* **As a** Developer running CodeMint with different CLI agents installed locally,
* **I want to** declare available **Providers** (OpenCode, Codex, Claude Code, …) in `config.yaml`, bind each Assistant to one Provider, and switch the default System Assistant Provider via configuration,
* **So that** CodeMint is not hardcoded to OpenCode and we can experiment with alternative ACP-compatible agents without recompiling. Default Provider is OpenCode; future Providers plug in by name.
* *Acceptance Criteria:*
    * `config.yaml` gains a `providers:` section listing available ACP-compatible CLI agents and a `assistants:` section binding logical Assistants (System Assistant, Brainstormer, etc.) to a Provider by name.
    * A built-in catalog ships sensible defaults for `opencode`, `codex`, and `claude-code` so most users only need to set `providers: [opencode]` (or omit it entirely).
    * `acp.Registry` and `agent.SystemAssistant` accept a `Provider` (binary, args, env, capabilities) instead of constructing one from `acp.DefaultConfig()`.
    * Switching the System Assistant from OpenCode to Codex requires only a config edit + restart — no code change, no rebuild.
    * Missing or non-executable Provider binaries fail loudly at startup with a friendly message naming the binary that wasn't found on `PATH`. Other Providers stay usable.
    * The legacy `CODEMINT_ACP_CMD` env var (Story 3.1) still wins over config so existing tests and dev hacks keep working.
    * Story 3.19's `SystemAssistant` resolves its Provider from the registry on first use, not at process start, so an unused Provider never spawns.
