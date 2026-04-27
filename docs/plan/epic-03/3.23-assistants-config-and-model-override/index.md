# User Story 3.23: `assistants:` Config Refactor & Per-Provider Model Override

* **As a** Developer who chose a specific LLM/model for a CodeMint Assistant,
* **I want** the YAML configuration to use an `assistants:` section (replacing the unused `agents:` section) and to spawn the underlying Provider binary with that Assistant's configured model,
* **So that** I can run the System Assistant with, e.g., `opencode --model github-copilot/claude-sonnet-4.6` simply by declaring `assistants.system.model: github-copilot/claude-sonnet-4.6` in `config.yaml`, without recompiling or hand-writing flags.
* *Acceptance Criteria:*
    * `Config.Agents []AgentConfig` is removed from the codebase. The legacy `agents:` YAML key is no longer recognized.
    * `Config.Assistants` (introduced in Story 3.22) becomes the **only** path for declaring per-Assistant Provider/model bindings.
    * Each `AssistantBinding` carries an optional `model` string; when set, the field is rendered into the Provider's spawn args using a Provider-specific model flag (e.g., OpenCode → `--model <value>`, Codex → `--model <value>`, Claude Code → `--model <value>`).
    * Booting with `assistants.system: {provider: opencode, model: "github-copilot/claude-sonnet-4.6"}` results in `pgrep -a opencode` showing the `--model github-copilot/claude-sonnet-4.6` argument.
    * Omitting `model` keeps the Provider's default model — the spawn args do **not** include an empty `--model` flag.
    * `configs/config.yaml.example` is updated: the `agents:` block is removed; an `assistants:` block with the OpenCode + custom model example is added (commented out, since defaults work without it).
    * Loading a `config.yaml` that still contains a top-level `agents:` key prints a one-line deprecation warning naming the file path and pointing at the new shape — but does not fail. (Six months from now this warning becomes an error; tracked separately.)
