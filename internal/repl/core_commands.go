package repl

import (
	"context"
	"fmt"

	"codemint.kanthorlabs.com/internal/registry"
)

// RegisterCoreCommands registers the built-in UI-agnostic REPL utilities
// (/help, /exit, /clear) into r. SupportedModes declarations enforce which
// environments each command may run in; the Dispatcher checks them before
// invoking any handler.
//
// Domain-specific commands (e.g. /yolo, /status) are registered by their
// respective packages in main.go.
func RegisterCoreCommands(r *registry.CommandRegistry) error {
	commands := []registry.Command{
		{
			Name:           "help",
			Description:    "Display this help menu.",
			Usage:          "/help",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        helpHandler(r),
		},
		{
			Name:           "exit",
			Description:    "Safely close the active session and exit CodeMint.",
			Usage:          "/exit",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI}, // CLI only
			Handler:        exitHandler,
		},
		{
			Name:           "clear",
			Description:    "Clear the UI screen.",
			Usage:          "/clear",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI}, // CLI only
			Handler:        clearHandler,
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register core command %q: %w", c.Name, err)
		}
	}
	return nil
}

// helpHandler returns a mode-filtered help table inside the CommandResult.
// Only commands supported in the caller's ClientMode are shown.
func helpHandler(r *registry.CommandRegistry) registry.Handler {
	return func(_ context.Context, active registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
		return registry.CommandResult{
			Message: r.HelpTextForMode(active.GetClientMode()),
			Action:  registry.ActionNone,
		}, nil
	}
}

// exitHandler returns an ActionExit intent. The Orchestrator will catch this,
// flush pending state, close the session cleanly, and terminate the process.
func exitHandler(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
	return registry.CommandResult{
		Message: "Goodbye. Closing session...",
		Action:  registry.ActionExit,
	}, nil
}

// clearHandler returns an ActionClear intent. The UIMediator translates this
// to an ANSI escape sequence in a TUI or a viewport reset in a CUI.
func clearHandler(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
	return registry.CommandResult{
		Message: "",
		Action:  registry.ActionClear,
	}, nil
}
