package ui

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/registry"
)

// FastMockAdapter returns a response quickly (10ms).
type FastMockAdapter struct {
	response registry.PromptResponse
	called   atomic.Bool
	// events stores received UIEvent notifications for test assertions.
	events   []registry.UIEvent
	eventsMu sync.Mutex
}

func (a *FastMockAdapter) NotifyEvent(event registry.UIEvent) {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	a.events = append(a.events, event)
}

func (a *FastMockAdapter) Events() []registry.UIEvent {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	result := make([]registry.UIEvent, len(a.events))
	copy(result, a.events)
	return result
}

func (a *FastMockAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	a.called.Store(true)
	select {
	case <-time.After(10 * time.Millisecond):
		return a.response
	case <-ctx.Done():
		return registry.PromptResponse{Error: ctx.Err()}
	}
}

// SlowMockAdapter blocks until context is canceled or returns after 100ms.
type SlowMockAdapter struct {
	response  registry.PromptResponse
	called    atomic.Bool
	canceled  atomic.Bool
	cancelErr error
	// events stores received UIEvent notifications for test assertions.
	events   []registry.UIEvent
	eventsMu sync.Mutex
}

func (a *SlowMockAdapter) NotifyEvent(event registry.UIEvent) {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	a.events = append(a.events, event)
}

func (a *SlowMockAdapter) Events() []registry.UIEvent {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	result := make([]registry.UIEvent, len(a.events))
	copy(result, a.events)
	return result
}

func (a *SlowMockAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	a.called.Store(true)
	select {
	case <-time.After(100 * time.Millisecond):
		return a.response
	case <-ctx.Done():
		a.canceled.Store(true)
		a.cancelErr = ctx.Err()
		return registry.PromptResponse{Error: ctx.Err()}
	}
}

func TestNewUIMediator(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)
	if m == nil {
		t.Fatal("NewUIMediator returned nil")
	}
	if len(m.Adapters()) != 0 {
		t.Errorf("expected 0 adapters, got %d", len(m.Adapters()))
	}
}

func TestUIMediator_RegisterAdapter(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)
	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	adapters := m.Adapters()
	if len(adapters) != 2 {
		t.Errorf("expected 2 adapters, got %d", len(adapters))
	}
}

func TestUIMediator_PromptDecision_NoAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	resp := m.PromptDecision(context.Background(), registry.PromptRequest{
		TaskID:  "task-1",
		Message: "Review changes?",
		Options: []string{"Accept", "Revert"},
	})

	if resp.Error != ErrNoAdapters {
		t.Errorf("expected ErrNoAdapters, got %v", resp.Error)
	}
}

func TestUIMediator_PromptDecision_FirstResponseWins(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	req := registry.PromptRequest{
		TaskID:  "task-1",
		Message: "Review changes?",
		Options: []string{"Accept", "Revert"},
	}

	resp := m.PromptDecision(context.Background(), req)

	// Fast adapter should win.
	if resp.SelectedOption != "Accept" {
		t.Errorf("expected 'Accept', got %q", resp.SelectedOption)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	// Both adapters should have been called.
	if !fast.called.Load() {
		t.Error("fast adapter was not called")
	}
	if !slow.called.Load() {
		t.Error("slow adapter was not called")
	}

	// Give a moment for the slow adapter to receive cancellation.
	time.Sleep(20 * time.Millisecond)

	// Slow adapter should have received cancellation.
	if !slow.canceled.Load() {
		t.Error("slow adapter did not receive cancellation")
	}
}

func TestUIMediator_PromptDecision_ParentContextCanceled(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Use only slow adapter so we can cancel before it responds.
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}
	m.RegisterAdapter(slow)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel almost immediately.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	req := registry.PromptRequest{
		TaskID:  "task-1",
		Message: "Review changes?",
		Options: []string{"Accept", "Revert"},
	}

	resp := m.PromptDecision(ctx, req)

	if resp.Error != ErrPromptCanceled {
		t.Errorf("expected ErrPromptCanceled, got %v", resp.Error)
	}
}

