package orchestrator

import (
	"context"
	"log"
	"time"

	"codemint.kanthorlabs.com/internal/repository"
)

// HeartbeatInterval is the duration between heartbeat ticks.
const HeartbeatInterval = 15 * time.Second

// Heartbeat maintains session ownership by periodically updating last_activity_at.
// It runs in a goroutine and stops when the context is canceled.
type Heartbeat struct {
	sessionRepo   repository.SessionRepository
	activeSession *ActiveSession
	interval      time.Duration
}

// NewHeartbeat creates a new Heartbeat with the default interval.
func NewHeartbeat(sessionRepo repository.SessionRepository, activeSession *ActiveSession) *Heartbeat {
	return &Heartbeat{
		sessionRepo:   sessionRepo,
		activeSession: activeSession,
		interval:      HeartbeatInterval,
	}
}

// Start begins the heartbeat loop. It runs until ctx is canceled.
// This should be called in a goroutine.
func (h *Heartbeat) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(ctx)
		}
	}
}

// tick updates the session's last_activity_at timestamp.
func (h *Heartbeat) tick(ctx context.Context) {
	// Only tick if we have an active session and are not suspended.
	if h.activeSession.Session == nil || h.activeSession.IsSuspended {
		return
	}

	now := time.Now().Unix()
	err := h.sessionRepo.SaveState(
		ctx,
		h.activeSession.Session.ID,
		h.activeSession.ClientID,
		now,
	)
	if err != nil {
		// Log but don't crash - network/DB issues shouldn't stop the heartbeat.
		log.Printf("heartbeat: failed to update session state: %v", err)
		return
	}

	// Update the in-memory session state.
	h.activeSession.Session.LastActivityAt.Int64 = now
	h.activeSession.Session.LastActivityAt.Valid = true
}

// TouchActivity updates the session's last_activity_at immediately.
// Called on user interactions (commands, messages) to keep the session fresh.
func (h *Heartbeat) TouchActivity(ctx context.Context) {
	h.tick(ctx)
}
