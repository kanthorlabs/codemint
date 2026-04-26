// Package ui provides the UI Mediator pattern implementation for decoupling
// the core execution loop from UI-specific concerns.
package ui

import (
	"context"

	"codemint.kanthorlabs.com/internal/registry"
)

// UIAdapter defines the interface that all concrete UIs (CLI, Web, Daemon)
// must implement to participate in the broadcast prompt flow.
type UIAdapter interface {
	// PromptDecision displays a prompt to the user and blocks until the user
	// selects an option or the context is canceled. Implementations must
	// dismiss the prompt immediately when ctx is canceled.
	PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse
}
