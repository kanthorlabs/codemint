// Package repl provides the Read-Eval-Print Loop functionality for CodeMint.
package repl

import (
	"codemint.kanthorlabs.com/internal/input"
)

// InboundMessage is an alias for input.InboundMessage for backward compatibility.
// Prefer using input.InboundMessage directly for new code.
type InboundMessage = input.InboundMessage

// InputMultiplexer is an alias for input.Multiplexer for backward compatibility.
// Prefer using input.Multiplexer directly for new code.
type InputMultiplexer = input.Multiplexer

// NewInputMultiplexer creates a new InputMultiplexer.
// Prefer using input.NewMultiplexer directly for new code.
func NewInputMultiplexer() *InputMultiplexer {
	return input.NewMultiplexer()
}
