// Package db provides database initialization and migration utilities
// for the CodeMint SQLite persistence layer.
package db

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// InitDB opens (or creates) a SQLite database at dbPath, ensures the parent
// directory exists, and automatically applies all pending goose migrations.
// Use "file::memory:?cache=shared" as dbPath for in-memory databases in tests.
func InitDB(dbPath string) (*sqlx.DB, error) {
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

	goose.SetBaseFS(migrations)
	goose.SetDialect("sqlite3") //nolint:errcheck

	if err := goose.Up(db.DB, "migrations"); err != nil {
		return nil, fmt.Errorf("db: run migrations: %w", err)
	}

	return db, nil
}
