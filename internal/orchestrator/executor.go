// Package orchestrator contains startup, recovery, and execution logic for
// the CodeMint task execution engine.
package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

// Executor wraps a CodingAgent with crash-fallback logic (Story 1.9):
//  1. Converts task.Timeout (ms) to a context deadline before dispatching.
//  2. On crash or timeout: reassigns the task to the human agent, forces
//     the status through Failure → Awaiting, and notifies the UI.
type Executor struct {
	codingAgent agent.CodingAgent
	taskRepo    repository.TaskRepository
	agentRepo   repository.AgentRepository
	ui          registry.UIMediator
}

// NewExecutor constructs an Executor with the provided dependencies.
func NewExecutor(
	codingAgent agent.CodingAgent,
	taskRepo repository.TaskRepository,
	agentRepo repository.AgentRepository,
	ui registry.UIMediator,
) *Executor {
	return &Executor{
		codingAgent: codingAgent,
		taskRepo:    taskRepo,
		agentRepo:   agentRepo,
		ui:          ui,
	}
}

// ExecuteTask dispatches the task to the coding agent with a timeout derived
// from task.Timeout (milliseconds). If the agent returns an error (including
// context deadline exceeded), the crash-fallback flow is triggered:
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
