# Tasks for 2.0.5: Skill Injection into ACP Prompt

## Task 2.0.5.1: Add `Skill` field to `TaskInput`

**Description:** Extend the `TaskInput` schema with an optional `Skill` field. Update parser, validator, and round-trip tests.

**Files to modify:**
- `internal/domain/task_input.go`
- `internal/domain/task_input_test.go`

**Implementation:**
```go
type TaskInput struct {
    Prompt       string            `json:"prompt"`
    ContextFiles []string          `json:"context_files,omitempty"`
    Tools        []string          `json:"tools,omitempty"`
    Command      string            `json:"command,omitempty"`
    Cwd          string            `json:"cwd,omitempty"`
    Metadata     map[string]string `json:"metadata,omitempty"`
    Skill        string            `json:"skill,omitempty"` // NEW
}
```

**Verification:**
- New round-trip test: marshal/unmarshal preserves `Skill` field byte-for-byte.
- Empty `Skill` continues to omit the JSON key (omitempty respected).
- Existing `TestParseTaskInput_RoundTrip` still passes.
- `go test ./internal/domain/...` passes.

**Estimated effort:** 0.25 day

---

## Task 2.0.5.2: Add `FailureSentinelSkillNotFound` and skill-resolution failure path

**Description:** Add a new failure sentinel for unresolvable skills; wire it into `failTaskWithReason`.

**Files to modify:**
- `internal/orchestrator/executor.go`
- `internal/orchestrator/executor_test.go`

**Implementation:**
```go
const FailureSentinelSkillNotFound = "skill_not_found"
```

Add a unit test that calls `failTaskWithReason(... FailureSentinelSkillNotFound, "skill not found: @codemint/gatherer")` and asserts the resulting `task.Output` JSON shape matches the existing sentinel-output convention.

**Verification:**
- Output JSON contains `"sentinel":"skill_not_found"` and the reason string.
- `go test ./internal/orchestrator/ -run TestFailTaskWithReason -v` passes.

**Estimated effort:** 0.25 day

---

## Task 2.0.5.3: Inject resolved skill body into `executeCoding`

**Description:** Modify `Executor.executeCoding` to (a) accept a `skills.Registry`, (b) resolve `taskInput.Skill` if non-empty, (c) prepend the framed skill body as the first `TextContent` block, (d) fail the task with `FailureSentinelSkillNotFound` when resolution fails.

**Files to modify:**
- `internal/orchestrator/executor.go`
- `internal/orchestrator/bootstrap.go` (Executor constructor)
- `internal/orchestrator/executor_test.go`
- `internal/orchestrator/runtime_e2e_test.go`

**Implementation sketch:**
```go
type Executor struct {
    // ... existing fields ...
    skills *skills.Registry // NEW
}

func NewExecutor(/* ..., */ skillsRegistry *skills.Registry) *Executor { ... }

// Inside executeCoding, BEFORE building promptBlocks:
var promptBlocks []acp.ContentBlock
if taskInput.Skill != "" {
    skill, ok := e.skills.Get(taskInput.Skill)
    if !ok {
        e.failTaskWithReason(ctx, task, FailureSentinelSkillNotFound,
            fmt.Sprintf("skill not found: %s", taskInput.Skill))
        return nil
    }
    if skill.Body == "" {
        e.failTaskWithReason(ctx, task, FailureSentinelSkillNotFound,
            fmt.Sprintf("skill body empty: %s", taskInput.Skill))
        return nil
    }
    framed := fmt.Sprintf("[skill: %s]\n\n%s", skill.ID, skill.Body)
    promptBlocks = append(promptBlocks, acp.TextContent(framed))
}
promptBlocks = append(promptBlocks, acp.TextContent(taskInput.Prompt))
// ... existing ContextFiles → resource_link logic ...
```

**Verification:**
- Unit test (mocked worker): task with `Skill = "@codemint/gatherer"` produces a `session/prompt` whose first block contains `[skill: @codemint/gatherer]` and the gatherer body.
- Unit test: task with empty `Skill` produces the existing 2-block layout (no regression).
- Unit test: task with `Skill = "@codemint/does-not-exist"` fails with sentinel `skill_not_found`; scheduler is not killed.
- Unit test: skill exists but `Body == ""` fails with sentinel `skill_not_found` and reason `"skill body empty: …"`.
- Existing e2e tests still pass.
- `go test ./internal/orchestrator/...` passes.

**Estimated effort:** 1 day

---

## Task 2.0.5.4: L1 validation in `FileRegistry.LoadAll`

