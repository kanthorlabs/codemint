package repl

import (
	"context"
	"fmt"

	"codemint.kanthorlabs.com/internal/registry"
)

// RegisterWorkflowExitCommands registers slash commands that trigger workflow
// story exit conditions. These commands are handled by the ExitOnDispatcher
// in the orchestrator; the REPL handlers are no-ops that exist only to:
//   - Prevent "unknown command" errors when the user types them
//   - Enable tab-completion in the REPL
//   - Show up in /help output with appropriate descriptions
//
// The actual logic (close task, invoke handler, advance scheduler) lives
// in orchestrator.ExitOnDispatcher.Dispatch().
func RegisterWorkflowExitCommands(r *registry.CommandRegistry) error {
	commands := []registry.Command{
		{
			Name:           "lock-goal",
			Description:    "Lock the captured goal and advance the workflow.",
			Usage:          "/lock-goal",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        lockGoalHandler,
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register workflow exit command %q: %w", c.Name, err)
		}
	}
	return nil
}

// lockGoalHandler is a no-op handler for /lock-goal.
// The actual work is done by ExitOnDispatcher when it intercepts this command.
// If no goal-capture task is currently active, this command does nothing.
func lockGoalHandler(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
	// No-op: the ExitOnDispatcher handles this command when a goal-capture task is active.
	// If we reach here, it means no goal-capture task is currently processing,
	// so we return a helpful message instead of silently doing nothing.
	return registry.CommandResult{
		Message: "No active goal capture in progress. Start a workflow first with `/workflow brainstorming`.",
		Action:  registry.ActionNone,
	}, nil
}
