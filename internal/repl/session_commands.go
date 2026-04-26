package repl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// SessionCommandDeps holds the dependencies needed for session-related commands.
type SessionCommandDeps struct {
	SessionRepo   repository.SessionRepository
	ProjectRepo   repository.ProjectRepository
	TaskRepo      repository.TaskRepository
	ActiveSession registry.MutableSessionInfo
}

// RegisterSessionCommands registers session management commands (/session-resume, /activity).
func RegisterSessionCommands(r *registry.CommandRegistry, deps *SessionCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "session-resume",
			Description:    "List active sessions or switch to a different session.",
			Usage:          "/session-resume [session-id]",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        sessionResumeHandler(deps),
		},
		{
			Name:           "activity",
			Description:    "Show recent session activity (last 20 interactions).",
			Usage:          "/activity",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        activityHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register session command %q: %w", c.Name, err)
		}
	}
	return nil
}

// sessionResumeHandler handles the /session-resume command.
// Without args: lists all active sessions.
// With session ID: switches to that session.
func sessionResumeHandler(deps *SessionCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// No args: list active sessions.
		if len(args) == 0 {
			return listActiveSessions(ctx, deps)
		}

		// With session ID: switch to that session.
		sessionID := args[0]
		return switchToSession(ctx, deps, sessionID)
	}
}

// listActiveSessions returns a formatted list of all active sessions.
func listActiveSessions(ctx context.Context, deps *SessionCommandDeps) (registry.CommandResult, error) {
	sessions, err := deps.SessionRepo.ListActive(ctx)
	if err != nil {
		return registry.CommandResult{}, fmt.Errorf("list active sessions: %w", err)
	}

	if len(sessions) == 0 {
		return registry.CommandResult{
			Message: "No active sessions. Use /project-open to start.",
			Action:  registry.ActionNone,
		}, nil
	}

	currentSessionID := deps.ActiveSession.GetSessionID()

	var sb strings.Builder
	sb.WriteString("Active sessions:\n\n")

	for _, s := range sessions {
		// Load project name.
		project, err := deps.ProjectRepo.FindByID(ctx, s.ProjectID)
		if err != nil {
			continue
		}
		if project == nil {
			continue
		}

		// Format session entry.
		isCurrent := currentSessionID != "" && currentSessionID == s.ID
		marker := ""
		if isCurrent {
			marker = " (current)"
		}

		shortID := s.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		// Show last activity time.
		lastActivity := "never"
		if s.LastActivityAt.Valid {
			t := time.Unix(s.LastActivityAt.Int64, 0)
			lastActivity = t.Format("15:04:05")
		}

		fmt.Fprintf(&sb, "  %s - %s%s (last: %s)\n", shortID, project.Name, marker, lastActivity)
	}

	return registry.CommandResult{
		Message: sb.String(),
		Action:  registry.ActionNone,
	}, nil
}

// switchToSession switches the active session to the specified session ID.
func switchToSession(ctx context.Context, deps *SessionCommandDeps, sessionID string) (registry.CommandResult, error) {
	// Find the target session (support prefix matching).
	session, err := findSessionByPrefix(ctx, deps.SessionRepo, sessionID)
	if err != nil {
		return registry.CommandResult{}, err
	}
	if session == nil {
		return registry.CommandResult{
			Message: fmt.Sprintf("Session not found: %s", sessionID),
			Action:  registry.ActionNone,
		}, nil
	}

	// Don't switch if already on this session.
	currentSessionID := deps.ActiveSession.GetSessionID()
	if currentSessionID != "" && currentSessionID == session.ID {
		return registry.CommandResult{
			Message: "Already on this session.",
			Action:  registry.ActionNone,
		}, nil
	}

	// Load the project for the target session.
	project, err := deps.ProjectRepo.FindByID(ctx, session.ProjectID)
	if err != nil {
		return registry.CommandResult{}, fmt.Errorf("find project: %w", err)
	}
	if project == nil {
		return registry.CommandResult{
			Message: "Session's project no longer exists.",
			Action:  registry.ActionNone,
		}, nil
	}

	// Clear ownership on current session (if any).
	if currentSessionID != "" {
		if err := deps.SessionRepo.ClearOwnership(ctx, currentSessionID); err != nil {
			// Log but don't fail - best effort.
		}
	}

	// Take ownership of the new session.
	clientID := deps.ActiveSession.GetClientID()
	now := time.Now().Unix()
	if err := deps.SessionRepo.SaveState(ctx, session.ID, clientID, now); err != nil {
		return registry.CommandResult{}, fmt.Errorf("save session state: %w", err)
	}

	// Update in-memory state.
	yoloEnabled := project.YoloMode == int(domain.YoloModeOn)
	deps.ActiveSession.SetSession(session, project, yoloEnabled)
	deps.ActiveSession.SetSuspended(false)

	shortID := session.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	return registry.CommandResult{
		Message: fmt.Sprintf("Switched to session %s for project %q", shortID, project.Name),
		Action:  registry.ActionNone,
	}, nil
}