**Description:** Make workflow loading validate every `Story.Skill` against the skills registry. Reject the workflow on miss.

**Files to modify:**
- `internal/workflow/file_registry.go`
- `internal/workflow/file_registry_test.go`

**Implementation:**
```go
// Signature change: LoadAll now requires the skills registry.
func (r *FileRegistry) LoadAll(skills *skills.Registry) error {
    // ... existing parse loop ...
    for _, wf := range loaded {
        for ei, epic := range wf.Epics {
            for si, story := range epic.Stories {
                if story.Skill == "" {
                    continue
                }
                if _, ok := skills.Get(story.Skill); !ok {
                    return fmt.Errorf(
                        "workflow %q references unknown skill %q at epic[%d].story[%d] %q",
                        wf.Name, story.Skill, ei, si, story.ID,
                    )
                }
            }
        }
    }
    return nil
}
```

**Verification:**
- New test: a WORKFLOW.yaml that references `@codemint/missing` fails LoadAll with the expected error message.
- New test: a WORKFLOW.yaml whose stories all resolve loads cleanly.
- New test: a WORKFLOW.yaml with one valid + one invalid skill rejects the whole file (no partial load).
- Existing FileRegistry tests adapted to pass a real `skills.Registry`.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.0.5.5: L2 re-validation in `TaskGenerator`

**Description:** Make `TaskGenerator` accept the skills registry and re-validate `Story.Skill` at generation time. Copy `Story.Skill` into `TaskInput.Skill` for every generated task.

**Files to modify:**
- `internal/workflow/task_generator.go`
- `internal/workflow/task_generator_test.go`

**Implementation sketch:**
```go
type TaskGenerator struct {
    humanAgentID     string
    assistantAgentID string
    yoloAgentID      string
    skills           *skills.Registry // NEW
}

func NewTaskGenerator(humanID, assistantID, yoloID string, skillsRegistry *skills.Registry) *TaskGenerator

// In createStoryTask:
if story.Skill != "" {
    if _, ok := g.skills.Get(story.Skill); !ok {
        return nil, fmt.Errorf("task generator: skill %q no longer in registry", story.Skill)
    }
}
input := domain.TaskInput{
    Prompt: story.Prompt,
    Skill:  story.Skill, // NEW
    // ...
}
```

**Verification:**
- Unit test: generated task for a skill-bearing story carries `TaskInput.Skill == story.Skill`.
- Unit test: skill missing at generation time returns the expected error.
- Existing `TestTaskGenerator_*` tests adapted to pass a real `skills.Registry` and still pass.
- `go test ./internal/workflow/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.0.5.6: Capture `available_commands_update` notifications

**Description:** Extend the ACP pipeline to recognize `session/update` notifications with `sessionUpdate: "available_commands_update"` and store the advertised command list on per-session `Runtime` state. No consumer yet; data is captured for future routing.

**Files to modify:**
- `internal/acp/protocol.go` (add `AvailableCommand` type if absent — verify against schema)
- `internal/acp/pipeline.go`
- `internal/orchestrator/runtime.go` (per-session command map)
- `internal/orchestrator/pipeline_consumer.go`

**Implementation sketch:**
```go
// internal/acp/protocol.go (verify shape against protocol/schema.md)
type AvailableCommand struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Input       *AvailableCommandInput `json:"input,omitempty"`
}
type AvailableCommandInput struct {
    Hint string `json:"hint"`
}

