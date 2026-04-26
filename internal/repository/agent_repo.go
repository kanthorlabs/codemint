package repository

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// AgentRepository defines persistence operations for Agent entities.
type AgentRepository interface {
	// EnsureSystemAgents idempotently seeds the required system agents
	// (e.g., "human", "sys-auto-approve") into the agent table.
	// It is safe to call multiple times; duplicate entries are silently ignored.
	EnsureSystemAgents(ctx context.Context) error

	// FindByName retrieves an agent by its unique name.
	// Returns nil and no error if the agent does not exist.
	FindByName(ctx context.Context, name string) (*domain.Agent, error)
}
