package sqlite

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite"

	"codemint.kanthorlabs.com/internal/db"
)

// openTestDB creates an in-memory SQLite DB and runs migrations using the
// canonical embedded FS from the db package — no SQL duplication.
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Connect("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	// Enable foreign key constraints (SQLite doesn't enforce them by default).
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	goose.SetBaseFS(db.Migrations)
	goose.SetDialect("sqlite3") //nolint:errcheck
	if err := goose.Up(conn.DB, "migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestEnsureSystemAgents_Seeds(t *testing.T) {
	conn := openTestDB(t)
	repo := NewAgentRepo(conn)
	ctx := context.Background()

	if err := repo.EnsureSystemAgents(ctx); err != nil {
		t.Fatalf("EnsureSystemAgents returned error: %v", err)
	}

	// Assert both agents exist with correct types.
	cases := []struct {
		name     string
		wantType int
	}{
		{"human", 0},
		{"sys-auto-approve", 2},
	}

	for _, tc := range cases {
		agent, err := repo.FindByName(ctx, tc.name)
		if err != nil {
			t.Errorf("FindByName(%q) error: %v", tc.name, err)
			continue
		}
		if agent == nil {
			t.Errorf("expected agent %q to exist, got nil", tc.name)
			continue
		}
		if int(agent.Type) != tc.wantType {
			t.Errorf("agent %q: got type %d, want %d", tc.name, agent.Type, tc.wantType)
		}
	}
}

func TestEnsureSystemAgents_Idempotent(t *testing.T) {
	conn := openTestDB(t)
	repo := NewAgentRepo(conn)
	ctx := context.Background()

	// Call twice – must not error and must not create duplicates.
	for range 2 {
		if err := repo.EnsureSystemAgents(ctx); err != nil {
			t.Fatalf("EnsureSystemAgents returned error on repeated call: %v", err)
		}
	}

	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent`).Scan(&count); err != nil {
		t.Fatalf("count agents: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 agents after idempotent seeding, got %d", count)
	}
}
