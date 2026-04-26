package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// Interceptor consumes halted events from the Pipeline and evaluates them
// against the project's permission whitelist. Commands that match allowed
// rules are executed locally and the result is injected back into the agent.
// Commands that are blocked or unknown are forwarded to the UI for human review.
type Interceptor struct {
	permRepo  repository.ProjectPermissionRepository
	taskRepo  repository.TaskRepository
	agentRepo repository.AgentRepository
	ui        registry.UIMediator
	worker    *acp.Worker
	logger    *slog.Logger
	runner    *LocalRunner

	// Project context for permission matching
	projectID  string
	workingDir string
}

// InterceptorConfig holds the dependencies for creating an Interceptor.
type InterceptorConfig struct {
	PermRepo   repository.ProjectPermissionRepository
	TaskRepo   repository.TaskRepository
	AgentRepo  repository.AgentRepository
	UI         registry.UIMediator
	Worker     *acp.Worker
	Logger     *slog.Logger
	ProjectID  string
	WorkingDir string
}

// NewInterceptor creates a new Interceptor with the provided dependencies.
func NewInterceptor(cfg InterceptorConfig) *Interceptor {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Interceptor{
		permRepo:   cfg.PermRepo,
		taskRepo:   cfg.TaskRepo,
		agentRepo:  cfg.AgentRepo,
		ui:         cfg.UI,
		worker:     cfg.Worker,
		logger:     logger,
		runner:     NewLocalRunner(),
		projectID:  cfg.ProjectID,
		workingDir: cfg.WorkingDir,
	}
}

// HandleContext contains additional context for processing an intercepted event.
type HandleContext struct {
	SessionID string
	TaskID    string
}

// Handle processes a halted event from the Pipeline. It evaluates the command
// against the project's permission whitelist and either:
// - Executes allowed commands locally and injects the result back
// - Blocks forbidden commands and notifies the UI
// - Forwards unknown commands to the UI for human review
func (i *Interceptor) Handle(ctx context.Context, ev acp.Event, hctx HandleContext) {
	switch ev.Kind {
	case acp.EventToolCall:
		i.handleToolCall(ctx, ev, hctx)
	case acp.EventPermissionRequest:
		i.handlePermissionRequest(ctx, ev, hctx)
	default:
		// Should not reach here as Pipeline only sends tool_call and permission_request to Halted
		i.logger.Warn("interceptor received unexpected event kind",
			"kind", ev.Kind.String(),
			"session", ev.ACPSessionID,
		)
	}
}

// handleToolCall processes a tool_call event by evaluating it against permissions.
func (i *Interceptor) handleToolCall(ctx context.Context, ev acp.Event, hctx HandleContext) {
	// Only evaluate shell commands; other tool calls go to UI
	if ev.Command == "" {
		i.forwardToUI(ev, hctx.TaskID, "unknown tool call without command")
		return
	}

	decision := i.evaluateCommand(ctx, ev.Command, ev.Cwd)

	switch decision {
	case DecisionAllow:
		i.executeAndRespond(ctx, ev, hctx)
	case DecisionBlock:
		i.logger.Warn("command blocked by permission rules",
			"command", ev.Command,
			"cwd", ev.Cwd,
			"task", hctx.TaskID,
		)
		i.forwardToUI(ev, hctx.TaskID, "blocked by permission rules")
	default:
		i.forwardToUI(ev, hctx.TaskID, "requires human approval")
	}
}

// handlePermissionRequest processes a session/request_permission event.
// Permission requests follow the same evaluation logic as tool calls.
func (i *Interceptor) handlePermissionRequest(ctx context.Context, ev acp.Event, hctx HandleContext) {
	// Only evaluate shell commands
	if ev.Command == "" {
		i.forwardToUI(ev, hctx.TaskID, "unknown permission request")
		return
	}

	decision := i.evaluateCommand(ctx, ev.Command, ev.Cwd)

	switch decision {
	case DecisionAllow:
		i.executeAndRespondPermission(ctx, ev, hctx)
	case DecisionBlock:
		i.logger.Warn("permission request blocked",
			"command", ev.Command,
			"request_id", ev.RequestID,
			"task", hctx.TaskID,
		)
		i.respondPermissionDenied(ctx, ev, "blocked by permission rules")
	default:
		i.forwardToUI(ev, hctx.TaskID, "requires human approval")
	}
}