// internal/orchestrator/runtime.go
func (r *Runtime) SetAvailableCommands(sessionID string, cmds []acp.AvailableCommand)
func (r *Runtime) GetAvailableCommands(sessionID string) []acp.AvailableCommand
```

**Verification:**
- Unit test: pipeline given a `session/update` frame with `sessionUpdate: "available_commands_update"` calls `Runtime.SetAvailableCommands` with the parsed slice.
- Unit test: `Runtime.GetAvailableCommands` returns the last set value.
- Wire conformance harness (Task C in `appendings.md`) is not required to drive this — unit-level only for v1.
- `go test ./internal/acp/... ./internal/orchestrator/...` passes.

**Estimated effort:** 0.5 day

---

## Task 2.0.5.7: Wire skills registry through `main.go`

**Description:** Pass the existing `skills.Registry` (already loaded for chat) into `FileRegistry.LoadAll`, `TaskGenerator`, and `Executor`. Verify boot order in `cmd/codemint/main.go::run` honors the new dependency edges.

**Files to modify:**
- `cmd/codemint/main.go`

**Implementation:**
- Existing skills load: confirm it runs before `workflow.LoadFromConfig` and `acp.NewRegistry`.
- Pass `skillsRegistry` into:
  - `workflow.NewFileRegistry(...)` / `LoadAll(skillsRegistry)`
  - `workflow.NewTaskGenerator(humanID, assistantID, yoloID, skillsRegistry)`
  - `orchestrator.NewExecutor(..., skillsRegistry)`

**Verification:**
- Boot succeeds with the embedded brainstorming workflow + embedded skills.
- Boot fails with a clear error if a workflow references an unknown skill (deliberately corrupt fixture).
- `make test` green.
- `make build && ./build/codemint -mode cli` enters REPL; `/workflow` lists `brainstorming`.

**Estimated effort:** 0.5 day

---

## Task 2.0.5.8: End-to-end test — skill body reaches the worker

**Description:** Add an orchestrator e2e test that drives a workflow story whose skill is `@codemint/gatherer`, captures the JSON-RPC frame sent on the worker's stdin, and asserts the first `prompt` block contains the gatherer skill body.

**Files to modify:**
- `internal/orchestrator/runtime_e2e_test.go` (or sibling `system_assistant_e2e_test.go`)

**Implementation:**
- Use the existing test harness with a fake ACP worker that records sent frames.
- Generate a single Coding task with `TaskInput.Skill = "@codemint/gatherer"`.
- Assert the captured `session/prompt` request:
  - `prompt[0].type == "text"`
  - `prompt[0].text` starts with `[skill: @codemint/gatherer]\n\n`
  - `prompt[0].text` contains a substring of the embedded gatherer body.
  - `prompt[1].type == "text"` and equals `taskInput.Prompt`.

**Verification:**
- Test passes.
- Negative case: same setup but `Skill = "@codemint/missing"` — task ends in `Failure` with output sentinel `skill_not_found`.

**Estimated effort:** 0.75 day

---

## Task 2.0.5.9: Skill install convention — `~/.agents/skills` + `~/.claude/skills` symlink

**Description:** Implement the install-location convention from §"Install Convention" of the index. Skills are installed at `~/.agents/skills/<name>/`. For Claude Code agent compatibility, a symlink at `~/.claude/skills/<name>` → `~/.agents/skills/<name>` is required. CodeMint warns at boot when symlinks are missing and exposes a helper to create them. The registry skips duplicate symlinks to avoid double-registration.

**Files to modify:**
- `internal/skills/registry.go` (add boot-time check + symlink-skip in `loadDir`)
- `internal/skills/symlink.go` (NEW — `EnsureClaudeSymlink` helper)
- `internal/skills/symlink_test.go` (NEW)
- `internal/skills/registry_test.go`

**Implementation sketch:**

```go
// internal/skills/symlink.go (NEW)
package skills

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
)

// EnsureClaudeSymlink creates ~/.claude/skills/<name> as a symlink to
// ~/.agents/skills/<name>. Returns an error if:
//   - the source ~/.agents/skills/<name> does not exist
//   - the destination exists and is not a symlink (would clobber real content)
//   - the destination exists as a symlink to a different target
//
// No-op if the correct symlink already exists.
func EnsureClaudeSymlink(skillName string) error {
    home, err := os.UserHomeDir()
    if err != nil {
        return fmt.Errorf("skills: resolve home dir: %w", err)
    }
    src := filepath.Join(home, ".agents", "skills", skillName)
    dst := filepath.Join(home, ".claude", "skills", skillName)

    if _, err := os.Stat(src); err != nil {
        return fmt.Errorf("skills: source %q missing: %w", src, err)
    }

    info, err := os.Lstat(dst)
    if errors.Is(err, os.ErrNotExist) {
        if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
            return err
        }
        return os.Symlink(src, dst)
    }
    if err != nil {
        return err
    }
    if info.Mode()&os.ModeSymlink == 0 {
        return fmt.Errorf("skills: %q exists and is not a symlink; refusing to overwrite", dst)
    }
    target, err := os.Readlink(dst)
    if err != nil {
        return err
    }
    if target != src {
        return fmt.Errorf("skills: %q points to %q, expected %q", dst, target, src)
    }
    return nil
}

