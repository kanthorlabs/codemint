package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// ApprovalOption IDs for permission prompts.
const (
	ApprovalOptionAllowOnce    = "allow_once"
	ApprovalOptionAllowSession = "allow_session"
	ApprovalOptionDeny         = "deny"
)

// DefaultApprovalTimeout is the default timeout for human approval (30 minutes).
const DefaultApprovalTimeout = 30 * time.Minute

// PendingApproval holds information about a permission request awaiting human decision.
type PendingApproval struct {
	// Event is the original ACP event that triggered the approval request.
	Event acp.Event
	// HandleContext contains session and task context.
	HandleContext HandleContext
	// Decision is the matcher's decision (Block or Unknown).
	Decision Decision
	// CreatedAt is when the approval request was created.
	CreatedAt time.Time
	// CancelFunc cancels the approval timeout goroutine.
	CancelFunc context.CancelFunc
}

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

	// pending stores approval requests awaiting human decision, keyed by task ID.
	// For ad-hoc prompts without a task, keyed by ACP session ID.
	pending   map[string]*PendingApproval
	pendingMu sync.RWMutex

	// sessionWhitelist tracks commands approved with "allow_session" for the
	// current process lifetime. Not persisted to project_permission.
	sessionWhitelist   map[string]struct{}
	sessionWhitelistMu sync.RWMutex

	// approvalTimeout is the maximum time to wait for human approval.
	approvalTimeout time.Duration
}

// InterceptorConfig holds the dependencies for creating an Interceptor.
type InterceptorConfig struct {
	PermRepo        repository.ProjectPermissionRepository
	TaskRepo        repository.TaskRepository
	AgentRepo       repository.AgentRepository
	UI              registry.UIMediator
	Worker          *acp.Worker
	Logger          *slog.Logger
	ProjectID       string
	WorkingDir      string
	ApprovalTimeout time.Duration // Defaults to DefaultApprovalTimeout if zero.
}

