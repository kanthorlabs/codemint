package db

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"codemint.kanthorlabs.com/internal/repository/sqlite"
)

// expectedTables lists all tables that must exist after migration Up.
var expectedTables = []string{
	"project",
	"project_permission",
	"agent",
	"session",
	"workflow",
	"task",
}

func TestInitDB(t *testing.T) {
	// Bootstrap a bare connection so we can construct the agentRepo before InitDB.
	// InitDB will run migrations on this same connection and then seed agents.
	bare, err := sqlx.Connect("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open bare connection: %v", err)
	}
	defer bare.Close()

	agentRepo := sqlite.NewAgentRepo(bare)

	db, err := InitDB(context.Background(), "file::memory:?cache=shared", agentRepo)
	if err != nil {
		t.Fatalf("InitDB returned error: %v", err)
	}
	if db == nil {
		t.Fatal("InitDB returned nil *sqlx.DB")
	}

	// Verify each expected table exists in sqlite_master.
	for _, table := range expectedTables {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&count); err != nil {
			t.Errorf("querying sqlite_master for table %q: %v", table, err)
			continue
		}
		if count != 1 {
			t.Errorf("expected table %q to exist, but it does not", table)
		}
	}

	// Verify rollback executes without errors (no foreign key violations, etc.).
	if err := goose.Down(db.DB, "migrations"); err != nil {
		t.Fatalf("goose.Down failed: %v", err)
	}
}
