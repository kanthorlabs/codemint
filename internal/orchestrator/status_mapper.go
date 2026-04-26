// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/repository/sqlite"
)

// StatusMapper translates ACP lifecycle events into database status transitions.
// It implements Task 3.7.1: centralizing the rules that turn ACP events into
// domain.TaskStatus transitions.
//
// Mapping table:
//
//	| Event                                  | New Status  |
//	|----------------------------------------|-------------|
//	| EventTurnStart                         | Processing  |
//	| EventPermissionRequest (no auto-allow) | Awaiting    |
//	| EventTurnEnd (success)                 | Success     |
//	| EventTurnEnd (with error)              | Failure     |
type StatusMapper struct {
	taskRepo repository.TaskRepository
	ui       registry.UIMediator
	logger   *slog.Logger

	// lastApplied tracks the last applied status per task to enable idempotency.
	// Key: taskID, Value: last applied TaskStatus.
	lastApplied   map[string]domain.TaskStatus
	lastAppliedMu sync.RWMutex

	// advanceCh signals the scheduler to advance to the next task after Success.
	advanceCh chan<- struct{}
}

// StatusMapperConfig holds the dependencies for creating a StatusMapper.
type StatusMapperConfig struct {
	TaskRepo  repository.TaskRepository
	UI        registry.UIMediator
	Logger    *slog.Logger
	AdvanceCh chan<- struct{} // Channel to signal scheduler to advance (Task 3.7.3)
}

// NewStatusMapper creates a new StatusMapper with the provided dependencies.
func NewStatusMapper(cfg StatusMapperConfig) *StatusMapper {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &StatusMapper{
		taskRepo:    cfg.TaskRepo,
		ui:          cfg.UI,
		logger:      logger,
		lastApplied: make(map[string]domain.TaskStatus),
		advanceCh:   cfg.AdvanceCh,
	}
}

// Apply translates an ACP event into a task status transition and updates the database.
// It is idempotent: applying the same event twice is a no-op.
// Returns nil if the transition was successful, skipped (idempotent), or if taskID is empty.
// Invalid transitions are logged and skipped (never panic).
func (m *StatusMapper) Apply(ctx context.Context, taskID string, ev acp.Event) error {
	// Short-circuit for ad-hoc /acp prompts (no associated task).
	if taskID == "" {
		return nil
	}

	newStatus, ok := m.mapEventToStatus(ev)
	if !ok {
		// Event doesn't map to a status transition (e.g., EventThinking, EventMessage).
		return nil
	}

	// Check idempotency: skip if we've already applied this status.
	if m.isAlreadyApplied(taskID, newStatus) {
		m.logger.Debug("status_mapper: skipping idempotent transition",
			"task_id", taskID,
			"status", newStatus,
			"event", ev.Kind.String(),
		)
		return nil
	}

	// Fetch the task to get the current status for the UI event.
	task, err := m.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		m.logger.Warn("status_mapper: failed to find task",
			"task_id", taskID,
			"error", err,
		)
		return err
	}

	fromStatus := task.Status

	// Attempt the status transition via the state machine.
	err = m.taskRepo.UpdateTaskStatus(ctx, taskID, newStatus)
	if err != nil {
		// Check if this is an invalid transition error (state machine rejection).
		if errors.Is(err, sqlite.ErrInvalidTransition) {
			m.logger.Warn("status_mapper: invalid transition rejected by state machine",
				"task_id", taskID,
				"from", fromStatus,
				"to", newStatus,
				"event", ev.Kind.String(),
			)
			return nil // Log and skip, never panic.
		}
		m.logger.Error("status_mapper: failed to update task status",
			"task_id", taskID,
			"status", newStatus,
			"error", err,
		)
		return err
	}

	// Record the applied status for idempotency.
	m.recordApplied(taskID, newStatus)

	m.logger.Info("status_mapper: task status updated",
		"task_id", taskID,
		"from", fromStatus,
		"to", newStatus,
		"event", ev.Kind.String(),
	)

	// Emit UI event (Task 3.7.4).
	m.emitStatusChangedEvent(taskID, fromStatus, newStatus, ev)

	// Signal scheduler to advance on Success (Task 3.7.3).
	if newStatus == domain.TaskStatusSuccess && m.advanceCh != nil {
		select {
		case m.advanceCh <- struct{}{}:
		default:
			// Non-blocking send; scheduler may already have a pending signal.
		}
	}

	return nil
}

