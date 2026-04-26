# Tasks: 1.14 XDG Base Directory Scaffolding

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.14-xdg-base-directory-scaffolding/`
**Tech Stack:** Go, `os`, `path/filepath`

---

## Task 1.14.1: Create XDG Path Resolution Module
* **Action:** Create `internal/xdg/paths.go`.
* **Details:**
  * Define constants for app name: `AppName = "codemint"`.
  * Implement `DataDir() string` returning `~/.local/share/codemint` (or `$XDG_DATA_HOME/codemint`).
  * Implement `ConfigDir() string` returning `~/.config/codemint` (or `$XDG_CONFIG_HOME/codemint`).
  * Implement `MemoryDir() string` returning `DataDir()/memory`.
  * Respect XDG environment variables when set, fallback to defaults.

## Task 1.14.2: Implement Directory Scaffolding Function
* **Action:** Update `internal/xdg/paths.go`.
* **Details:**
  * Implement `EnsureDirs() error` that creates all required directories:
    * `~/.local/share/codemint/memory` - For Adaptive Learning System / LLM Wiki
    * `~/.config/codemint/` - For user configuration files
  * Use `os.MkdirAll(path, 0o755)` for idempotent creation.
  * Return wrapped error with path context on failure.

## Task 1.14.3: Integrate XDG Scaffolding into App Startup
* **Action:** Update `main.go` or application initialization.
* **Details:**
  * Call `xdg.EnsureDirs()` early in startup sequence.
  * Fail fast with clear error message if directory creation fails.
  * Log created paths at debug level for troubleshooting.

## Task 1.14.4: Update Database Initialization to Use XDG Paths
* **Action:** Update `internal/db/database.go`.
* **Details:**
  * Default database path should be `xdg.DataDir()/codemint.db`.
  * Allow override via CLI flag or environment variable.
  * Remove hardcoded paths in favor of XDG module.

## Task 1.14.5: Add Platform-Specific Path Handling
* **Action:** Update `internal/xdg/paths.go`.
* **Details:**
  * Linux/macOS: Use XDG spec (`~/.local/share`, `~/.config`).
  * Windows: Use `%LOCALAPPDATA%\codemint` for data, `%APPDATA%\codemint` for config.
  * Use `runtime.GOOS` for platform detection.

## Task 1.14.6: Write Unit Tests for XDG Module
* **Action:** Create `internal/xdg/paths_test.go`.
* **Details:**
  * Test `DataDir()` returns correct path with/without `$XDG_DATA_HOME`.
  * Test `ConfigDir()` returns correct path with/without `$XDG_CONFIG_HOME`.
  * Test `EnsureDirs()` creates directories idempotently.
  * Test `EnsureDirs()` is safe to call multiple times.
  * Use `t.TempDir()` for isolated test environments.
