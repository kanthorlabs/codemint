package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repl"
	"codemint.kanthorlabs.com/internal/workflow"
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
	registry         *registry.CommandRegistry
	ui               registry.UIMediator
	systemAssistant  func(ctx context.Context, input string) error
	workflowRegistry *workflow.WorkflowRegistry
}

// NewDispatcher constructs a Dispatcher backed by the given registry and UI
// mediator. systemAssistant may be nil during development; a placeholder error
// is returned when natural-language global input arrives without one.
// workflowRegistry may be nil; workflow routing will be skipped in that case.
func NewDispatcher(
	r *registry.CommandRegistry,
	ui registry.UIMediator,
	systemAssistant func(ctx context.Context, input string) error,
	workflowRegistry *workflow.WorkflowRegistry,
) *Dispatcher {
	return &Dispatcher{
		registry:         r,
		ui:               ui,
		systemAssistant:  systemAssistant,
		workflowRegistry: workflowRegistry,
	}
}

// WorkflowRegistry returns the workflow registry associated with this dispatcher.
// May return nil if no registry was configured.
func (d *Dispatcher) WorkflowRegistry() *workflow.WorkflowRegistry {
	return d.workflowRegistry
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

	// Project session: use workflow registry to route if available.
	if d.workflowRegistry != nil {
		if def, found := d.workflowRegistry.FindByTrigger(rawArgs); found {
			// Route to the matched workflow (EPIC-02 will implement handlers).
			return d.handleWorkflow(ctx, active, def, rawArgs)
		}

		// No trigger matched: default to ProjectCoding workflow.
		def, err := d.workflowRegistry.Lookup(domain.WorkflowTypeProjectCoding)
		if err == nil {
			return d.handleWorkflow(ctx, active, def, rawArgs)
		}
	}

	// Fallback: hand off to Brainstormer placeholder (EPIC-02).
	return fmt.Errorf("%w: input=%q", ErrNoBrainstormer, rawArgs)
}

// handleWorkflow routes input to the appropriate workflow handler.
// Currently a placeholder that logs the routing decision; EPIC-02 will implement
// the actual workflow execution logic.
func (d *Dispatcher) handleWorkflow(
	ctx context.Context,
	active *ActiveSession,
	def domain.WorkflowDefinition,
	input string,
) error {
	// For now, return an informative error indicating which workflow was selected.
	// EPIC-02 will replace this with actual workflow execution.
	return fmt.Errorf("%w: routed to %s workflow (type=%d), input=%q",
		ErrNoBrainstormer, def.Name, def.Type, input)
}