// mapEventToStatus maps an ACP event kind to a task status.
// Returns the new status and true if a mapping exists, false otherwise.
func (m *StatusMapper) mapEventToStatus(ev acp.Event) (domain.TaskStatus, bool) {
	switch ev.Kind {
	case acp.EventTurnStart:
		return domain.TaskStatusProcessing, true

	case acp.EventPermissionRequest:
		// Permission requests that reach the mapper are those not auto-allowed
		// (Block/Unknown path from Story 3.6), hence Awaiting.
		return domain.TaskStatusAwaiting, true

	case acp.EventTurnEnd:
		// Determine success vs failure based on the event payload.
		if m.isTurnEndError(ev) {
			return domain.TaskStatusFailure, true
		}
		return domain.TaskStatusSuccess, true

	default:
		// Events like EventThinking, EventMessage, EventPlan, EventToolCall,
		// EventToolUpdate don't directly map to status transitions.
		return 0, false
	}
}

// isTurnEndError checks if a turn_end event indicates an error.
// It parses the raw message to look for error indicators.
func (m *StatusMapper) isTurnEndError(ev acp.Event) bool {
	// Parse the raw message to check for error field.
	// The turn_end event structure varies by ACP implementation.
	// We check for common error indicators in the raw JSON.
	if len(ev.Raw) == 0 {
		return false
	}

	// Try to parse as a generic structure with error field.
	var payload struct {
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
		Params struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"params"`
	}

	if err := unmarshalJSON(ev.Raw, &payload); err != nil {
		return false
	}

	// Check both top-level error and nested params.error.
	return payload.Error != nil || payload.Params.Error != nil
}

// isAlreadyApplied checks if the given status was already applied to the task.
func (m *StatusMapper) isAlreadyApplied(taskID string, status domain.TaskStatus) bool {
	m.lastAppliedMu.RLock()
	defer m.lastAppliedMu.RUnlock()

	last, ok := m.lastApplied[taskID]
	return ok && last == status
}

// recordApplied records that a status was applied to a task.
func (m *StatusMapper) recordApplied(taskID string, status domain.TaskStatus) {
	m.lastAppliedMu.Lock()
	defer m.lastAppliedMu.Unlock()

	m.lastApplied[taskID] = status
}

// ClearTask removes the idempotency tracking for a task.
// Call this when a task is completed or a new task is started.
func (m *StatusMapper) ClearTask(taskID string) {
	m.lastAppliedMu.Lock()
	defer m.lastAppliedMu.Unlock()

	delete(m.lastApplied, taskID)
}

// emitStatusChangedEvent sends a EventTaskStatusChanged event to the UI (Task 3.7.4).
func (m *StatusMapper) emitStatusChangedEvent(taskID string, from, to domain.TaskStatus, ev acp.Event) {
	if m.ui == nil {
		return
	}

	payload := TaskStatusChangedPayload{
		TaskID: taskID,
		From:   int(from),
		To:     int(to),
		Reason: ev.Kind.String(),
	}

	m.ui.NotifyAll(registry.UIEvent{
		Type:    registry.EventTaskStatusChanged,
		TaskID:  taskID,
		Message: formatStatusChangeMessage(from, to),
		Payload: payload,
	})
}

// TaskStatusChangedPayload contains details about a task status change.
type TaskStatusChangedPayload struct {
	TaskID string `json:"task_id"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Reason string `json:"reason"`
}

// formatStatusChangeMessage creates a human-readable message for a status change.
func formatStatusChangeMessage(from, to domain.TaskStatus) string {
	return "Task status changed: " + statusName(from) + " → " + statusName(to)
}

// statusName returns a human-readable name for a TaskStatus.
func statusName(s domain.TaskStatus) string {
	switch s {
	case domain.TaskStatusPending:
		return "Pending"
	case domain.TaskStatusProcessing:
		return "Processing"
	case domain.TaskStatusAwaiting:
		return "Awaiting"
	case domain.TaskStatusSuccess:
		return "Success"
	case domain.TaskStatusFailure:
		return "Failure"
	case domain.TaskStatusCompleted:
		return "Completed"
	case domain.TaskStatusReverted:
		return "Reverted"
	case domain.TaskStatusCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// unmarshalJSON is a helper to unmarshal JSON with error handling.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
