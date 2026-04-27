// Package ui provides user interface adapters for CodeMint.
package ui

import (
	"context"

	"codemint.kanthorlabs.com/internal/input"
)

// InboundBackend represents an input source that delivers user messages
// from external systems (Telegram, Slack, webhooks, etc.) into CodeMint.
// Implementations are responsible for:
//   - Connecting to their respective transport (e.g., Telegram Bot API)
//   - Converting incoming messages to input.InboundMessage
//   - Sending messages to the provided sink channel
//
// EPIC-04 §4.5 will deliver the production Telegram transport against this interface.
type InboundBackend interface {
	// Start begins receiving messages and sends them to sink.
	// The backend should run until ctx is cancelled or Stop is called.
	// Returns immediately after starting background processing.
	Start(ctx context.Context, sink chan<- input.InboundMessage) error

	// Stop gracefully shuts down the backend.
	// It should wait for any in-flight messages to be delivered
	// before returning, with a reasonable timeout.
	Stop(ctx context.Context) error
}
