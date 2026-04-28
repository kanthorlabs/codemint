# User Story 2.0.5: Skill Injection into ACP Prompt

* **As the** Go Orchestrator,
* **I want to** resolve every workflow step's skill reference and inject the skill body into the ACP `session/prompt` payload,
* **So that** each workflow step (Gatherer, Goal Capture, Spec Writer, Options Proposer, Task Generator, etc.) executes with its own knowledge injection — and an unresolvable reference fails loudly at the earliest possible point, never silently.

> **Status:** NEW (added 2026-04-27 alongside the GROW revision). Closes the gap that `Story.Skill` is parsed in WORKFLOW.yaml and `Skill` bodies live in `internal/skills`, but no code currently bridges them into the ACP `session/prompt` payload that `executor.go::executeCoding` sends.

> **Spec reference:** [agentclientprotocol.com/protocol/slash-commands.md](https://agentclientprotocol.com/protocol/slash-commands.md). Skills are injected as `TextContent` ContentBlocks in the `prompt` array of `session/prompt` — the same channel ACP uses for slash commands. We do **not** rely on agent-advertised `available_commands_update` (those are agent-side commands like OpenCode's `/test`); CodeMint skills are client-side knowledge injection.

## Acceptance Criteria

### Resolution guarantee (find-or-throw at three layers)

1. **L1 — Workflow load time (`workflow.FileRegistry.LoadAll`):** every `Story.Skill` reference in every loaded WORKFLOW.yaml must resolve against the skills registry. Unresolvable references fail the load with a clear error: `workflow %q references unknown skill %q at story %q`. The workflow is rejected; `/workflow` does not list it.
2. **L2 — Task creation time (`TaskGenerator.GenerateTasks`):** TaskGenerator copies the (already-L1-validated) skill ID into `TaskInput.Skill`. If the registry no longer contains the skill (skills directory was modified between load and generation), generation fails with `task generator: skill %q no longer in registry`.
3. **L3 — Task execution time (`Executor.executeCoding`):** before sending `session/prompt`, the Executor resolves `taskInput.Skill` once more. On miss, the task fails with sentinel `FailureSentinelSkillNotFound` and reason `skill not found: <id>`. The scheduler advances; the worker is not killed.

A user therefore cannot reach an ACP `session/prompt` for a skill-bearing task without that skill resolving — three independent checkpoints.

### Injection mechanism

4. New field `TaskInput.Skill string` (optional). Empty for non-skill tasks (Verification commands, Confirmation prompts, Retrospective prompts, raw chat).
5. Executor builds the `prompt` array per ACP spec:
    1. **Block 0** — `TextContent` containing the resolved `skill.Body` (the `SKILL.md` content body, frontmatter excluded), framed with a single-line preamble: `[skill: <skill.ID>]\n\n<body>`.
    2. **Block 1** — `TextContent` with `taskInput.Prompt` (the per-task ask).
    3. **Blocks 2..N** — `ResourceLinkContent` for each entry in `taskInput.ContextFiles` (existing behavior, unchanged).
6. Order matters: skill body is always first so the agent reads its instructions before the per-task prompt.
7. If `taskInput.Skill` is empty, Executor sends the existing 2-block layout (prompt + context links). No regression for non-skill Coding tasks.

### Failure surface

8. New failure sentinel `FailureSentinelSkillNotFound = "skill_not_found"` exposed alongside the existing `FailureSentinelInvalidInput` etc. in `internal/orchestrator/executor.go`.
9. Resolution errors at L1 prevent workflow registration; user-visible at `/workflow` listing time.
10. Resolution errors at L3 mark the task `Failure` with `task.Output` containing `{"sentinel":"skill_not_found","reason":"skill not found: @codemint/gatherer"}`. The retrospective phase can read this via the existing failure-context loopback.

### Skill ID resolution rules

11. `@codemint/<name>` → embedded skill in `internal/skills/embedded/<name>/SKILL.md`.
12. `./<path>` or `../<path>` → relative to the workflow file's directory (`workflow.SourcePath`).
13. Bare `<name>` → external skill at `~/.agents/skills/<name>/SKILL.md` (canonical install location, see "Install Convention" below).
14. Embedded entries always win on ID collision (existing rule, retained from `internal/skills/registry.go`).
15. Resolution is case-sensitive and trims whitespace.

### Install Convention (`~/.agents/skills` + Claude symlink)

Skills live at a single canonical path; CodeMint mirrors them where ACP agents expect to find them. CodeMint also auto-installs its own bundled "system skills" so the convention is satisfied out of the box.

20. **Canonical install location:** `~/.agents/skills/<name>/SKILL.md`. This is the open-standard skills directory that CodeMint, OpenCode, and Codex all read.
21. **Claude Code compatibility:** the Claude Code ACP agent reads only `~/.claude/skills/`, not `~/.agents/skills/`. Therefore every shared skill MUST be exposed at `~/.claude/skills/<name>` as a symlink → `~/.agents/skills/<name>`. Without the symlink the same skill body would not be visible to Claude-Code-backed sessions even though CodeMint sees it.
22. The convention is one-way: `~/.agents/skills` is the source of truth; `~/.claude/skills/<name>` is always the symlink. CodeMint never writes content into `~/.claude/skills/`.
23. **Boot-time verification (third-party skills):** during skills load, CodeMint walks `~/.agents/skills/`. For every skill directory **not owned by CodeMint** it checks `~/.claude/skills/<name>`:
    - Missing → log `WARN skill %q has no Claude symlink at %s; Claude Code agents will not see it`.
    - Exists but is not a symlink → log `WARN skill %q at %s is a regular dir, not a symlink; may diverge from canonical %s`.
    - Symlink with wrong target → log `WARN skill %q symlink points to %s, expected %s`.
    - Correct symlink → silent.
    Warnings never fail boot — they are advisory for third-party skills the user installed manually.
24. **Self-healing helper:** new exported function `skills.EnsureClaudeSymlink(skillName string) error` creates the symlink if missing. Returns an error if the target dir is absent or a non-symlink already occupies the path. Idempotent. Called automatically by the system-skill installer (AC #27–32) and by future `/install-skill` commands.
25. **Duplicate-load protection:** the registry's `loadDir` for `~/.claude/skills/` skips any entry that is a symlink pointing into `~/.agents/skills/`. Prevents double-registration of the same skill under two MD5-derived IDs.
26. The registry continues to scan `~/.claude/skills/` (some users author Claude-only skills there directly); only symlinked entries are deduplicated.

### System Skill Auto-Install

CodeMint's bundled skills (Gatherer, Goal Capture, Spec Writer, Options Proposer, Task Generator, Targeted Re-gather, Goal Verifier, etc.) are owned by the CodeMint binary. They are auto-installed to disk at boot so they participate in the same `~/.agents/skills` + `~/.claude/skills` convention as third-party skills, instead of living in a throwaway tempdir as today.

27. **Naming on disk:** each embedded skill `<name>` lands at `~/.agents/skills/codemint-<name>/`. Flat layout with the `codemint-` prefix — avoids namespace nesting (the registry's `loadDir` only reads immediate subdirs). The corresponding Claude symlink lives at `~/.claude/skills/codemint-<name>`.
28. **Boot install behavior:** at orchestrator boot, before `Registry.LoadAll`, CodeMint calls `skills.InstallSystemSkills(home)`. This walks the embedded FS and, for each skill:
    1. If `~/.agents/skills/codemint-<name>/` does not exist → write the bundled content, then write a marker file `.codemint-managed` containing the binary's `version` + `commit` (already injected via LDFLAGS in `cmd/codemint/main.go`).
    2. If the dir exists and `.codemint-managed` is **present and matches** the current binary version → no-op.
    3. If the dir exists and `.codemint-managed` is **present but stale** (older binary version) → overwrite the bundled files (SKILL.md and refs/scripts), refresh the marker. User edits are intentionally clobbered for managed skills — the marker file's existence is the consent signal.
    4. If the dir exists and `.codemint-managed` is **absent** → the path is occupied by a user-authored skill that happens to share the `codemint-<name>` prefix. Refuse to overwrite; log `WARN system skill %q cannot install: %s exists without .codemint-managed marker`.
29. **Auto-symlink:** after each successful write (cases 1 and 3), `InstallSystemSkills` calls `EnsureClaudeSymlink("codemint-<name>")`. The user does not have to do anything to make Claude Code see the system skills.
30. **Boot order:** `InstallSystemSkills` runs **before** `Registry.LoadAll`. The registry then picks the system skills up via the standard `~/.agents/skills` scan path — no special-case loading. The current `loadEmbedded` path becomes a fallback used only when on-disk install fails (e.g., read-only HOME); see AC #32.
31. **Resolution under `@codemint/<name>`:** the parser's logical-ID rule `@codemint/<name>` resolves to the on-disk skill at `~/.agents/skills/codemint-<name>/`. (How logical IDs map to the registry's MD5-derived internal keys is implementation detail — the contract is that `@codemint/<name>` always resolves when `InstallSystemSkills` succeeded.)
32. **Read-only HOME fallback:** if `InstallSystemSkills` fails (permission denied, read-only filesystem, sandbox), boot logs a single WARN and continues. The registry then loads the embedded skills via the legacy temp-dir path so CodeMint's own workflows still work, but Claude Code agents will not see the system skills (no symlinks possible). The WARN names the failed write path so the user can fix it.
33. **Reinstall command (out of scope, future):** a `/reinstall-system-skills` REPL command will force-write all system skills, ignoring the marker. Not part of v1; noted so the API in AC #28 leaves room for it.

### Slash-command compatibility (forward-looking, not required for v1)

16. The Executor records, per session, the `availableCommands` advertised by the agent via `session/update` notifications with `sessionUpdate: "available_commands_update"`. Stored in `Runtime` per session, queryable but not yet used for routing.
17. Future iteration may elide skill body injection when our skill name matches an agent-advertised command. **Out of scope for 2.0.5** — v1 always inlines. This AC documents the field so the Pipeline data model is ready.

## Technical Design

### Schema change

```go
// internal/domain/task_input.go
type TaskInput struct {
    Prompt       string            `json:"prompt"`
    ContextFiles []string          `json:"context_files,omitempty"`
    Tools        []string          `json:"tools,omitempty"`
    Command      string            `json:"command,omitempty"`
    Cwd          string            `json:"cwd,omitempty"`
    Metadata     map[string]string `json:"metadata,omitempty"`

    // NEW: skill ID copied from Story.Skill at task creation time.
    // Empty for non-skill tasks (verification command, confirmation prompt, etc.).
    Skill string `json:"skill,omitempty"`
}
```

### Workflow load-time validation (L1)

```go
// internal/workflow/file_registry.go
func (r *FileRegistry) LoadAll(skills *skills.Registry) error {
    // ... existing parse logic ...
    for _, wf := range loaded {
        for epicIdx, epic := range wf.Epics {
            for storyIdx, story := range epic.Stories {
                if story.Skill == "" {
                    continue // skill is optional; some stories are pure routing
                }
                if _, ok := skills.Get(story.Skill); !ok {
                    return fmt.Errorf(
                        "workflow %q references unknown skill %q at epic[%d].story[%d] %q",
                        wf.Name, story.Skill, epicIdx, storyIdx, story.ID,
                    )
                }
            }
        }
    }
    return nil
}
```

The skills registry is a new dependency of `FileRegistry.LoadAll`. Update call sites in `cmd/codemint/main.go` to pass it.

### TaskGenerator (L2)

```go
// internal/workflow/task_generator.go
func (g *TaskGenerator) createStoryTask(
    story domain.Story,
    workflow *domain.Workflow,
    epicIdx, storyIdx int,
) (*domain.Task, error) {
    if story.Skill != "" {
        if _, ok := g.skills.Get(story.Skill); !ok {
            return nil, fmt.Errorf("task generator: skill %q no longer in registry", story.Skill)
        }
    }

    inputJSON, _ := json.Marshal(domain.TaskInput{
        Prompt: story.Prompt,
        Skill:  story.Skill,
        // ... ContextFiles etc.
    })

    task := domain.NewTask(/* ... */)
    task.Input = sql.NullString{String: string(inputJSON), Valid: true}
    return task, nil
}
```

### Executor (L3 + injection)

```go
// internal/orchestrator/executor.go

const FailureSentinelSkillNotFound = "skill_not_found"

func (e *Executor) executeCoding(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
    // ... existing TaskInput parsing (steps 1-4) ...

    var promptBlocks []acp.ContentBlock

    // NEW: prepend skill body if skill ID is present.
    if taskInput.Skill != "" {
        skill, ok := e.skills.Get(taskInput.Skill)
        if !ok {
            e.failTaskWithReason(ctx, task, FailureSentinelSkillNotFound,
                fmt.Sprintf("skill not found: %s", taskInput.Skill))
            return nil // per-task failure, scheduler continues
        }
        if skill.Body == "" {
            e.failTaskWithReason(ctx, task, FailureSentinelSkillNotFound,
                fmt.Sprintf("skill body empty: %s", taskInput.Skill))
            return nil
        }
        framed := fmt.Sprintf("[skill: %s]\n\n%s", skill.ID, skill.Body)
        promptBlocks = append(promptBlocks, acp.TextContent(framed))
    }

    // Existing: per-task prompt.
    promptBlocks = append(promptBlocks, acp.TextContent(taskInput.Prompt))

    // Existing: context files as resource_link blocks.
    // ...

    // Existing: send session/prompt.
    params := acp.SessionPromptParams{
        SessionID: sess.GetACPSessionID(),
        Prompt:    promptBlocks,
    }
    // ...
}
```

The `Executor` gains a new `skills *skills.Registry` field, wired in `NewExecutor` and threaded from `main.go`.

### available_commands_update capture (forward-looking, AC #16)

The Pipeline already routes `session/update` notifications. Add a case for `sessionUpdate == "available_commands_update"` that stores the advertised command list on the per-session `Runtime` state. No consumer in v1; just captured for future use.

```go
// internal/acp/pipeline.go (sketch)
case "available_commands_update":
    var upd struct {
        AvailableCommands []domain.AvailableCommand `json:"availableCommands"`
    }
    _ = json.Unmarshal(raw, &upd)
    runtime.SetAvailableCommands(sessionID, upd.AvailableCommands)
```

### ACP wire format (per spec)

The actual `session/prompt` request after injection looks like:

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "session/prompt",
  "params": {
    "sessionId": "sess-…",
    "prompt": [
      { "type": "text", "text": "[skill: @codemint/gatherer]\n\n# Context Gatherer\n\nYou are…" },
      { "type": "text", "text": "Goal: locked. Cheap pass over the project tree." },
      { "type": "resource_link", "uri": "file:///abs/README.md", "name": "README.md" }
    ]
  }
}
```

This matches the spec at `agentclientprotocol.com/protocol/slash-commands.md`: prompt is an array of typed ContentBlocks; text and resource_link coexist freely.

## Dependencies

- 2.0.1 Workflow File Infrastructure (provides `Story.Skill` parser field)
- `internal/skills` Registry (already exists; provides `Get(id)`)
- `internal/orchestrator/executor.go` (existing ACP send path)
- ACP Schema audit (`appendings.md` Task A — must be conformant before any prompt-shape change)

## Blocks

- 2.1 Cheap Context Intake (Gatherer skill must execute via this path)
- 2.1.1 Targeted Re-gather
- 2.2 Living Spec
- 2.2.1 Goal Capture
- 2.3 Hierarchical Task Generation
- 2.3.1 Options Proposer
- Every other workflow story that names a `skill:` in WORKFLOW.yaml.

## Out of Scope

- Agent-advertised slash-command routing (AC #16 captures the data; routing is future work).
- Skill versioning / pinning (`@codemint/gatherer@1.0`) — v1 resolves by name only.
- Per-task skill override at runtime — only TaskGenerator writes `TaskInput.Skill`.
- Streaming skill bodies (some skills are large) — body sent as a single TextContent block; ACP spec permits this.
- **Third-party** skill auto-install or auto-symlink — only warned, never silently created. CodeMint owns the `codemint-*` prefix; everything else is the user's domain. (System skills under the `codemint-*` prefix ARE auto-installed and auto-symlinked — see §"System Skill Auto-Install".)
- `/reinstall-system-skills` REPL command — forced reinstall noted in AC #33; deferred to a follow-up.
- Windows symlink support — `~/.agents/skills` + `~/.claude/skills` convention is POSIX-first; on Windows the install step writes the directory but skips the Claude symlink and emits a single WARN.

## Notes

- The triple-checkpoint design (L1/L2/L3) is intentional defense in depth. L1 keeps unloadable workflows out of the listing; L2 catches mid-session skill removal; L3 is the last guardrail before a corrupt prompt hits the wire.
- `FailureSentinelSkillNotFound` is a new failure class. Update the retrospective skill (if any future skill consumes failure sentinels) to recognize it.
- The `[skill: <id>]` preamble is a CodeMint convention, not part of ACP. It exists so the agent's logs show which skill drove a given prompt.
