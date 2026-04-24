// Package project registers project-scoped slash commands for the CodeMint
// dispatcher, including the human review flow introduced in User Story 1.8.
package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"codemint.kanthorlabs.com/internal/agent"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// ErrTaskNotAwaiting is returned when an accept or revert command targets a
// task that is not in the TaskStatusAwaiting state.
var ErrTaskNotAwaiting = errors.New("project: task is not in Awaiting state")

// ErrTaskNotCodingType is returned when the discard command targets a task
// that is not of type TaskTypeCoding.
var ErrTaskNotCodingType = errors.New("project: discard is only available for Coding tasks")

// ReviewCommands holds the dependencies required by the accept/revert handlers.
type ReviewCommands struct {
	taskRepo    repository.TaskRepository
	codingAgent agent.CodingAgent
}

// NewReviewCommands constructs a ReviewCommands with the given task repository
// and coding agent.
func NewReviewCommands(taskRepo repository.TaskRepository, codingAgent agent.CodingAgent) *ReviewCommands {
	return &ReviewCommands{taskRepo: taskRepo, codingAgent: codingAgent}
}

// Register wires the `/task accept`, `/task revert`, and `/task discard`
// sub-commands into r. All commands are available in all client modes.
func (rc *ReviewCommands) Register(r *registry.CommandRegistry) error {
	commands := []registry.Command{
		{
			Name:        "task accept",
			Description: "Accept an awaiting task, finalising the agent's changes.",
			Usage:       "/task accept <task_id>",
			Handler:     rc.acceptHandler(),
		},
		{
			Name:        "task revert",
			Description: "Revert an awaiting task, rolling back the agent's changes.",
			Usage:       "/task revert <task_id>",
			Handler:     rc.revertHandler(),
		},
		{
			Name:        "task discard",
			Description: "Discard the working-directory changes left by a crashed Coding Agent.",
			Usage:       "/task discard <task_id>",
			Handler:     rc.discardHandler(),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("project: register command %q: %w", c.Name, err)
		}
	}
	return nil
}

// acceptHandler returns the Handler for `/task accept <task_id>`.
//
// Flow:
//  1. Verify the task is in TaskStatusAwaiting.
//  2. Call CodingAgent.Accept.
//  3. Update task status to TaskStatusSuccess.
//  4. Return a CommandResult notifying the UI.
func (rc *ReviewCommands) acceptHandler() registry.Handler {
	return func(ctx context.Context, _ registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		taskID, err := requireTaskID(args)
		if err != nil {
			return registry.CommandResult{}, err
		}

		task, err := rc.taskRepo.FindByID(ctx, taskID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("project: accept: fetch task %q: %w", taskID, err)
		}
		if task.Status != domain.TaskStatusAwaiting {
			return registry.CommandResult{}, fmt.Errorf("%w: task %q has status %d", ErrTaskNotAwaiting, taskID, task.Status)
		}

		if err := rc.codingAgent.Accept(ctx, task); err != nil {
			return registry.CommandResult{}, fmt.Errorf("project: accept: agent error for task %q: %w", taskID, err)
		}

		if err := rc.taskRepo.UpdateTaskStatus(ctx, taskID, domain.TaskStatusSuccess); err != nil {
			return registry.CommandResult{}, fmt.Errorf("project: accept: update status for task %q: %w", taskID, err)
		}

		return registry.CommandResult{
			Message: fmt.Sprintf("Task %q accepted. Changes finalised.", taskID),
			Action:  registry.ActionNone,
		}, nil
	}
}

// revertHandler returns the Handler for `/task revert <task_id>`.
//
// Flow:
//  1. Verify the task is in TaskStatusAwaiting.
//  2. Call CodingAgent.Revert.
//  3. On agent error: log, update status to TaskStatusFailure (crash fallback).
//  4. On success: update status to TaskStatusReverted.
//  5. Return a CommandResult reflecting the outcome.
func (rc *ReviewCommands) revertHandler() registry.Handler {
	return func(ctx context.Context, _ registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		taskID, err := requireTaskID(args)
		if err != nil {
			return registry.CommandResult{}, err
		}

		task, err := rc.taskRepo.FindByID(ctx, taskID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("project: revert: fetch task %q: %w", taskID, err)
		}
		if task.Status != domain.TaskStatusAwaiting {
			return registry.CommandResult{}, fmt.Errorf("%w: task %q has status %d", ErrTaskNotAwaiting, taskID, task.Status)
		}

		if agentErr := rc.codingAgent.Revert(ctx, task); agentErr != nil {
			// Agent crash: log, transition to Failure, trigger fallback (Story 1.9).
			slog.Error("project: revert: agent crash detected",
				"task_id", taskID,
				"error", agentErr,
			)
			if updateErr := rc.taskRepo.UpdateTaskStatus(ctx, taskID, domain.TaskStatusFailure); updateErr != nil {
				slog.Error("project: revert: failed to update task status to Failure",
					"task_id", taskID,
					"error", updateErr,
				)
			}
			return registry.CommandResult{
				Message: fmt.Sprintf("Task %q revert failed: agent crash. Task marked as failed.", taskID),
				Action:  registry.ActionNone,
			}, fmt.Errorf("project: revert: agent crash for task %q: %w", taskID, agentErr)
		}

		if err := rc.taskRepo.UpdateTaskStatus(ctx, taskID, domain.TaskStatusReverted); err != nil {
			return registry.CommandResult{}, fmt.Errorf("project: revert: update status for task %q: %w", taskID, err)
		}

		return registry.CommandResult{
			Message: fmt.Sprintf("Task %q reverted. Changes rolled back.", taskID),
			Action:  registry.ActionNone,
		}, nil
	}
}

// requireTaskID extracts the task ID from the command args, returning an error
// if no argument was provided.
func requireTaskID(args []string) (string, error) {
	if len(args) == 0 || args[0] == "" {
		return "", fmt.Errorf("project: task_id argument is required")
	}
	return args[0], nil
}

// discardHandler returns the Handler for `/task discard <task_id>`.
//
// This command is a placeholder for the OS-level git restore/clean operation
// that will discard working-directory changes left by a crashed Coding Agent.
// It is only available for tasks of type TaskTypeCoding.
//
// Flow:
//  1. Verify the task exists and is of type TaskTypeCoding.
//  2. Stub: TODO: Implement OS-level git restore/clean.
//  3. Return a CommandResult informing the user.
func (rc *ReviewCommands) discardHandler() registry.Handler {
	return func(ctx context.Context, _ registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		taskID, err := requireTaskID(args)
		if err != nil {
			return registry.CommandResult{}, err
		}

		task, err := rc.taskRepo.FindByID(ctx, taskID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("project: discard: fetch task %q: %w", taskID, err)
		}
		if task.Type != domain.TaskTypeCoding {
			return registry.CommandResult{}, fmt.Errorf("%w: task %q has type %d", ErrTaskNotCodingType, taskID, task.Type)
		}

		// TODO: Implement OS-level git restore/clean to discard working-directory
		// changes left by the crashed Coding Agent.

		return registry.CommandResult{
			Message: fmt.Sprintf("Task %q discard is not yet implemented. Please manually run `git restore .` and `git clean -fd` in the working directory.", taskID),
			Action:  registry.ActionNone,
		}, nil
	}
}
