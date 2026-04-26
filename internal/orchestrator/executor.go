// Package orchestrator contains startup, recovery, and execution logic for
// the CodeMint task execution engine.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/agent"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// CrashMessage is the exact UI text displayed to the user when an agent
// crashes or times out, as specified in User Story 1.9 acceptance criteria.
// The message is intentionally a format string so callers can embed the task
// ID into the discard button hint.
const CrashMessage = "⚠️ Agent crashed or timed out. Please manually reconcile the working directory and resolve the task status."

// crashMessageWithDiscard returns the full crash notification including the
// discard command hint for the given task ID.
func crashMessageWithDiscard(taskID string) string {
	return CrashMessage + "\n\nRun `/task discard " + taskID + "` to discard the agent's changes."
}

// ErrTaskTimeout is returned when a task exceeds its timeout duration.
var ErrTaskTimeout = errors.New("executor: task timed out")

// ErrUserAbort is returned when the user aborts a Confirmation/Retrospective task.
var ErrUserAbort = errors.New("executor: user aborted")

// ErrCommandNotWhitelisted is returned when a Verification task's command is not whitelisted.
var ErrCommandNotWhitelisted = errors.New("executor: command not whitelisted")

// ErrInvalidTaskInput is returned when task.Input JSON is invalid or missing required fields.
var ErrInvalidTaskInput = errors.New("executor: invalid task input")

// ErrContextFileMissing is returned when a context file does not exist.
var ErrContextFileMissing = errors.New("executor: context file missing")

// ErrPathEscape is returned when a context file path escapes the project directory.
var ErrPathEscape = errors.New("executor: path escape detected")

// ConfirmationOption constants for confirmation tasks.
const (
	ConfirmationOptionApprove = "approve"
	ConfirmationOptionRevise  = "revise"
	ConfirmationOptionAbort   = "abort"
)

// RetrospectiveOption constants for retrospective tasks.
const (
	RetrospectiveOptionShare = "share"
	RetrospectiveOptionSkip  = "skip"
)

// PromptKindFreeform is used for free-form text input prompts.
const PromptKindFreeform registry.PromptKind = "freeform"

// PromptKindConfirmation is used for task confirmation prompts.
const PromptKindConfirmation registry.PromptKind = "confirmation"

// PromptKindRetrospective is used for retrospective feedback prompts.
const PromptKindRetrospective registry.PromptKind = "retrospective"

// CodingInputSchema represents the JSON schema for Coding task input.
type CodingInputSchema struct {
	Prompt string `json:"prompt"`
}

// VerificationInputSchema represents the JSON schema for Verification task input.
type VerificationInputSchema struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

// VerificationOutputSchema represents the JSON schema for Verification task output.
type VerificationOutputSchema struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ConfirmationInputSchema represents the JSON schema for Confirmation task input.
type ConfirmationInputSchema struct {
	Prompt string `json:"prompt"`
}

// TaskFailureOutput represents a structured failure output for tasks.
// It provides a machine-readable error sentinel and human-readable detail.
type TaskFailureOutput struct {
	Error  string `json:"error"`  // Sentinel: invalid_input, context_file_missing, path_escape
	Detail string `json:"detail"` // Human-readable error message
}

// Failure sentinels for task output.
const (
	FailureSentinelInvalidInput      = "invalid_input"
	FailureSentinelContextFileMissing = "context_file_missing"
	FailureSentinelPathEscape        = "path_escape"
)

// resolveContextFiles resolves relative file paths under the project working
// directory and rejects path escapes or missing files.
//
// For each file path:
//   - filepath.Clean is applied to normalize the path
//   - Paths starting with ".." or absolute paths are rejected with ErrPathEscape
//   - The path is prefixed with project.WorkingDir
//   - os.Stat confirms the file exists; missing files return ErrContextFileMissing
//
// Returns the list of absolute paths or an error with details about which file
// failed validation.
func resolveContextFiles(project *domain.Project, files []string) ([]string, error) {
	if project == nil || project.WorkingDir == "" {
		return nil, fmt.Errorf("%w: project working directory not set", ErrInvalidTaskInput)
	}

	if len(files) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(files))

	for _, file := range files {
		// Normalize the path.
		cleaned := filepath.Clean(file)

		// Reject absolute paths.
		if filepath.IsAbs(cleaned) {
			return nil, fmt.Errorf("%w: absolute path not allowed: %s", ErrPathEscape, file)
		}

		// Reject paths that escape the project directory.
		if strings.HasPrefix(cleaned, "..") {
			return nil, fmt.Errorf("%w: path escapes project directory: %s", ErrPathEscape, file)
		}

		// Build the absolute path.
		absPath := filepath.Join(project.WorkingDir, cleaned)

		// Additional safety check: ensure the resolved path is within WorkingDir.
		// This handles edge cases like "foo/../../../etc/passwd".
		relPath, err := filepath.Rel(project.WorkingDir, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return nil, fmt.Errorf("%w: path escapes project directory: %s", ErrPathEscape, file)
		}

		// Verify the file exists.
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrContextFileMissing, file)
		}

		resolved = append(resolved, absPath)
	}

	return resolved, nil
}