func TestUIMediator_RenderMessage(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	m.RenderMessage("Hello, World!")

	expected := "Hello, World!\n"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestUIMediator_ClearScreen(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	m.ClearScreen()

	// ANSI escape sequence for clear screen + move cursor to top-left.
	expected := "\033[2J\033[H"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestUIMediator_ConcurrentRegistration(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Register adapters concurrently.
	done := make(chan struct{})
	for i := range 10 {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			adapter := &FastMockAdapter{
				response: registry.PromptResponse{SelectedOption: "Accept"},
			}
			m.RegisterAdapter(adapter)
		}(i)
	}

	// Wait for all goroutines to complete.
	for range 10 {
		<-done
	}

	adapters := m.Adapters()
	if len(adapters) != 10 {
		t.Errorf("expected 10 adapters, got %d", len(adapters))
	}
}

func TestUIMediator_ImplementsRegistryUIMediator(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// This assignment will fail to compile if UIMediator doesn't implement
	// registry.UIMediator.
	var _ registry.UIMediator = m
}

// Compile-time check that mock adapters implement UIAdapter.
var (
	_ UIAdapter = (*FastMockAdapter)(nil)
	_ UIAdapter = (*SlowMockAdapter)(nil)
)

func TestUIMediator_NotifyAll_BroadcastsToAllAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	event := registry.UIEvent{
		Type:    registry.EventTaskStarted,
		TaskID:  "task-123",
		Message: "Task execution started",
		Payload: map[string]string{"priority": "high"},
	}

	m.NotifyAll(event)

	// Give goroutines time to process the event.
	time.Sleep(20 * time.Millisecond)

	// Both adapters should have received the event.
	fastEvents := fast.Events()
	if len(fastEvents) != 1 {
		t.Errorf("fast adapter: expected 1 event, got %d", len(fastEvents))
	} else {
		if fastEvents[0].Type != registry.EventTaskStarted {
			t.Errorf("fast adapter: expected EventTaskStarted, got %v", fastEvents[0].Type)
		}
		if fastEvents[0].TaskID != "task-123" {
			t.Errorf("fast adapter: expected task-123, got %q", fastEvents[0].TaskID)
		}
	}

	slowEvents := slow.Events()
	if len(slowEvents) != 1 {
		t.Errorf("slow adapter: expected 1 event, got %d", len(slowEvents))
	} else {
		if slowEvents[0].Type != registry.EventTaskStarted {
			t.Errorf("slow adapter: expected EventTaskStarted, got %v", slowEvents[0].Type)
		}
	}
}

func TestUIMediator_NotifyAll_NoAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Should not panic with no adapters.
	event := registry.UIEvent{
		Type:    registry.EventTaskCompleted,
		TaskID:  "task-456",
		Message: "Task completed",
	}

	m.NotifyAll(event) // Should complete without error.
}

func TestUIMediator_NotifyAll_MultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	m.RegisterAdapter(fast)

	events := []registry.UIEvent{
		{Type: registry.EventTaskStarted, TaskID: "task-1", Message: "Started"},
		{Type: registry.EventProgress, TaskID: "task-1", Message: "50% complete"},
		{Type: registry.EventTaskCompleted, TaskID: "task-1", Message: "Done"},
	}

	for _, e := range events {
		m.NotifyAll(e)
	}

	// Give goroutines time to process all events.
	time.Sleep(30 * time.Millisecond)

	received := fast.Events()
	if len(received) != 3 {
		t.Errorf("expected 3 events, got %d", len(received))
	}
}

func TestUIMediator_NotifyAll_FireAndForget(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Create a slow adapter that takes time to process events.
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}
	m.RegisterAdapter(slow)

	event := registry.UIEvent{
		Type:    registry.EventAgentCrashed,
		TaskID:  "task-error",
		Message: "Agent crashed unexpectedly",
	}

	start := time.Now()
	m.NotifyAll(event)
	elapsed := time.Since(start)

	// NotifyAll should return immediately (fire-and-forget).
	// It should not block waiting for the adapter to process.
	if elapsed > 10*time.Millisecond {
		t.Errorf("NotifyAll blocked for %v, expected immediate return", elapsed)
	}

	// Give slow adapter time to receive.
	time.Sleep(20 * time.Millisecond)

	received := slow.Events()
	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d", len(received))
	}
}
