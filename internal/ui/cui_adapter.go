// Package ui provides UI adapters for different client modes.
package ui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/xdg"
)

// CUIAdapter is the adapter for Chat UI mode (daemon mode).
// It filters events to only forward terminal state changes and critical events,
// reducing notification spam for low-bandwidth chat clients like Telegram/Slack.
//
// Forwarded events:
//   - EventTaskStatusChanged (when To is Awaiting, Success, Failure, or Reverted)
//   - EventSessionTakeover
//   - EventSessionReclaimed
//   - EventACPAwaitingApproval
//   - EventACPApprovalResolved
//
// Dropped events:
//   - EventACPStream (micro-events: thinking, tool_update, etc.)
//   - EventProgress
//   - All other non-terminal events
type CUIAdapter struct {
	logger *slog.Logger
	logMu  sync.Mutex
	logFd  *os.File

	// Approval queue for PromptDecision flow.
	// Key: prompt ID (auto-incremented), Value: channel waiting for response.
	pendingPrompts   map[int]*pendingPrompt
	pendingPromptsMu sync.RWMutex
	nextPromptID     int
}

// pendingPrompt tracks a prompt awaiting user response via /approve or /deny.
type pendingPrompt struct {
	Request registry.PromptRequest
	RespCh  chan registry.PromptResponse
}

// CUIAdapterConfig holds configuration for creating a CUIAdapter.
type CUIAdapterConfig struct {
	Logger *slog.Logger
}

// NewCUIAdapter creates a new CUIAdapter with the given configuration.
func NewCUIAdapter(cfg CUIAdapterConfig) *CUIAdapter {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	a := &CUIAdapter{
		logger:         logger,
		pendingPrompts: make(map[int]*pendingPrompt),
		nextPromptID:   1,
	}

	// Initialize the log file for daemon mode.
	if err := a.initLogFile(); err != nil {
		logger.Warn("cui: failed to initialize log file", slog.String("error", err.Error()))
	}

	return a
}

// initLogFile creates the state directory and opens the CUI log file.
func (a *CUIAdapter) initLogFile() error {
	stateDir := xdg.StateDir()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	logPath := filepath.Join(stateDir, "cui.log")
	fd, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	a.logFd = fd
	return nil
}

// Close releases resources held by the adapter.
func (a *CUIAdapter) Close() error {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	if a.logFd != nil {
		if err := a.logFd.Close(); err != nil {
			return err
		}
		a.logFd = nil
	}
	return nil
}

// Compile-time check that CUIAdapter implements UIAdapter.
var _ UIAdapter = (*CUIAdapter)(nil)

// NotifyEvent receives a fire-and-forget event notification.
// It filters out micro-events (EventACPStream) and only forwards terminal
// state changes to the daemon log.
func (a *CUIAdapter) NotifyEvent(event registry.UIEvent) {
	// Apply low-bandwidth filter.
	if !a.shouldForwardEvent(event) {
		return
	}

	// Format and log the event.
	msg := a.formatEvent(event)
	a.writeLog(msg)
}

// shouldForwardEvent returns true if the event should be forwarded to the CUI.
func (a *CUIAdapter) shouldForwardEvent(event registry.UIEvent) bool {
	switch event.Type {
	case registry.EventTaskStatusChanged:
		// Only forward terminal states: Awaiting, Success, Failure, Reverted.
		payload, ok := event.Payload.(registry.TaskStatusChangedPayload)
		if !ok {
			return false
		}
		to := domain.TaskStatus(payload.To)
		return isTerminalStatus(to)

	case registry.EventSessionTakeover,
		registry.EventSessionReclaimed,
		registry.EventACPAwaitingApproval,
		registry.EventACPApprovalResolved:
		// Always forward these critical events.
		return true

	case registry.EventACPStream,
		registry.EventProgress,
		registry.EventACPAutoApproved:
		// Drop micro-events and progress updates.
		return false

	default:
		// Drop unknown event types by default.
		return false
	}
}

// isTerminalStatus returns true if the status is a terminal state for CUI filtering.
func isTerminalStatus(status domain.TaskStatus) bool {
	switch status {
	case domain.TaskStatusAwaiting,
		domain.TaskStatusSuccess,
		domain.TaskStatusFailure,
		domain.TaskStatusReverted:
		return true
	default:
		return false
	}
}

// formatEvent creates a human-readable log line for an event.
func (a *CUIAdapter) formatEvent(event registry.UIEvent) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	switch event.Type {
	case registry.EventTaskStatusChanged:
		return fmt.Sprintf("[%s] STATUS: %s (task=%s)", timestamp, event.Message, event.TaskID)

	case registry.EventSessionTakeover:
		return fmt.Sprintf("[%s] TAKEOVER: %s", timestamp, event.Message)

	case registry.EventSessionReclaimed:
		return fmt.Sprintf("[%s] RECLAIMED: %s", timestamp, event.Message)

	case registry.EventACPAwaitingApproval:
		return fmt.Sprintf("[%s] AWAITING_APPROVAL: %s", timestamp, event.Message)

	case registry.EventACPApprovalResolved:
		return fmt.Sprintf("[%s] APPROVAL_RESOLVED: %s", timestamp, event.Message)

	default:
		return fmt.Sprintf("[%s] %s: %s", timestamp, event.Type, event.Message)
	}
}