// NewInterceptor creates a new Interceptor with the provided dependencies.
func NewInterceptor(cfg InterceptorConfig) *Interceptor {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	timeout := cfg.ApprovalTimeout
	if timeout == 0 {
		timeout = DefaultApprovalTimeout
	}

	return &Interceptor{
		permRepo:         cfg.PermRepo,
		taskRepo:         cfg.TaskRepo,
		agentRepo:        cfg.AgentRepo,
		ui:               cfg.UI,
		worker:           cfg.Worker,
		logger:           logger,
		runner:           NewLocalRunner(),
		projectID:        cfg.ProjectID,
		workingDir:       cfg.WorkingDir,
		pending:          make(map[string]*PendingApproval),
		sessionWhitelist: make(map[string]struct{}),
		approvalTimeout:  timeout,
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

	// Check session whitelist first (commands approved with allow_session)
	if i.isSessionWhitelisted(ev.Command) {
		i.executeAndRespond(ctx, ev, hctx)
		return
	}

	decision := i.evaluateCommand(ctx, ev.Command, ev.Cwd)

	switch decision {
	case DecisionAllow:
		i.executeAndRespond(ctx, ev, hctx)
	case DecisionBlock, DecisionUnknown:
		// Both Block and Unknown require human approval per Story 3.6
		i.escalateForApproval(ctx, ev, hctx, decision)
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

	// Check session whitelist first (commands approved with allow_session)
	if i.isSessionWhitelisted(ev.Command) {
		i.executeAndRespondPermission(ctx, ev, hctx)
		return
	}

	decision := i.evaluateCommand(ctx, ev.Command, ev.Cwd)

	switch decision {
	case DecisionAllow:
		i.executeAndRespondPermission(ctx, ev, hctx)
	case DecisionBlock, DecisionUnknown:
		// Both Block and Unknown require human approval per Story 3.6
		i.escalateForApproval(ctx, ev, hctx, decision)
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

// escalateForApproval halts the worker and prompts the user for approval.
// This implements Task 3.6.1: Halt + Mark Task Awaiting.
func (i *Interceptor) escalateForApproval(ctx context.Context, ev acp.Event, hctx HandleContext, decision Decision) {
	// Use task ID as key, fall back to ACP session ID for ad-hoc prompts
	key := hctx.TaskID
	if key == "" {
		key = ev.ACPSessionID
	}

	i.logger.Info("escalating command for human approval",
		"command", ev.Command,
		"cwd", ev.Cwd,
		"decision", decision.String(),
		"task", hctx.TaskID,
		"key", key,
	)

	// Update task status to Awaiting if we have a task
	if hctx.TaskID != "" && i.taskRepo != nil {
		if err := i.taskRepo.UpdateTaskStatus(ctx, hctx.TaskID, domain.TaskStatusAwaiting); err != nil {
			i.logger.Warn("failed to update task status to awaiting",
				"task", hctx.TaskID,
				"error", err,
			)
		}
	}

	// Create timeout context for the approval
	timeoutCtx, cancel := context.WithTimeout(ctx, i.approvalTimeout)

	// Cache the pending approval
	pending := &PendingApproval{
		Event:         ev,
		HandleContext: hctx,
		Decision:      decision,
		CreatedAt:     time.Now(),
		CancelFunc:    cancel,
	}

	i.pendingMu.Lock()
	i.pending[key] = pending
	i.pendingMu.Unlock()

	// Notify UI that approval is awaiting
	i.notifyAwaitingApproval(ev, hctx, decision)

	// Prompt the user for a decision (Task 3.6.2)
	go i.promptForApproval(timeoutCtx, key, pending)
}

// notifyAwaitingApproval sends a notification to the UI about a pending approval.
func (i *Interceptor) notifyAwaitingApproval(ev acp.Event, hctx HandleContext, decision Decision) {
	if i.ui == nil {
		return
	}

	reason := "requires human approval"
	if decision == DecisionBlock {
		reason = "blocked by permission rules"
	}

	payload := AwaitingApprovalPayload{
		Command:  ev.Command,
		Cwd:      ev.Cwd,
		Reason:   reason,
		TaskID:   hctx.TaskID,
		Decision: decision.String(),
	}

	msg := fmt.Sprintf("[ACP] Command awaiting approval: `%s` (%s)", ev.Command, reason)

	i.ui.NotifyAll(registry.UIEvent{
		Type:    registry.EventACPAwaitingApproval,
		TaskID:  hctx.TaskID,
		Message: msg,
		Payload: payload,
	})
}

// AwaitingApprovalPayload contains details about a command awaiting approval.
type AwaitingApprovalPayload struct {
	Command  string `json:"command"`
	Cwd      string `json:"cwd"`
	Reason   string `json:"reason"`
	TaskID   string `json:"task_id,omitempty"`
	Decision string `json:"decision"`
}

// promptForApproval shows the approval prompt and handles the user's response.
// This implements Task 3.6.2: Approval Prompt via UIMediator.
func (i *Interceptor) promptForApproval(ctx context.Context, key string, pending *PendingApproval) {
	defer pending.CancelFunc()

	// If no UI, auto-deny
	if i.ui == nil {
		i.logger.Warn("no UI available for approval prompt, auto-denying",
			"command", pending.Event.Command,
		)
		i.handleApprovalResponse(ctx, key, pending, registry.PromptResponse{
			Error: fmt.Errorf("no UI available"),
		})
		return
	}

	req := registry.PromptRequest{
		Kind:   registry.PromptKindACPCommandApproval,
		TaskID: pending.HandleContext.TaskID,
		Title:  "Allow command?",
		Body:   fmt.Sprintf("%s\nin %s", pending.Event.Command, pending.Event.Cwd),
		PromptOptions: []registry.PromptOption{
			{ID: ApprovalOptionAllowOnce, Label: "Allow once", Description: "Execute this command once"},
			{ID: ApprovalOptionAllowSession, Label: "Allow for this session", Description: "Allow this command for the rest of this session"},
			{ID: ApprovalOptionDeny, Label: "Deny", Description: "Reject this command"},
		},
	}

	resp := i.ui.PromptDecision(ctx, req)

	// Handle the response (Task 3.6.3)
	i.handleApprovalResponse(ctx, key, pending, resp)
}

// handleApprovalResponse processes the user's approval decision.
// This implements Task 3.6.3: Reply to Worker Based on User Choice.
func (i *Interceptor) handleApprovalResponse(ctx context.Context, key string, pending *PendingApproval, resp registry.PromptResponse) {
	// Remove from pending map
	i.pendingMu.Lock()
	delete(i.pending, key)
	i.pendingMu.Unlock()

	// Handle errors (timeout, cancellation)
	if resp.Error != nil {
		i.logger.Warn("approval prompt failed or timed out",
			"command", pending.Event.Command,
			"task", pending.HandleContext.TaskID,
			"error", resp.Error,
		)
		// Task 3.6.4: On timeout, auto-deny
		i.denyApproval(ctx, pending, "approval timed out")
		if i.ui != nil {
			i.ui.RenderMessage("[ACP] Approval timed out — denied automatically.")
		}
		return
	}

	// Determine which option was selected
	optionID := resp.SelectedOptionID
	if optionID == "" {
		// Fall back to legacy SelectedOption matching
		switch resp.SelectedOption {
		case "Allow once":
			optionID = ApprovalOptionAllowOnce
		case "Allow for this session":
			optionID = ApprovalOptionAllowSession
		case "Deny":
			optionID = ApprovalOptionDeny
		}
	}

	switch optionID {
	case ApprovalOptionAllowOnce:
		i.approveOnce(ctx, pending)
	case ApprovalOptionAllowSession:
		i.approveSession(ctx, pending)
	case ApprovalOptionDeny:
		i.denyApproval(ctx, pending, "denied by user")
	default:
		i.logger.Warn("unknown approval option selected",
			"option", optionID,
			"legacy", resp.SelectedOption,
		)
		i.denyApproval(ctx, pending, "unknown response")
	}
}

// approveOnce executes the command once and resumes the agent.
func (i *Interceptor) approveOnce(ctx context.Context, pending *PendingApproval) {
	ev := pending.Event
	hctx := pending.HandleContext

	i.logger.Info("command approved (once)",
		"command", ev.Command,
		"task", hctx.TaskID,
	)

	// Restore task to Processing
	i.restoreTaskToProcessing(ctx, hctx.TaskID)

	// Execute and respond based on event type
	if ev.Kind == acp.EventPermissionRequest {
		i.executeAndRespondPermission(ctx, ev, hctx)
	} else {
		i.executeAndRespond(ctx, ev, hctx)
	}

	// Notify UI of resolution
	i.notifyApprovalResolved(ev, hctx, "allowed_once")
}

// approveSession adds command to session whitelist and executes it.
func (i *Interceptor) approveSession(ctx context.Context, pending *PendingApproval) {
	ev := pending.Event
	hctx := pending.HandleContext

	i.logger.Info("command approved (session)",
		"command", ev.Command,
		"task", hctx.TaskID,
	)

	// Add to session whitelist
	i.addToSessionWhitelist(ev.Command)

	// Restore task to Processing
	i.restoreTaskToProcessing(ctx, hctx.TaskID)

	// Execute and respond based on event type
	if ev.Kind == acp.EventPermissionRequest {
		i.executeAndRespondPermission(ctx, ev, hctx)
	} else {
		i.executeAndRespond(ctx, ev, hctx)
	}

	// Notify UI of resolution
	i.notifyApprovalResolved(ev, hctx, "allowed_session")
}

// denyApproval rejects the command and notifies the agent.
func (i *Interceptor) denyApproval(ctx context.Context, pending *PendingApproval, reason string) {
	ev := pending.Event
	hctx := pending.HandleContext

	i.logger.Info("command denied",
		"command", ev.Command,
		"reason", reason,
		"task", hctx.TaskID,
	)

	// Restore task to Processing (agent will decide next steps)
	// Per spec: "Transition the task back to processing once the worker is unblocked"
	i.restoreTaskToProcessing(ctx, hctx.TaskID)

	// Send denial response to agent
	if ev.Kind == acp.EventPermissionRequest {
		i.respondPermissionDenied(ctx, ev, reason)
	}
	// For tool_call events, the agent doesn't expect a formal response,
	// but we forward to UI so the user can see what was denied.
	i.forwardToUI(ev, hctx.TaskID, reason)

	// Notify UI of resolution
	i.notifyApprovalResolved(ev, hctx, "denied")
}

// restoreTaskToProcessing transitions a task back to Processing status.
func (i *Interceptor) restoreTaskToProcessing(ctx context.Context, taskID string) {
	if taskID == "" || i.taskRepo == nil {
		return
	}

	if err := i.taskRepo.UpdateTaskStatus(ctx, taskID, domain.TaskStatusProcessing); err != nil {
		i.logger.Warn("failed to restore task to processing",
			"task", taskID,
			"error", err,
		)
	}
}

// notifyApprovalResolved sends a notification about a resolved approval.
func (i *Interceptor) notifyApprovalResolved(ev acp.Event, hctx HandleContext, resolution string) {
	if i.ui == nil {
		return
	}

	payload := ApprovalResolvedPayload{
		Command:    ev.Command,
		Cwd:        ev.Cwd,
		TaskID:     hctx.TaskID,
		Resolution: resolution,
	}

	msg := fmt.Sprintf("[ACP] Command %s: `%s`", resolution, ev.Command)

	i.ui.NotifyAll(registry.UIEvent{
		Type:    registry.EventACPApprovalResolved,
		TaskID:  hctx.TaskID,
		Message: msg,
		Payload: payload,
	})
}

// ApprovalResolvedPayload contains details about a resolved approval.
type ApprovalResolvedPayload struct {
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	TaskID     string `json:"task_id,omitempty"`
	Resolution string `json:"resolution"` // allowed_once, allowed_session, denied
}

// isSessionWhitelisted checks if a command has been approved for this session.
func (i *Interceptor) isSessionWhitelisted(command string) bool {
	i.sessionWhitelistMu.RLock()
	defer i.sessionWhitelistMu.RUnlock()
	_, ok := i.sessionWhitelist[command]
	return ok
}

// addToSessionWhitelist adds a command to the session whitelist.
func (i *Interceptor) addToSessionWhitelist(command string) {
	i.sessionWhitelistMu.Lock()
	defer i.sessionWhitelistMu.Unlock()
	i.sessionWhitelist[command] = struct{}{}
	i.logger.Debug("added command to session whitelist", "command", command)
}

// CancelPendingApprovals cancels all pending approval prompts.
// This should be called when archiving/shutting down a session (Task 3.6.4).
func (i *Interceptor) CancelPendingApprovals() {
	i.pendingMu.Lock()
	defer i.pendingMu.Unlock()

	for key, pending := range i.pending {
		i.logger.Info("cancelling pending approval",
			"key", key,
			"command", pending.Event.Command,
		)
		pending.CancelFunc()
		delete(i.pending, key)
	}
}

// PendingApprovalCount returns the number of pending approval requests.
// Useful for testing and monitoring.
func (i *Interceptor) PendingApprovalCount() int {
	i.pendingMu.RLock()
	defer i.pendingMu.RUnlock()
	return len(i.pending)
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
