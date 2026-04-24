# Tasks: 1.7 Dynamic Workflow & Skill Registry

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.7-dynamic-skill-registry/`
**Tech Stack:** Go, `go:embed`, Frontmatter parser (e.g., `github.com/yuin/goldmark-meta`)

---

## Architectural Concept: The AgentSkills Paradigm & Strategy Parsers

CodeMint adopts the directory-based "Skill" architecture per [AgentSkills Specification](https://agentskills.io/specification). A skill is defined by a `SKILL.md` file (metadata + instructions) and optional directories (`scripts/`, `references/`, `assets/`).

Because different ecosystems (Cursor, Claude, Codex) may subtly modify the AgentSkills specification, CodeMint uses **Provider-Specific Parsers**. Currently only `GeneralParser` is implemented; others return "not implemented" error until concrete drift cases are identified.

**Precedence Rule:** CodeMint embedded skills win collisions. External directories are sub-agents for CodeMint.

---

## Task 1.7.1: Define the Skill Domain Models

* **Action:** Create `internal/domain/skill.go`.
* **Details:**
    * Define structs per [AgentSkills Frontmatter Spec](https://agentskills.io/specification#frontmatter):
      ```go
      type Skill struct {
          ID            string            // MD5 hash of absolute path to SKILL.md
          Name          string            // Required. Must match parent directory name. 1-64 chars, lowercase, hyphens.
          Description   string            // Required. 1-1024 chars. What skill does + when to use.
          License       string            // Optional. License name or reference to bundled file.
          Compatibility string            // Optional. 1-500 chars. Environment requirements.
          Metadata      map[string]string // Optional. Arbitrary key-value pairs.
          AllowedTools  string            // Optional. Space-separated pre-approved tools (experimental).
          Instruction   string            // Parsed from SKILL.md Markdown body.
          Scripts       []SkillScript     // Parsed from scripts/ directory.
          References    []string          // Parsed from references/ directory (file paths).
          SourceDir     string            // Absolute path to skill directory.
      }

      type SkillScript struct {
          Name       string // e.g., "lint"
          Executable string // e.g., "scripts/lint.sh"
      }
      ```
    * **ID Generation:** `ID = MD5(absolute_path_to_SKILL.md)` — deterministic across environments.
    * **Validation:** `Name` must match parent directory name at parse time.

## Task 1.7.2: Implement the Strategy Parsers

* **Action:** Create `internal/skills/parser.go`.
* **Details:**
    * Define base interface:
      ```go
      type SkillParser interface {
          Parse(dirPath string) (*domain.Skill, error)
      }
      ```
    * **Implementations:**
        * `GeneralParser`: Fully implements AgentSkills spec. Used for all directories initially.
        * `CursorParser`: Returns `ErrNotImplemented` — add when Cursor-specific drift identified.
        * `ClaudeParser`: Returns `ErrNotImplemented` — add when Claude CLI drift identified.
        * `CodexParser`: Returns `ErrNotImplemented` — add when Codex drift identified.
    * **Parsing Logic (GeneralParser):**
        1. Read `SKILL.md`, parse YAML frontmatter
        2. Validate `name` matches directory name
        3. Generate ID via MD5 hash
        4. Scan `scripts/` for executables
        5. Scan `references/` for markdown files
        6. Return populated `Skill` struct

## Task 1.7.3: The Multi-Directory Aggregator

* **Action:** Create `internal/skills/registry.go`.
* **Details:**
    * Create `Registry` struct holding `map[string]domain.Skill` (key = Skill ID).
    * Implement `LoadAll()`. Load in reverse-precedence order so CodeMint defaults win:
      ```go
      targets := []struct{ Path string; Parser SkillParser }{
          {"~/.agents/skills", GeneralParser{}},
          {"~/.codex/skills", GeneralParser{}},   // Uses GeneralParser until CodexParser implemented
          {"~/.claude/skills", GeneralParser{}},  // Uses GeneralParser until ClaudeParser implemented
          {"~/.cursor/skills", GeneralParser{}},  // Uses GeneralParser until CursorParser implemented
          {"~/.local/share/codemint/skills", GeneralParser{}},
      }
      ```
    * On ID collision: later entry (higher precedence) overwrites earlier.
    * Embedded skills loaded last = highest precedence.

## Task 1.7.4: Embed the Core CodeMint Skills

* **Action:** Create `internal/skills/embedded/seniorgodev/`.
* **Details:**
    * **Directory Structure:**
      ```
      seniorgodev/
      ├── SKILL.md
      ├── scripts/
      │   ├── lint.sh
      │   └── test.sh
      └── references/
          ├── introduction.md
          ├── formatting.md
          ├── commentary.md
          ├── names.md
          ├── semicolons.md
          ├── control-structures.md
          ├── functions.md
          ├── data.md
          ├── methods.md
          ├── interfaces-and-types.md
          ├── generics.md
          ├── iterators.md
          ├── blank-identifier.md
          ├── embedding.md
          ├── concurrency.md
          ├── errors.md
          ├── panic-and-recover.md
          ├── modules-and-dependencies.md
          ├── testing.md
          ├── security-and-cryptography.md
          ├── performance.md
          └── version-history.md
      ```
    * **SKILL.md Content:**
      ```markdown
      ---
      name: seniorgodev
      description: Senior Go developer skill. Code review, refactoring, testing, performance optimization following Effective Go 2026 guidelines. Use for Go projects requiring idiomatic, production-quality code.
      compatibility: Requires Go 1.22+, staticcheck, gofmt
      metadata:
        author: kanthorlabs
        version: "1.0"
        go-version: "1.26"
      ---

      # Senior Go Developer

      Expert Go development following Effective Go 2026 best practices.

      ## Best Practices Summary

      1. **Format with gofmt** - always
      2. **Handle all errors** - never ignore
      3. **Use short variable names** - especially for locals with small scope
      4. **Accept interfaces, return concrete types** - for flexibility
      5. **Use generics judiciously** - when type safety adds value, not for every function
      6. **Prefer composition over inheritance** - via embedding
      7. **Channel-based communication** - for coordinating goroutines
      8. **Context for cancellation** - pass `context.Context` as first parameter
      9. **Test thoroughly** - table-driven tests, synctest for concurrency
      10. **Profile before optimizing** - measure, don't guess

      ## References

      Detailed guidelines by topic:

      - [Introduction](references/introduction.md)
      - [Formatting](references/formatting.md)
      - [Commentary](references/commentary.md)
      - [Names](references/names.md)
      - [Semicolons](references/semicolons.md)
      - [Control Structures](references/control-structures.md)
      - [Functions](references/functions.md)
      - [Data](references/data.md)
      - [Methods](references/methods.md)
      - [Interfaces and Types](references/interfaces-and-types.md)
      - [Generics](references/generics.md)
      - [Iterators](references/iterators.md)
      - [Blank Identifier](references/blank-identifier.md)
      - [Embedding](references/embedding.md)
      - [Concurrency](references/concurrency.md)
      - [Errors](references/errors.md)
      - [Panic and Recover](references/panic-and-recover.md)
      - [Modules and Dependencies](references/modules-and-dependencies.md)
      - [Testing](references/testing.md)
      - [Security and Cryptography](references/security-and-cryptography.md)
      - [Performance](references/performance.md)
      - [Version History](references/version-history.md)

      ## Available Scripts

      - `scripts/lint.sh` - Run gofmt, go vet, staticcheck
      - `scripts/test.sh` - Run tests with coverage report
      ```
    * **scripts/lint.sh:**
      ```bash
      #!/bin/bash
      set -e
      gofmt -l -w .
      go vet ./...
      staticcheck ./...
      ```
    * **scripts/test.sh:**
      ```bash
      #!/bin/bash
      set -e
      go test -race -coverprofile=coverage.out ./...
      go tool cover -func=coverage.out
      ```
    * Use `//go:embed` to bake into binary.
    * `Registry.LoadAll()` loads embedded skills last (highest precedence).

## Task 1.7.5: Registry Unit Tests

* **Action:** Create `internal/skills/registry_test.go`.
* **Details:**
    * *Test A (ID Generation):* Assert MD5 hash of path produces consistent ID.
    * *Test B (Name Validation):* Assert parse fails if frontmatter `name` != directory name.
    * *Test C (Frontmatter Parsing):* Assert all AgentSkills fields parsed correctly.
    * *Test D (Precedence):* If `~/.cursor/skills/git` and embedded `git` both exist, assert embedded wins.
    * *Test E (Not Implemented):* Assert CursorParser/ClaudeParser/CodexParser return `ErrNotImplemented`.
