package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/workflow"
)

// ExitOnDispatcher routes slash commands that match a story's `exit_on.command`
// to close the active task with Success and invoke the output handler.
//
// This is the shared wiring for story exit conditions as specified in 2.2.4:
//   - When a workflow task is Processing and its story has ExitOn.Command set,
//     incoming REPL slash commands matching that command close the task.
//   - If Story.Output.Handler is set, the handler is invoked.
//   - On handler error, task transitions to Failure instead of Success.
type ExitOnDispatcher struct {
	mu sync.RWMutex

	// registrations maps command name (e.g., "/lock-goal") to active task registrations.
	// Multiple tasks could theoretically register for the same command (though typically only one).
	registrations map[string][]*exitOnRegistration

	// handlerRegistry is the workflow output handler registry.
	handlerRegistry *workflow.HandlerRegistry

	// taskRepo is used to update task status and output.
	taskRepo repository.TaskRepository

	// workflowRepo is used to invoke handlers that need workflow access.
	workflowRepo repository.WorkflowRepository

	// advanceCh signals the scheduler when a task is closed via exit_on.
	advanceCh chan<- struct{}

	logger *slog.Logger
}

// exitOnRegistration represents a single task registered for exit_on command.
type exitOnRegistration struct {
	TaskID        string
	WorkflowID    string
	Command       string   // The exit_on.command (e.g., "/lock-goal")
	HandlerName   string   // Story.Output.Handler name (e.g., "lock_workflow_goal")
	SessionID     string   // Session this task belongs to
}

// ExitOnDispatcherConfig holds configuration for creating an ExitOnDispatcher.
type ExitOnDispatcherConfig struct {
	HandlerRegistry *workflow.HandlerRegistry
	TaskRepo        repository.TaskRepository
	WorkflowRepo    repository.WorkflowRepository
	AdvanceCh       chan<- struct{}
	Logger          *slog.Logger
}

// NewExitOnDispatcher creates a new ExitOnDispatcher.
func NewExitOnDispatcher(cfg ExitOnDispatcherConfig) *ExitOnDispatcher {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &ExitOnDispatcher{
		registrations:   make(map[string][]*exitOnRegistration),
		handlerRegistry: cfg.HandlerRegistry,
		taskRepo:        cfg.TaskRepo,
		workflowRepo:    cfg.WorkflowRepo,
		advanceCh:       cfg.AdvanceCh,
		logger:          logger,
	}
}

// Register adds an exit_on registration for a task.
// Call this when a task transitions to Processing and has ExitOn.Command set.
func (d *ExitOnDispatcher) Register(taskID, workflowID, command, handlerName, sessionID string) {
	if command == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	reg := &exitOnRegistration{
		TaskID:      taskID,
		WorkflowID:  workflowID,
		Command:     command,
		HandlerName: handlerName,
		SessionID:   sessionID,
	}

	d.registrations[command] = append(d.registrations[command], reg)
	d.logger.Debug("exit_on: registered",
		"task_id", taskID,
		"command", command,
		"handler", handlerName,
	)
}

