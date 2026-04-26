// Package repl provides slash command handlers for the CodeMint REPL.
package repl

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/ui"
)

// DaemonCommandDeps holds the dependencies needed for daemon-related commands.
type DaemonCommandDeps struct {
	TaskRepo      repository.TaskRepository
	ActiveSession registry.MutableSessionInfo
	ACPRegistry   *acp.Registry
	CUIAdapter    *ui.CUIAdapter // Set when running in daemon mode.
}

// RegisterDaemonCommands registers daemon-specific commands (/tasks, /status, /approve, /deny).
// These commands support the CUI low-bandwidth pulse workflow.
func RegisterDaemonCommands(r *registry.CommandRegistry, deps *DaemonCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "tasks",
			Description:    "List tasks grouped by epic/story with status indicators.",
			Usage:          "/tasks",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        tasksHandler(deps),
		},
		{
			Name:           "status",
			Description:    "Show active task, worker status, and pending approvals.",
			Usage:          "/status",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        statusHandler(deps),
		},
		{
			Name:           "approve",
			Description:    "Approve a pending prompt (daemon mode).",
			Usage:          "/approve <prompt-id> <option-id>",
			SupportedModes: []registry.ClientMode{registry.ClientModeDaemon},
			Handler:        approveHandler(deps),
		},
		{
			Name:           "deny",
			Description:    "Deny a pending prompt (daemon mode).",
			Usage:          "/deny <prompt-id>",
			SupportedModes: []registry.ClientMode{registry.ClientModeDaemon},
			Handler:        denyHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register daemon command %q: %w", c.Name, err)
		}
	}
	return nil
}

// tasksHandler handles the /tasks command.
// Displays tasks grouped by (seq_epic, seq_story) with status indicators.
//
// Status indicators:
//
//	P - Pending
//	R - Processing (Running)
//	A - Awaiting
//	S - Success
//	F - Failure
//	C - Completed
//	V - Reverted
//	X - Cancelled
func tasksHandler(deps *DaemonCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// Check if we're in a session.
		sessionID := deps.ActiveSession.GetSessionID()
		if sessionID == "" {
			return registry.CommandResult{
				Message: "No active session. Use /project-open to start.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Check TaskRepo availability.
		if deps.TaskRepo == nil {
			return registry.CommandResult{
				Message: "Task tracking not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Fetch all tasks for the session.
		tasks, err := deps.TaskRepo.ListBySession(ctx, sessionID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("list tasks: %w", err)
		}

		if len(tasks) == 0 {
			return registry.CommandResult{
				Message: "No tasks in this session.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Format tasks grouped by epic/story.
		output := formatTaskHierarchy(tasks)

		return registry.CommandResult{
			Message: output,
			Action:  registry.ActionNone,
		}, nil
	}
}

// formatTaskHierarchy formats tasks as a hierarchical text display.
func formatTaskHierarchy(tasks []*domain.Task) string {
	if len(tasks) == 0 {
		return "No tasks."
	}

	// Group tasks by (epic, story).
	type groupKey struct {
		Epic  int
		Story int
	}
	groups := make(map[groupKey][]*domain.Task)
	var keys []groupKey

	for _, t := range tasks {
		key := groupKey{Epic: t.SeqEpic, Story: t.SeqStory}
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], t)
	}

	// Sort keys by epic, then story.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Epic != keys[j].Epic {
			return keys[i].Epic < keys[j].Epic
		}
		return keys[i].Story < keys[j].Story
	})

	var sb strings.Builder
	sb.WriteString("Tasks:\n\n")

	for _, key := range keys {
		groupTasks := groups[key]

		// Epic/Story header.
		fmt.Fprintf(&sb, "Epic %d / Story %d:\n", key.Epic, key.Story)

		// List tasks under this story.
		for _, t := range groupTasks {
			indicator := statusIndicator(t.Status)
			typeLabel := taskTypeLabel(t.Type)

			// Truncate task ID for display.
			shortID := t.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}

			fmt.Fprintf(&sb, "  [%s] %s (%s) %s\n", indicator, shortID, typeLabel, extractTaskSummary(t))
		}
		sb.WriteString("\n")
	}

	// Legend.
	sb.WriteString("Legend: P=Pending R=Running A=Awaiting S=Success F=Failure C=Completed V=Reverted X=Cancelled\n")

	return sb.String()
}

// statusIndicator returns a single-character status indicator.
func statusIndicator(status domain.TaskStatus) string {
	switch status {
	case domain.TaskStatusPending:
		return "P"
	case domain.TaskStatusProcessing:
		return "R" // Running
	case domain.TaskStatusAwaiting:
		return "A"
	case domain.TaskStatusSuccess:
		return "S"
	case domain.TaskStatusFailure:
		return "F"
	case domain.TaskStatusCompleted:
		return "C"
	case domain.TaskStatusReverted:
		return "V"
	case domain.TaskStatusCancelled:
		return "X"
	default:
		return "?"
	}
}

// taskTypeLabel returns a short label for the task type.
func taskTypeLabel(taskType domain.TaskType) string {
	switch taskType {
	case domain.TaskTypeCoding:
		return "coding"
	case domain.TaskTypeVerification:
		return "verify"
	case domain.TaskTypeConfirmation:
		return "confirm"
	case domain.TaskTypeCoordination:
		return "coord"
	default:
		return "unknown"
	}
}