// VerifyClaudeSymlinks walks ~/.agents/skills and returns one warning per
// skill whose Claude symlink is missing, wrong, or replaced by a real dir.
// Never fails — used to feed boot-time slog.Warn lines.
func VerifyClaudeSymlinks() []string {
    // ... walk ~/.agents/skills, lstat each ~/.claude/skills/<name>,
    //     return human-readable warning strings.
}
```

```go
// internal/skills/registry.go — extend loadDir for ~/.claude/skills only.
func (r *Registry) loadDir(dirPath string, parser SkillParser) error {
    // ... existing logic ...
    for _, entry := range entries {
        if !entry.IsDir() { continue }
        skillDir := filepath.Join(dirPath, entry.Name())

        // NEW: in ~/.claude/skills, skip symlinks pointing into ~/.agents/skills
        if strings.Contains(dirPath, filepath.Join(".claude", "skills")) {
            if info, err := os.Lstat(skillDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
                target, _ := os.Readlink(skillDir)
                home, _ := os.UserHomeDir()
                if strings.HasPrefix(target, filepath.Join(home, ".agents", "skills")) {
                    continue // canonical copy is loaded via ~/.agents/skills target
                }
            }
        }
        // ... existing parse + register logic ...
    }
}

// LoadAll calls VerifyClaudeSymlinks() after the scan and emits slog.Warn
// for each entry. Warnings never fail boot.
```

**Verification:**
- Unit test: `EnsureClaudeSymlink` creates a missing symlink in a temp HOME and the resulting `os.Readlink` matches the source.
- Unit test: `EnsureClaudeSymlink` is idempotent — second call on a correct symlink returns nil.
- Unit test: `EnsureClaudeSymlink` refuses to clobber a real directory at the destination.
- Unit test: `EnsureClaudeSymlink` returns the "points to X, expected Y" error when the symlink target differs.
- Unit test: registry `loadDir` for `~/.claude/skills` skips a symlinked entry pointing into `~/.agents/skills` (asserts the skill is registered exactly once via the canonical path, not twice).
- Unit test: registry `loadDir` for `~/.claude/skills` still loads a regular (non-symlink) skill directory there (Claude-only skills).
- Unit test: `VerifyClaudeSymlinks` returns the expected warning strings for missing, wrong-target, and not-a-symlink cases.
- Boot smoke test: with a fixture home that has `~/.agents/skills/foo` but no `~/.claude/skills/foo`, boot succeeds and the slog output contains a WARN line naming `foo`.
- Boot smoke test on darwin/linux only — Windows skipped via build tag or runtime check.

**Estimated effort:** 1 day

---

## Task 2.0.5.11: Auto-install system skills + auto-symlink at boot

**Description:** Implement `skills.InstallSystemSkills(home)` per index AC #27–32. At boot, write each embedded skill to `~/.agents/skills/codemint-<name>/`, drop a `.codemint-managed` marker containing the binary's version + commit, and call `EnsureClaudeSymlink("codemint-<name>")` for each. Honors the marker so user edits are protected (case 4 in AC #28); refreshes content when the marker version is stale (case 3).

**Files to modify:**
- `internal/skills/install.go` (NEW)
- `internal/skills/install_test.go` (NEW)
- `internal/skills/registry.go` (drop or guard the legacy temp-dir `loadEmbedded` path; use it only as fallback when `InstallSystemSkills` errored)
- `cmd/codemint/main.go` (call `InstallSystemSkills` before `skills.Registry.LoadAll`; thread the binary `version` + `commit` strings already used by LDFLAGS)
- `internal/skills/embedded.go` (verify `embeddedFS` is exposed at package scope for the installer to walk)

**Implementation sketch:**

```go
// internal/skills/install.go (NEW)
package skills

import (
    "errors"
    "fmt"
    "io/fs"
    "log/slog"
    "os"
    "path/filepath"
    "runtime"
)

const managedMarkerFile = ".codemint-managed"

// SystemSkillName returns the on-disk directory name for an embedded skill.
// Embedded skill source dir name "gatherer" → on-disk "codemint-gatherer".
func SystemSkillName(embeddedDirName string) string {
    return "codemint-" + embeddedDirName
}