// Executor wraps a CodingAgent with crash-fallback logic (Story 1.9) and
// task-type routing (Story 3.14).
//
// It branches on task.Type before forwarding to the appropriate handler:
//   - TaskTypeCoding (0): forwarded to ACP worker via session/prompt
//   - TaskTypeVerification (1): runs command via LocalRunner
//   - TaskTypeConfirmation (2): pauses for human approval
//   - TaskTypeCoordination (3): no-op (command echoes)
//   - TaskTypeRetrospective (4): pauses for optional user feedback
type Executor struct {
	codingAgent agent.CodingAgent
	taskRepo    repository.TaskRepository
	sessionRepo repository.SessionRepository
	agentRepo   repository.AgentRepository
	ui          registry.UIMediator
	runner      *LocalRunner

	// doneChannels tracks per-task completion signals for Coding tasks.
	// Used to block until StatusMapper signals Success/Failure.
	doneChannels   map[string]chan domain.TaskStatus
	doneChannelsMu sync.Mutex

	// advanceCh receives signals from StatusMapper when task completes.
	advanceCh <-chan struct{}
}

// ExecutorConfig holds the dependencies for creating an Executor.
type ExecutorConfig struct {
	CodingAgent agent.CodingAgent
	TaskRepo    repository.TaskRepository
	SessionRepo repository.SessionRepository
	AgentRepo   repository.AgentRepository
	UI          registry.UIMediator
	Runner      *LocalRunner
	AdvanceCh   <-chan struct{}
}

// NewExecutor constructs an Executor with the provided dependencies.
func NewExecutor(
	codingAgent agent.CodingAgent,
	taskRepo repository.TaskRepository,
	agentRepo repository.AgentRepository,
	ui registry.UIMediator,
) *Executor {
	return &Executor{
		codingAgent:  codingAgent,
		taskRepo:     taskRepo,
		agentRepo:    agentRepo,
		ui:           ui,
		runner:       NewLocalRunner(),
		doneChannels: make(map[string]chan domain.TaskStatus),
	}
}

// NewExecutorWithConfig constructs an Executor with the provided configuration.
func NewExecutorWithConfig(cfg ExecutorConfig) *Executor {
	runner := cfg.Runner
	if runner == nil {
		runner = NewLocalRunner()
	}
	return &Executor{
		codingAgent:  cfg.CodingAgent,
		taskRepo:     cfg.TaskRepo,
		sessionRepo:  cfg.SessionRepo,
		agentRepo:    cfg.AgentRepo,
		ui:           cfg.UI,
		runner:       runner,
		doneChannels: make(map[string]chan domain.TaskStatus),
		advanceCh:    cfg.AdvanceCh,
	}
}

// Execute dispatches a task to the appropriate handler based on task.Type.
// This is the main entry point for task execution as defined in Task 3.14.1.
func (e *Executor) Execute(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
	switch task.Type {
	case domain.TaskTypeCoding:
		return e.executeCoding(ctx, sess, task)
	case domain.TaskTypeVerification:
		return e.executeVerification(ctx, sess, task)
	case domain.TaskTypeConfirmation:
		return e.executeConfirmation(ctx, sess, task)
	case domain.TaskTypeCoordination:
		return e.skipCoordination(ctx, task)
	case domain.TaskTypeRetrospective:
		return e.executeRetrospective(ctx, sess, task)
	default:
		return fmt.Errorf("executor: unknown task type %d", task.Type)
	}
}

