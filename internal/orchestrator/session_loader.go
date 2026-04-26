package orchestrator

import (
	"context"
	"fmt"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// StalenessThreshold is the duration after which a session's active_client
// is considered stale and can be auto-taken over without notification.
const StalenessThreshold = 60 * time.Second

// SessionLoader handles session auto-resume and handoff logic at startup.
type SessionLoader struct {
	sessionRepo repository.SessionRepository
	projectRepo repository.ProjectRepository
}

// NewSessionLoader creates a new SessionLoader.
func NewSessionLoader(sessionRepo repository.SessionRepository, projectRepo repository.ProjectRepository) *SessionLoader {
	return &SessionLoader{
		sessionRepo: sessionRepo,
		projectRepo: projectRepo,
	}
}

// LoadResult contains the result of loading a session at startup.
type LoadResult struct {
	// Session is the active session (nil if no active session found).
	Session *domain.Session
	// Project is the project associated with the session (nil if no active session).
	Project *domain.Project
	// PreviousClient is the client ID that was owning the session (empty if stale or no previous owner).
	PreviousClient string
	// WasStale indicates whether the previous owner was considered stale (> 60s inactive).
	WasStale bool
	// Message is a user-facing message describing what happened.
	Message string
}

// LoadMostRecentSession attempts to load the most recently active session.
// It handles ownership transfer and returns information about what was loaded.
func (l *SessionLoader) LoadMostRecentSession(ctx context.Context, clientMode registry.ClientMode) (*LoadResult, error) {
	// Generate a new client ID for this instance.
	clientID := GenerateClientID(clientMode)

	// Find the most recently active session.
	session, err := l.sessionRepo.GetMostRecentActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("get most recent active session: %w", err)
	}

	// No active sessions - start in global mode.
	if session == nil {
		return &LoadResult{
			Session: nil,
			Project: nil,
			Message: "No active session. Use /project-open to start.",
		}, nil
	}

	// Load the associated project.
	project, err := l.projectRepo.FindByID(ctx, session.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("find project %q: %w", session.ProjectID, err)
	}

	// Handle edge case: session exists but project was deleted.
	if project == nil {
		return &LoadResult{
			Session: nil,
			Project: nil,
			Message: "No active session. Use /project-open to start.",
		}, nil
	}

	// Check if there's a previous owner.
	var previousClient string
	var wasStale bool

	if session.ActiveClient.Valid && session.ActiveClient.String != "" {
		previousClient = session.ActiveClient.String

		// Check staleness.
		if session.LastActivityAt.Valid {
			lastActivity := time.Unix(session.LastActivityAt.Int64, 0)
			wasStale = time.Since(lastActivity) > StalenessThreshold
		} else {
			// No last activity recorded - treat as stale.
			wasStale = true
		}
	}

	// Take ownership of the session.
	now := time.Now().Unix()
	if err := l.sessionRepo.SaveState(ctx, session.ID, clientID, now); err != nil {
		return nil, fmt.Errorf("save session state: %w", err)
	}

	// Update the session object with new ownership.
	session.ActiveClient.String = clientID
	session.ActiveClient.Valid = true
	session.LastActivityAt.Int64 = now
	session.LastActivityAt.Valid = true

	// Build message.
	message := fmt.Sprintf("Resuming session %s for project %q", shortID(session.ID), project.Name)
	if previousClient != "" && !wasStale {
		// Fresh takeover - the other client should be notified.
		message = fmt.Sprintf("Resuming session %s for project %q (took over from %s)", shortID(session.ID), project.Name, shortClientID(previousClient))
	}

	return &LoadResult{
		Session:        session,
		Project:        project,
		PreviousClient: previousClient,
		WasStale:       wasStale,
		Message:        message,
	}, nil
}

// CreateActiveSession creates an ActiveSession from a LoadResult.
func (l *SessionLoader) CreateActiveSession(result *LoadResult, clientMode registry.ClientMode) *ActiveSession {
	clientID := GenerateClientID(clientMode)

	if result.Session == nil {
		return &ActiveSession{
			ClientMode:  clientMode,
			ClientID:    clientID,
			IsGlobal:    true,
			IsSuspended: false,
			Project:     nil,
			Session:     nil,
			YoloEnabled: false,
		}
	}

	// Use the client ID from the session if already set.
	if result.Session.ActiveClient.Valid {
		clientID = result.Session.ActiveClient.String
	}

	yoloEnabled := result.Project != nil && result.Project.YoloMode == int(domain.YoloModeOn)

	return &ActiveSession{
		ClientMode:  clientMode,
		ClientID:    clientID,
		IsGlobal:    false,
		IsSuspended: false,
		Project:     result.Project,
		Session:     result.Session,
		YoloEnabled: yoloEnabled,
	}
}

// GenerateClientID creates a unique client identifier in the format "{mode}:{uuid}".
func GenerateClientID(mode registry.ClientMode) string {
	return fmt.Sprintf("%s:%s", mode, idgen.MustNew())
}

// shortID returns a shortened version of a UUID for display purposes.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// shortClientID extracts and shortens the client ID for display.
func shortClientID(clientID string) string {
	// Format: "cli:01abc..." or "daemon:01xyz..."
	if len(clientID) > 12 {
		return clientID[:12] + "..."
	}
	return clientID
}
