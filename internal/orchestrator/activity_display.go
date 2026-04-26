package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// ActivityDisplay handles displaying missed interactions and recent activity.
type ActivityDisplay struct {
	taskRepo repository.TaskRepository
}

// NewActivityDisplay creates a new ActivityDisplay.
func NewActivityDisplay(taskRepo repository.TaskRepository) *ActivityDisplay {
	return &ActivityDisplay{
		taskRepo: taskRepo,
	}
}

// MissedActivity represents activity that occurred on other clients.
type MissedActivity struct {
	// HasActivity indicates whether there is any missed activity.
	HasActivity bool
	// Activities is the list of missed coordination tasks.
	Activities []ActivityEntry
	// Message is the formatted message to display.
	Message string
}

// ActivityEntry represents a single coordination task for display.
type ActivityEntry struct {
	ClientID  string
	Timestamp time.Time
	Command   string
	Response  string
	IsError   bool
}

// GetMissedActivity retrieves activity that occurred since lastSeenTaskID
// from other clients (not currentClientID).
func (a *ActivityDisplay) GetMissedActivity(ctx context.Context, sessionID, lastSeenTaskID, currentClientID string) (*MissedActivity, error) {
	// Get coordination tasks after the last seen task.
	tasks, err := a.taskRepo.ListCoordinationAfter(ctx, sessionID, lastSeenTaskID)
	if err != nil {
		return nil, fmt.Errorf("list coordination tasks: %w", err)
	}

	// Filter to tasks from other clients.
	var activities []ActivityEntry
	for _, t := range tasks {
		// Skip tasks from the current client.
		if t.ClientID.Valid && t.ClientID.String == currentClientID {
			continue
		}

		// Parse the task to extract activity info.
		entry := a.parseTaskToActivity(t)
		activities = append(activities, entry)
	}

	if len(activities) == 0 {
		return &MissedActivity{
			HasActivity: false,
		}, nil
	}

	// Format the message.
	message := a.formatActivities(activities)

	return &MissedActivity{
		HasActivity: true,
		Activities:  activities,
		Message:     message,
	}, nil
}

// GetRecentActivity retrieves recent coordination tasks for the /activity command.
func (a *ActivityDisplay) GetRecentActivity(ctx context.Context, sessionID string, limit int) (string, error) {
	// Get all coordination tasks (no filter).
	tasks, err := a.taskRepo.ListCoordinationAfter(ctx, sessionID, "")
	if err != nil {
		return "", fmt.Errorf("list coordination tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No recent activity in this session.", nil
	}

	// Take the last N tasks.
	start := 0
	if len(tasks) > limit {
		start = len(tasks) - limit
	}
	recentTasks := tasks[start:]

	// Convert to activities.
	var activities []ActivityEntry
	for _, t := range recentTasks {
		entry := a.parseTaskToActivity(t)
		activities = append(activities, entry)
	}

	// Format the output.
	return a.formatActivities(activities), nil
}

// parseTaskToActivity converts a coordination task to an activity entry.
func (a *ActivityDisplay) parseTaskToActivity(t *domain.Task) ActivityEntry {
	entry := ActivityEntry{
		IsError: false,
	}

	// Extract client ID.
	if t.ClientID.Valid {
		entry.ClientID = shortClientID(t.ClientID.String)
	} else {
		entry.ClientID = "unknown"
	}

	// Extract timestamp from UUIDv7.
	entry.Timestamp = idgen.ExtractTime(t.ID)

	// Parse input payload.
	if t.Input.Valid {
		var input InteractionInputPayload
		if err := json.Unmarshal([]byte(t.Input.String), &input); err == nil {
			entry.Command = input.Command
		}
	}

	// Parse output payload.
	if t.Output.Valid {
		var output InteractionOutputPayload
		if err := json.Unmarshal([]byte(t.Output.String), &output); err == nil {
			if output.Error != "" {
				entry.Response = output.Error
				entry.IsError = true
			} else {
				entry.Response = output.Message
			}
		}
	}

	return entry
}

// formatActivities formats a list of activities for display.
func (a *ActivityDisplay) formatActivities(activities []ActivityEntry) string {
	var sb strings.Builder

	for _, act := range activities {
		// Format: [client @ HH:MM:SS] command
		// > response (truncated if too long)
		timeStr := act.Timestamp.Format("15:04:05")
		fmt.Fprintf(&sb, "[%s @ %s] %s\n", act.ClientID, timeStr, act.Command)

		if act.Response != "" {
			// Truncate long responses.
			response := act.Response
			if len(response) > 200 {
				response = response[:200] + "..."
			}
			// Indent response lines.
			lines := strings.Split(response, "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Fprintf(&sb, "> %s\n", line)
				}
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