// ExecuteTask dispatches the task to the coding agent with a timeout derived
// from task.Timeout (milliseconds). This is the legacy entry point that
// preserves backward compatibility with Story 1.9 crash handling.
//
// If the agent returns an error (including context deadline exceeded), the
// crash-fallback flow is triggered:
//  1. task.assignee_id is reassigned to the human agent.
//  2. task status is forced to TaskStatusFailure then TaskStatusAwaiting.
//  3. The UI renders CrashMessage.
func (e *Executor) ExecuteTask(ctx context.Context, task *domain.Task) error {
	timeout := time.Duration(task.Timeout) * time.Millisecond
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := e.codingAgent.ExecuteTask(tCtx, task); err != nil {
		slog.Error("orchestrator: agent crash or timeout detected",
			"task_id", task.ID,
			"timeout_ms", task.Timeout,
			"error", err,
		)
		e.handleCrash(ctx, task)
		return fmt.Errorf("orchestrator: execute task %q: %w", task.ID, err)
	}
	return nil
}

// executeCoding forwards Coding tasks to the ACP worker via session/prompt.
// (Task 3.14.2)
func (e *Executor) executeCoding(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
	// 1. Mark task as processing.
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusProcessing); err != nil {
		slog.Error("executor: failed to update task status to processing",
			"task_id", task.ID,
			"error", err,
		)
		return fmt.Errorf("executor: update status processing: %w", err)
	}

	// 2. Get the ACP worker for this session.
	runtime := sess.ACPRuntime()
	if runtime == nil {
		return fmt.Errorf("executor: ACP runtime not available")
	}

	worker, ok := runtime.Registry().Get(sess.GetSessionID())
	if !ok || worker == nil {
		return fmt.Errorf("executor: ACP worker not found for session %s", sess.GetSessionID())
	}

	// 3. Set current task on worker for StatusMapper attribution.
	worker.SetCurrentTask(task.ID)
	defer worker.SetCurrentTask("")

	// 4. Parse task.Input using the TaskInput schema (EPIC-02 §2.13).
	var taskInput *domain.TaskInput
	var parseErr error

	if task.Input.Valid {
		taskInput, parseErr = domain.ParseTaskInput(task.Input.String)
		if parseErr != nil {
			if errors.Is(parseErr, domain.ErrEmptyInput) {
				e.failTaskWithReason(ctx, task, FailureSentinelInvalidInput, "empty input")
				return nil // Per-task failure, don't kill the scheduler
			}
			if errors.Is(parseErr, domain.ErrLegacyText) {
				// Legacy plain-text input - log warning but continue.
				slog.Warn("executor: legacy plain-text task.input",
					"task_id", task.ID,
				)
			}
		}
	} else {
		e.failTaskWithReason(ctx, task, FailureSentinelInvalidInput, "no input provided")
		return nil // Per-task failure, don't kill the scheduler
	}

	// Validate required fields.
	if taskInput == nil || taskInput.Prompt == "" {
		e.failTaskWithReason(ctx, task, FailureSentinelInvalidInput, "missing prompt")
		return nil // Per-task failure, don't kill the scheduler
	}

	// 5. Resolve context files relative to project working directory.
	var contextRefs []acp.PromptContextRef
	if len(taskInput.ContextFiles) > 0 {
		resolvedPaths, err := resolveContextFiles(sess.Project, taskInput.ContextFiles)
		if err != nil {
			// Map error to appropriate sentinel.
			var sentinel string
			switch {
			case errors.Is(err, ErrPathEscape):
				sentinel = FailureSentinelPathEscape
			case errors.Is(err, ErrContextFileMissing):
				sentinel = FailureSentinelContextFileMissing
			default:
				sentinel = FailureSentinelInvalidInput
			}
			e.failTaskWithReason(ctx, task, sentinel, err.Error())
			return nil // Per-task failure, don't kill the scheduler
		}

		contextRefs = make([]acp.PromptContextRef, len(resolvedPaths))
		for i, path := range resolvedPaths {
			contextRefs[i] = acp.PromptContextRef{
				Path: path,
				Kind: "file",
			}
		}
	}

	// 6. Build and send session/prompt request with context and tools.
	params := acp.SessionPromptParams{
		SessionID: sess.GetACPSessionID(),
		Prompt:    taskInput.Prompt,
		Context:   contextRefs,
		Tools:     taskInput.Tools,
	}

	req, err := acp.NewRequest(acp.MethodSessionPrompt, params)
	if err != nil {
		return fmt.Errorf("executor: create prompt request: %w", err)
	}

	if err := worker.Send(req); err != nil {
		return fmt.Errorf("executor: send prompt: %w", err)
	}

	// 7. Block until StatusMapper signals completion or timeout.
	timeout := time.Duration(task.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = time.Duration(domain.DefaultTaskTimeout) * time.Millisecond
	}

	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Register done channel for this task.
	doneCh := make(chan domain.TaskStatus, 1)
	e.doneChannelsMu.Lock()
	e.doneChannels[task.ID] = doneCh
	e.doneChannelsMu.Unlock()
	defer func() {
		e.doneChannelsMu.Lock()
		delete(e.doneChannels, task.ID)
		e.doneChannelsMu.Unlock()
	}()

	// Wait for completion signal.
	select {
	case <-tCtx.Done():
		// Timeout - send cancel request to agent.
		cancelParams := acp.SessionCancelParams{
			SessionID: sess.GetACPSessionID(),
		}
		cancelReq, _ := acp.NewRequest(acp.MethodSessionCancel, cancelParams)
		if cancelReq != nil {
			_ = worker.Send(cancelReq)
		}

		// Mark task as failure.
		_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure)
		return ErrTaskTimeout

	case status := <-doneCh:
		if status == domain.TaskStatusFailure {
			return fmt.Errorf("executor: task failed")
		}
		return nil

	case <-e.advanceCh:
		// StatusMapper signaled task completion.
		// Check task status to determine success/failure.
		updatedTask, err := e.taskRepo.FindByID(ctx, task.ID)
		if err != nil {
			return nil // Assume success if we can't verify.
		}
		if updatedTask.Status == domain.TaskStatusFailure {
			return fmt.Errorf("executor: task failed")
		}
		return nil
	}
}