// InstallSystemSkills writes every embedded skill to ~/.agents/skills/codemint-<name>/
// and ensures a Claude symlink exists for each. version is the binary version
// string (e.g. "0.1.3+abcd123") used for the .codemint-managed marker.
//
// Returns nil even if individual skills fail to install — failures are logged
// at WARN and the embedded fallback in Registry.loadEmbedded() will pick them up.
// Returns a hard error only if HOME cannot be resolved.
func InstallSystemSkills(version string) error {
    home, err := os.UserHomeDir()
    if err != nil {
        return fmt.Errorf("skills: resolve home: %w", err)
    }
    targetRoot := filepath.Join(home, ".agents", "skills")
    if err := os.MkdirAll(targetRoot, 0o755); err != nil {
        slog.Warn("skills: cannot create install root", "path", targetRoot, "err", err)
        return nil // soft-fail; Registry will use embedded fallback
    }

    return fs.WalkDir(embeddedFS, "embedded", func(p string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        // Find each skill's root: "embedded/<name>/SKILL.md"
        if d.IsDir() || filepath.Base(p) != "SKILL.md" { return nil }

        // p == "embedded/<name>/SKILL.md" (or deeper if scripts nested) — take the
        // immediate parent of SKILL.md.
        skillSrcDir := filepath.Dir(p)
        skillName := filepath.Base(skillSrcDir)
        if skillSrcDir == "embedded" || skillName == "embedded" {
            return nil // skip non-skill SKILL.md at root if any
        }

        targetDir := filepath.Join(targetRoot, SystemSkillName(skillName))
        if err := installOne(skillSrcDir, targetDir, version); err != nil {
            slog.Warn("skills: install system skill failed",
                "skill", skillName, "target", targetDir, "err", err)
            return nil // don't abort the whole walk
        }

        if runtime.GOOS == "windows" {
            slog.Warn("skills: skipping Claude symlink on Windows", "skill", SystemSkillName(skillName))
            return nil
        }
        if err := EnsureClaudeSymlink(SystemSkillName(skillName)); err != nil {
            slog.Warn("skills: symlink failed",
                "skill", SystemSkillName(skillName), "err", err)
        }
        return nil
    })
}

func installOne(srcEmbeddedDir, targetDir, version string) error {
    info, err := os.Stat(targetDir)
    switch {
    case errors.Is(err, os.ErrNotExist):
        // Case 1: fresh install.
        return writeAndMark(srcEmbeddedDir, targetDir, version)
    case err != nil:
        return err
    case !info.IsDir():
        return fmt.Errorf("%q exists and is not a directory", targetDir)
    }

    markerPath := filepath.Join(targetDir, managedMarkerFile)
    markerData, err := os.ReadFile(markerPath)
    if errors.Is(err, os.ErrNotExist) {
        // Case 4: user-authored skill, do not clobber.
        return fmt.Errorf("%q exists without %s marker", targetDir, managedMarkerFile)
    }
    if err != nil {
        return err
    }
    if string(markerData) == version {
        return nil // Case 2: managed and up-to-date.
    }
    // Case 3: managed but stale → overwrite.
    if err := os.RemoveAll(targetDir); err != nil {
        return fmt.Errorf("remove stale %q: %w", targetDir, err)
    }
    return writeAndMark(srcEmbeddedDir, targetDir, version)
}

