// Package db provides database initialization and migration utilities
// for the CodeMint SQLite persistence layer.
package db

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"codemint.kanthorlabs.com/internal/repository"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations is the embedded filesystem containing all goose SQL migration files.
// It is exported so that other packages (e.g. test helpers) can run migrations
// without duplicating the SQL files on disk.
var Migrations = migrations

// InitDB opens (or creates) a SQLite database at dbPath, ensures the parent
// directory exists, applies all pending goose migrations, and then calls
// agentRepo.EnsureSystemAgents so that required seed data is present before
// the function returns.
//
// Use "file::memory:?cache=shared" as dbPath for in-memory databases in tests.
//
// Deprecated: Use OpenDB followed by RunMigrations for better control over
// connection parameters (e.g., busy_timeout, WAL mode).
func InitDB(ctx context.Context, dbPath string, agentRepo repository.AgentRepository) (*sqlx.DB, error) {
	if dbPath != "file::memory:?cache=shared" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("db: create parent directory: %w", err)
		}
	}

	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("db: open connection: %w", err)
	}

	// Enable foreign key constraints (SQLite doesn't enforce them by default).
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("db: enable foreign keys: %w", err)
	}

	goose.SetBaseFS(migrations)
	goose.SetDialect("sqlite3") //nolint:errcheck

	if err := goose.Up(db.DB, "migrations"); err != nil {
		return nil, fmt.Errorf("db: run migrations: %w", err)
	}

	if err := agentRepo.EnsureSystemAgents(ctx); err != nil {
		return nil, fmt.Errorf("db: seed system agents: %w", err)
	}

	return db, nil
}

// OpenDB opens a SQLite database at dbPath with recommended pragmas for
// reliability and performance:
//   - busy_timeout=5000: Wait up to 5 seconds for locks.
//   - journal_mode=WAL: Write-Ahead Logging for better concurrent reads.
//   - foreign_keys=ON: Enforce foreign key constraints.
//
// The connection pool is limited to 1 to prevent SQLite locking issues.
// The parent directory is created if it does not exist.
func OpenDB(dbPath string) (*sqlx.DB, error) {
	if dbPath != "file::memory:?cache=shared" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("db: create parent directory: %w", err)
		}
	}

	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("db: open connection: %w", err)
	}

	// Set max open connections to 1 to prevent SQLite locking issues.
	db.SetMaxOpenConns(1)

	// Configure SQLite pragmas for reliability and performance.
	// These must be executed as separate statements after connecting.
	pragmas := []string{
		"PRAGMA busy_timeout = 5000",   // Wait up to 5 seconds for locks
		"PRAGMA journal_mode = WAL",    // Write-Ahead Logging
		"PRAGMA foreign_keys = ON",     // Enforce FK constraints
		"PRAGMA synchronous = NORMAL",  // Good durability with WAL
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("db: execute pragma %q: %w", pragma, err)
		}
	}

	return db, nil
}

// RunMigrations applies all pending goose migrations to the given database
// connection. This function should be called after OpenDB to initialize
// the schema.
func RunMigrations(db *sqlx.DB) error {
	goose.SetBaseFS(migrations)
	goose.SetDialect("sqlite3") //nolint:errcheck

	if err := goose.Up(db.DB, "migrations"); err != nil {
		return fmt.Errorf("db: run migrations: %w", err)
	}

	return nil
}