// SignalTaskDone notifies the executor that a task has completed.
// Called by StatusMapper when it detects a terminal status.
func (e *Executor) SignalTaskDone(taskID string, status domain.TaskStatus) {
	e.doneChannelsMu.Lock()
	if ch, ok := e.doneChannels[taskID]; ok {
		select {
		case ch <- status:
		default:
		}
	}
	e.doneChannelsMu.Unlock()
}

// executeVerification runs the task's command via LocalRunner.
// (Task 3.14.3)
func (e *Executor) executeVerification(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
	// 1. Parse task.Input JSON.
	var input VerificationInputSchema
	if task.Input.Valid {
		if err := json.Unmarshal([]byte(task.Input.String), &input); err != nil {
			return fmt.Errorf("%w: invalid JSON: %v", ErrInvalidTaskInput, err)
		}
	}
	if input.Command == "" {
		return fmt.Errorf("%w: missing command", ErrInvalidTaskInput)
	}

	// Use project working directory if not specified.
	cwd := input.Cwd
	if cwd == "" && sess != nil && sess.Project != nil {
		cwd = sess.Project.WorkingDir
	}

	// 2. Check if command is whitelisted.
	// Load permissions and evaluate.
	if sess != nil && sess.ACPRuntime() != nil {
		interceptor, ok := sess.ACPRuntime().GetInterceptor(sess.GetSessionID())
		if ok && interceptor != nil {
			decision := interceptor.EvaluateCommand(ctx, input.Command, cwd)
			if decision == DecisionBlock {
				output := VerificationOutputSchema{
					ExitCode: -1,
					Stderr:   "command not whitelisted",
				}
				e.setTaskOutput(ctx, task, output)
				_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure)
				return ErrCommandNotWhitelisted
			}
			// For DecisionUnknown, we still reject for verification tasks.
			if decision == DecisionUnknown {
				output := VerificationOutputSchema{
					ExitCode: -1,
					Stderr:   "command not whitelisted (verification commands must be pre-approved)",
				}
				e.setTaskOutput(ctx, task, output)
				_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure)
				return ErrCommandNotWhitelisted
			}
		}
	}

	// 3. Mark task as processing.
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusProcessing); err != nil {
		slog.Error("executor: failed to update task status to processing",
			"task_id", task.ID,
			"error", err,
		)
	}

	// 4. Execute command via LocalRunner.
	timeout := time.Duration(task.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = DefaultRunTimeout
	}

	result, err := e.runner.Run(ctx, input.Command, cwd, timeout)
	if err != nil {
		// Command couldn't be executed (not found, etc.)
		output := VerificationOutputSchema{
			ExitCode: -1,
			Stderr:   err.Error(),
		}
		e.setTaskOutput(ctx, task, output)
		_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure)
		return fmt.Errorf("executor: run verification command: %w", err)
	}

	// 5. Persist result in task.Output.
	output := VerificationOutputSchema{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}
	e.setTaskOutput(ctx, task, output)

	// 6. Map exit code to status.
	if result.ExitCode == 0 {
		_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusSuccess)
		return nil
	}

	_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure)
	return fmt.Errorf("executor: verification failed with exit code %d", result.ExitCode)
}

