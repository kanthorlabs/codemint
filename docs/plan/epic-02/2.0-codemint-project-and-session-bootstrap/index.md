# User Story 2.0: CodeMint Project & Session Bootstrap

* **As a** user launching CodeMint on a fresh install (or any install without an open user project),
* **I want** an always-available "CodeMint" project and session pre-wired for non-project work (chat, research, blogging),
* **So that** I land in a usable session immediately, my SystemAssistant chats are persisted and reviewable, and EPIC-02's Brainstormer can ship without breaking the conversational path.

* *Acceptance Criteria:*
  * On every launch, exactly one **CodeMint project** exists (`name="codemint"`, `kind="codemint"`, `working_dir=$XDG_DATA_HOME/codemint/workspace`). Created idempotently if missing.
  * On every launch, at least one CodeMint session has `status=Active`. If none, the bootstrap creates one. The user-facing "No active session" message is unreachable in the healthy path.
  * The workspace directory exists on disk (no `git init`).
  * Freeform input in a CodeMint session routes to `SystemAssistant`. Freeform input in a Coding-kind session returns `ErrNoBrainstormer` (placeholder for EPIC-02).
  * `ActiveSession.IsGlobal` is removed from the codebase. Routing decisions consult `Project.Kind`.
  * Tool calls inside a CodeMint session are auto-approved by the Interceptor without consulting the (empty) `project_permission` row. Tool calls in Coding-kind sessions retain existing whitelist/blocklist behaviour.
  * `/project-open <path>` slash command exists, bound to CLI and Daemon modes, and creates a Coding-kind project + initial active session for any git-initialised path the user supplies.
  * `/project-assistant <provider> [model]` slash command updates the active project's assistant binding. With no args, it prints the current binding (project columns, falling back to `config.yaml`).
  * SystemAssistant resolution honours per-project override: `project.assistant_provider`/`project.assistant_model` take precedence over `assistants.codemint.*` from `config.yaml`.
  * No regression in Suite 0/14 of `docs/plan/epic-03/mvp.md` — fresh-DB launch lands the user in a CodeMint session ready to chat without any SQL seeding.

* *Out of Scope (deferred to `docs/plan/appendings.md`):*
  * Directory-scoped Interceptor enforcement (today CodeMint sessions get a kind-based bypass; later we tighten by `cwd`/path-arg checks).
  * Per-session assistant override (this story ships per-project only).
  * Configurable workspace path via env or config (this story hardcodes `xdg.DataDir() + "/workspace"`).
  * Concurrent multiple-active sessions for the CodeMint project (the existing single-active-session index applies uniformly).
