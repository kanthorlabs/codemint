package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codemint.kanthorlabs.com/internal/agent"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/input"
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

// ErrSystemAssistantDisabled is returned when freeform input arrives but
// the system assistant was not configured (e.g., --with-assistant=false).
var ErrSystemAssistantDisabled = errors.New("orchestrator: system assistant disabled (run with --with-assistant)")

// Dispatcher routes user input to slash-command handlers or the appropriate
// AI assistant path depending on the current ActiveSession.
type Dispatcher struct {
	registry            *registry.CommandRegistry
	ui                  registry.UIMediator
	systemAssistant     agent.SystemAssistant
	workflowRegistry    *workflow.WorkflowRegistry
	interactionRecorder *InteractionRecorder
}

// NewDispatcher constructs a Dispatcher backed by the given registry and UI
// mediator. systemAssistant may be nil; a friendly error is returned when
// natural-language global input arrives without one.
// workflowRegistry may be nil; workflow routing will be skipped in that case.
func NewDispatcher(
	r *registry.CommandRegistry,
	ui registry.UIMediator,
	systemAssistant agent.SystemAssistant,
	workflowRegistry *workflow.WorkflowRegistry,
) *Dispatcher {
	return &Dispatcher{
		registry:         r,
		ui:               ui,
		systemAssistant:  systemAssistant,
		workflowRegistry: workflowRegistry,
	}
}

// SetInteractionRecorder sets the interaction recorder for persisting user commands.
func (d *Dispatcher) SetInteractionRecorder(recorder *InteractionRecorder) {
	d.interactionRecorder = recorder
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
			// Record the blocked command attempt.
			d.recordInteraction(ctx, active, input, true, cmd, msg, nil)
			return nil // handled gracefully; not a failure
		}

		result, err := c.Handler(ctx, active, args, rawArgs)
		if err != nil {
			// Record the failed command.
			d.recordInteraction(ctx, active, input, true, cmd, "", err)
			return err
		}

		if result.Message != "" && d.ui != nil {
			d.ui.RenderMessage(result.Message)
		}

		// Record the successful command.
		d.recordInteraction(ctx, active, input, true, cmd, result.Message, nil)

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
		return d.dispatchToSystemAssistant(ctx, active, rawArgs)
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

// dispatchToSystemAssistant routes freeform text to the system assistant.
// It streams the response back to all registered adapters via the mediator.
func (d *Dispatcher) dispatchToSystemAssistant(ctx context.Context, active *ActiveSession, text string) error {
	if d.systemAssistant == nil {
		if d.ui != nil {
			d.ui.RenderMessage("System Assistant is not available. Run CodeMint with --with-assistant to enable it.")
		}
		return ErrSystemAssistantDisabled
	}

	// Build the assistant session from ActiveSession.
	sess := agent.AssistantSession{
		Session:  active.Session,
		Project:  active.Project,
		IsGlobal: active.IsGlobal,
	}

	// Call the assistant.
	chunks, err := d.systemAssistant.Ask(ctx, sess, text)
	if err != nil {
		errMsg := fmt.Sprintf("System Assistant error: %v", err)
		if d.ui != nil {
			d.ui.RenderMessage(errMsg)
		}
		d.recordChat(ctx, active, text, errMsg, err)
		return nil // Handled gracefully, not a failure to the caller
	}

	// Collect and broadcast the response.
	var response strings.Builder
	for chunk := range chunks {
		if chunk.Err != nil {
			errMsg := fmt.Sprintf("System Assistant error: %v", chunk.Err)
			if d.ui != nil {
				d.ui.RenderMessage(errMsg)
			}
			d.recordChat(ctx, active, text, response.String(), chunk.Err)
			return nil
		}

		if chunk.Text != "" {
			response.WriteString(chunk.Text)

			// Broadcast the chunk to all adapters.
			if d.ui != nil {
				d.ui.NotifyAll(registry.UIEvent{
					Type:    registry.EventChatChunk,
					Message: chunk.Text,
					Payload: registry.ChatChunkPayload{
						Source: "system-assistant",
						Text:   chunk.Text,
						Final:  false,
					},
				})
			}
		}

		if chunk.Done {
			// Send final marker.
			if d.ui != nil {
				d.ui.NotifyAll(registry.UIEvent{
					Type: registry.EventChatChunk,
					Payload: registry.ChatChunkPayload{
						Source: "system-assistant",
						Final:  true,
					},
				})
			}
			break
		}
	}

	// Record the conversation for /activity.
	d.recordChat(ctx, active, text, response.String(), nil)

	return nil
}

// DispatchInbound implements repl.MuxDispatcher. It routes an InboundMessage
// through the standard Dispatch path while recording the source metadata.
func (d *Dispatcher) DispatchInbound(ctx context.Context, active *ActiveSession, msg input.InboundMessage) error {
	// Stash source info in active session for recording.
	active.SetInputSource(msg.Source, msg.UserID)
	defer active.ClearInputSource()

	return d.Dispatch(ctx, active, msg.Text)
}

// recordInteraction records a user interaction as a Coordination task.
func (d *Dispatcher) recordInteraction(ctx context.Context, active *ActiveSession, input string, isSlash bool, cmdName string, response string, err error) {
	if d.interactionRecorder != nil {
		source, userID := active.GetInputSource()
		d.interactionRecorder.RecordWithSource(ctx, active, input, isSlash, cmdName, response, source, userID, err)
	}
}

// recordChat records a conversational exchange as a Coordination task.
func (d *Dispatcher) recordChat(ctx context.Context, active *ActiveSession, userText string, assistantText string, err error) {
	if d.interactionRecorder != nil {
		source, userID := active.GetInputSource()
		d.interactionRecorder.RecordChatWithSource(ctx, active, userText, assistantText, source, userID, err)
	}
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
