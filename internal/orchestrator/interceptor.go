package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// Interceptor consumes halted events from the Pipeline and evaluates them
// against the project's permission whitelist. In this initial implementation
// (Story 3.4), it simply forwards all tool calls to the UI with a halt message.
// Story 3.5 adds whitelist matching; Story 3.6 adds the formal awaiting-approval flow.
type Interceptor struct {
	permRepo repository.ProjectPermissionRepository
	taskRepo repository.TaskRepository
	ui       registry.UIMediator
	worker   *acp.Worker
	logger   *slog.Logger
}

// InterceptorConfig holds the dependencies for creating an Interceptor.
type InterceptorConfig struct {
	PermRepo repository.ProjectPermissionRepository
	TaskRepo repository.TaskRepository
	UI       registry.UIMediator
	Worker   *acp.Worker
	Logger   *slog.Logger
}

// NewInterceptor creates a new Interceptor with the provided dependencies.
func NewInterceptor(cfg InterceptorConfig) *Interceptor {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Interceptor{
		permRepo: cfg.PermRepo,
		taskRepo: cfg.TaskRepo,
		ui:       cfg.UI,
		worker:   cfg.Worker,
		logger:   logger,
	}
}

// Handle processes a halted event from the Pipeline. In this story (3.4),
// all tool calls are forwarded to the UI as "halted" — no whitelist matching
// is performed yet. That logic is added in Story 3.5.
//
// The currentTaskID parameter identifies the task that triggered this tool call,
// which is used for context in UI messages and future approval flows.
func (i *Interceptor) Handle(ctx context.Context, ev acp.Event, currentTaskID string) {
	switch ev.Kind {
	case acp.EventToolCall:
		i.handleToolCall(ctx, ev, currentTaskID)
	case acp.EventPermissionRequest:
		i.handlePermissionRequest(ctx, ev, currentTaskID)
	default:
		// Should not reach here as Pipeline only sends tool_call and permission_request to Halted
		i.logger.Warn("interceptor received unexpected event kind",
			"kind", ev.Kind.String(),
			"session", ev.ACPSessionID,
		)
	}
}

// handleToolCall processes a tool_call event. In this skeleton implementation,
// it simply notifies the UI that a tool call was halted.
func (i *Interceptor) handleToolCall(ctx context.Context, ev acp.Event, currentTaskID string) {
	msg := formatHaltMessage(ev)

	i.logger.Info("tool call halted",
		"tool", ev.ToolName,
		"command", ev.Command,
		"session", ev.ACPSessionID,
		"task", currentTaskID,
	)

	if i.ui != nil {
		i.ui.RenderMessage(msg)
	}
}

// handlePermissionRequest processes a session/request_permission event.
// In this skeleton implementation, it notifies the UI and does not respond
// to the ACP agent (approval flow is in Story 3.6).
func (i *Interceptor) handlePermissionRequest(ctx context.Context, ev acp.Event, currentTaskID string) {
	msg := formatHaltMessage(ev)

	i.logger.Info("permission request halted",
		"tool", ev.ToolName,
		"command", ev.Command,
		"request_id", ev.RequestID,
		"session", ev.ACPSessionID,
		"task", currentTaskID,
	)

	if i.ui != nil {
		i.ui.RenderMessage(msg)
	}
}

// formatHaltMessage creates a user-friendly message describing the halted tool call.
func formatHaltMessage(ev acp.Event) string {
	if ev.Command != "" {
		return fmt.Sprintf("[ACP] Tool call halted: %s `%s`", ev.ToolName, ev.Command)
	}
	return fmt.Sprintf("[ACP] Tool call halted: %s", ev.ToolName)
}

// Run starts the interceptor, consuming events from the halted channel and
// processing them. It blocks until the channel is closed or the context is
// cancelled.
func (i *Interceptor) Run(ctx context.Context, halted <-chan acp.Event, currentTaskID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-halted:
			if !ok {
				return
			}
			i.Handle(ctx, ev, currentTaskID)
		}
	}
}