// executeConfirmation pauses for human approval/rejection.
// (Task 3.14.4)
func (e *Executor) executeConfirmation(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
	// 1. Set task and session status to Awaiting.
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusAwaiting); err != nil {
		slog.Error("executor: failed to update task status to awaiting",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Update session status to Awaiting if sessionRepo is available.
	if e.sessionRepo != nil && sess != nil && sess.Session != nil {
		// Note: SessionRepository may not have UpdateStatus; skip for now.
		// This would require adding UpdateStatus to the SessionRepository interface.
	}

	// 2. Parse task input for prompt body.
	body := "Please review the changes."
	var input ConfirmationInputSchema
	if task.Input.Valid {
		if err := json.Unmarshal([]byte(task.Input.String), &input); err == nil && input.Prompt != "" {
			body = input.Prompt
		}
	}

	// Build title from parent story context.
	title := fmt.Sprintf("Confirmation Required (Story %d.%d)", task.SeqEpic, task.SeqStory)

	// 3. Build PromptRequest.
	req := registry.PromptRequest{
		Kind:   PromptKindConfirmation,
		TaskID: task.ID,
		Title:  title,
		Body:   body,
		PromptOptions: []registry.PromptOption{
			{ID: ConfirmationOptionApprove, Label: "Approve & Continue", Description: "Accept the changes and proceed to the next task"},
			{ID: ConfirmationOptionRevise, Label: "Revise", Description: "Request changes before proceeding"},
			{ID: ConfirmationOptionAbort, Label: "Abort Session", Description: "Cancel the session and discard pending tasks"},
		},
	}

	// 4. Prompt user via mediator.
	resp := e.ui.PromptDecision(ctx, req)
	if resp.Error != nil {
		return fmt.Errorf("executor: prompt decision: %w", resp.Error)
	}

	// 5. Handle response.
	switch resp.SelectedOptionID {
	case ConfirmationOptionApprove:
		_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusSuccess)
		return nil

	case ConfirmationOptionRevise:
		// Task stays awaiting. Emit EventReviseRequested for EPIC-02 §2.8.
		e.ui.NotifyAll(registry.UIEvent{
			Type:    registry.EventReviseRequested,
			TaskID:  task.ID,
			Message: "Revision requested for task",
		})
		// Return without error - task remains awaiting for revision handling.
		return nil

	case ConfirmationOptionAbort:
		e.setTaskOutputJSON(ctx, task, map[string]string{"reason": "user_abort"})
		_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure)
		return ErrUserAbort

	default:
		// Unknown option, treat as abort.
		return ErrUserAbort
	}
}

// executeRetrospective prompts for optional user feedback.
// (Task 3.14.5)
func (e *Executor) executeRetrospective(ctx context.Context, sess *ActiveSession, task *domain.Task) error {
	// 1. Set task status to Awaiting.
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusAwaiting); err != nil {
		slog.Error("executor: failed to update task status to awaiting",
			"task_id", task.ID,
			"error", err,
		)
	}

	// 2. Build PromptRequest with EPIC-02 §2.6 template.
	title := fmt.Sprintf("Epic %d Retrospective", task.SeqEpic)
	body := fmt.Sprintf("Epic %d just wrapped. Did I do anything annoying? Any feedback helps me improve!", task.SeqEpic)

	req := registry.PromptRequest{
		Kind:   PromptKindRetrospective,
		TaskID: task.ID,
		Title:  title,
		Body:   body,
		PromptOptions: []registry.PromptOption{
			{ID: RetrospectiveOptionShare, Label: "Share Feedback", Description: "Provide feedback to improve future performance"},
			{ID: RetrospectiveOptionSkip, Label: "Skip", Description: "Skip feedback and continue"},
		},
	}

	// 3. Prompt user.
	resp := e.ui.PromptDecision(ctx, req)
	if resp.Error != nil {
		// On error, still mark as success (retrospective never blocks indefinitely).
		_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusSuccess)
		return nil
	}

	// 4. Handle response.
	switch resp.SelectedOptionID {
	case RetrospectiveOptionShare:
		// Open a follow-up freeform prompt for feedback.
		freeformReq := registry.PromptRequest{
			Kind:    PromptKindFreeform,
			TaskID:  task.ID,
			Title:   "Share Your Feedback",
			Body:    "What feedback would you like to share?",
			Message: "Enter your feedback below:",
		}

		freeformResp := e.ui.PromptDecision(ctx, freeformReq)
		if freeformResp.Error == nil && freeformResp.SelectedOption != "" {
			// Persist feedback in task.Output.
			e.setTaskOutputJSON(ctx, task, map[string]string{"feedback": freeformResp.SelectedOption})
		}

	case RetrospectiveOptionSkip:
		// Skip - no output needed.
	}

	// Always mark as success.
	_ = e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusSuccess)
	return nil
}

