// Package orchestrator contains the core orchestration logic for CodeMint.
package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// PreStartupHookConfig holds the dependencies for the pre-startup hook.
type PreStartupHookConfig struct {
	// Config is the loaded application configuration (required).
	Config *config.Config
	// AgentRepo is the agent repository for seeding sys-default (required).
	AgentRepo repository.AgentRepository
	// KnownProviders is a map of provider names that are available (required).
	KnownProviders map[string]bool
	// Logger is the logger for hook operations (optional).
	Logger *slog.Logger
}

// PreStartupHook validates configuration and seeds required data before the
// application fully starts. It performs the following checks:
//
//  1. Validates that sys-default assistant is configured with a valid provider
//  2. Ensures the sys-default agent is seeded in the database
//
// Returns an error if any validation fails, which should cause startup to abort.
func PreStartupHook(ctx context.Context, cfg PreStartupHookConfig) error {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Step 1: Validate sys-default assistant configuration.
	if err := config.ValidateSysDefault(cfg.Config, cfg.KnownProviders); err != nil {
		return fmt.Errorf("pre-startup: %w", err)
	}

	logger.Info("pre-startup: sys-default assistant configured",
		"provider", cfg.Config.GetSysDefault().Provider,
		"model", cfg.Config.GetSysDefault().Model,
	)

	// Step 2: Ensure sys-default agent exists in the database.
	// The EnsureSystemAgents call in main.go seeds all system agents including sys-default.
	// Here we verify it exists after seeding.
	agent, err := cfg.AgentRepo.FindByName(ctx, config.SysDefaultAssistant)
	if err != nil {
		return fmt.Errorf("pre-startup: find %s agent: %w", config.SysDefaultAssistant, err)
	}
	if agent == nil {
		return fmt.Errorf("pre-startup: %s agent not found in database — ensure EnsureSystemAgents was called", config.SysDefaultAssistant)
	}

	logger.Info("pre-startup: sys-default agent verified",
		"agent_id", agent.ID,
		"agent_type", agent.Type,
	)

	return nil
}

// PreExitHookConfig holds the dependencies for the pre-exit hook.
type PreExitHookConfig struct {
	// Logger is the logger for hook operations (optional).
	Logger *slog.Logger
}

// PreExitHook is called before the application exits. It provides a place for
// cleanup logic such as:
//   - Flushing pending writes
//   - Closing external connections gracefully
//   - Recording session statistics
//
// Currently a placeholder for future use.
func PreExitHook(ctx context.Context, cfg PreExitHookConfig) error {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("pre-exit: hook invoked")

	// Placeholder for future cleanup logic.
	// Examples:
	// - Flush any buffered logs or metrics
	// - Save session state to disk
	// - Notify external services of shutdown

	return nil
}

// EnsureSysDefaultAgent ensures the sys-default agent exists in the database
// with the correct type. This is called as part of EnsureSystemAgents but can
// also be called independently for verification.
func EnsureSysDefaultAgent(ctx context.Context, agentRepo repository.AgentRepository) (*domain.Agent, error) {
	agent, err := agentRepo.FindByName(ctx, config.SysDefaultAssistant)
	if err != nil {
		return nil, fmt.Errorf("find %s agent: %w", config.SysDefaultAssistant, err)
	}

	if agent == nil {
		return nil, fmt.Errorf("%s agent not seeded — call EnsureSystemAgents first", config.SysDefaultAssistant)
	}

	if agent.Type != domain.AgentTypeAssistant {
		return nil, fmt.Errorf("%s agent has wrong type %d, expected %d (Assistant)",
			config.SysDefaultAssistant, agent.Type, domain.AgentTypeAssistant)
	}

	return agent, nil
}
