package repl

import (
	"context"
	"fmt"
	"os"

	"codemint.kanthorlabs.com/internal/registry"
	"golang.org/x/term"
)

// ModeCommandDeps holds the dependencies needed for mode-related commands.
type ModeCommandDeps struct {
	ActiveSession registry.MutableSessionInfo
}

// RegisterModeCommands registers mode management commands (/mode).
func RegisterModeCommands(r *registry.CommandRegistry, deps *ModeCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "mode",
			Description:    "Display or switch the current client mode (cli/daemon).",
			Usage:          "/mode [cli|daemon]",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        modeHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register mode command %q: %w", c.Name, err)
		}
	}
	return nil
}

// modeHandler handles the /mode command.
// Without args: displays current mode.
// With mode arg: switches to that mode.
func modeHandler(deps *ModeCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// No args: display current mode.
		if len(args) == 0 {
			return registry.CommandResult{
				Message: fmt.Sprintf("Current mode: %s", active.GetClientMode()),
				Action:  registry.ActionNone,
			}, nil
		}

		// Parse target mode.
		targetMode := args[0]
		switch targetMode {
		case "cli":
			return switchToCLIMode(deps)
		case "daemon":
			return switchToDaemonMode(deps)
		default:
			return registry.CommandResult{
				Message: fmt.Sprintf("Invalid mode %q. Use 'cli' or 'daemon'.", targetMode),
				Action:  registry.ActionNone,
			}, nil
		}
	}
}

// switchToCLIMode attempts to switch to CLI mode.
// Requires stdout to be a TTY.
func switchToCLIMode(deps *ModeCommandDeps) (registry.CommandResult, error) {
	// Check if stdout is a TTY.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return registry.CommandResult{
			Message: "Cannot switch to CLI mode: stdout is not a TTY.",
			Action:  registry.ActionNone,
		}, nil
	}

	// Already in CLI mode?
	if deps.ActiveSession.GetClientMode() == registry.ClientModeCLI {
		return registry.CommandResult{
			Message: "Already in CLI mode.",
			Action:  registry.ActionNone,
		}, nil
	}

	// Switch mode.
	deps.ActiveSession.SetClientMode(registry.ClientModeCLI)

	return registry.CommandResult{
		Message: "Switched to CLI mode.",
		Action:  registry.ActionNone,
	}, nil
}

// switchToDaemonMode switches to daemon mode.
// Always allowed (no TTY requirement).
func switchToDaemonMode(deps *ModeCommandDeps) (registry.CommandResult, error) {
	// Already in daemon mode?
	if deps.ActiveSession.GetClientMode() == registry.ClientModeDaemon {
		return registry.CommandResult{
			Message: "Already in daemon mode.",
			Action:  registry.ActionNone,
		}, nil
	}

	// Switch mode.
	deps.ActiveSession.SetClientMode(registry.ClientModeDaemon)

	return registry.CommandResult{
		Message: "Switched to daemon mode. CLI-only commands (e.g. /exit, /clear) are now unavailable.",
		Action:  registry.ActionNone,
	}, nil
}
