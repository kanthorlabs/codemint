// Package agent provides agent implementations for CodeMint.
// This file implements the CodingAgent interface for the Accept/Revert contract
// used in atomic undo (Story 1.9).
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
)

// ErrCodingAgentNotConfigured is returned when the coding agent is not configured.
var ErrCodingAgentNotConfigured = errors.New("agent: coding agent not configured")

// ErrSessionRequired is returned when a session is required but not provided.
var ErrSessionRequired = errors.New("agent: session is required")

// ErrProjectRequired is returned when a project is required but not provided.
var ErrProjectRequired = errors.New("agent: project is required for coding tasks")

// acpCodingAgent implements CodingAgent using the ACP worker plumbing.
// It provides the Accept/Revert contract for atomic undo operations.
type acpCodingAgent struct {
	attacher WorkerAttacher
	provider *Provider
	agentID  string
	logger   *slog.Logger

	// mu protects session-to-worker mapping.
	mu sync.RWMutex
	// workers tracks active workers per session for cleanup.
	workers map[string]*acp.Worker
}

// ACPCodingAgentConfig holds configuration for creating an ACP-backed coding agent.
type ACPCodingAgentConfig struct {
	// Attacher is the worker attacher for spawning workers (required).
	Attacher WorkerAttacher
	// Provider is the resolved AI provider (required).
	Provider *Provider
	// Logger is the logger instance (optional, defaults to slog.Default()).
	Logger *slog.Logger
}

// NewACPCodingAgent creates a new CodingAgent backed by the ACP worker.
// It validates that the provider's binary exists before returning.
func NewACPCodingAgent(cfg ACPCodingAgentConfig) (CodingAgent, error) {
	if cfg.Attacher == nil {
		return nil, errors.New("agent: attacher is required")
	}
	if cfg.Provider == nil {
		return nil, errors.New("agent: provider is required")
	}

	// Check if the provider binary is available in PATH.
	if _, err := exec.LookPath(cfg.Provider.Command); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderBinaryMissing, cfg.Provider.Command)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &acpCodingAgent{
		attacher: cfg.Attacher,
		provider: cfg.Provider,
		agentID:  "sys-coding",
		logger:   logger,
		workers:  make(map[string]*acp.Worker),
	}, nil
}

// Accept implements CodingAgent.Accept.
// It finalizes the agent's changes for the given task.
func (a *acpCodingAgent) Accept(ctx context.Context, task *domain.Task) error {
	if task == nil {
		return errors.New("agent: task is required")
	}

	a.logger.Info("coding agent: accepting task",
		"task_id", task.ID,
		"session_id", task.SessionID,
	)

	// Accept is called after the user approves the changes.
	// The actual "accept" behavior (e.g., git commit) is handled by the ACP agent
	// through permission approval flow. This method is a semantic marker.
	return nil
}

// Revert implements CodingAgent.Revert.
// It triggers the agent's undo mechanism to roll back changes.
func (a *acpCodingAgent) Revert(ctx context.Context, task *domain.Task) error {
	if task == nil {
		return errors.New("agent: task is required")
	}

	a.logger.Info("coding agent: reverting task",
		"task_id", task.ID,
		"session_id", task.SessionID,
	)

	// Revert is called when the user denies the changes.
	// The actual "revert" behavior (e.g., git reset) is handled by the ACP agent
	// through the deny flow. This method is a semantic marker.
	return nil
}

// AgentID returns the unique identifier for this coding agent.
func (a *acpCodingAgent) AgentID() string {
	return a.agentID
}

// Provider returns the resolved provider for auditing and status reporting.
func (a *acpCodingAgent) Provider() *Provider {
	return a.provider
}

// nullCodingAgent is a no-op implementation for testing or when no agent is configured.
type nullCodingAgent struct{}

// NewNullCodingAgent creates a no-op CodingAgent for testing.
func NewNullCodingAgent() CodingAgent {
	return &nullCodingAgent{}
}

// Accept implements CodingAgent.Accept as a no-op.
func (n *nullCodingAgent) Accept(ctx context.Context, task *domain.Task) error {
	return nil
}

// Revert implements CodingAgent.Revert as a no-op.
func (n *nullCodingAgent) Revert(ctx context.Context, task *domain.Task) error {
	return nil
}
