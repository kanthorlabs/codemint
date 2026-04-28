package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"codemint.kanthorlabs.com/internal/registry"
)

// Compile-time interface satisfaction check.
var _ registry.UIMediator = (*UIMediator)(nil)

// ErrNoAdapters is returned when PromptDecision is called without any
// registered adapters.
var ErrNoAdapters = errors.New("ui: no adapters registered")

// ErrPromptCanceled is returned when the parent context is canceled before
// any adapter responds.
var ErrPromptCanceled = errors.New("ui: prompt canceled")

// ErrAllAdaptersFailed is returned when all adapters return errors.
// The underlying error contains the first adapter's error for debugging.
var ErrAllAdaptersFailed = errors.New("ui: all adapters failed")

// UIMediator manages multiple UIAdapter instances and broadcasts prompt
// requests concurrently. It implements a "first response wins" racing pattern
// using Go channels and context cancellation.
//
// UIMediator implements the registry.UIMediator interface, providing both
// the basic RenderMessage/ClearScreen methods and the concurrent PromptDecision
// broadcast capability.
type UIMediator struct {
	adapters []UIAdapter
	mu       sync.RWMutex
	// writer is the output destination for RenderMessage (defaults to os.Stdout).
	writer io.Writer
}

// NewUIMediator creates a new UIMediator with no registered adapters.
// The writer parameter specifies the output destination for RenderMessage.
func NewUIMediator(w io.Writer) *UIMediator {
	return &UIMediator{
		adapters: make([]UIAdapter, 0),
		writer:   w,
	}
}

// RegisterAdapter adds a UIAdapter to the mediator's broadcast list.
// Thread-safe for concurrent registration.
func (m *UIMediator) RegisterAdapter(a UIAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters = append(m.adapters, a)
}

// Adapters returns a snapshot of the currently registered adapters.
// Useful for testing and introspection.
func (m *UIMediator) Adapters() []UIAdapter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]UIAdapter, len(m.adapters))
	copy(result, m.adapters)
	return result
}

// RenderMessage displays msg to all registered adapters. For simple output,
// it writes to the configured io.Writer.
func (m *UIMediator) RenderMessage(msg string) {
	if m.writer != nil {
		_, _ = m.writer.Write([]byte(msg + "\n"))
	}
}

// ClearScreen sends a clear signal. The actual implementation depends on
// the registered adapters; for terminal output this is typically an ANSI
// escape sequence.
func (m *UIMediator) ClearScreen() {
	// Write ANSI escape sequence to clear screen and move cursor to top-left.
	if m.writer != nil {
		_, _ = m.writer.Write([]byte("\033[2J\033[H"))
	}
}

// NotifyAll broadcasts a fire-and-forget event to all registered UI adapters.
// Events are delivered asynchronously in separate goroutines; the method
// returns immediately without waiting for adapters to process the event.
func (m *UIMediator) NotifyAll(event registry.UIEvent) {
	m.mu.RLock()
	adapters := make([]UIAdapter, len(m.adapters))
	copy(adapters, m.adapters)
	m.mu.RUnlock()

	for _, adapter := range adapters {
		go adapter.NotifyEvent(event)
	}
}

// PromptDecision broadcasts the prompt request to all registered adapters
// concurrently. The first adapter to respond with a non-error response wins;
// the mediator immediately calls CancelPrompt on all other adapters so their
// UIs dismiss the dialog.
//
// If all adapters return errors, the mediator returns ErrAllAdaptersFailed
// wrapping the first error so callers can distinguish "user said no" from
// "all UIs broken".
//
// Returns ErrNoAdapters if no adapters are registered.
// Returns ErrPromptCanceled if the parent context is canceled before any response.
func (m *UIMediator) PromptDecision(parentCtx context.Context, req registry.PromptRequest) registry.PromptResponse {
	m.mu.RLock()
	adapters := make([]UIAdapter, len(m.adapters))
	copy(adapters, m.adapters)
	m.mu.RUnlock()

	if len(adapters) == 0 {
		return registry.PromptResponse{Error: ErrNoAdapters}
	}

	// Create a cancellable context from the parent. Calling cancel() signals
	// all goroutines to dismiss their prompts.
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	// Result type for channel communication.
	type result struct {
		idx  int
		resp registry.PromptResponse
	}

	// Buffered channel captures all responses.
	resultCh := make(chan result, len(adapters))

	// Spin up a goroutine for each adapter.
	for i, adapter := range adapters {
		go func(idx int, a UIAdapter) {
			resp := a.PromptDecision(ctx, req)
			resultCh <- result{idx: idx, resp: resp}
		}(i, adapter)
	}

	var firstErr error

	// Collect responses until we get a success or all fail.
	for received := 0; received < len(adapters); received++ {
		select {
		case r := <-resultCh:
			if r.resp.Error == nil {
				// First non-error response wins!
				cancel() // Signal others to abort via context.

				// Call CancelPrompt on all other adapters.
				m.cancelOthers(adapters, r.idx, req.TaskID)

				return r.resp
			}
			// Track first error for ErrAllAdaptersFailed.
			if firstErr == nil {
				firstErr = r.resp.Error
			}

		case <-parentCtx.Done():
			return registry.PromptResponse{Error: ErrPromptCanceled}
		}
	}

	// All adapters returned errors.
	return registry.PromptResponse{
		Error: fmt.Errorf("%w: %v", ErrAllAdaptersFailed, firstErr),
	}
}

// cancelOthers calls CancelPrompt on every adapter except the winner.
func (m *UIMediator) cancelOthers(adapters []UIAdapter, winnerIdx int, promptID string) {
	for i, adapter := range adapters {
		if i != winnerIdx {
			go adapter.CancelPrompt(promptID)
		}
	}
}
