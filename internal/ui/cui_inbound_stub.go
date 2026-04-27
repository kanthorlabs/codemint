// Package ui provides user interface adapters for CodeMint.
package ui

import (
	"context"
	"sync"
	"time"

	"codemint.kanthorlabs.com/internal/input"
)

// StubInbound is a deterministic InboundBackend for tests and local development.
// It allows programmatic injection of messages into the multiplexer without
// standing up a real Telegram bot or other external transport.
type StubInbound struct {
	source string
	sink   chan<- input.InboundMessage
	mu     sync.Mutex
	closed bool
}

// NewStubInbound creates a StubInbound that registers with the multiplexer
// using "cui-stub" as the source name.
func NewStubInbound(mux *input.Multiplexer) *StubInbound {
	return NewStubInboundWithSource(mux, "cui-stub")
}

// NewStubInboundWithSource creates a StubInbound with a custom source name.
func NewStubInboundWithSource(mux *input.Multiplexer, source string) *StubInbound {
	return &StubInbound{
		source: source,
		sink:   mux.RegisterSource(source, 16),
	}
}

// Start implements InboundBackend. For the stub, this is a no-op since
// messages are injected programmatically via Inject.
func (s *StubInbound) Start(ctx context.Context, sink chan<- input.InboundMessage) error {
	// StubInbound doesn't need the sink parameter since it already has
	// the multiplexer channel from construction. However, we accept it
	// to satisfy the InboundBackend interface.
	return nil
}

// Stop implements InboundBackend. Marks the stub as closed.
func (s *StubInbound) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Inject sends a message through the multiplexer as if it came from
// an external source. This is the primary method for tests.
func (s *StubInbound) Inject(text string) bool {
	return s.InjectWithUserID(text, "")
}

// InjectWithUserID sends a message with a specific user ID.
func (s *StubInbound) InjectWithUserID(text, userID string) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()

	select {
	case s.sink <- input.InboundMessage{
		Source: s.source,
		UserID: userID,
		Text:   text,
		At:     time.Now(),
	}:
		return true
	default:
		// Channel full, message dropped.
		return false
	}
}

// Source returns the source name used by this stub.
func (s *StubInbound) Source() string {
	return s.source
}

// Compile-time assertion: StubInbound implements InboundBackend.
var _ InboundBackend = (*StubInbound)(nil)