// evaluateCommand loads project permissions and evaluates the command.
func (i *Interceptor) evaluateCommand(ctx context.Context, command, cwd string) Decision {
	if i.projectID == "" {
		return DecisionUnknown
	}

	// Use working directory as fallback if cwd is not provided
	if cwd == "" {
		cwd = i.workingDir
	}

	perm, err := i.permRepo.FindByProjectID(ctx, i.projectID)
	if err != nil {
		i.logger.Debug("failed to load project permissions",
			"project_id", i.projectID,
			"error", err,
		)
		return DecisionUnknown
	}

	matcher := NewMatcher(perm)
	return matcher.Evaluate(command, cwd)
}

// executeAndRespond runs an allowed command locally and injects the result.
// This is used for tool_call events where we don't need to send a formal response.
func (i *Interceptor) executeAndRespond(ctx context.Context, ev acp.Event, hctx HandleContext) {
	cwd := ev.Cwd
	if cwd == "" {
		cwd = i.workingDir
	}

	result, err := i.runner.Run(ctx, ev.Command, cwd, 0)

	i.logger.Info("auto-executed allowed command",
		"command", ev.Command,
		"cwd", cwd,
		"exit_code", result.ExitCode,
		"duration", result.Duration,
		"killed", result.Killed,
		"task", hctx.TaskID,
	)

	// Record the auto-approval in audit trail
	i.recordAutoApproval(ctx, ev, hctx, result, err)

	// Notify UI about the auto-approval
	i.notifyAutoApproval(ev, result)

	// Note: For tool_call events (not permission_request), the ACP agent
	// typically proceeds automatically. The auto-approval is recorded for
	// audit purposes. If the agent expects a response, Story 3.6 will
	// handle the formal approval flow.
}

// executeAndRespondPermission runs an allowed command and responds to the permission request.
func (i *Interceptor) executeAndRespondPermission(ctx context.Context, ev acp.Event, hctx HandleContext) {
	cwd := ev.Cwd
	if cwd == "" {
		cwd = i.workingDir
	}

	result, err := i.runner.Run(ctx, ev.Command, cwd, 0)

	i.logger.Info("auto-executed allowed permission request",
		"command", ev.Command,
		"cwd", cwd,
		"exit_code", result.ExitCode,
		"duration", result.Duration,
		"request_id", ev.RequestID,
		"task", hctx.TaskID,
	)

	// Record the auto-approval in audit trail
	i.recordAutoApproval(ctx, ev, hctx, result, err)

	// Notify UI about the auto-approval
	i.notifyAutoApproval(ev, result)

	// Send permission granted response to the agent
	i.respondPermissionGranted(ctx, ev, result)
}

// respondPermissionGranted sends a positive permission response to the ACP agent.
func (i *Interceptor) respondPermissionGranted(ctx context.Context, ev acp.Event, result RunResult) {
	if i.worker == nil {
		return
	}

	resp := acp.PermissionResponse{
		RequestID: ev.RequestID,
		Granted:   true,
		Reason:    fmt.Sprintf("auto-approved (exit=%d, duration=%s)", result.ExitCode, result.Duration),
	}

	msg, err := acp.NewResponse(nil, resp)
	if err != nil {
		i.logger.Error("failed to create permission response", "error", err)
		return
	}
	// Set the ID from the original request if we can extract it
	if len(ev.Raw) > 0 {
		var rawMsg acp.Message
		if json.Unmarshal(ev.Raw, &rawMsg) == nil && len(rawMsg.ID) > 0 {
			msg.ID = rawMsg.ID
		}
	}

	if err := i.worker.Send(msg); err != nil {
		i.logger.Error("failed to send permission response", "error", err)
	}
}

// respondPermissionDenied sends a negative permission response to the ACP agent.
func (i *Interceptor) respondPermissionDenied(ctx context.Context, ev acp.Event, reason string) {
	if i.worker == nil {
		return
	}

	resp := acp.PermissionResponse{
		RequestID: ev.RequestID,
		Granted:   false,
		Reason:    reason,
	}

	msg, err := acp.NewResponse(nil, resp)
	if err != nil {
		i.logger.Error("failed to create permission denial response", "error", err)
		return
	}
	// Set the ID from the original request if we can extract it
	if len(ev.Raw) > 0 {
		var rawMsg acp.Message
		if json.Unmarshal(ev.Raw, &rawMsg) == nil && len(rawMsg.ID) > 0 {
			msg.ID = rawMsg.ID
		}
	}

	if err := i.worker.Send(msg); err != nil {
		i.logger.Error("failed to send permission denial", "error", err)
	}
}