func writeAndMark(srcEmbeddedDir, targetDir, version string) error {
    if err := os.MkdirAll(targetDir, 0o755); err != nil {
        return err
    }
    err := fs.WalkDir(embeddedFS, srcEmbeddedDir, func(p string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        rel, _ := filepath.Rel(srcEmbeddedDir, p)
        out := filepath.Join(targetDir, rel)
        if d.IsDir() {
            return os.MkdirAll(out, 0o755)
        }
        data, err := embeddedFS.ReadFile(p)
        if err != nil { return err }
        return os.WriteFile(out, data, 0o644)
    })
    if err != nil {
        return err
    }
    return os.WriteFile(filepath.Join(targetDir, managedMarkerFile), []byte(version), 0o644)
}
```

```go
// cmd/codemint/main.go — add before skills.Registry.LoadAll:
if err := skills.InstallSystemSkills(version + "+" + commit); err != nil {
    slog.Error("skills: install system skills", "err", err)
}
// Then existing:
skillsRegistry := skills.NewRegistry()
if err := skillsRegistry.LoadAll(); err != nil { ... }
```

```go
// internal/skills/registry.go — guard loadEmbedded so it only runs when no
// system skills made it onto disk. The simplest signal: if any
// "~/.agents/skills/codemint-*" directory exists, skip loadEmbedded.
func (r *Registry) loadEmbeddedIfNeeded(home string) error {
    matches, _ := filepath.Glob(filepath.Join(home, ".agents", "skills", "codemint-*"))
    if len(matches) > 0 {
        return nil // disk install succeeded; system skills will be loaded via the standard scan
    }
    return r.loadEmbedded() // fallback for read-only HOME
}
```

**Verification:**
- Unit test (fresh install): with empty fixture HOME, `InstallSystemSkills("v1")` creates `~/.agents/skills/codemint-gatherer/SKILL.md`, the marker file equals `"v1"`, and `~/.claude/skills/codemint-gatherer` is a symlink to the agents dir. Asserted for at least three system skills.
- Unit test (idempotent up-to-date): re-running with same version is a no-op — file mtimes unchanged; marker still equals `"v1"`.
- Unit test (stale upgrade): pre-populate target dir with `.codemint-managed` containing `"v0"` and a modified SKILL.md → install with `"v1"` overwrites SKILL.md and updates marker.
- Unit test (user-authored guard): pre-populate target dir without the marker → install logs WARN, leaves the dir alone, returns nil. Use a slog handler that records into a buffer to assert the warning.
- Unit test (read-only HOME): make `~/.agents/skills` un-writable → install logs WARN and returns nil; Registry's `loadEmbeddedIfNeeded` still loads the system skills via the legacy temp path.
- Unit test (Windows skip): on `runtime.GOOS == "windows"` (or via a build tag stub) the symlink step is skipped and a WARN is emitted; the on-disk write still happens.
- Integration test: full boot in a fresh fixture HOME → embedded brainstorming workflow loads, references like `@codemint/gatherer` resolve, e2e test from 2.0.5.8 still passes.
- `make test` green; `make build && ./build/codemint -mode cli` produces the expected files in `~/.agents/skills/codemint-*` and symlinks in `~/.claude/skills/codemint-*` on first run.

**Estimated effort:** 1 day

---

## Task 2.0.5.12: Documentation — link from CLAUDE.md and ACP coverage map

**Description:** Update the architecture docs so future contributors know how skill injection, the install convention, and the system-skill auto-install work, and where to look.

**Files to modify:**
- `CLAUDE.md` (under "Skills, workflows, providers" section)
- `docs/coding/acp-coverage.md` (Task B in `appendings.md` — add a row noting Slash-Commands as **Partial** with link to this story)

**Implementation:**
- Add one short paragraph in CLAUDE.md noting:
  1. Workflow steps inject their skill body as the first `TextContent` block of `session/prompt`, with three-layer find-or-throw guarantees (L1 load, L2 generate, L3 execute).
  2. Third-party skills install at `~/.agents/skills/<name>/`. A symlink at `~/.claude/skills/<name>` → `~/.agents/skills/<name>` is required for Claude Code agent compatibility; CodeMint warns at boot when the symlink is missing.
  3. CodeMint's bundled "system skills" auto-install at boot to `~/.agents/skills/codemint-<name>/` with a `.codemint-managed` marker file, and the Claude symlinks are auto-created. User edits to non-managed (third-party) skills are never clobbered; managed ones are refreshed on binary upgrade.
- In `acp-coverage.md`: row for Slash Commands → **Partial** — "skills injected as TextContent (2.0.5); agent-advertised commands captured but not yet routed."

**Verification:**
- Docs render cleanly.
- A teammate not on this PR can find skill-injection logic, the install convention, and the system-skill auto-install behavior from CLAUDE.md alone.

**Estimated effort:** 0.25 day

---

## Dependency Order

```
2.0.5.1 (TaskInput.Skill field)
    │
    ├──► 2.0.5.2 (Failure sentinel)
    │
    ▼
2.0.5.3 (Executor injection — L3)
    │
    ├──► 2.0.5.4 (FileRegistry validation — L1)
    │       │
    │       ▼
    │   2.0.5.5 (TaskGenerator — L2, copies Skill into TaskInput)
    │
    ├──► 2.0.5.6 (available_commands_update capture)
    │
    └──► 2.0.5.9 (EnsureClaudeSymlink + boot warn for third-party)
                │
                ▼
            2.0.5.11 (InstallSystemSkills + auto-symlink)
                │
                ▼
            2.0.5.7 (Wire through main.go — calls InstallSystemSkills before Registry.LoadAll)
                │
                ▼
            2.0.5.8 (E2E test)
                │
                ▼
            2.0.5.12 (Docs)
```

**Note:** 2.0.5 must complete before any of 2.1, 2.1.1, 2.2, 2.2.1, 2.3, 2.3.1 can be exercised end-to-end — those stories' acceptance criteria assume their skill body actually reaches the agent. 2.0.5.11 depends on 2.0.5.9 (uses `EnsureClaudeSymlink`); both feed into 2.0.5.7 which calls `InstallSystemSkills` before `Registry.LoadAll` and surfaces third-party warnings.

## Total Estimated Effort: ~6.5 days
