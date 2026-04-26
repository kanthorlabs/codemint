package ui

import (
	"context"
	"errors"

	"codemint.kanthorlabs.com/internal/registry"
)

// ErrCUINotImplemented is returned when CUI operations are attempted before
// the actual implementation is available (EPIC-02).
var ErrCUINotImplemented = errors.New("ui: CUI adapter not implemented (EPIC-02)")

// CUIAdapter is a placeholder adapter for Chat UI mode (Telegram bot, WebSocket, etc.).
// It satisfies the UIAdapter interface but returns stub responses. The actual
// implementation will be completed in EPIC-02.
type CUIAdapter struct {
	// TODO: EPIC-02 - Add fields for Telegram bot client, WebSocket connection, etc.
}

// NewCUIAdapter creates a new CUIAdapter.
func NewCUIAdapter() *CUIAdapter {
	return &CUIAdapter{}
}

// Compile-time check that CUIAdapter implements UIAdapter.
var _ UIAdapter = (*CUIAdapter)(nil)

// NotifyEvent receives a fire-and-forget event notification.
// Currently a no-op placeholder for EPIC-02.
func (a *CUIAdapter) NotifyEvent(event registry.UIEvent) {
	// TODO: EPIC-02 - Send event to chat interface.
	// Examples:
	//   - EventTaskStarted: "Started task {id}: {message}"
	//   - EventTaskCompleted: "Completed task {id}"
	//   - EventSessionTakeover: "Session taken over by {payload}"
	//
	// For now, silently discard events.
}

// PromptDecision displays a prompt to the user and blocks until response.
// Currently returns a cancellation error as the implementation is pending EPIC-02.
func (a *CUIAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	// TODO: EPIC-02 - Display prompt in chat, await response.
	// Implementation will:
	//   1. Send prompt message with inline keyboard (Telegram) or buttons (Web)
	//   2. Wait for user selection or context cancellation
	//   3. Return selected option
	//
	// For now, wait for context cancellation.
	<-ctx.Done()
	return registry.PromptResponse{
		Error: ErrPromptCanceled,
	}
}
