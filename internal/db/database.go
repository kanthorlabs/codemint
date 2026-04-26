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