// skipCoordination is a no-op for Coordination tasks.
// (Task 3.14.6)
func (e *Executor) skipCoordination(_ context.Context, task *domain.Task) error {
	slog.Debug("executor: skipping coordination task", "id", task.ID)
	// Coordination tasks are already completed - they're just user command echoes.
	// No action needed.
	return nil
}

// setTaskOutput marshals the output and persists it to the task.
func (e *Executor) setTaskOutput(ctx context.Context, task *domain.Task, output any) {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		slog.Error("executor: failed to marshal task output",
			"task_id", task.ID,
			"error", err,
		)
		return
	}
	e.setTaskOutputRaw(ctx, task, string(outputJSON))
}

// setTaskOutputJSON marshals a map and persists it to the task.
func (e *Executor) setTaskOutputJSON(ctx context.Context, task *domain.Task, output map[string]string) {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		slog.Error("executor: failed to marshal task output",
			"task_id", task.ID,
			"error", err,
		)
		return
	}
	e.setTaskOutputRaw(ctx, task, string(outputJSON))
}

// setTaskOutputRaw persists raw JSON string to the task output.
func (e *Executor) setTaskOutputRaw(ctx context.Context, task *domain.Task, output string) {
	// Note: This requires adding UpdateOutput to TaskRepository.
	// For now, we'll update the task object and rely on status updates.
	task.Output = domain.NewNullString(output)
	// TODO: Implement taskRepo.UpdateOutput(ctx, task.ID, output)
	slog.Debug("executor: task output set", "task_id", task.ID, "output_len", len(output))
}

// failTaskWithReason marks a task as failed with a structured error output.
// This is used for input validation failures that should not crash the scheduler.
// The task output is set to {"error": "<sentinel>", "detail": "<message>"}.
func (e *Executor) failTaskWithReason(ctx context.Context, task *domain.Task, sentinel, detail string) {
	output := TaskFailureOutput{
		Error:  sentinel,
		Detail: detail,
	}
	e.setTaskOutput(ctx, task, output)
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure); err != nil {
		slog.Error("executor: failed to update task status to failure",
			"task_id", task.ID,
			"sentinel", sentinel,
			"error", err,
		)
	}
	slog.Info("executor: task failed with reason",
		"task_id", task.ID,
		"sentinel", sentinel,
		"detail", detail,
	)
}

// handleCrash performs the state reassignment and UI notification after a
// crash. It uses the background ctx (not the timed-out one) for DB writes.
func (e *Executor) handleCrash(ctx context.Context, task *domain.Task) {
	// 1. Reassign to human agent.
	human, err := e.agentRepo.FindByName(ctx, "human")
	if err != nil {
		slog.Error("orchestrator: crash fallback: find human agent",
			"task_id", task.ID,
			"error", err,
		)
	} else if human != nil {
		if err := e.taskRepo.UpdateAssignee(ctx, task.ID, human.ID); err != nil {
			slog.Error("orchestrator: crash fallback: reassign task to human",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	// 2. Transition to TaskStatusFailure (from Processing).
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusFailure); err != nil {
		slog.Error("orchestrator: crash fallback: set status Failure",
			"task_id", task.ID,
			"error", err,
		)
		// Still attempt to surface the UI notification even if DB write failed.
	}

	// 3. Transition to TaskStatusAwaiting for human review.
	// Failure → Awaiting is not in the standard forward table; we go via
	// the allowed Failure→Processing path then Processing→Awaiting.
	// To keep the implementation simple and avoid a spurious Processing blip
	// we attempt a direct status update. If the state machine rejects it,
	// the task remains Failure — still visible to the human — and we log.
	if err := e.taskRepo.UpdateTaskStatus(ctx, task.ID, domain.TaskStatusAwaiting); err != nil {
		slog.Error("orchestrator: crash fallback: set status Awaiting",
			"task_id", task.ID,
			"error", err,
		)
	}

	// 4. Notify the UI.
	e.ui.RenderMessage(crashMessageWithDiscard(task.ID))
}