// Deregister removes all registrations for a task.
// Call this when a task transitions out of Processing.
func (d *ExitOnDispatcher) Deregister(taskID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for cmd, regs := range d.registrations {
		filtered := make([]*exitOnRegistration, 0, len(regs))
		for _, r := range regs {
			if r.TaskID != taskID {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(d.registrations, cmd)
		} else {
			d.registrations[cmd] = filtered
		}
	}

	d.logger.Debug("exit_on: deregistered", "task_id", taskID)
}

// DispatchResult is returned by Dispatch to indicate what happened.
type DispatchResult struct {
	// Handled is true if the command was handled (matched a registration).
	Handled bool
	// TaskID is the task that was closed (if any).
	TaskID string
	// Error is set if the handler invocation failed.
	Error error
}

// Dispatch checks if the slash command matches any active registration.
// If matched:
//  1. Fetches the task's output from the database
//  2. Invokes the handler (if configured)
//  3. On handler success: marks task Success
//  4. On handler error: marks task Failure with error in output
//  5. Deregisters the task
//  6. Signals the scheduler to advance
//
// Returns DispatchResult indicating whether the command was handled.
func (d *ExitOnDispatcher) Dispatch(ctx context.Context, command string, sessionID string) DispatchResult {
	d.mu.RLock()
	regs := d.registrations[command]
	if len(regs) == 0 {
		d.mu.RUnlock()
		return DispatchResult{Handled: false}
	}

	// Find registration for this session.
	var reg *exitOnRegistration
	for _, r := range regs {
		if r.SessionID == sessionID {
			reg = r
			break
		}
	}
	d.mu.RUnlock()

	if reg == nil {
		return DispatchResult{Handled: false}
	}

	d.logger.Info("exit_on: dispatching",
		"command", command,
		"task_id", reg.TaskID,
		"handler", reg.HandlerName,
	)

	// Get the task to retrieve its output.
	task, err := d.taskRepo.FindByID(ctx, reg.TaskID)
	if err != nil || task == nil {
		d.logger.Error("exit_on: task not found",
			"task_id", reg.TaskID,
			"error", err,
		)
		return DispatchResult{Handled: false}
	}

	// Check task is still Processing.
	if task.Status != domain.TaskStatusProcessing {
		d.logger.Warn("exit_on: task not processing, ignoring",
			"task_id", reg.TaskID,
			"status", task.Status,
		)
		return DispatchResult{Handled: false}
	}

	var handlerErr error

	// Invoke handler if configured.
	if reg.HandlerName != "" && d.handlerRegistry != nil {
		// Get the task output (the agent's last response).
		output := ""
		if task.Output.Valid {
			output = task.Output.String
		}

		args := workflow.HandlerArgs{
			WorkflowID: reg.WorkflowID,
			Task:       task,
			Output:     output,
			ExitCmd:    command,
		}

		handlerErr = d.handlerRegistry.Invoke(ctx, reg.HandlerName, args)
		if handlerErr != nil {
			d.logger.Error("exit_on: handler failed",
				"task_id", reg.TaskID,
				"handler", reg.HandlerName,
				"error", handlerErr,
			)
		}
	}

	// Update task status based on handler result.
	if handlerErr != nil {
		// Handler failed: mark task as Failure with error details.
		if err := d.taskRepo.UpdateTaskStatus(ctx, reg.TaskID, domain.TaskStatusFailure); err != nil {
			d.logger.Error("exit_on: failed to mark task failure",
				"task_id", reg.TaskID,
				"error", err,
			)
		}
	} else {
		// Success: mark task as Success.
		if err := d.taskRepo.UpdateTaskStatus(ctx, reg.TaskID, domain.TaskStatusSuccess); err != nil {
			d.logger.Error("exit_on: failed to mark task success",
				"task_id", reg.TaskID,
				"error", err,
			)
		}
	}

	// Deregister the task.
	d.Deregister(reg.TaskID)

	// Signal the scheduler to advance.
	if d.advanceCh != nil {
		select {
		case d.advanceCh <- struct{}{}:
		default:
			// Channel full, coalesce signal.
		}
	}

	return DispatchResult{
		Handled: true,
		TaskID:  reg.TaskID,
		Error:   handlerErr,
	}
}

// HasRegistration returns true if there's an active registration for the command in the session.
func (d *ExitOnDispatcher) HasRegistration(command string, sessionID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	regs := d.registrations[command]
	for _, r := range regs {
		if r.SessionID == sessionID {
			return true
		}
	}
	return false
}

// ActiveRegistrations returns all active registrations (for debugging/testing).
func (d *ExitOnDispatcher) ActiveRegistrations() map[string][]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string][]string)
	for cmd, regs := range d.registrations {
		taskIDs := make([]string, len(regs))
		for i, r := range regs {
			taskIDs[i] = r.TaskID
		}
		result[cmd] = taskIDs
	}
	return result
}

// ExitOnRegistrationInfo holds metadata about a registration (for testing).
type ExitOnRegistrationInfo struct {
	TaskID      string
	WorkflowID  string
	Command     string
	HandlerName string
	SessionID   string
}

// GetRegistration returns registration info for a task (for testing).
func (d *ExitOnDispatcher) GetRegistration(taskID string) (*ExitOnRegistrationInfo, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, regs := range d.registrations {
		for _, r := range regs {
			if r.TaskID == taskID {
				return &ExitOnRegistrationInfo{
					TaskID:      r.TaskID,
					WorkflowID:  r.WorkflowID,
					Command:     r.Command,
					HandlerName: r.HandlerName,
					SessionID:   r.SessionID,
				}, true
			}
		}
	}
	return nil, false
}

// RegisterForSession is a convenience method to register a task using its metadata.
// It extracts the exit_on command from the story definition stored in task input.
func (d *ExitOnDispatcher) RegisterFromStory(taskID, workflowID, sessionID string, story *domain.StoryDefinition) {
	if story == nil || story.ExitOn == nil || story.ExitOn.Command == "" {
		return
	}

	handlerName := ""
	if story.Output != nil {
		handlerName = story.Output.Handler
	}

	d.Register(taskID, workflowID, story.ExitOn.Command, handlerName, sessionID)
}

// DeregisterSession removes all registrations for a session.
// Call this when a session is archived or detached.
func (d *ExitOnDispatcher) DeregisterSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for cmd, regs := range d.registrations {
		filtered := make([]*exitOnRegistration, 0, len(regs))
		for _, r := range regs {
			if r.SessionID != sessionID {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(d.registrations, cmd)
		} else {
			d.registrations[cmd] = filtered
		}
	}

	d.logger.Debug("exit_on: deregistered session", "session_id", sessionID)
}

// Count returns the total number of active registrations (for testing).
func (d *ExitOnDispatcher) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	count := 0
	for _, regs := range d.registrations {
		count += len(regs)
	}
	return count
}

// IsEmpty returns true if there are no active registrations.
func (d *ExitOnDispatcher) IsEmpty() bool {
	return d.Count() == 0
}

// String returns a debug representation of active registrations.
func (d *ExitOnDispatcher) String() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.registrations) == 0 {
		return "ExitOnDispatcher{empty}"
	}

	var result string
	for cmd, regs := range d.registrations {
		for _, r := range regs {
			result += fmt.Sprintf("  %s → task=%s handler=%s\n", cmd, r.TaskID[:8], r.HandlerName)
		}
	}
	return fmt.Sprintf("ExitOnDispatcher{\n%s}", result)
}
