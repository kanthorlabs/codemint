// Package input defines types for user input handling across the CodeMint system.
// This package is designed to be imported by both internal/repl and internal/ui
// without creating import cycles.
package input

import (
	"time"
)

// InboundMessage represents a user input from any source.
type InboundMessage struct {
	Source string    // "tui", "cui-telegram", "cui-stub", etc.
	UserID string    // Optional, source-specific user identifier.
	Text   string    // The actual user input.
	At     time.Time // When the message was received.
}