// findSessionByPrefix finds a session by ID or ID prefix.
func findSessionByPrefix(ctx context.Context, repo repository.SessionRepository, idPrefix string) (*domain.Session, error) {
	// First try exact match.
	session, err := repo.FindByID(ctx, idPrefix)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}

	// Try prefix match against active sessions.
	sessions, err := repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	for _, s := range sessions {
		if strings.HasPrefix(s.ID, idPrefix) {
			return s, nil
		}
	}

	return nil, nil
}

// activityHandler handles the /activity command.
// Shows recent coordination tasks for the current session.
func activityHandler(deps *SessionCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// Check if we're in a project session.
		sessionID := deps.ActiveSession.GetSessionID()
		if sessionID == "" {
			return registry.CommandResult{
				Message: "No active session. Use /project-open to start.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Check if TaskRepo is available.
		if deps.TaskRepo == nil {
			return registry.CommandResult{
				Message: "Activity tracking not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Get recent coordination tasks.
		tasks, err := deps.TaskRepo.ListCoordinationAfter(ctx, sessionID, "")
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("list activity: %w", err)
		}

		if len(tasks) == 0 {
			return registry.CommandResult{
				Message: "No recent activity in this session.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Take the last 20 tasks.
		limit := 20
		start := 0
		if len(tasks) > limit {
			start = len(tasks) - limit
		}
		recentTasks := tasks[start:]

		// Format output.
		var sb strings.Builder
		sb.WriteString("Recent activity:\n\n")

		for _, t := range recentTasks {
			sb.WriteString(formatActivityEntry(t))
		}

		return registry.CommandResult{
			Message: sb.String(),
			Action:  registry.ActionNone,
		}, nil
	}
}

// formatActivityEntry formats a single coordination task for display.
func formatActivityEntry(t *domain.Task) string {
	var sb strings.Builder

	// Extract client ID.
	clientID := "unknown"
	if t.ClientID.Valid {
		clientID = t.ClientID.String
		if len(clientID) > 12 {
			clientID = clientID[:12] + "..."
		}
	}

	// Extract timestamp from task ID (UUIDv7).
	// For simplicity, use the current time if parsing fails.
	// A proper implementation would extract from UUIDv7.
	timeStr := "??:??:??"
	// UUIDv7 format: first 8 hex chars are part of timestamp
	// This is a simplified extraction.
	if len(t.ID) >= 8 {
		timeStr = t.ID[:8]
	}

	// Extract command from input.
	command := ""
	if t.Input.Valid {
		// Try to parse JSON.
		if strings.Contains(t.Input.String, "command") {
			// Simple extraction - in production, use proper JSON parsing.
			start := strings.Index(t.Input.String, `"command":"`)
			if start != -1 {
				start += len(`"command":"`)
				end := strings.Index(t.Input.String[start:], `"`)
				if end != -1 {
					command = t.Input.String[start : start+end]
				}
			}
		}
	}
	if command == "" {
		command = "(unknown command)"
	}

	// Extract response from output.
	response := ""
	if t.Output.Valid {
		// Try to parse JSON.
		if strings.Contains(t.Output.String, "message") {
			start := strings.Index(t.Output.String, `"message":"`)
			if start != -1 {
				start += len(`"message":"`)
				end := strings.Index(t.Output.String[start:], `"`)
				if end != -1 {
					response = t.Output.String[start : start+end]
				}
			}
		}
	}

	// Format output.
	fmt.Fprintf(&sb, "[%s @ %s] %s\n", clientID, timeStr, command)
	if response != "" {
		// Truncate long responses.
		if len(response) > 100 {
			response = response[:100] + "..."
		}
		fmt.Fprintf(&sb, "> %s\n", response)
	}
	sb.WriteString("\n")

	return sb.String()
}
