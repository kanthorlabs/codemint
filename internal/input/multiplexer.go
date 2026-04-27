// Package input defines types for user input handling across the CodeMint system.
// This package is designed to be imported by both internal/repl and internal/ui
// without creating import cycles.
package input

import (
	"log/slog"
	"sync"
)

// Multiplexer aggregates inbound messages from multiple sources into a
// single output channel. Each source has a bounded buffer; if a source overruns,
// the multiplexer drops the oldest message rather than blocking.
type Multiplexer struct {
	out     chan InboundMessage
	sources map[string]*sourceBuffer
	mu      sync.Mutex
	wg      sync.WaitGroup
	closed  bool
}

// sourceBuffer holds a bounded channel for a single input source.
type sourceBuffer struct {
	ch       chan InboundMessage
	source   string
	capacity int
}

// NewMultiplexer creates a new Multiplexer with a shared output
// channel sized at 64.
func NewMultiplexer() *Multiplexer {
	return &Multiplexer{
		out:     make(chan InboundMessage, 64),
		sources: make(map[string]*sourceBuffer),
	}
}

// RegisterSource registers a new input source with the given capacity.
// Returns a write-only channel that the source should send messages to.
// If capacity is <= 0, defaults to 32.
func (m *Multiplexer) RegisterSource(source string, capacity int) chan<- InboundMessage {
	if capacity <= 0 {
		capacity = 32
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		// Return a closed channel if multiplexer is already closed.
		ch := make(chan InboundMessage)
		close(ch)
		return ch
	}

	// If source already exists, return the existing channel.
	if sb, ok := m.sources[source]; ok {
		return sb.ch
	}

	sb := &sourceBuffer{
		ch:       make(chan InboundMessage, capacity),
		source:   source,
		capacity: capacity,
	}
	m.sources[source] = sb

	// Start a goroutine to forward messages from this source to the shared output.
	m.wg.Add(1)
	go m.forwardLoop(sb)

	return sb.ch
}

// forwardLoop reads from a source buffer and forwards to the shared output.
// Implements drop-oldest policy when the output channel is full.
func (m *Multiplexer) forwardLoop(sb *sourceBuffer) {
	defer m.wg.Done()

	for msg := range sb.ch {
		// Try to send to output channel.
		select {
		case m.out <- msg:
			// Sent successfully.
		default:
			// Output channel is full. Drop the oldest message from this source
			// if there are more messages queued, otherwise drop this one.
			slog.Warn("input: dropping message due to back-pressure",
				"source", sb.source,
				"text_preview", truncatePreview(msg.Text, 50),
			)
			// Try non-blocking send; if still blocked, the message is dropped.
			select {
			case m.out <- msg:
			default:
			}
		}
	}
}

// Recv returns the receive-only channel for consuming multiplexed messages.
func (m *Multiplexer) Recv() <-chan InboundMessage {
	return m.out
}

// Close shuts down all forwarders and closes the output channel.
// Safe to call multiple times.
func (m *Multiplexer) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true

	// Close all source channels to stop forwarders.
	for _, sb := range m.sources {
		close(sb.ch)
	}
	m.mu.Unlock()

	// Wait for all forwarders to finish.
	m.wg.Wait()

	// Close the output channel.
	close(m.out)
}

// truncatePreview returns the first n characters of s, appending "..." if truncated.
func truncatePreview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
