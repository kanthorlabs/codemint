package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repl"
)

// ErrNoBrainstormer is returned when natural-language input arrives in a
// project session but no Brainstormer handler has been wired up yet (EPIC-02).
var ErrNoBrainstormer = errors.New("orchestrator: brainstormer not available (EPIC-02)")

// ErrShutdownGracefully is an alias for registry.ErrShutdownGracefully for
// backward compatibility. Prefer using registry.ErrShutdownGracefully directly.
var ErrShutdownGracefully = registry.ErrShutdownGracefully

// Dispatcher routes user input to slash-command handlers or the appropriate
// AI assistant path depending on the current ActiveSession.
type Dispatcher struct {
	registry        *registry.CommandRegistry
	ui              registry.UIMediator
	systemAssistant func(ctx context.Context, input string) error
}

// NewDispatcher constructs a Dispatcher backed by the given registry and UI
// mediator. systemAssistant may be nil during development; a placeholder error
// is returned when natural-language global input arrives without one.
func NewDispatcher(
	r *registry.CommandRegistry,
	ui registry.UIMediator,
	systemAssistant func(ctx context.Context, input string) error,
) *Dispatcher {
	return &Dispatcher{registry: r, ui: ui, systemAssistant: systemAssistant}
}

// Dispatch parses input and routes it:
//
//   - /command → looks up in the registry; checks SupportedModes against
//     active.ClientMode; calls Handler(ctx, active, args, rawArgs); renders
//     Message via UIMediator; acts on SystemAction.
//   - natural language + IsGlobal  → delegates to systemAssistant.
//   - natural language + !IsGlobal → placeholder for EPIC-02 Brainstormer.
func (d *Dispatcher) Dispatch(ctx context.Context, active *ActiveSession, input string) error {
	isSlash, cmd, args, rawArgs, err := repl.ParseInput(input)
	if err != nil {
		return fmt.Errorf("orchestrator: parse input: %w", err)
	}

	if isSlash {
		c, err := d.registry.Lookup(cmd)
		if err != nil {
			return fmt.Errorf("orchestrator: unknown command %q (type /help for a list): %w",
				cmd, err)
		}

		// Declarative capability enforcement.
		if !c.SupportsMode(active.ClientMode) {
			msg := fmt.Sprintf("The `/%s` command is not available in %s mode.", c.Name, active.ClientMode)
			if d.ui != nil {
				d.ui.RenderMessage(msg)
			}
			return nil // handled gracefully; not a failure
		}

		result, err := c.Handler(ctx, active, args, rawArgs)
		if err != nil {
			return err
		}

		if result.Message != "" && d.ui != nil {
			d.ui.RenderMessage(result.Message)
		}

		switch result.Action {
		case registry.ActionExit:
			return ErrShutdownGracefully
		case registry.ActionClear:
			if d.ui != nil {
				d.ui.ClearScreen()
			}
		}
		return nil
	}

	// Natural-language path.
	if active.IsGlobal {
		if d.systemAssistant == nil {
			return fmt.Errorf("orchestrator: system assistant not configured")
		}
		return d.systemAssistant(ctx, rawArgs)
	}

	// Project session: hand off to Brainstormer (EPIC-02).
	return fmt.Errorf("%w: input=%q", ErrNoBrainstormer, rawArgs)
}