// extractTaskSummary extracts a brief summary from the task input.
func extractTaskSummary(t *domain.Task) string {
	if !t.Input.Valid || t.Input.String == "" {
		return ""
	}

	// Try to extract a meaningful summary from input.
	input := t.Input.String
	if len(input) > 60 {
		input = input[:60] + "..."
	}

	// Clean up JSON formatting for readability.
	input = strings.ReplaceAll(input, `"`, "")
	input = strings.ReplaceAll(input, `{`, "")
	input = strings.ReplaceAll(input, `}`, "")
	input = strings.TrimSpace(input)

	return input
}

// statusHandler handles the /status command.
// Shows active task, worker PID, and last-status timestamp.
func statusHandler(deps *DaemonCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		var sb strings.Builder
		sb.WriteString("Status:\n\n")

		// Session info.
		sessionID := deps.ActiveSession.GetSessionID()
		if sessionID == "" {
			sb.WriteString("Session: (none - global mode)\n")
		} else {
			shortID := sessionID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			sb.WriteString(fmt.Sprintf("Session: %s\n", shortID))
		}

		// Worker status.
		if deps.ACPRegistry != nil && sessionID != "" {
			workerStatus := deps.ACPRegistry.Status(sessionID)
			if workerStatus != nil {
				sb.WriteString(fmt.Sprintf("Worker:  PID %d (%s)\n", workerStatus.PID, workerStatus.State))
				if !workerStatus.StartedAt.IsZero() {
					uptime := time.Since(workerStatus.StartedAt).Round(time.Second)
					sb.WriteString(fmt.Sprintf("Uptime:  %s\n", uptime))
				}
			} else {
				sb.WriteString("Worker:  not running\n")
			}
		} else {
			sb.WriteString("Worker:  n/a\n")
		}

		// Active task.
		if deps.TaskRepo != nil && sessionID != "" {
			nextTask, err := deps.TaskRepo.Next(ctx, sessionID)
			if err == nil && nextTask != nil {
				shortTaskID := nextTask.ID
				if len(shortTaskID) > 8 {
					shortTaskID = shortTaskID[:8]
				}
				statusName := statusIndicator(nextTask.Status)
				sb.WriteString(fmt.Sprintf("Task:    %s [%s]\n", shortTaskID, statusName))
			} else {
				sb.WriteString("Task:    (none pending)\n")
			}
		}

		// Pending approvals (daemon mode).
		if deps.CUIAdapter != nil {
			prompts := deps.CUIAdapter.ListPendingPrompts()
			if len(prompts) > 0 {
				sb.WriteString("\nPending Approvals:\n")
				for _, p := range prompts {
					sb.WriteString(fmt.Sprintf("  #%d: %s\n", p.ID, p.Title))
				}
			}
		}

		sb.WriteString(fmt.Sprintf("\nTimestamp: %s\n", time.Now().Format("2006-01-02 15:04:05")))

		return registry.CommandResult{
			Message: sb.String(),
			Action:  registry.ActionNone,
		}, nil
	}
}

// approveHandler handles the /approve command.
// Usage: /approve <prompt-id> <option-id>
func approveHandler(deps *DaemonCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		if deps.CUIAdapter == nil {
			return registry.CommandResult{
				Message: "Approval not available in this mode.",
				Action:  registry.ActionNone,
			}, nil
		}

		if len(args) < 2 {
			return registry.CommandResult{
				Message: "Usage: /approve <prompt-id> <option-id>\nExample: /approve 1 allow_once",
				Action:  registry.ActionNone,
			}, nil
		}

		promptID, err := strconv.Atoi(args[0])
		if err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Invalid prompt ID: %s (must be a number)", args[0]),
				Action:  registry.ActionNone,
			}, nil
		}

		optionID := args[1]

		if err := deps.CUIAdapter.ResolvePrompt(promptID, optionID); err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Failed to approve: %s", err.Error()),
				Action:  registry.ActionNone,
			}, nil
		}

		return registry.CommandResult{
			Message: fmt.Sprintf("Approved prompt #%d with option: %s", promptID, optionID),
			Action:  registry.ActionNone,
		}, nil
	}
}

// denyHandler handles the /deny command.
// Usage: /deny <prompt-id>
func denyHandler(deps *DaemonCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		if deps.CUIAdapter == nil {
			return registry.CommandResult{
				Message: "Approval not available in this mode.",
				Action:  registry.ActionNone,
			}, nil
		}

		if len(args) < 1 {
			return registry.CommandResult{
				Message: "Usage: /deny <prompt-id>\nExample: /deny 1",
				Action:  registry.ActionNone,
			}, nil
		}

		promptID, err := strconv.Atoi(args[0])
		if err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Invalid prompt ID: %s (must be a number)", args[0]),
				Action:  registry.ActionNone,
			}, nil
		}

		if err := deps.CUIAdapter.DenyPrompt(promptID); err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Failed to deny: %s", err.Error()),
				Action:  registry.ActionNone,
			}, nil
		}

		return registry.CommandResult{
			Message: fmt.Sprintf("Denied prompt #%d", promptID),
			Action:  registry.ActionNone,
		}, nil
	}
}
