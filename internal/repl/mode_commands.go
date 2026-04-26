package repl

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"codemint.kanthorlabs.com/internal/registry"
	"golang.org/x/term"
)

// ModeCommandDeps holds the dependencies needed for mode-related commands.
type ModeCommandDeps struct {
	ActiveSession registry.MutableSessionInfo
}

// VerbosityCommandDeps holds the dependencies needed for verbosity commands.
type VerbosityCommandDeps struct {
	ActiveSession registry.VerbositySessionInfo
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

// RegisterVerbosityCommands registers the /verbosity command.
func RegisterVerbosityCommands(r *registry.CommandRegistry, deps *VerbosityCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "verbosity",
			Description:    "Display or set the output verbosity level (0=task, 1=story, 2=epic).",
			Usage:          "/verbosity [0|1|2|task|story|epic]",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI},
			Handler:        verbosityHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register verbosity command %q: %w", c.Name, err)
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

// verbosityHandler handles the /verbosity command.
// Without args: displays current verbosity level.
// With level arg: sets the verbosity level.
func verbosityHandler(deps *VerbosityCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// No args: display current verbosity.
		if len(args) == 0 {
			level := deps.ActiveSession.GetVerbosity()
			return registry.CommandResult{
				Message: fmt.Sprintf("Current verbosity: %d (%s)\n\nLevels:\n  0 (task)  - Show everything (thinking, tool updates, messages)\n  1 (story) - Hide thinking and tool updates\n  2 (epic)  - Show only turn end and task status changes", level, verbosityName(level)),
				Action:  registry.ActionNone,
			}, nil
		}

		// Parse target level.
		level, err := parseVerbosityLevel(args[0])
		if err != nil {
			return registry.CommandResult{
				Message: err.Error(),
				Action:  registry.ActionNone,
			}, nil
		}

		deps.ActiveSession.SetVerbosity(level)
		return registry.CommandResult{
			Message: fmt.Sprintf("Verbosity set to %d (%s).", level, verbosityName(level)),
			Action:  registry.ActionNone,
		}, nil
	}
}

// parseVerbosityLevel parses a string into a verbosity level.
func parseVerbosityLevel(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "0", "task":
		return 0, nil
	case "1", "story":
		return 1, nil
	case "2", "epic":
		return 2, nil
	default:
		// Try parsing as integer.
		level, err := strconv.Atoi(s)
		if err != nil || level < 0 || level > 2 {
			return 0, fmt.Errorf("invalid verbosity level %q: use 0/task, 1/story, or 2/epic", s)
		}
		return level, nil
	}
}

// verbosityName returns the human-readable name for a verbosity level.
func verbosityName(level int) string {
	switch level {
	case 0:
		return "task"
	case 1:
		return "story"
	case 2:
		return "epic"
	default:
		return "unknown"
	}
}