// forwardToUI sends the event to the UI for human review.
func (i *Interceptor) forwardToUI(ev acp.Event, taskID, reason string) {
	msg := formatHaltMessage(ev)

	i.logger.Info("tool call halted",
		"tool", ev.ToolName,
		"command", ev.Command,
		"reason", reason,
		"session", ev.ACPSessionID,
		"task", taskID,
	)

	if i.ui != nil {
		i.ui.RenderMessage(msg)
	}
}

// notifyAutoApproval sends a notification to the UI about an auto-approved command.
func (i *Interceptor) notifyAutoApproval(ev acp.Event, result RunResult) {
	if i.ui == nil {
		return
	}

	payload := AutoApprovalPayload{
		Command:  ev.Command,
		Cwd:      ev.Cwd,
		ExitCode: result.ExitCode,
		Duration: result.Duration,
		Killed:   result.Killed,
	}

	msg := fmt.Sprintf("[ACP] Auto-approved: `%s` (exit=%d, %s)", ev.Command, result.ExitCode, result.Duration)

	i.ui.NotifyAll(registry.UIEvent{
		Type:    registry.EventACPAutoApproved,
		Message: msg,
		Payload: payload,
	})
}

// AutoApprovalPayload contains details about an auto-approved command execution.
type AutoApprovalPayload struct {
	Command  string        `json:"command"`
	Cwd      string        `json:"cwd"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Killed   bool          `json:"killed"`
}

// AutoApprovalInputPayload is the JSON structure for audit task input.
type AutoApprovalInputPayload struct {
	Intercepted bool   `json:"intercepted"`
	Command     string `json:"command"`
	Cwd         string `json:"cwd"`
}

// AutoApprovalOutputPayload is the JSON structure for audit task output.
type AutoApprovalOutputPayload struct {
	Exit       int    `json:"exit"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
	Killed     bool   `json:"killed,omitempty"`
	Error      string `json:"error,omitempty"`
}

// recordAutoApproval persists the auto-approved execution as a Coordination task.
func (i *Interceptor) recordAutoApproval(ctx context.Context, ev acp.Event, hctx HandleContext, result RunResult, runErr error) {
	if i.taskRepo == nil || i.agentRepo == nil {
		return
	}

	// Get the sys-auto-approve agent
	agent, err := i.agentRepo.FindByName(ctx, "sys-auto-approve")
	if err != nil || agent == nil {
		// Fall back to human agent if sys-auto-approve doesn't exist
		agent, err = i.agentRepo.FindByName(ctx, "human")
		if err != nil || agent == nil {
			i.logger.Warn("failed to find agent for audit trail", "error", err)
			return
		}
	}

	// Build input payload
	input := AutoApprovalInputPayload{
		Intercepted: true,
		Command:     ev.Command,
		Cwd:         ev.Cwd,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		i.logger.Warn("failed to marshal audit input", "error", err)
		return
	}

	// Build output payload
	output := AutoApprovalOutputPayload{
		Exit:       result.ExitCode,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		DurationMs: result.Duration.Milliseconds(),
		Killed:     result.Killed,
	}
	if runErr != nil {
		output.Error = runErr.Error()
	}
	outputJSON, err := json.Marshal(output)
	if err != nil {
		i.logger.Warn("failed to marshal audit output", "error", err)
		return
	}

	// Get session ID from handle context or extract from project
	sessionID := hctx.SessionID
	if sessionID == "" {
		i.logger.Debug("no session ID for audit trail")
		return
	}

	// Create the audit task
	task := &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  i.projectID,
		SessionID:  sessionID,
		WorkflowID: sql.NullString{}, // No workflow for coordination tasks
		AssigneeID: agent.ID,
		SeqEpic:    0,
		SeqStory:   0,
		SeqTask:    0,
		Type:       domain.TaskTypeCoordination,
		Status:     domain.TaskStatusCompleted, // Immediately completed
		Timeout:    domain.DefaultTaskTimeout,
		Input:      sql.NullString{String: string(inputJSON), Valid: true},
		Output:     sql.NullString{String: string(outputJSON), Valid: true},
	}

	if err := i.taskRepo.Create(ctx, task); err != nil {
		i.logger.Warn("failed to record audit trail", "error", err)
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
func (i *Interceptor) Run(ctx context.Context, halted <-chan acp.Event, hctx HandleContext) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-halted:
			if !ok {
				return
			}
			i.Handle(ctx, ev, hctx)
		}
	}
}