// writeLog writes a message to the CUI log file.
func (a *CUIAdapter) writeLog(msg string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	if a.logFd == nil {
		// Log file not initialized; fall back to slog.
		a.logger.Info("cui_event", slog.String("message", msg))
		return
	}

	_, err := fmt.Fprintln(a.logFd, msg)
	if err != nil {
		a.logger.Warn("cui: failed to write to log", slog.String("error", err.Error()))
	}
}

// PromptDecision displays a prompt to the user and blocks until response.
// In daemon mode, this enqueues the prompt for resolution via /approve or /deny
// REPL commands.
func (a *CUIAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	// Assign a prompt ID and register it.
	promptID := a.registerPrompt(req)
	defer a.unregisterPrompt(promptID)

	// Log the prompt for visibility.
	a.logPrompt(promptID, req)

	// Wait for response via /approve or /deny, or context cancellation.
	pending := a.getPendingPrompt(promptID)
	if pending == nil {
		return registry.PromptResponse{Error: ErrPromptCanceled}
	}

	select {
	case <-ctx.Done():
		return registry.PromptResponse{Error: ErrPromptCanceled}
	case resp := <-pending.RespCh:
		return resp
	}
}

// registerPrompt adds a prompt to the pending queue and returns its ID.
func (a *CUIAdapter) registerPrompt(req registry.PromptRequest) int {
	a.pendingPromptsMu.Lock()
	defer a.pendingPromptsMu.Unlock()

	id := a.nextPromptID
	a.nextPromptID++

	a.pendingPrompts[id] = &pendingPrompt{
		Request: req,
		RespCh:  make(chan registry.PromptResponse, 1),
	}

	return id
}

// unregisterPrompt removes a prompt from the pending queue.
func (a *CUIAdapter) unregisterPrompt(id int) {
	a.pendingPromptsMu.Lock()
	defer a.pendingPromptsMu.Unlock()

	if pending, ok := a.pendingPrompts[id]; ok {
		close(pending.RespCh)
		delete(a.pendingPrompts, id)
	}
}

// getPendingPrompt returns the pending prompt for the given ID.
func (a *CUIAdapter) getPendingPrompt(id int) *pendingPrompt {
	a.pendingPromptsMu.RLock()
	defer a.pendingPromptsMu.RUnlock()

	return a.pendingPrompts[id]
}

// logPrompt writes the prompt details to the log for the daemon client to see.
func (a *CUIAdapter) logPrompt(id int, req registry.PromptRequest) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Build the prompt message.
	var msg string
	msg += fmt.Sprintf("[%s] PROMPT #%d: %s\n", timestamp, id, req.Title)

	if req.Body != "" {
		msg += fmt.Sprintf("  Body: %s\n", req.Body)
	} else if req.Message != "" {
		msg += fmt.Sprintf("  Message: %s\n", req.Message)
	}

	// List options.
	if len(req.PromptOptions) > 0 {
		msg += "  Options:\n"
		for _, opt := range req.PromptOptions {
			msg += fmt.Sprintf("    [%s] %s", opt.ID, opt.Label)
			if opt.Description != "" {
				msg += fmt.Sprintf(" - %s", opt.Description)
			}
			msg += "\n"
		}
	} else if len(req.Options) > 0 {
		msg += "  Options:\n"
		for i, opt := range req.Options {
			msg += fmt.Sprintf("    [%d] %s\n", i+1, opt)
		}
	}

	msg += fmt.Sprintf("  Use: /approve %d <option_id> or /deny %d", id, id)

	a.writeLog(msg)
}

// ResolvePrompt resolves a pending prompt with the given response.
// Called by the /approve command handler.
// Returns an error if the prompt ID is not found.
func (a *CUIAdapter) ResolvePrompt(id int, optionID string) error {
	a.pendingPromptsMu.RLock()
	pending, ok := a.pendingPrompts[id]
	a.pendingPromptsMu.RUnlock()

	if !ok {
		return fmt.Errorf("prompt #%d not found", id)
	}

	// Send the response.
	select {
	case pending.RespCh <- registry.PromptResponse{
		SelectedOptionID: optionID,
		SelectedOption:   optionID, // Legacy field.
	}:
		return nil
	default:
		return fmt.Errorf("prompt #%d already resolved", id)
	}
}

// DenyPrompt denies a pending prompt (resolves with cancel).
// Called by the /deny command handler.
// Returns an error if the prompt ID is not found.
func (a *CUIAdapter) DenyPrompt(id int) error {
	a.pendingPromptsMu.RLock()
	pending, ok := a.pendingPrompts[id]
	a.pendingPromptsMu.RUnlock()

	if !ok {
		return fmt.Errorf("prompt #%d not found", id)
	}

	// Send denial response.
	select {
	case pending.RespCh <- registry.PromptResponse{
		SelectedOptionID: "deny",
		Error:            ErrPromptCanceled,
	}:
		return nil
	default:
		return fmt.Errorf("prompt #%d already resolved", id)
	}
}

// ListPendingPrompts returns a list of all pending prompts for display.
// Used by the /status command.
func (a *CUIAdapter) ListPendingPrompts() []PendingPromptInfo {
	a.pendingPromptsMu.RLock()
	defer a.pendingPromptsMu.RUnlock()

	var prompts []PendingPromptInfo
	for id, p := range a.pendingPrompts {
		prompts = append(prompts, PendingPromptInfo{
			ID:    id,
			Title: p.Request.Title,
			Kind:  string(p.Request.Kind),
		})
	}
	return prompts
}

// PendingPromptInfo contains summary information about a pending prompt.
type PendingPromptInfo struct {
	ID    int
	Title string
	Kind  string
}
