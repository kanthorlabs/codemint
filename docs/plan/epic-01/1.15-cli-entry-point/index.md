# User Story 1.15: CLI Entry Point & Component Wiring

* **As a** User,
* **I want** CodeMint to ship as a standalone CLI binary with proper startup, shutdown, and configuration handling,
* **So that** I can install and run the orchestrator as a single executable without external dependencies.
* *Acceptance Criteria:*
    * A `cmd/codemint/main.go` entry point exists and compiles to a single binary.
    * CLI supports `--version`, `--help`, and `--config` flags.
    * Application handles SIGINT/SIGTERM for graceful shutdown.
    * All components (DB, repositories, registries, mediator) are wired together at startup.
    * SQLite is configured with busy timeout to prevent "database is locked" errors.
    * XDG directories are created on first boot before any other initialization.
