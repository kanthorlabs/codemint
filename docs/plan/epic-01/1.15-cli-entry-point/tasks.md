# Tasks: 1.15 CLI Entry Point & Component Wiring

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.15-cli-entry-point/`
**Tech Stack:** Go, cobra/urfave-cli, os/signal, context
**Priority:** P0 (Blocking - Cannot ship without this)

---

## Task 1.15.1: Create CLI Directory Structure
* **Action:** Create `cmd/codemint/` directory with `main.go`.
* **Details:**
  * Create `cmd/codemint/main.go` as the entry point.
  * Define `main()` function that calls `run()` returning error.
  * Use `os.Exit(1)` only in `main()`, keep `run()` testable.
  * Add build metadata variables: `version`, `commit`, `buildDate`.
* **Verification:**
  * `go build -o codemint ./cmd/codemint` produces a binary.
  * `./codemint --version` prints version info.

## Task 1.15.2: Implement Flag Parsing
* **Action:** Add CLI flag parsing using `flag` stdlib or `cobra`.
* **Details:**
  * `--version` / `-v`: Print version and exit.
  * `--help` / `-h`: Print usage and exit.
  * `--config` / `-c`: Path to config.yaml (default: `$XDG_CONFIG_HOME/codemint/config.yaml`).
  * `--db`: Override database path (default: `xdg.DatabasePath()`).
  * `--mode`: Client mode (`cli` or `daemon`, default: `cli`).
* **Verification:**
  * `./codemint --help` shows all flags with descriptions.
  * `./codemint --config /tmp/test.yaml` accepts custom config path.
  * Invalid flags print error and usage.

## Task 1.15.3: Implement Signal Handling for Graceful Shutdown
* **Action:** Create `internal/app/shutdown.go` or inline in main.
* **Details:**
  * Use `signal.NotifyContext` (Go 1.16+) for clean cancellation.
  * Listen for `SIGINT` (Ctrl+C) and `SIGTERM`.
  * Pass cancellable context to all long-running goroutines.
  * Log shutdown initiation and completion.
  * Set timeout (e.g., 10s) for graceful shutdown before force exit.
* **Verification:**
  * Start app, press Ctrl+C → logs "shutting down" and exits cleanly.
  * Start app, `kill -TERM <pid>` → same graceful shutdown.
  * Verify no goroutine leaks with `-race` flag.

## Task 1.15.4: Configure SQLite with Busy Timeout
* **Action:** Update `internal/db/database.go`.
* **Details:**
  * Add `_busy_timeout=5000` pragma to connection string (5 seconds).
  * Add `_journal_mode=WAL` for better concurrent read performance.
  * Set `db.SetMaxOpenConns(1)` to prevent SQLite locking issues.
  * Add `_foreign_keys=ON` to enforce FK constraints.
* **Connection string example:**
  ```
  file:/path/to/db.sqlite?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=ON
  ```
* **Verification:**
  * Two goroutines writing concurrently don't get "database is locked".
  * FK violations return errors (not silently ignored).
  * Write test that spawns concurrent writers.

## Task 1.15.5: Wire All Components in main()
* **Action:** Update `cmd/codemint/main.go`.
* **Details:**
  * Call `xdg.EnsureDirs()` first.
  * Initialize database with `db.InitDB(dbPath)`.
  * Create repositories: `TaskRepo`, `AgentRepo`, `ProjectRepo`, `SessionRepo`.
  * Create `CommandRegistry` and register all commands.
  * Create `UIMediator` with `os.Stdout` writer.
  * Create `Dispatcher` with registry and mediator.
  * Create `ActiveSession` (global mode by default).
  * Start REPL loop reading from `os.Stdin`.
* **Dependency order:**
  ```
  xdg.EnsureDirs() → db.InitDB() → Repos → Registry → Mediator → Dispatcher → REPL
  ```
* **Verification:**
  * `./codemint` starts and shows prompt.
  * `/help` lists all registered commands.
  * `/exit` triggers graceful shutdown.

## Task 1.15.6: Implement Basic REPL Loop
* **Action:** Create `internal/repl/loop.go`.
* **Details:**
  * Use `bufio.Scanner` to read lines from stdin.
  * Trim whitespace, skip empty lines.
  * Call `dispatcher.Dispatch(ctx, activeSession, line)`.
  * Handle `ErrShutdownGracefully` to exit loop.
  * Print errors to stderr with prefix.
  * Support line editing if terminal is TTY (optional: readline).
* **Verification:**
  * Type `/help` → see command list.
  * Type natural language → see "brainstormer not available" error (expected).
  * Type `/exit` → clean exit.
  * Ctrl+D (EOF) → clean exit.

## Task 1.15.7: Add Build Metadata via ldflags
* **Action:** Update `Makefile` or build script.
* **Details:**
  * Inject version from git tag: `-X main.version=$(git describe --tags)`.
  * Inject commit hash: `-X main.commit=$(git rev-parse --short HEAD)`.
  * Inject build date: `-X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)`.
* **Example Makefile:**
  ```makefile
  VERSION := $(shell git describe --tags --always)
  COMMIT := $(shell git rev-parse --short HEAD)
  DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
  LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)"
  
  build:
  	go build $(LDFLAGS) -o codemint ./cmd/codemint
  ```
* **Verification:**
  * `make build && ./codemint --version` shows git tag, commit, date.
  * Version info matches current git state.

## Task 1.15.8: Write Integration Test for Startup/Shutdown
* **Action:** Create `cmd/codemint/main_test.go`.
* **Details:**
  * Test that `run()` returns nil on clean shutdown.
  * Test that SIGINT triggers graceful shutdown.
  * Test that invalid flags return non-nil error.
  * Use `t.TempDir()` for isolated database.
* **Verification:**
  * `go test ./cmd/codemint -v` passes.
  * Test covers startup, command dispatch, and shutdown.
