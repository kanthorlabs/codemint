// Package sqlite provides SQLite-backed implementations of the repository interfaces.
package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// systemAgents lists the agents that must always exist in the database.
// Using INSERT OR IGNORE makes each insertion idempotent on the unique name column.
var systemAgents = []domain.Agent{
	{Name: "human", Type: domain.AgentTypeHuman},
	{Name: "sys-auto-approve", Type: domain.AgentTypeSystem},
}

// agentRepo is the SQLite implementation of repository.AgentRepository.
type agentRepo struct {
	db *sqlx.DB
}

// NewAgentRepo constructs an AgentRepository backed by the given SQLite connection.
func NewAgentRepo(db *sqlx.DB) repository.AgentRepository {
	return &agentRepo{db: db}
}

// EnsureSystemAgents idempotently inserts all system agents using INSERT OR IGNORE
// so that repeated calls do not produce duplicates or errors.
func (r *agentRepo) EnsureSystemAgents(ctx context.Context) error {
	const query = `INSERT OR IGNORE INTO agent (id, name, type, assistant) VALUES (?, ?, ?, ?)`

	for _, a := range systemAgents {
		id := idgen.MustNew()
		if _, err := r.db.ExecContext(ctx, query, id, a.Name, int(a.Type), ""); err != nil {
			return fmt.Errorf("sqlite: ensure system agent %q: %w", a.Name, err)
		}
	}
	return nil
}

// FindByName retrieves an agent by its unique name.
// Returns nil, nil when no matching row exists.
func (r *agentRepo) FindByName(ctx context.Context, name string) (*domain.Agent, error) {
	var a domain.Agent
	err := r.db.GetContext(ctx, &a, `SELECT id, name, type, assistant FROM agent WHERE name = ?`, name)
	if err != nil {
		// sqlx returns sql.ErrNoRows when the query returns no rows.
		// Return nil, nil to match the interface contract.
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: find agent by name %q: %w", name, err)
	}
	return &a, nil
}
